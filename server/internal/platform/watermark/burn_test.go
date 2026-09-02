package watermark_test

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"testing"

	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/stretchr/testify/require"
)

func whitePagePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestImageWatermark_BurnMatchesBurnImage(t *testing.T) {
	wm, err := watermark.New()
	require.NoError(t, err)

	src := whitePagePNG(t, 240, 320)
	mark := watermark.Mark{Primary: "guest@example.com", Secondary: "2026-09-02 · 10.0.0.1"}

	img, err := wm.BurnImage(src, mark)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 240, 320), img.Bounds())

	white := image.NewRGBA(img.Bounds())
	draw.Draw(white, white.Bounds(), image.White, image.Point{}, draw.Src)
	require.NotEqual(t, white.Pix, img.Pix)

	encoded, err := wm.Burn(src, mark)
	require.NoError(t, err)
	decoded, err := png.Decode(bytes.NewReader(encoded))
	require.NoError(t, err)

	rgba, ok := decoded.(*image.RGBA)
	require.True(t, ok)
	require.Equal(t, img.Pix, rgba.Pix)
}
