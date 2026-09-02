package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/findardi/rakda/server/internal/app"
	contentservice "github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/findardi/rakda/server/internal/platform/convert"
	"github.com/findardi/rakda/server/internal/platform/database"
	"github.com/findardi/rakda/server/internal/platform/diskcache"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/spool"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/findardi/rakda/server/internal/platform/watermark"
)

func main() {
	if err := config.LoadEnvFile("configs/.env"); err != nil {
		log.Fatal(err)
	}

	if err := spool.CheckWritable(); err != nil {
		log.Fatal(err)
	}

	removed, err := spool.SweepOrphans()
	if err != nil {
		log.Printf("spool: sapu sisa spool tidak tuntas: %v", err)
	}

	log.Printf("spool: %d sisa spool dihapus dari %s", removed, os.TempDir())

	dbCfg, err := config.LoadDatabaseConfig()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.New(context.Background(), dbCfg)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	minioCfg := config.LoadMinioConfig()
	store, err := storage.NewMinio(minioCfg)
	if err != nil {
		log.Fatal(err)
	}

	if minioCfg.RequireEncryption && !minioCfg.SslMode {
		log.Fatal("MINIO_REQUIRE_ENCRYPTION=true menuntut MINIO_SSL_MODE=true")
	}

	if err := store.EnsureEncryption(context.Background()); err != nil {
		log.Printf("storage: gagal menyetel enkripsi bucket, status tetap diperiksa: %v", err)
	}

	switch algo, err := store.EncryptionStatus(context.Background()); {
	case err != nil:
		if minioCfg.RequireEncryption {
			log.Fatalf("MINIO_REQUIRE_ENCRYPTION=true tetapi status enkripsi bucket tidak bisa ditentukan: %v", err)
		}
		log.Printf("storage: status enkripsi bucket tidak bisa ditentukan: %v", err)
	case algo == "":
		if minioCfg.RequireEncryption {
			log.Fatalf("MINIO_REQUIRE_ENCRYPTION=true tetapi bucket %q tidak terenkripsi at-rest", minioCfg.BucketName)
		}
		log.Printf("storage: bucket %q TIDAK terenkripsi at-rest", minioCfg.BucketName)
	default:
		log.Printf("storage: bucket %q terenkripsi at-rest (%s)", minioCfg.BucketName, algo)
	}

	viewerCfg, err := config.LoadViewerConfig()
	if err != nil {
		log.Fatal(err)
	}

	renderer, err := render.NewPoppler(viewerCfg)
	if err != nil {
		log.Fatal(err)
	}

	sweepRenderer, err := render.NewPoppler(viewerCfg,
		render.WithPopplerConcurrency(viewerCfg.SweepConcurrency),
		render.WithPopplerNice(viewerCfg.SweepNice),
	)
	if err != nil {
		log.Fatal(err)
	}

	ocrDPI, err := config.GetEnvInt("OCR_DPI", viewerCfg.DPI)
	if err != nil {
		log.Printf("invalid OCR_DPI, fallback to viewer DPI: %v", err)
		ocrDPI = viewerCfg.DPI
	}

	ocrConcurrency, err := config.GetEnvInt("OCR_CONCURRENCY", 1)
	if err != nil {
		log.Printf("invalid OCR_CONCURRENCY, fallback to 1: %v", err)
		ocrConcurrency = 1
	}

	ocrNice, err := config.GetEnvInt("OCR_NICE", 10)
	if err != nil {
		log.Printf("invalid OCR_NICE, fallback to 10: %v", err)
		ocrNice = 10
	}

	ocr, err := render.NewTesseract(ocrDPI, viewerCfg.RenderTimeout,
		render.WithOCRConcurrency(ocrConcurrency),
		render.WithOCRNice(ocrNice),
	)
	if err != nil {
		log.Fatal(err)
	}

	wm, err := watermark.New()
	if err != nil {
		log.Fatal(err)
	}

	viewer := contentservice.Viewer{
		Converter:     convert.NewGotenberg(viewerCfg),
		Renderer:      renderer,
		Watermark:     wm,
		TextExtractor: sweepRenderer,
		WordBoxes:     sweepRenderer,
		OCR:           ocr,
		DPI:           viewerCfg.DPI,
	}

	cacheCfg, err := config.LoadDiskCacheConfig()
	if err != nil {
		log.Fatal(err)
	}

	caches, err := openDiskCaches(cacheCfg)
	if err != nil {
		log.Fatal(err)
	}

	otpSecret := config.GetEnv("OTP_SECRET", "")
	jwtSecret := config.GetEnv("JWT_SECRET", "")
	addr := config.GetEnv("ADDR", ":8181")

	if otpSecret == "" || jwtSecret == "" {
		log.Fatal("OTP_SECRET and JWT_SECRET must be set")
	}

	if err := app.New(db, otpSecret, addr, jwtSecret, store, viewer, caches).Run(); err != nil {
		log.Fatal(err)
	}
}

func openDiskCaches(cfg config.DiskCacheConfig) (contentservice.CacheDeps, error) {
	if !cfg.Enabled() {
		log.Printf("diskcache: nonaktif (DISK_CACHE_DIR kosong)")
		return contentservice.CacheDeps{}, nil
	}

	type tier struct {
		name   string
		budget int64
		dst    **diskcache.Cache
	}

	var caches contentservice.CacheDeps
	tiers := []tier{
		{"renditions", cfg.RenditionBudget, &caches.Renditions},
		{"pages", cfg.PageBudget, &caches.Pages},
		{"downloads", cfg.DownloadBudget, &caches.Downloads},
	}

	for _, tier := range tiers {
		c, err := diskcache.New(filepath.Join(cfg.Dir, tier.name), tier.budget, cfg.MinFree, cfg.Key)
		if err != nil {
			return contentservice.CacheDeps{}, fmt.Errorf("diskcache %s: %w", tier.name, err)
		}

		entries, bytes := c.Stats()
		log.Printf("diskcache: %s aktif di %s (anggaran %d MiB, min-free %d MiB, %d entri / %d MiB dari disk)",
			tier.name, c.Dir(), tier.budget>>20, cfg.MinFree>>20, entries, bytes>>20)

		*tier.dst = c
	}

	return caches, nil
}
