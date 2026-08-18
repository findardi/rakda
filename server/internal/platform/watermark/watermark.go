package watermark

import "io"

type Mark struct {
	Primary   string
	Secondary string
}

type Watermarker interface {
	Burn(src []byte, m Mark) ([]byte, error)
}

type PDFStamper interface {
	Stamp(src io.ReadSeeker, dst io.Writer, m Mark) error
}
