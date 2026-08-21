package render

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/findardi/Riksa-App/server/internal/platform/config"
)

type PopplerRenderer struct {
	dpi     int
	timeout time.Duration
	sem     chan struct{}
}

func NewPoppler(cfg config.ViewerConfig) (*PopplerRenderer, error) {
	for _, bin := range []string{"pdfinfo", "pdftoppm", "pdftotext"} {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("poppler: %s not found in PATH: %w", bin, err)
		}
	}

	return &PopplerRenderer{
		dpi:     cfg.DPI,
		timeout: cfg.RenderTimeout,
		sem:     make(chan struct{}, cfg.RenderConcurrency),
	}, nil
}

func (p *PopplerRenderer) PageCount(ctx context.Context, pdf io.Reader) (int, error) {
	work, cleanup, err := spool(pdf)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	out, err := p.run(ctx, "pdfinfo", work.pdf)
	if err != nil {
		return 0, err
	}

	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Pages:") {
			continue
		}

		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("%w: bad page count %q", ErrRenderFailed, line)
		}

		return n, nil
	}

	return 0, fmt.Errorf("%w: page count not found", ErrRenderFailed)
}

func (p *PopplerRenderer) RenderPage(ctx context.Context, pdf io.Reader, page int) ([]byte, error) {
	if page < 1 {
		return nil, ErrPageOutOfRange
	}

	work, cleanup, err := spool(pdf)
	if err != nil {
		return nil, err
	}

	defer cleanup()

	n := strconv.Itoa(page)
	prefix := filepath.Join(work.dir, "page")

	if _, err := p.run(ctx, "pdftoppm",
		"-png",
		"-r", strconv.Itoa(p.dpi),
		"-f", n,
		"-l", n,
		"-singlefile",
		work.pdf, prefix,
	); err != nil {
		return nil, err
	}

	out, err := os.ReadFile(prefix + ".png")
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrPageOutOfRange
	}

	if err != nil {
		return nil, fmt.Errorf("%w: read page: %v", ErrRenderFailed, err)
	}

	if len(out) == 0 {
		return nil, ErrPageOutOfRange
	}

	return out, nil
}

type spooled struct {
	dir string
	pdf string
}

func spool(r io.Reader) (spooled, func(), error) {
	dir, err := os.MkdirTemp("", "riksa-view-*")
	if err != nil {
		return spooled{}, nil, fmt.Errorf("temp dir: %w", err)
	}

	cleanup := func() { os.RemoveAll(dir) }
	path := filepath.Join(dir, "in.pdf")

	f, err := os.Create(path)
	if err != nil {
		cleanup()
		return spooled{}, nil, fmt.Errorf("temp file: %w", err)
	}

	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		cleanup()
		return spooled{}, nil, fmt.Errorf("spool pdf: %w", err)
	}

	if err := f.Close(); err != nil {
		cleanup()
		return spooled{}, nil, fmt.Errorf("close temp: %w", err)
	}

	return spooled{dir: dir, pdf: path}, cleanup, nil
}

func (p *PopplerRenderer) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	select {
	case p.sem <- struct{}{}:
		defer func() {
			<-p.sem
		}()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())

		if strings.Contains(msg, "Wrong page range") {
			return nil, ErrPageOutOfRange
		}

		return nil, fmt.Errorf("%w: %s: %v: %s", ErrRenderFailed, name, err, msg)
	}

	return stdout.Bytes(), nil
}

func (p *PopplerRenderer) ExtractText(ctx context.Context, pdf io.Reader) (string, error) {
	work, cleanup, err := spool(pdf)
	if err != nil {
		return "", err
	}
	defer cleanup()

	out, err := p.run(ctx, "pdftotext", work.pdf, "-")
	if err != nil {
		return "", err
	}

	return string(out), nil
}

// ExtractWordBoxes memulangkan koordinat kata untuk satu halaman PDF
// berteks asli (pdftotext -bbox), dinormalkan ke pecahan 0..1.
func (p *PopplerRenderer) ExtractWordBoxes(ctx context.Context, pdf io.Reader, page int) ([]Word, error) {
	if page < 1 {
		return nil, ErrPageOutOfRange
	}

	work, cleanup, err := spool(pdf)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	n := strconv.Itoa(page)

	out, err := p.run(ctx, "pdftotext",
		"-bbox",
		"-f", n,
		"-l", n,
		work.pdf, "-",
	)
	if err != nil {
		return nil, err
	}

	return parseWordBoxes(out)
}

// parseWordBoxes membaca keluaran `pdftotext -bbox`: satu halaman dalam
// elemen <page width height> berisi <word xMin yMin xMax yMax>teks</word>.
type bboxWord struct {
	XMin float64 `xml:"xMin,attr"`
	YMin float64 `xml:"yMin,attr"`
	XMax float64 `xml:"xMax,attr"`
	YMax float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

type bboxPage struct {
	Width  float64    `xml:"width,attr"`
	Height float64    `xml:"height,attr"`
	Words  []bboxWord `xml:"word"`
}

type bboxDoc struct {
	Pages []bboxPage `xml:"page"`
}

// pdftotext -bbox memulangkan <html><body><doc><page>…; Unmarshal ke tipe
// root langsung akan melewatkan <page> yang bersarang.
type bboxRoot struct {
	Doc bboxDoc `xml:"body>doc"`
}

func parseWordBoxes(out []byte) ([]Word, error) {
	var root bboxRoot

	if err := xml.Unmarshal(out, &root); err != nil {
		return nil, fmt.Errorf("%w: parse bbox xml: %v", ErrRenderFailed, err)
	}

	doc := root.Doc
	if len(doc.Pages) == 0 || doc.Pages[0].Width <= 0 || doc.Pages[0].Height <= 0 {
		return nil, fmt.Errorf("%w: bbox page missing", ErrRenderFailed)
	}

	pg := doc.Pages[0]
	words := make([]Word, 0, len(pg.Words))
	for _, w := range pg.Words {
		text := strings.TrimSpace(w.Text)
		if text == "" {
			continue
		}

		words = append(words, Word{
			Text: text,
			X:    w.XMin / pg.Width,
			Y:    w.YMin / pg.Height,
			W:    (w.XMax - w.XMin) / pg.Width,
			H:    (w.YMax - w.YMin) / pg.Height,
		})
	}

	return words, nil
}
