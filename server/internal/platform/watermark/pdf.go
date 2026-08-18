package watermark

import (
	"fmt"
	"io"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const stampDescription = "points:18, scale:0.9 rel, rotation:-30, opacity:0.15, fillcolor:#000000, strokecolor:#000000"

type PDFStamp struct{}

func NewPDFStamp() *PDFStamp {
	return &PDFStamp{}
}

func (s *PDFStamp) Stamp(src io.ReadSeeker, dst io.Writer, m Mark) error {
	if m.Primary == "" && m.Secondary == "" {
		return fmt.Errorf("empty watermark mark")
	}

	wm, err := pdfcpu.ParseTextWatermarkDetails(m.Primary+"\n"+m.Secondary, stampDescription, true, types.POINTS)
	if err != nil {
		return fmt.Errorf("build stamp: %w", err)
	}

	if err := api.AddWatermarks(src, dst, nil, wm, model.NewDefaultConfiguration()); err != nil {
		return fmt.Errorf("stamp pdf: %w", err)
	}

	return nil
}
