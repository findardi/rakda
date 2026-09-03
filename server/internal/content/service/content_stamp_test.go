package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stampPages(dims ...[2]float64) []burnedPage {
	pages := make([]burnedPage, len(dims))
	for i, d := range dims {
		pages[i] = burnedPage{path: fmt.Sprintf("p%04d.jpg", i+1), w: d[0], h: d[1]}
	}
	return pages
}

func runPaths(runs []pageRun) []string {
	var out []string
	for _, r := range runs {
		out = append(out, r.images...)
	}
	return out
}

func TestGroupPageRuns_SplitsAtMaxPerRunAndKeepsOrder(t *testing.T) {
	dims := make([][2]float64, 7)
	for i := range dims {
		dims[i] = [2]float64{1275, 1650}
	}
	pages := stampPages(dims...)

	runs := groupPageRuns(pages, 3)

	require.Len(t, runs, 3)
	assert.Len(t, runs[0].images, 3)
	assert.Len(t, runs[1].images, 3)
	assert.Len(t, runs[2].images, 1)

	want := make([]string, len(pages))
	for i, p := range pages {
		want[i] = p.path
	}
	assert.Equal(t, want, runPaths(runs))
}

func TestGroupPageRuns_SplitsOnDimensionChange(t *testing.T) {
	pages := stampPages(
		[2]float64{1275, 1650},
		[2]float64{1275, 1650},
		[2]float64{1650, 1275},
		[2]float64{1275, 1650},
	)

	runs := groupPageRuns(pages, 25)

	require.Len(t, runs, 3)
	assert.Equal(t, []string{"p0001.jpg", "p0002.jpg"}, runs[0].images)
	assert.Equal(t, 1275.0, runs[0].w)
	assert.Equal(t, []string{"p0003.jpg"}, runs[1].images)
	assert.Equal(t, 1650.0, runs[1].w)
	assert.Equal(t, 1275.0, runs[1].h)
	assert.Equal(t, []string{"p0004.jpg"}, runs[2].images)
	assert.Equal(t, 1275.0, runs[2].w)
}

func TestGroupPageRuns_DimensionChangeResetsCount(t *testing.T) {
	pages := stampPages(
		[2]float64{100, 200},
		[2]float64{100, 200},
		[2]float64{300, 400},
		[2]float64{300, 400},
		[2]float64{300, 400},
	)

	runs := groupPageRuns(pages, 3)

	require.Len(t, runs, 2)
	assert.Len(t, runs[0].images, 2)
	assert.Len(t, runs[1].images, 3)
}

func TestGroupPageRuns_Empty(t *testing.T) {
	assert.Empty(t, groupPageRuns(nil, 25))
}

func requirePoppler(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"pdfinfo", "pdftoppm", "pdftotext", "pdfimages"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not in PATH", bin)
		}
	}
}

func blankJPEG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	path := filepath.Join(dir, name)
	require.NoError(t, writeJPEG(path, img))
	return path
}

var pdfinfoPageSize = regexp.MustCompile(`Page\s+(\d+) size:\s+([\d.]+) x ([\d.]+) pts`)

func pageSizes(t *testing.T, pdfPath string, n int) [][2]float64 {
	t.Helper()
	out, err := exec.Command("pdfinfo", "-f", "1", "-l", strconv.Itoa(n), pdfPath).Output()
	require.NoError(t, err)
	sizes := make([][2]float64, 0, n)
	for _, m := range pdfinfoPageSize.FindAllStringSubmatch(string(out), -1) {
		w, _ := strconv.ParseFloat(m[2], 64)
		h, _ := strconv.ParseFloat(m[3], 64)
		sizes = append(sizes, [2]float64{w, h})
	}
	return sizes
}

func nonWhitePixels(t *testing.T, pdfPath string, page int) int {
	t.Helper()
	dir := t.TempDir()
	prefix := filepath.Join(dir, "p")
	require.NoError(t, exec.Command("pdftoppm", "-png", "-r", "150", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile", pdfPath, prefix).Run())
	f, err := os.Open(prefix + ".png")
	require.NoError(t, err)
	defer f.Close()
	img, err := png.Decode(f)
	require.NoError(t, err)
	count := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r < 0xf000 || g < 0xf000 || bl < 0xf000 {
				count++
			}
		}
	}
	return count
}

