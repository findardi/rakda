package main

import (
	"context"
	"log"

	"github.com/findardi/Riksa-App/server/internal/app"
	contentservice "github.com/findardi/Riksa-App/server/internal/content/service"
	"github.com/findardi/Riksa-App/server/internal/platform/config"
	"github.com/findardi/Riksa-App/server/internal/platform/convert"
	"github.com/findardi/Riksa-App/server/internal/platform/database"
	"github.com/findardi/Riksa-App/server/internal/platform/render"
	"github.com/findardi/Riksa-App/server/internal/platform/storage"
	"github.com/findardi/Riksa-App/server/internal/platform/watermark"
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

	viewerCfg, err := config.LoadViewerConfig()
	if err != nil {
		log.Fatal(err)
	}

	renderer, err := render.NewPoppler(viewerCfg)
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
		PDFStamp:      watermark.NewPDFStamp(),
		TextExtractor: renderer,
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
