package render

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TesseractOCR mengenali satu halaman PDF hasil pindai. Mengikuti pola
// poppler.go: cek biner saat init, semafor + timeout, spool ke berkas
// sementara. OMP_THREAD_LIMIT=1 wajib (keputusan 9-e) — tesseract dengan
// 4 thread membakar 2,2x CPU untuk 1,4x kecepatan dan membuat viewer
// tersendat.
type TesseractOCR struct {
	dpi     int
	timeout time.Duration
	sem     chan struct{}
	langs   string
}

func NewTesseract(dpi int, timeout time.Duration, concurrency int) (*TesseractOCR, error) {
	for _, bin := range []string{"tesseract", "pdftoppm"} {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("ocr: %s not found in PATH: %w", bin, err)
		}
	}

	langs, err := detectOCRLangs()
	if err != nil {
		return nil, err
	}

	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 2 {
		concurrency = 2
	}

	return &TesseractOCR{
		dpi:     dpi,
		timeout: timeout,
		sem:     make(chan struct{}, concurrency),
		langs:   langs,
	}, nil
}

// detectOCRLangs memakai "ind+eng" bila ind.traineddata terpasang, "eng"
// bila tidak (keputusan 9-e).
func detectOCRLangs() (string, error) {
	out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ocr: tesseract --list-langs: %w: %s", err, strings.TrimSpace(string(out)))
	}

	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "ind" {
			return "ind+eng", nil
		}
	}

	return "eng", nil
}

func (o *TesseractOCR) OCRPage(ctx context.Context, pdf io.Reader, page int) (OCRResult, error) {
	if page < 1 {
		return OCRResult{}, ErrPageOutOfRange
	}

	work, cleanup, err := spool(pdf)
	if err != nil {
		return OCRResult{}, err
	}
	defer cleanup()

	prefix := filepath.Join(work.dir, "page")
	if _, err := o.run(ctx, "pdftoppm",
		"-png",
		"-r", strconv.Itoa(o.dpi),
		"-f", strconv.Itoa(page),
		"-l", strconv.Itoa(page),
		"-singlefile",
		work.pdf, prefix,
	); err != nil {
		return OCRResult{}, err
	}

	img := prefix + ".png"
	if _, err := os.Stat(img); err != nil {
		return OCRResult{}, ErrPageOutOfRange
	}

	out, err := o.run(ctx, "tesseract", img, "stdout", "tsv", "-l", o.langs)
	if err != nil {
		return OCRResult{}, err
	}

	return parseTesseractTSV(out)
}

func (o *TesseractOCR) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	select {
	case o.sem <- struct{}{}:
		defer func() {
			<-o.sem
		}()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())

		if strings.Contains(msg, "Wrong page range") {
			return nil, ErrPageOutOfRange
		}

		return nil, fmt.Errorf("%w: %s: %v: %s", ErrRenderFailed, name, err, msg)
	}

	return stdout.Bytes(), nil
}

// parseTesseractTSV membaca keluaran `tesseract ... tsv`:
//   - baris level 1 membawa dimensi halaman di kolom left/top
//   - baris level 5 adalah satu kata dengan koordinat piksel + confidence
//
// Koordinat dinormalkan ke 0..1; teks disusun mengikuti urutan baca TSV
// (baris baru saat line_num berganti).
func parseTesseractTSV(out []byte) (OCRResult, error) {
	var (
		pageW, pageH float64
		words        []Word
		textParts    []string
		lastLine     = -1
		lineHasWord  bool
	)

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 12 {
			continue
		}

		switch fields[0] {
		case "1":
			// Baris level-1 membawa dimensi halaman di kolom width/height.
			w, errW := strconv.ParseFloat(fields[8], 64)
			h, errH := strconv.ParseFloat(fields[9], 64)
			if errW == nil && errH == nil && w > 0 && h > 0 {
				pageW, pageH = w, h
			}
		case "5":
			text := fields[11]
			if text == "" || pageW <= 0 || pageH <= 0 {
				continue
			}

			left, errL := strconv.ParseFloat(fields[6], 64)
			top, errT := strconv.ParseFloat(fields[7], 64)
			width, errWd := strconv.ParseFloat(fields[8], 64)
			height, errHt := strconv.ParseFloat(fields[9], 64)
			if errL != nil || errT != nil || errWd != nil || errHt != nil {
				continue
			}

			conf, _ := strconv.ParseFloat(fields[10], 64)

			words = append(words, Word{
				Text: text,
				X:    left / pageW,
				Y:    top / pageH,
				W:    width / pageW,
				H:    height / pageH,
				Conf: conf,
			})

			lineNo, _ := strconv.Atoi(fields[4])
			if lastLine >= 0 && lineNo != lastLine {
				textParts = append(textParts, "\n")
			} else if lineHasWord {
				textParts = append(textParts, " ")
			}
			textParts = append(textParts, text)
			lastLine = lineNo
			lineHasWord = true
		}
	}

	if err := sc.Err(); err != nil {
		return OCRResult{}, fmt.Errorf("%w: read tsv: %v", ErrRenderFailed, err)
	}

	return OCRResult{
		Text:  strings.Join(textParts, ""),
		Words: words,
	}, nil
}
