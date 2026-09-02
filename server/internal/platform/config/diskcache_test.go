package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "plain bytes", input: "1024", want: 1024},
		{name: "kilo", input: "8k", want: 8 << 10},
		{name: "mega upper", input: "16M", want: 16 << 20},
		{name: "giga with spaces", input: " 20g ", want: 20 << 30},
		{name: "zero", input: "0", want: 0},
		{name: "empty", input: "", wantErr: true},
		{name: "negative", input: "-1g", wantErr: true},
		{name: "unknown suffix", input: "5t", wantErr: true},
		{name: "not a number", input: "abc", wantErr: true},
		{name: "overflow", input: "9223372036854775807g", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBytes(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadDiskCacheConfig(t *testing.T) {
	validKey := base64.StdEncoding.EncodeToString(make([]byte, diskCacheKeyLen))

	tests := []struct {
		name        string
		env         map[string]string
		wantEnabled bool
		wantErr     bool
		check       func(t *testing.T, cfg DiskCacheConfig)
	}{
		{
			name: "disabled when dir empty",
			env:  map[string]string{"DISK_CACHE_DIR": ""},
		},
		{
			name:    "dir without key",
			env:     map[string]string{"DISK_CACHE_DIR": "/var/cache/rakda"},
			wantErr: true,
		},
		{
			name:    "key wrong length",
			env:     map[string]string{"DISK_CACHE_DIR": "/var/cache/rakda", "DISK_CACHE_KEY": base64.StdEncoding.EncodeToString([]byte("short"))},
			wantErr: true,
		},
		{
			name:    "key not base64",
			env:     map[string]string{"DISK_CACHE_DIR": "/var/cache/rakda", "DISK_CACHE_KEY": "***"},
			wantErr: true,
		},
		{
			name:        "defaults",
			env:         map[string]string{"DISK_CACHE_DIR": "/var/cache/rakda", "DISK_CACHE_KEY": validKey},
			wantEnabled: true,
			check: func(t *testing.T, cfg DiskCacheConfig) {
				assert.Equal(t, int64(20<<30), cfg.RenditionBudget)
				assert.Equal(t, int64(20<<30), cfg.PageBudget)
				assert.Equal(t, int64(5<<30), cfg.DownloadBudget)
				assert.Equal(t, int64(5<<30), cfg.MinFree)
				assert.Len(t, cfg.Key, diskCacheKeyLen)
			},
		},
		{
			name: "overrides",
			env: map[string]string{
				"DISK_CACHE_DIR":              "/var/cache/rakda",
				"DISK_CACHE_KEY":              validKey,
				"DISK_CACHE_RENDITION_BUDGET": "1g",
				"DISK_CACHE_PAGE_BUDGET":      "512m",
				"DISK_CACHE_DOWNLOAD_BUDGET":  "256m",
				"DISK_CACHE_MIN_FREE":         "0",
			},
			wantEnabled: true,
			check: func(t *testing.T, cfg DiskCacheConfig) {
				assert.Equal(t, int64(1<<30), cfg.RenditionBudget)
				assert.Equal(t, int64(512<<20), cfg.PageBudget)
				assert.Equal(t, int64(256<<20), cfg.DownloadBudget)
				assert.Equal(t, int64(0), cfg.MinFree)
			},
		},
		{
			name:    "bad budget",
			env:     map[string]string{"DISK_CACHE_DIR": "/var/cache/rakda", "DISK_CACHE_KEY": validKey, "DISK_CACHE_PAGE_BUDGET": "lots"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"DISK_CACHE_DIR", "DISK_CACHE_KEY", "DISK_CACHE_RENDITION_BUDGET", "DISK_CACHE_PAGE_BUDGET", "DISK_CACHE_DOWNLOAD_BUDGET", "DISK_CACHE_MIN_FREE"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := LoadDiskCacheConfig()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantEnabled, cfg.Enabled())
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}
