package spool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const Prefix = "rakda-"

func CheckWritable() error {
	dir, err := os.MkdirTemp("", Prefix+"probe-*")
	if err != nil {
		return fmt.Errorf("spool dir not writable: %w", err)
	}

	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("remove probe: %w", err)
	}

	return nil
}

func SweepOrphans() (int, error) {
	dir := os.TempDir()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read spool dir: %w", err)
	}

	removed := 0
	var errs []error

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), Prefix) {
			continue
		}

		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			errs = append(errs, err)
			continue
		}

		removed++
	}

	return removed, errors.Join(errs...)
}
