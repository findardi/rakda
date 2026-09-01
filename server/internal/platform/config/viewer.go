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
}

func LoadViewerConfig() (ViewerConfig, error) {
	convertTimeout, err := GetEnvDuration("VIEWER_CONVERT_TIMEOUT", 2*time.Minute)
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

	if dpi <= 0 {
		dpi = 150
	}

	if concurrency <= 0 {
		concurrency = 1
	}

	if sweepConcurrency <= 0 {
		sweepConcurrency = 1
	}

	if sweepNice < 0 {
		sweepNice = 0
	}

	return ViewerConfig{
		GotenbergURL:      GetEnv("GOTENBERG_URL", "http://localhost:3000"),
		ConvertTimeout:    convertTimeout,
		DPI:               dpi,
		RenderTimeout:     renderTimeout,
		RenderConcurrency: concurrency,
		SweepConcurrency:  sweepConcurrency,
		SweepNice:         sweepNice,
	}, nil
}
