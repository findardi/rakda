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
	"github.com/findardi/rakda/server/internal/platform/sender"
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

	renderer, err := render.NewPoppler(viewerCfg, viewerCfg.RenderConcurrency, 0)
	if err != nil {
		log.Fatal(err)
	}

	sweepRenderer, err := render.NewPoppler(viewerCfg, viewerCfg.SweepConcurrency, viewerCfg.SweepNice)
	if err != nil {
		log.Fatal(err)
	}

	downloadRenderer, err := render.NewPoppler(viewerCfg, viewerCfg.DownloadConcurrency, viewerCfg.DownloadNice)
	if err != nil {
		log.Fatal(err)
	}

	// Konstruktor ContentService diam-diam memakai Renderer bila kolam unduhan
	// nil; baris ini membuat wiring yang terlupa terlihat di journald.
	log.Printf("viewer: poppler pools request=%d sweep=%d (nice %d) downloadjob=%d (nice %d)",
		viewerCfg.RenderConcurrency, viewerCfg.SweepConcurrency, viewerCfg.SweepNice,
		viewerCfg.DownloadConcurrency, viewerCfg.DownloadNice)

	// Alamat ini harus bisa dijangkau dari dalam container api; `localhost`
	// di sini adalah api sendiri, bukan host.
	log.Printf("viewer: gotenberg %s (convert timeout %s)", viewerCfg.GotenbergURL, viewerCfg.ConvertTimeout)

	ocrDPI := config.EnvIntOr("OCR_DPI", viewerCfg.DPI)

	ocrConcurrency := config.EnvIntOr("OCR_CONCURRENCY", 1)

	ocrNice := config.EnvIntOr("OCR_NICE", 10)

	ocr, err := render.NewTesseract(ocrDPI, viewerCfg.RenderTimeout, ocrConcurrency, ocrNice)
	if err != nil {
		log.Fatal(err)
	}

	wm, err := watermark.New()
	if err != nil {
		log.Fatal(err)
	}

	viewer := contentservice.Viewer{
		Converter:           convert.NewGotenberg(viewerCfg),
		Renderer:            renderer,
		DownloadJobRenderer: downloadRenderer,
		Watermark:           wm,
		TextExtractor:       sweepRenderer,
		WordBoxes:           sweepRenderer,
		OCR:                 ocr,
		DPI:                 viewerCfg.DPI,
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

	mailCfg, err := config.LoadMailConfig()
	if err != nil {
		log.Fatal(err)
	}

	mailer, err := sender.New(mailCfg)
	if err != nil {
		log.Fatal(err)
	}

	// Transport yang terpilih terlihat di journald, sepola log kolam poppler,
	// supaya prod yang tanpa sengaja masih smtp ketahuan dari baris pertama.
	log.Printf("mail: provider=%s from=%q", mailCfg.Provider, mailCfg.From)

	if err := app.New(db, otpSecret, addr, jwtSecret, store, viewer, caches, mailer).Run(); err != nil {
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
