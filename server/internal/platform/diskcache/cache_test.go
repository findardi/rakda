package diskcache

import (
	"bytes"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testKey    = bytes.Repeat([]byte{7}, MasterKeyLen)
	testKeyAlt = bytes.Repeat([]byte{9}, MasterKeyLen)
)

func newTestCache(t *testing.T, dir string, budget, minFree int64, key []byte) *Cache {
	t.Helper()
	c, err := New(dir, budget, minFree, key)
	require.NoError(t, err)
	return c
}

func payload(n int) []byte {
	r := rand.New(rand.NewPCG(uint64(n), 42))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.IntN(256))
	}
	return b
}

func readAll(t *testing.T, c *Cache, key string) []byte {
	t.Helper()
	r, ok := c.Open(key)
	require.True(t, ok, "expected hit for %q", key)
	defer r.Close()
	b, err := io.ReadAll(r)
	require.NoError(t, err)
	return b
}

func TestNewRejectsBadArguments(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		budget  int64
		minFree int64
		key     []byte
	}{
		{name: "short key", budget: 1 << 20, key: []byte("short")},
		{name: "zero budget", budget: 0, key: testKey},
		{name: "negative min free", budget: 1 << 20, minFree: -1, key: testKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(dir, tt.budget, tt.minFree, tt.key)
			assert.Error(t, err)
		})
	}
}

func TestRoundtrip(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)

	sizes := []int{0, 1, chunkSize - 1, chunkSize, chunkSize + 1, 2*chunkSize + 123}
	for _, n := range sizes {
		key := "renditions/" + string(rune('a'+len(sizes))) + "/" + itoa(n)
		want := payload(n)

		require.NoError(t, c.Put(key, bytes.NewReader(want)))

		r, ok := c.Open(key)
		require.True(t, ok)
		assert.Equal(t, int64(n), r.Size())
		assert.Equal(t, key, r.Key())

		got, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		assert.True(t, bytes.Equal(want, got), "size %d", n)
	}

	entries, total := c.Stats()
	assert.Equal(t, len(sizes), entries)
	assert.Greater(t, total, int64(0))
}

func TestReadSeek(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)
	want := payload(2*chunkSize + 777)
	require.NoError(t, c.Put("k", bytes.NewReader(want)))

	r, ok := c.Open("k")
	require.True(t, ok)
	defer r.Close()

	tests := []struct {
		name   string
		offset int64
		whence int
		length int
	}{
		{name: "start of second chunk", offset: chunkSize, whence: io.SeekStart, length: 10},
		{name: "across chunk boundary", offset: chunkSize - 5, whence: io.SeekStart, length: 10},
		{name: "relative", offset: 3, whence: io.SeekCurrent, length: 4},
		{name: "tail", offset: -20, whence: io.SeekEnd, length: 20},
		{name: "past end", offset: 5, whence: io.SeekEnd, length: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abs, err := r.Seek(tt.offset, tt.whence)
			require.NoError(t, err)

			buf := make([]byte, tt.length)
			n, err := io.ReadFull(r, buf)

			if abs >= int64(len(want)) {
				assert.Zero(t, n)
				assert.ErrorIs(t, err, io.EOF)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, want[abs:abs+int64(tt.length)], buf)
		})
	}

	_, err := r.Seek(-1, io.SeekStart)
	assert.Error(t, err)
}

func TestFileHoldsNoPlaintext(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)
	plain := bytes.Repeat([]byte("%PDF-1.7 \x89PNG rahasia "), 5000)
	require.NoError(t, c.Put("doc", bytes.NewReader(plain)))

	raw, err := os.ReadFile(c.pathFor("doc"))
	require.NoError(t, err)
	assert.False(t, bytes.Contains(raw, []byte("%PDF")))
	assert.False(t, bytes.Contains(raw, []byte("PNG")))
	assert.False(t, bytes.Contains(raw, []byte("rahasia")))
	assert.True(t, bytes.HasPrefix(raw, []byte(magic)))
}

