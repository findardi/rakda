package watermark

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"

	_ "embed"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/f64"
	"golang.org/x/image/math/fixed"
)

//go:embed fonts/DejaVuSans.ttf
var fontData []byte

const (
	angleDegree = -30.0
	tileGapX    = 80
	tileGapY    = 60
	lineGap     = 6
)

var inkColor = color.RGBA{
	R: 0,
	G: 0,
	B: 0,
	A: 38,
}

type faceSet struct {
	primary   font.Face
	secondary font.Face
}

type ImageWatermark struct {
	pool sync.Pool
}

func New() (*ImageWatermark, error) {
	if _, err := newFaceSet(); err != nil {
		return nil, err
	}

	return &ImageWatermark{
		pool: sync.Pool{
			New: func() any {
				fs, err := newFaceSet()
				if err != nil {
					return err
				}
				return fs
			},
		},
	}, nil
}

func newFaceSet() (*faceSet, error) {
	f, err := opentype.Parse(fontData)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}

	primary, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    22,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("primary face: %w", err)
	}

	secondary, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    15,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("secondary face: %w", err)
	}

	return &faceSet{
		primary:   primary,
		secondary: secondary,
	}, nil
}

func (w *ImageWatermark) Burn(src []byte, m Mark) ([]byte, error) {
	page, err := w.BurnImage(src, m)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := png.Encode(&out, page); err != nil {
		return nil, fmt.Errorf("encode page: %w", err)
	}

	return out.Bytes(), nil
}

func (w *ImageWatermark) BurnImage(src []byte, m Mark) (*image.RGBA, error) {
	got := w.pool.Get()
	fs, ok := got.(*faceSet)
	if !ok {
		return nil, fmt.Errorf("watermark font unavailable")
	}
	defer w.pool.Put(fs)

	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode page: %w", err)
	}

	bounds := img.Bounds()
	page := image.NewRGBA(bounds)
	draw.Draw(page, bounds, img, bounds.Min, draw.Src)

	tile := fs.tile(m)
	if tile != nil {
		overlay := tileOverlay(bounds, tile)
		rotateOnto(page, overlay, angleDegree)
	}

	return page, nil
}

func (fs *faceSet) tile(m Mark) *image.RGBA {
	if m.Primary == "" && m.Secondary == "" {
		return nil
	}

	primaryW := font.MeasureString(fs.primary, m.Primary).Ceil()
	secondaryW := font.MeasureString(fs.secondary, m.Secondary).Ceil()

	primaryH := fs.primary.Metrics().Height.Ceil()
	secondaryH := fs.secondary.Metrics().Height.Ceil()

	width := max(primaryW, secondaryW) + tileGapX
	height := primaryH + secondaryH + lineGap + tileGapY

	tile := image.NewRGBA(image.Rect(0, 0, width, height))

	drawer := &font.Drawer{
		Dst:  tile,
		Src:  image.NewUniform(inkColor),
		Face: fs.primary,
		Dot:  fixed.P(0, fs.primary.Metrics().Ascent.Ceil()),
	}

	drawer.DrawString(m.Primary)

	drawer.Face = fs.secondary
	drawer.Dot = fixed.P(0, primaryH+lineGap+fs.secondary.Metrics().Ascent.Ceil())
	drawer.DrawString(m.Secondary)

	return tile
}

func tileOverlay(page image.Rectangle, tile *image.RGBA) *image.RGBA {
	side := int(math.Ceil(math.Hypot(float64(page.Dx()), float64(page.Dy()))))
	overlay := image.NewRGBA(image.Rect(0, 0, side, side))

	tb := tile.Bounds()

	for y := 0; y < side; y += tb.Dy() {
		for x := 0; x < side; x += tb.Dx() {
			r := image.Rect(x, y, x+tb.Dx(), y+tb.Dy())
			draw.Draw(overlay, r, tile, tb.Min, draw.Over)
		}
	}

	return overlay
}

func rotateOnto(dst *image.RGBA, overlay *image.RGBA, degree float64) {
	rad := degree * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)

	ob := overlay.Bounds()
	ocx := float64(ob.Dx()) / 2
	ocy := float64(ob.Dy()) / 2

	db := dst.Bounds()
	dcx := float64(db.Min.X) + float64(db.Dx())/2
	dcy := float64(db.Min.Y) + float64(db.Dy())/2

	m := f64.Aff3{
		cos, -sin, dcx - (cos*ocx - sin*ocy),
		sin, cos, dcy - (sin*ocx + cos*ocy),
	}

	xdraw.BiLinear.Transform(dst, m, overlay, ob, xdraw.Over, nil)
}