func imageEncodings(t *testing.T, pdfPath string) map[int]string {
	t.Helper()
	out, err := exec.Command("pdfimages", "-list", pdfPath).Output()
	require.NoError(t, err)
	enc := map[int]string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		page, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		enc[page] = fields[8]
	}
	return enc
}

func TestRasterWatermarkPDF_MixedSizesBatched(t *testing.T) {
	requirePoppler(t)

	const dpi = 150
	layout := [][2]int{{300, 400}, {300, 400}, {300, 400}, {400, 300}, {400, 300}, {300, 400}, {300, 400}}

	srcDir := t.TempDir()
	srcPages := make([]burnedPage, len(layout))
	for i, d := range layout {
		path := blankJPEG(t, srcDir, fmt.Sprintf("s%02d.jpg", i), d[0], d[1])
		srcPages[i] = burnedPage{path: path, w: float64(d[0]), h: float64(d[1])}
	}
	srcRuns, err := importPageRuns(srcDir, srcPages, dpi, 100)
	require.NoError(t, err)
	srcPDF, err := mergeRuns(srcDir, srcRuns)
	require.NoError(t, err)

	wantSizes := make([][2]float64, len(layout))
	for i, d := range layout {
		wantSizes[i] = [2]float64{float64(d[0]) / dpi * 72, float64(d[1]) / dpi * 72}
	}
	require.Equal(t, wantSizes, pageSizes(t, srcPDF, len(layout)), "source fixture geometry")

	renderer, err := render.NewPoppler(config.ViewerConfig{DPI: dpi, RenderTimeout: 30 * time.Second, RenderConcurrency: 2})
	require.NoError(t, err)
	wm, err := watermark.New()
	require.NoError(t, err)

	srcBytes, err := os.ReadFile(srcPDF)
	require.NoError(t, err)
	store := fakeStorage{getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
		if key != "rendition" {
			return nil, errors.New("miss")
		}
		return io.NopCloser(bytes.NewReader(srcBytes)), nil
	}}
	svc := NewContentService(nil, store, Viewer{Renderer: renderer, Watermark: wm, DPI: dpi}, 0, nil, StampDeps{Sync: 1, Async: 1}, ArchiveDeps{}, CacheDeps{}, RenditionDeps{})

	doc, err := renderer.Open(bytes.NewReader(srcBytes))
	require.NoError(t, err)
	defer doc.Close()

	workDir := t.TempDir()
	pagesPath := filepath.Join(workDir, "pages")
	require.NoError(t, os.Mkdir(pagesPath, 0o700))

	mark := watermark.Mark{Primary: "guest@example.com", Secondary: "2026-08-21"}
	pages, err := svc.burnPages(context.Background(), "w", "v", doc, pagesPath, len(layout), mark)
	require.NoError(t, err)

	runFiles, err := importPageRuns(workDir, pages, dpi, 2)
	require.NoError(t, err)
	assert.Len(t, runFiles, 4, "runs: [1,2] [3] [4,5] [6,7] split by size and by maxPerRun")

	outPDF, err := mergeRuns(workDir, runFiles)
	require.NoError(t, err)

	assert.Equal(t, wantSizes, pageSizes(t, outPDF, len(layout)))
	enc := imageEncodings(t, outPDF)
	require.Len(t, enc, len(layout))
	for pg := 1; pg <= len(layout); pg++ {
		assert.Equal(t, "jpeg", enc[pg], "page %d embedded as DCTDecode", pg)
	}
	for _, pg := range []int{1, 4, 7} {
		assert.Greater(t, nonWhitePixels(t, outPDF, pg), 0, "page %d carries the burned mark", pg)
	}

	for _, p := range pages {
		_, err := os.Stat(p.path)
		assert.True(t, errors.Is(err, os.ErrNotExist), "page file removed after import: %s", p.path)
	}
	for _, rf := range runFiles {
		_, err := os.Stat(rf)
		assert.True(t, errors.Is(err, os.ErrNotExist), "run pdf removed after merge: %s", rf)
	}

	rc, err := svc.rasterWatermarkPDF(context.Background(), renderer, "w", "v", "rendition", len(layout), mark)
	require.NoError(t, err)
	full, err := io.ReadAll(rc)
	require.NoError(t, rc.Close())
	fullPath := filepath.Join(t.TempDir(), "full.pdf")
	require.NoError(t, os.WriteFile(fullPath, full, 0o600))
	assert.Equal(t, wantSizes, pageSizes(t, fullPath, len(layout)))
}