func TestTamperingDetected(t *testing.T) {
	hdrLen := fixedHdrSize + len("k") + 4
	frame := chunkSize + tagSize

	tests := []struct {
		name   string
		mutate func(raw []byte) []byte
	}{
		{
			name:   "truncated by one byte",
			mutate: func(raw []byte) []byte { return raw[:len(raw)-1] },
		},
		{
			name:   "truncated at chunk boundary",
			mutate: func(raw []byte) []byte { return raw[:hdrLen+2*frame] },
		},
		{
			name: "chunks reordered",
			mutate: func(raw []byte) []byte {
				out := bytes.Clone(raw)
				copy(out[hdrLen:], raw[hdrLen+frame:hdrLen+2*frame])
				copy(out[hdrLen+frame:], raw[hdrLen:hdrLen+frame])
				return out
			},
		},
		{
			name: "bit flipped in second chunk",
			mutate: func(raw []byte) []byte {
				out := bytes.Clone(raw)
				out[hdrLen+frame+100] ^= 0x01
				return out
			},
		},
		{
			name:   "garbage appended",
			mutate: func(raw []byte) []byte { return append(bytes.Clone(raw), payload(40)...) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)
			require.NoError(t, c.Put("k", bytes.NewReader(payload(2*chunkSize+500))))

			path := c.pathFor("k")
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, tt.mutate(raw), 0o600))

			r, ok := c.Open("k")
			if !ok {
				assert.False(t, c.Has("k"))
				return
			}

			_, err = io.ReadAll(r)
			r.Close()
			assert.ErrorIs(t, err, ErrCorrupt)
			assert.False(t, c.Has("k"), "corrupt entry must be dropped")
			assert.NoFileExists(t, path)
		})
	}
}

func TestWrongMasterKeyWipes(t *testing.T) {
	dir := t.TempDir()

	c := newTestCache(t, dir, 1<<30, 0, testKey)
	require.NoError(t, c.Put("k", bytes.NewReader(payload(100))))
	path := c.pathFor("k")
	require.FileExists(t, path)

	c2 := newTestCache(t, dir, 1<<30, 0, testKeyAlt)
	assert.False(t, c2.Has("k"))
	assert.NoFileExists(t, path)

	c3 := newTestCache(t, dir, 1<<30, 0, testKey)
	assert.False(t, c3.Has("k"))
}

func TestCreateRefusedWhenLowOnSpace(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, math.MaxInt64, testKey)

	_, err := c.Create("k")
	assert.ErrorIs(t, err, ErrNoSpace)

	assert.ErrorIs(t, c.Put("k", bytes.NewReader(payload(10))), ErrNoSpace)
	assert.False(t, c.Has("k"))

	leftovers, err := os.ReadDir(filepath.Join(c.Dir(), tmpDirName))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestAbortLeavesNothing(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)

	w, err := c.Create("k")
	require.NoError(t, err)
	_, err = w.Write(payload(100))
	require.NoError(t, err)
	w.Abort()
	require.NoError(t, w.Close())

	assert.False(t, c.Has("k"))
	leftovers, err := os.ReadDir(filepath.Join(c.Dir(), tmpDirName))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestBudgetEvictsLeastRecentlyUsed(t *testing.T) {
	dir := t.TempDir()
	c := newTestCache(t, dir, 1<<30, 0, testKey)

	for _, k := range []string{"a", "b", "c"} {
		require.NoError(t, c.Put(k, bytes.NewReader(payload(1000))))
	}
	_, total := c.Stats()
	perEntry := total / 3

	base := time.Now().Add(-time.Hour)
	for i, k := range []string{"a", "b", "c"} {
		ts := base.Add(time.Duration(i) * time.Minute)
		require.NoError(t, os.Chtimes(c.pathFor(k), ts, ts))
	}

	c = newTestCache(t, dir, total, 0, testKey)
	assert.True(t, c.Has("a") && c.Has("b") && c.Has("c"))

	_ = readAll(t, c, "a")

	require.NoError(t, c.Put("d", bytes.NewReader(payload(1000))))

	assert.True(t, c.Has("a"), "touched entry survives")
	assert.False(t, c.Has("b"), "least recently used goes first")
	assert.False(t, c.Has("c"), "eviction continues down to the low-water mark")
	assert.True(t, c.Has("d"))

	_, total = c.Stats()
	assert.LessOrEqual(t, total, 3*perEntry-3*perEntry/10)
}

