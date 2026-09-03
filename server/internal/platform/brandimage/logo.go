// Package brandimage turns a user-supplied picture into a room logo the app can
// serve safely. The whole point is the re-encode: the stored file is always a
// PNG the server drew itself, so it carries no metadata, no script, and no
// format the browser might interpret (an SVG served inline from the app origin
// would be stored XSS inside a data room).
package brandimage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net/http"

	xdraw "golang.org/x/image/draw"

	// Decoders registered for image.Decode. WebP is decode-only in x/image,
	// which is fine: the output format is always PNG.
	_ "golang.org/x/image/webp"
	_ "image/jpeg"
)

var (
	ErrTooLarge    = errors.New("brandimage: file exceeds the size limit")
	ErrUnsupported = errors.New("brandimage: unsupported image format")
	ErrInvalid     = errors.New("brandimage: image cannot be decoded")
	ErrDimensions  = errors.New("brandimage: image dimensions out of range")
)

const (
	// minEdge rejects tracking-pixel sized uploads that would only ever render
	// as a blur; maxPixels caps what image.Decode may allocate (16 MP ≈ 64 MB
	// of RGBA) so a small file with a huge header cannot exhaust the box.
	minEdge   = 16
	maxPixels = 16_000_000
)

// NormalizeLogo reads at most maxBytes from r, sniffs the format from the
// bytes (never from a header the client chose), accepts PNG, JPEG and WebP
// only, checks the dimensions before touching pixels, scales the long edge
// down to maxEdge with a Catmull-Rom filter, and returns the result as PNG.
// Transparency survives; everything else about the source does not.
func NormalizeLogo(r io.Reader, maxBytes int64, maxEdge int) ([]byte, error) {
	if maxBytes <= 0 || maxEdge <= 0 {
		return nil, fmt.Errorf("brandimage: invalid limits %d/%d", maxBytes, maxEdge)
	}

	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("brandimage: read: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}
	if len(data) == 0 {
		return nil, ErrInvalid
	}

	switch http.DetectContentType(data) {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return nil, ErrUnsupported
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}
	if cfg.Width < minEdge || cfg.Height < minEdge || cfg.Width*cfg.Height > maxPixels {
		return nil, ErrDimensions
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalid
	}

	dst := image.NewNRGBA(fitWithin(src.Bounds(), maxEdge))
	if dst.Bounds().Eq(src.Bounds()) {
		// Same size: a plain copy into a fresh canvas still drops every byte
		// of the original container, which is the guarantee this package makes.
		draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("brandimage: encode png: %w", err)
	}
	return out.Bytes(), nil
}

// fitWithin returns a zero-origin rectangle with b's aspect ratio whose long
// edge is at most maxEdge. Images already inside the limit keep their size.
func fitWithin(b image.Rectangle, maxEdge int) image.Rectangle {
	w, h := b.Dx(), b.Dy()
	long := max(w, h)
	if long <= maxEdge {
		return image.Rect(0, 0, w, h)
	}
	// Scale the long edge exactly and round the other so a 1024×256 source
	// lands on 512×128, not 512×127.
	if w >= h {
		return image.Rect(0, 0, maxEdge, max(1, (h*maxEdge+w/2)/w))
	}
	return image.Rect(0, 0, max(1, (w*maxEdge+h/2)/h), maxEdge)
}
