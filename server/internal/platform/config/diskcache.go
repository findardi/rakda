package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const diskCacheKeyLen = 32

type DiskCacheConfig struct {
	Dir             string
	Key             []byte
	RenditionBudget int64
	PageBudget      int64
	DownloadBudget  int64
	MinFree         int64
}

func (c DiskCacheConfig) Enabled() bool { return c.Dir != "" }

func LoadDiskCacheConfig() (DiskCacheConfig, error) {
	dir := strings.TrimSpace(GetEnv("DISK_CACHE_DIR", ""))
	if dir == "" {
		return DiskCacheConfig{}, nil
	}

	rawKey := strings.TrimSpace(GetEnv("DISK_CACHE_KEY", ""))
	if rawKey == "" {
		return DiskCacheConfig{}, errors.New("DISK_CACHE_KEY wajib diisi bila DISK_CACHE_DIR terisi")
	}

	key, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil {
		return DiskCacheConfig{}, fmt.Errorf("invalid DISK_CACHE_KEY: %w", err)
	}

	if len(key) != diskCacheKeyLen {
		return DiskCacheConfig{}, fmt.Errorf("invalid DISK_CACHE_KEY: want %d bytes, got %d", diskCacheKeyLen, len(key))
	}

	renditionBudget, err := GetEnvBytes("DISK_CACHE_RENDITION_BUDGET", 20<<30)
	if err != nil {
		return DiskCacheConfig{}, err
	}

	pageBudget, err := GetEnvBytes("DISK_CACHE_PAGE_BUDGET", 20<<30)
	if err != nil {
		return DiskCacheConfig{}, err
	}

	downloadBudget, err := GetEnvBytes("DISK_CACHE_DOWNLOAD_BUDGET", 5<<30)
	if err != nil {
		return DiskCacheConfig{}, err
	}

	minFree, err := GetEnvBytes("DISK_CACHE_MIN_FREE", 5<<30)
	if err != nil {
		return DiskCacheConfig{}, err
	}

	return DiskCacheConfig{
		Dir:             dir,
		Key:             key,
		RenditionBudget: renditionBudget,
		PageBudget:      pageBudget,
		DownloadBudget:  downloadBudget,
		MinFree:         minFree,
	}, nil
}

func GetEnvBytes(key string, fallback int64) (int64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}

	n, err := ParseBytes(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return n, nil
}

func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, errors.New("empty size")
	}

	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "g"):
		mult, s = 1<<30, strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		mult, s = 1<<20, strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		mult, s = 1<<10, strings.TrimSuffix(s, "k")
	}

	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, err
	}

	if n < 0 {
		return 0, errors.New("negative size")
	}

	if mult > 1 && n > (1<<62)/mult {
		return 0, errors.New("size overflows int64")
	}

	return n * mult, nil
}