func TestSweepMaxAgeAndBudget(t *testing.T) {
	dir := t.TempDir()
	c := newTestCache(t, dir, 1<<30, 0, testKey)

	for _, k := range []string{"old", "fresh1", "fresh2"} {
		require.NoError(t, c.Put(k, bytes.NewReader(payload(500))))
	}
	stale := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(c.pathFor("old"), stale, stale))

	c = newTestCache(t, dir, 1<<30, 0, testKey)
	removed, freed := c.Sweep(24 * time.Hour)
	assert.Equal(t, 1, removed)
	assert.Greater(t, freed, int64(0))
	assert.False(t, c.Has("old"))
	assert.True(t, c.Has("fresh1") && c.Has("fresh2"))

	_, total := c.Stats()
	c = newTestCache(t, dir, total*3/4, 0, testKey)
	removed, _ = c.Sweep(0)
	assert.Equal(t, 1, removed)
	entries, total2 := c.Stats()
	assert.Equal(t, 1, entries)
	assert.LessOrEqual(t, total2, total*3/4)
}

func TestSweepYieldsToFreeSpacePressure(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)
	for _, k := range []string{"a", "b"} {
		require.NoError(t, c.Put(k, bytes.NewReader(payload(500))))
	}

	c.minFree = math.MaxInt64
	removed, _ := c.Sweep(0)
	assert.Equal(t, 2, removed)
	entries, total := c.Stats()
	assert.Zero(t, entries)
	assert.Zero(t, total)
}

func TestRemoveAndRemovePrefix(t *testing.T) {
	c := newTestCache(t, t.TempDir(), 1<<30, 0, testKey)
	for _, k := range []string{"ws/renditions/v1/rendition.pdf", "ws/renditions/v2/rendition.pdf", "ws/raw/v1"} {
		require.NoError(t, c.Put(k, bytes.NewReader(payload(10))))
	}

	require.NoError(t, c.Remove("ws/raw/v1"))
	require.NoError(t, c.Remove("ws/raw/v1"))
	assert.False(t, c.Has("ws/raw/v1"))

	n, err := c.RemovePrefix("ws/renditions/")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	entries, total := c.Stats()
	assert.Zero(t, entries)
	assert.Zero(t, total)
}

func TestIndexRebuildAfterRestart(t *testing.T) {
	dir := t.TempDir()
	c := newTestCache(t, dir, 1<<30, 0, testKey)
	for _, k := range []string{"a", "b", "c"} {
		require.NoError(t, c.Put(k, bytes.NewReader(payload(300))))
	}
	entriesBefore, totalBefore := c.Stats()

	require.NoError(t, os.WriteFile(filepath.Join(dir, tmpDirName, "put-leftover"), []byte("x"), 0o600))

	shard := filepath.Dir(c.pathFor("a"))
	foreign := filepath.Join(shard, "not-a-cache-file")
	require.NoError(t, os.WriteFile(foreign, []byte("garbage"), 0o600))

	misplaced := filepath.Join(shard, "0000000000000000000000000000000000000000000000000000000000000000")
	raw, err := os.ReadFile(c.pathFor("b"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(misplaced, raw, 0o600))

	c = newTestCache(t, dir, 1<<30, 0, testKey)

	entriesAfter, totalAfter := c.Stats()
	assert.Equal(t, entriesBefore, entriesAfter)
	assert.Equal(t, totalBefore, totalAfter)
	for _, k := range []string{"a", "b", "c"} {
		assert.Equal(t, payload(300), readAll(t, c, k))
	}

	assert.NoFileExists(t, filepath.Join(dir, tmpDirName, "put-leftover"))
	assert.NoFileExists(t, foreign)
	assert.NoFileExists(t, misplaced)
}

func TestNilCacheIsDisabled(t *testing.T) {
	var c *Cache

	_, ok := c.Open("k")
	assert.False(t, ok)

	_, err := c.Create("k")
	assert.ErrorIs(t, err, ErrDisabled)
	assert.ErrorIs(t, c.Put("k", bytes.NewReader(nil)), ErrDisabled)

	assert.NoError(t, c.Remove("k"))
	n, err := c.RemovePrefix("k")
	assert.NoError(t, err)
	assert.Zero(t, n)

	removed, freed := c.Sweep(time.Hour)
	assert.Zero(t, removed)
	assert.Zero(t, freed)
	assert.False(t, c.Has("k"))
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
