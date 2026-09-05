package config

import (
	"bufio"
	"fmt"
	"log"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open file env: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid env line %d: missing '='", lineNo)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)

		if key == "" {
			return fmt.Errorf("empty key on line %d", lineNo)
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s:%w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read env file: %w", err)
	}

	return nil

}

func GetEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}

	return fallback
}

func GetEnvInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return n, nil
}

// EnvIntOr and EnvDurationOr read a tuning knob. A value that does not parse
// is logged and replaced by the fallback, never fatal: a typo in a knob must
// not stop the boot. Config that must be right (DB, storage, viewer) keeps the
// error-returning getters.
func EnvIntOr(key string, fallback int) int {
	n, err := GetEnvInt(key, fallback)
	if err != nil {
		log.Printf("%v, fallback to %d", err, fallback)
		return fallback
	}
	return n
}

func EnvDurationOr(key string, fallback time.Duration) time.Duration {
	d, err := GetEnvDuration(key, fallback)
	if err != nil {
		log.Printf("%v, fallback to %s", err, fallback)
		return fallback
	}
	return d
}

func GetEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}

	t, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}

	return t, nil
}

// GetEnvCIDRList membaca daftar CIDR dipisah koma; entri kosong dilewati,
// entri tidak valid → error (jangan diam-diam dilewati). Env tidak diset →
// fallback.
func GetEnvCIDRList(key string, fallback []netip.Prefix) ([]netip.Prefix, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}

	var out []netip.Prefix
	for _, raw := range strings.Split(v, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid %s entry %q: %w", key, raw, err)
		}
		out = append(out, p)
	}

	return out, nil
}
