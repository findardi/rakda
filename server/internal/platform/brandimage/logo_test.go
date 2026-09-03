package brandimage

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func encodePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Half-transparent gradient: proves alpha survives the round trip.
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: uint8(128 + x%128)})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}))
	return buf.Bytes()
}

// pngHeaderOnly builds a PNG whose IHDR claims w×h but carries no pixel data:
// DecodeConfig succeeds, so the dimension guard must fire before Decode
// would try to allocate the canvas.
func pngHeaderOnly(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9], ihdr[10], ihdr[11], ihdr[12] = 8, 6, 0, 0, 0
	chunk := append([]byte("IHDR"), ihdr...)
	_ = binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

func decodeOut(t *testing.T, out []byte) image.Image {
	t.Helper()
	require.True(t, bytes.HasPrefix(out, []byte{0x89, 'P', 'N', 'G'}), "output must be PNG")
	img, err := png.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	return img
}

func TestNormalizeLogo(t *testing.T) {
	const maxBytes = 2 << 20

	t.Run("png inside the limit is re-encoded at the same size with alpha kept", func(t *testing.T) {
		out, err := NormalizeLogo(bytes.NewReader(encodePNG(t, 300, 120)), maxBytes, 512)
		require.NoError(t, err)
		img := decodeOut(t, out)
		assert.Equal(t, image.Rect(0, 0, 300, 120), img.Bounds())
		_, _, _, a := img.At(10, 10).RGBA()
		assert.Less(t, a, uint32(0xffff), "alpha must survive")
	})

	t.Run("wide png is scaled to the long edge, aspect kept", func(t *testing.T) {
		out, err := NormalizeLogo(bytes.NewReader(encodePNG(t, 1024, 256)), maxBytes, 512)
		require.NoError(t, err)
		assert.Equal(t, image.Rect(0, 0, 512, 128), decodeOut(t, out).Bounds())
	})

	t.Run("tall jpeg becomes png scaled on height", func(t *testing.T) {
		out, err := NormalizeLogo(bytes.NewReader(encodeJPEG(t, 200, 1000)), maxBytes, 512)
		require.NoError(t, err)
		assert.Equal(t, image.Rect(0, 0, 102, 512), decodeOut(t, out).Bounds())
	})

	t.Run("svg is refused by content, whatever it is called", func(t *testing.T) {
		svg := `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`
		_, err := NormalizeLogo(strings.NewReader(svg), maxBytes, 512)
		assert.ErrorIs(t, err, ErrUnsupported)
	})

	t.Run("gif is refused", func(t *testing.T) {
		gif := append([]byte("GIF89a"), make([]byte, 64)...)
		_, err := NormalizeLogo(bytes.NewReader(gif), maxBytes, 512)
		assert.ErrorIs(t, err, ErrUnsupported)
	})

	t.Run("oversized upload is refused before decoding", func(t *testing.T) {
		data := encodePNG(t, 64, 64)
		_, err := NormalizeLogo(bytes.NewReader(data), int64(len(data))-1, 512)
		assert.ErrorIs(t, err, ErrTooLarge)
	})

	t.Run("empty body is invalid", func(t *testing.T) {
		_, err := NormalizeLogo(bytes.NewReader(nil), maxBytes, 512)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("truncated png is invalid, not a panic", func(t *testing.T) {
		data := encodePNG(t, 64, 64)
		_, err := NormalizeLogo(bytes.NewReader(data[:len(data)/2]), maxBytes, 512)
		assert.ErrorIs(t, err, ErrInvalid)
	})

	t.Run("tiny image is refused", func(t *testing.T) {
		_, err := NormalizeLogo(bytes.NewReader(encodePNG(t, 8, 8)), maxBytes, 512)
		assert.ErrorIs(t, err, ErrDimensions)
	})

	t.Run("decode bomb header is refused before allocation", func(t *testing.T) {
		_, err := NormalizeLogo(bytes.NewReader(pngHeaderOnly(20000, 20000)), maxBytes, 512)
		assert.ErrorIs(t, err, ErrDimensions)
	})
}

func TestFitWithin(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		want image.Rectangle
	}{
		{"inside limit keeps size", 300, 120, image.Rect(0, 0, 300, 120)},
		{"square at limit keeps size", 512, 512, image.Rect(0, 0, 512, 512)},
		{"wide scales width", 1024, 256, image.Rect(0, 0, 512, 128)},
		{"tall scales height", 256, 1024, image.Rect(0, 0, 128, 512)},
		{"extreme strip never collapses to zero", 5000, 3, image.Rect(0, 0, 512, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, fitWithin(image.Rect(0, 0, c.w, c.h), 512))
		})
	}
}
