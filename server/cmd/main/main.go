package main

import (
	"context"
	"log"

	"github.com/findardi/rakda/server/internal/app"
	contentservice "github.com/findardi/rakda/server/internal/content/service"
	"github.com/findardi/rakda/server/internal/platform/config"
	"github.com/findardi/rakda/server/internal/platform/convert"
	"github.com/findardi/rakda/server/internal/platform/database"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/findardi/rakda/server/internal/platform/watermark"
)

func main() {
	if err := config.LoadEnvFile("configs/.env"); err != nil {
		log.Fatal(err)
	}

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

	ocr, err := render.NewTesseract(ocrDPI, viewerCfg.RenderTimeout, ocrConcurrency)
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
		TextExtractor: renderer,
		WordBoxes:     renderer,
		OCR:           ocr,
		DPI:           viewerCfg.DPI,
	}

	otpSecret := config.GetEnv("OTP_SECRET", "")
	jwtSecret := config.GetEnv("JWT_SECRET", "")
	addr := config.GetEnv("ADDR", ":8181")

	if otpSecret == "" || jwtSecret == "" {
		log.Fatal("OTP_SECRET and JWT_SECRET must be set")
	}

	if err := app.New(db, otpSecret, addr, jwtSecret, store, viewer).Run(); err != nil {
		log.Fatal(err)
	}
}
