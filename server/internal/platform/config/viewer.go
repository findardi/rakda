package config

import "time"

type ViewerConfig struct {
	GotenbergURL      string
	ConvertTimeout    time.Duration
	DPI               int
	RenderTimeout     time.Duration
	RenderConcurrency int
	SweepConcurrency  int
	SweepNice         int

	// Kolam poppler job unduhan ber-watermark latar. Default 2 = stampWorkers
	// di content/service (tidak bisa diimpor: render → config, service →
	// render). Belum diukur di box 2 vCPU (U-70).
	DownloadConcurrency int
	DownloadNice        int
}

func LoadViewerConfig() (ViewerConfig, error) {
	convertTimeout, err := GetEnvDuration("VIEWER_CONVERT_TIMEOUT", 3*time.Minute)
	if err != nil {
		return ViewerConfig{}, err
	}

	renderTimeout, err := GetEnvDuration("VIEWER_RENDER_TIMEOUT", 30*time.Second)
	if err != nil {
		return ViewerConfig{}, err
	}

	dpi, err := GetEnvInt("VIEWER_DPI", 150)
	if err != nil {
		return ViewerConfig{}, err
	}

	concurrency, err := GetEnvInt("VIEWER_RENDER_CONCURRENCY", 2)
	if err != nil {
		return ViewerConfig{}, err
	}

	sweepConcurrency, err := GetEnvInt("VIEWER_SWEEP_CONCURRENCY", 1)
	if err != nil {
		return ViewerConfig{}, err
	}

	sweepNice, err := GetEnvInt("VIEWER_SWEEP_NICE", 10)
	if err != nil {
		return ViewerConfig{}, err
	}

	downloadConcurrency, err := GetEnvInt("VIEWER_DOWNLOAD_CONCURRENCY", 2)
	if err != nil {
		return ViewerConfig{}, err
	}

	downloadNice, err := GetEnvInt("VIEWER_DOWNLOAD_NICE", 10)
	if err != nil {
		return ViewerConfig{}, err
	}

	if dpi <= 0 {
		dpi = 150
	}

	return ViewerConfig{
		GotenbergURL:        GetEnv("GOTENBERG_URL", "http://localhost:3000"),
		ConvertTimeout:      convertTimeout,
		DPI:                 dpi,
		RenderTimeout:       renderTimeout,
		RenderConcurrency:   max(concurrency, 1),
		SweepConcurrency:    max(sweepConcurrency, 1),
		SweepNice:           max(sweepNice, 0),
		DownloadConcurrency: max(downloadConcurrency, 1),
		DownloadNice:        max(downloadNice, 0),
	}, nil
}
