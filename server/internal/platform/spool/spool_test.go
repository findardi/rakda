package spool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckWritable(t *testing.T) {
	tests := []struct {
		name    string
		dir     func(t *testing.T) string
		wantErr bool
	}{
		{
			name: "writable dir",
			dir:  func(t *testing.T) string { return t.TempDir() },
		},
		{
			name:    "missing dir",
			dir:     func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") },
			wantErr: true,
		},
		{
			name:    "read-only dir",
			dir:     readOnlyDir,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", tt.dir(t))

			err := CheckWritable()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			entries, err := os.ReadDir(os.TempDir())
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestSweepOrphans(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, dir string)
		wantRemoved int
		wantErr     bool
		wantLeft    []string
		check       func(t *testing.T, dir string)
	}{
		{
			name:  "empty dir",
			setup: func(t *testing.T, dir string) {},
		},
		{
			name: "removes prefixed entries and keeps the rest",
			setup: func(t *testing.T, dir string) {
				mkdirAll(t, filepath.Join(dir, Prefix+"wm-1", "pages"))
				writeFile(t, filepath.Join(dir, Prefix+"wm-1", "pages", "1.jpg"))
				writeFile(t, filepath.Join(dir, Prefix+"rendition-2.pdf"))
				mkdirAll(t, filepath.Join(dir, "other-app"))
				writeFile(t, filepath.Join(dir, "notes.txt"))
				writeFile(t, filepath.Join(dir, "xrakda-suffix"))
			},
			wantRemoved: 2,
			wantLeft:    []string{"notes.txt", "other-app", "xrakda-suffix"},
		},
		{
			name: "prefixed symlink is unlinked without touching its target",
			setup: func(t *testing.T, dir string) {
				target := filepath.Join(dir, "target")
				mkdirAll(t, target)
				writeFile(t, filepath.Join(target, "keep.txt"))
				require.NoError(t, os.Symlink(target, filepath.Join(dir, Prefix+"link")))
			},
			wantRemoved: 1,
			wantLeft:    []string{"target"},
			check: func(t *testing.T, dir string) {
				assert.FileExists(t, filepath.Join(dir, "target", "keep.txt"))
			},
		},
		{
			name: "unremovable entry is reported and does not stop the sweep",
			setup: func(t *testing.T, dir string) {
				if os.Geteuid() == 0 {
					t.Skip("root bypasses directory permissions")
				}
				mkdirAll(t, filepath.Join(dir, Prefix+"a"))
				locked := filepath.Join(dir, Prefix+"locked")
				mkdirAll(t, locked)
				writeFile(t, filepath.Join(locked, "child"))
				require.NoError(t, os.Chmod(locked, 0o500))
				t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
				mkdirAll(t, filepath.Join(dir, Prefix+"z"))
			},
			wantRemoved: 2,
			wantErr:     true,
			wantLeft:    []string{Prefix + "locked"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			tt.setup(t, dir)

			removed, err := SweepOrphans()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantRemoved, removed)

			entries, readErr := os.ReadDir(dir)
			require.NoError(t, readErr)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}
			assert.ElementsMatch(t, tt.wantLeft, names)

			if tt.check != nil {
				tt.check(t, dir)
			}
		})
	}
}

func TestSweepOrphansMissingDir(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	removed, err := SweepOrphans()
	assert.Error(t, err)
	assert.Equal(t, 0, removed)
}

func readOnlyDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	return dir
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, 0o700))
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
}
