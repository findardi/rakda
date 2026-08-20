package service

import (
	"errors"
	"testing"
)

func TestAssertUploadSize(t *testing.T) {
	tests := []struct {
		name    string
		size    int64
		wantErr bool
	}{
		{"tepat di batas", maxUploadBytes, false},
		{"satu byte di atas batas", maxUploadBytes + 1, true},
		{"jauh di atas batas", 2 << 30, true},
		{"jauh di bawah batas", 1 << 20, false},
		{"nol", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := assertUploadSize(tt.size)

			if tt.wantErr {
				if !errors.Is(err, ErrUploadTooLarge) {
					t.Errorf("assertUploadSize(%d) = %v, want ErrUploadTooLarge", tt.size, err)
				}

				return
			}

			if err != nil {
				t.Errorf("assertUploadSize(%d) = %v, want nil", tt.size, err)
			}
		})
	}
}

func TestAssertUploadable(t *testing.T) {
	if err := assertUploadable("model.xlsx"); err != nil {
		t.Errorf("assertUploadable(model.xlsx) = %v, want nil", err)
	}

	err := assertUploadable("arsip.zip")
	if !errors.Is(err, ErrNotUploadable) {
		t.Fatalf("assertUploadable(arsip.zip) = %v, want ErrNotUploadable", err)
	}

	if got := err.Error(); got != "file type cannot be stored, no PDF can be produced: .zip" {
		t.Errorf("pesan error = %q, ekstensi harus ikut", got)
	}

	if err := assertUploadable("README"); !errors.Is(err, ErrNotUploadable) {
		t.Errorf("assertUploadable(README) = %v, want ErrNotUploadable", err)
	}
}
