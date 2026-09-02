package render

import (
	"context"
	"errors"
	"io"
)

var (
	ErrRenderFailed   = errors.New("render failed")
	ErrPageOutOfRange = errors.New("page out of range")
)

type Render interface {
	PageCount(ctx context.Context, pdf io.Reader) (int, error)
	RenderPage(ctx context.Context, pdf io.Reader, page int) ([]byte, error)
	Open(pdf io.Reader) (Document, error)
}

type Document interface {
	RenderPage(ctx context.Context, page int) ([]byte, error)
	Close() error
}

type TextExtractor interface {
	ExtractText(ctx context.Context, pdf io.Reader) (string, error)
}

type WordBoxExtractor interface {
	OpenWordBoxes(pdf io.Reader) (WordBoxDocument, error)
}

type WordBoxDocument interface {
	ExtractWordBoxes(ctx context.Context, page int) ([]Word, error)
	Close() error
}

// Word — satu kata dengan koordinat dinormalkan ke pecahan 0..1 terhadap
// lebar/tinggi halaman sehingga bebas DPI. Conf = 0 untuk bbox pdftotext
// (tidak punya confidence), nilai nyata untuk hasil OCR.
type Word struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Conf float64 `json:"conf"`
}

type OCRResult struct {
	Text  string
	Words []Word
}

type OCR interface {
	OpenOCR(pdf io.Reader) (OCRDocument, error)
}

type OCRDocument interface {
	OCRPage(ctx context.Context, page int) (OCRResult, error)
	Close() error
}
