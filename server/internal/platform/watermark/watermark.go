package watermark

import "image"

type Mark struct {
	Primary   string
	Secondary string
}

type Watermarker interface {
	Burn(src []byte, m Mark) ([]byte, error)
	BurnImage(src []byte, m Mark) (*image.RGBA, error)
}
