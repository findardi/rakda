package watermark

type Mark struct {
	Primary   string
	Secondary string
}

type Watermarker interface {
	Burn(src []byte, m Mark) ([]byte, error)
}
