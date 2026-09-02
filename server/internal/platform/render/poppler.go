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

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/findardi/rakda/server/internal/platform/spool"
)

type PopplerRenderer struct {
	dpi     int
	timeout time.Duration
	nice    int
	sem     chan struct{}
}

type PopplerOption func(*popplerOptions)

type popplerOptions struct {
	concurrency int
	nice        int
}

func WithPopplerConcurrency(n int) PopplerOption {
	return func(o *popplerOptions) { o.concurrency = n }
}

func WithPopplerNice(n int) PopplerOption {
	return func(o *popplerOptions) { o.nice = n }
}

func NewPoppler(cfg config.ViewerConfig, opts ...PopplerOption) (*PopplerRenderer, error) {
	o := popplerOptions{concurrency: cfg.RenderConcurrency}
	for _, opt := range opts {
		opt(&o)
	}

	bins := []string{"pdfinfo", "pdftoppm", "pdftotext"}
	if o.nice > 0 {
		bins = append(bins, "nice")
	}

	for _, bin := range bins {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("poppler: %s not found in PATH: %w", bin, err)
		}
	}

	if o.concurrency < 1 {
		o.concurrency = 1
	}

	return &PopplerRenderer{
		dpi:     cfg.DPI,
		timeout: cfg.RenderTimeout,
		nice:    o.nice,
		sem:     make(chan struct{}, o.concurrency),
	}, nil
}

func (p *PopplerRenderer) PageCount(ctx context.Context, pdf io.Reader) (int, error) {
	work, cleanup, err := spoolPDF(pdf)
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

	doc, err := p.Open(pdf)
	if err != nil {
		return nil, err
	}
	defer doc.Close()

	return doc.RenderPage(ctx, page)
}

func (p *PopplerRenderer) Open(pdf io.Reader) (Document, error) {
	work, cleanup, err := spoolPDF(pdf)
	if err != nil {
		return nil, err
	}

	return &popplerDocument{p: p, work: work, cleanup: cleanup}, nil
}

type popplerDocument struct {
	p       *PopplerRenderer
	work    spooled
	cleanup func()
}

func (d *popplerDocument) RenderPage(ctx context.Context, page int) ([]byte, error) {
	if page < 1 {
		return nil, ErrPageOutOfRange
	}

	n := strconv.Itoa(page)
	prefix := filepath.Join(d.work.dir, "page-"+n)

	if _, err := d.p.run(ctx, "pdftoppm",
		"-png",
		"-r", strconv.Itoa(d.p.dpi),
		"-f", n,
		"-l", n,
		"-singlefile",
		d.work.pdf, prefix,
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

	os.Remove(prefix + ".png")

	if len(out) == 0 {
		return nil, ErrPageOutOfRange
	}

	return out, nil
}

func (d *popplerDocument) Close() error {
	d.cleanup()
	return nil
}

type spooled struct {
	dir string
	pdf string
}

func spoolPDF(r io.Reader) (spooled, func(), error) {
	dir, err := os.MkdirTemp("", spool.Prefix+"view-*")
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

	if p.nice > 0 {
		args = append([]string{"-n", strconv.Itoa(p.nice), name}, args...)
		name = "nice"
	}

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
	work, cleanup, err := spoolPDF(pdf)
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

func (p *PopplerRenderer) OpenWordBoxes(pdf io.Reader) (WordBoxDocument, error) {
	work, cleanup, err := spoolPDF(pdf)
	if err != nil {
		return nil, err
	}

	return &popplerWordBoxes{p: p, work: work, cleanup: cleanup}, nil
}

type popplerWordBoxes struct {
	p       *PopplerRenderer
	work    spooled
	cleanup func()
}

func (d *popplerWordBoxes) ExtractWordBoxes(ctx context.Context, page int) ([]Word, error) {
	if page < 1 {
		return nil, ErrPageOutOfRange
	}

	n := strconv.Itoa(page)

	out, err := d.p.run(ctx, "pdftotext",
		"-bbox",
		"-f", n,
		"-l", n,
		d.work.pdf, "-",
	)
	if err != nil {
		return nil, err
	}

	return parseWordBoxes(out)
}

func (d *popplerWordBoxes) Close() error {
	d.cleanup()
	return nil
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
