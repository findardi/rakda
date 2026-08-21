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
}

type TextExtractor interface {
	ExtractText(ctx context.Context, pdf io.Reader) (string, error)
}

// OCRWord — satu kata hasil OCR, koordinat dinormalkan ke pecahan 0..1
// terhadap lebar/tinggi halaman sehingga bebas DPI.
type OCRWord struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
	Conf float64 `json:"conf"`
}

type OCRResult struct {
	Text  string
	Words []OCRWord
}

type OCR interface {
	OCRPage(ctx context.Context, pdf io.Reader, page int) (OCRResult, error)
}
