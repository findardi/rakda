//go:build viewer

package watermark_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/findardi/rakda/server/internal/platform/convert"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/stretchr/testify/require"
)

func TestViewerPipeline(t *testing.T) {
	cfg, err := config.LoadViewerConfig()
	require.NoError(t, err)

	ctx := context.Background()

	var body strings.Builder
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&body, "Baris %d — Rakda secure viewer smoke test\n", i)
	}

	pdf, err := convert.NewGotenberg(cfg).ToPDF(ctx, strings.NewReader(body.String()), "sample.txt")
	require.NoError(t, err)
	defer pdf.Close()

	raw, err := io.ReadAll(pdf)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	renderer, err := render.NewPoppler(cfg, cfg.RenderConcurrency, 0)
	require.NoError(t, err)

	pages, err := renderer.PageCount(ctx, bytes.NewReader(raw))
	require.NoError(t, err)
	require.Positive(t, pages)

	page, err := renderer.RenderPage(ctx, bytes.NewReader(raw), 1)
	require.NoError(t, err)
	require.NotEmpty(t, page)

	_, err = renderer.RenderPage(ctx, bytes.NewReader(raw), pages+1)
	require.ErrorIs(t, err, render.ErrPageOutOfRange)

	wm, err := watermark.New()
	require.NoError(t, err)

	marked, err := wm.Burn(page, watermark.Mark{
		Primary:   "Budi Santoso · budi@acme.com",
		Secondary: "2026-07-14 09:12 UTC · 103.12.4.9",
	})
	require.NoError(t, err)
	require.NotEqual(t, page, marked)

	out := os.Getenv("VIEWER_TEST_OUT")
	if out == "" {
		out = filepath.Join(t.TempDir(), "page1.png")
	}

	require.NoError(t, os.WriteFile(out, marked, 0o644))
	t.Logf("pages=%d out=%s", pages, out)

	t.Run("burn is concurrency safe", func(t *testing.T) {
		var wg sync.WaitGroup

		for i := range 8 {
			wg.Add(1)

			go func() {
				defer wg.Done()

				_, err := wm.Burn(page, watermark.Mark{
					Primary:   fmt.Sprintf("guest-%d@acme.com", i),
					Secondary: "2026-07-14 09:12 UTC",
				})
				require.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}
