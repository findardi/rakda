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
