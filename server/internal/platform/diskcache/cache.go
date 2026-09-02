package diskcache

import (
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	keyFile         = "KEY"
	tmpDirName      = "tmp"
	fingerprintInfo = "rakda-diskcache-fingerprint"
	maxKeyLen       = 1 << 12
)

var (
	ErrDisabled = errors.New("diskcache: disabled")
	ErrNoSpace  = errors.New("diskcache: not enough free space")
)

type entry struct {
	path     string
	size     int64
	lastUsed time.Time
}

type Cache struct {
	dir     string
	budget  int64
	minFree int64
	key     []byte

	mu    sync.Mutex
	index map[string]entry
	total int64
}

func New(dir string, budget, minFree int64, masterKey []byte) (*Cache, error) {
	if len(masterKey) != MasterKeyLen {
		return nil, fmt.Errorf("diskcache: master key must be %d bytes, got %d", MasterKeyLen, len(masterKey))
	}

	if budget <= 0 {
		return nil, errors.New("diskcache: budget must be positive")
	}

	if minFree < 0 {
		return nil, errors.New("diskcache: min free must not be negative")
	}

	if err := os.MkdirAll(filepath.Join(dir, tmpDirName), 0o700); err != nil {
		return nil, fmt.Errorf("diskcache: create dir: %w", err)
	}

	c := &Cache{
		dir:     dir,
		budget:  budget,
		minFree: minFree,
		key:     slices.Clone(masterKey),
		index:   make(map[string]entry),
	}

	if err := c.ensureFingerprint(); err != nil {
		return nil, err
	}

	if err := c.clearTmp(); err != nil {
		return nil, err
	}

	if err := c.rebuildIndex(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Cache) Dir() string { return c.dir }

func (c *Cache) fingerprint() (string, error) {
	fp, err := hkdf.Key(sha256.New, c.key, nil, fingerprintInfo, 32)
	if err != nil {
		return "", fmt.Errorf("diskcache: fingerprint: %w", err)
	}

	return hex.EncodeToString(fp), nil
}

func (c *Cache) ensureFingerprint() error {
	want, err := c.fingerprint()
	if err != nil {
		return err
	}

	path := filepath.Join(c.dir, keyFile)
	got, err := os.ReadFile(path)
	if err == nil && strings.TrimSpace(string(got)) == want {
		return nil
	}

	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("diskcache: read fingerprint: %w", err)
	}

	if err := c.wipeShards(); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(want+"\n"), 0o600); err != nil {
		return fmt.Errorf("diskcache: write fingerprint: %w", err)
	}

	return nil
}

func (c *Cache) wipeShards() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("diskcache: read dir: %w", err)
	}

	for _, e := range entries {
		if !isShard(e) {
			continue
		}

		if err := os.RemoveAll(filepath.Join(c.dir, e.Name())); err != nil {
			return fmt.Errorf("diskcache: wipe shard %s: %w", e.Name(), err)
		}
	}

	return nil
}

func isShard(e fs.DirEntry) bool {
	if !e.IsDir() || len(e.Name()) != 2 {
		return false
	}

	_, err := hex.DecodeString(e.Name())
	return err == nil
}

func (c *Cache) clearTmp() error {
	dir := filepath.Join(c.dir, tmpDirName)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("diskcache: read tmp: %w", err)
	}

	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("diskcache: clear tmp: %w", err)
		}
	}

	return nil
}

func (c *Cache) rebuildIndex() error {
	shards, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("diskcache: read dir: %w", err)
	}

	for _, shard := range shards {
		if !isShard(shard) {
			continue
		}

		files, err := os.ReadDir(filepath.Join(c.dir, shard.Name()))
		if err != nil {
			return fmt.Errorf("diskcache: read shard %s: %w", shard.Name(), err)
		}

		for _, f := range files {
			if f.IsDir() {
				continue
			}

			c.adopt(filepath.Join(c.dir, shard.Name(), f.Name()))
		}
	}

	return nil
}

func (c *Cache) adopt(path string) {
	key, size, mtime, ok := inspect(path)
	if !ok || c.pathFor(key) != path {
		os.Remove(path)
		return
	}

	c.index[key] = entry{path: path, size: size, lastUsed: mtime}
	c.total += size
}

func inspect(path string) (key string, size int64, mtime time.Time, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, time.Time{}, false
	}
	defer f.Close()

	h, err := readHeader(f)
	if err != nil {
		return "", 0, time.Time{}, false
	}

	fi, err := f.Stat()
	if err != nil {
		return "", 0, time.Time{}, false
	}

	return h.key, fi.Size(), fi.ModTime(), true
}

func (c *Cache) pathFor(key string) string {
	sum := sha256.Sum256([]byte(key))
	name := hex.EncodeToString(sum[:])
	return filepath.Join(c.dir, name[:2], name)
}

func (c *Cache) Has(key string) bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.index[key]
	return ok
}

func (c *Cache) Stats() (entries int, bytes int64) {
	if c == nil {
		return 0, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.index), c.total
}

func (c *Cache) Open(key string) (*Reader, bool) {
	if c == nil {
		return nil, false
	}

	c.mu.Lock()
	e, ok := c.index[key]
	c.mu.Unlock()

	if !ok {
		return nil, false
	}

	r, err := openReader(e.path, c.key, func() { _ = c.Remove(key) })
	if err != nil {
		_ = c.Remove(key)
		return nil, false
	}

	if r.Key() != key {
		r.Close()
		_ = c.Remove(key)
		return nil, false
	}

	c.touch(key)
	return r, true
}

func (c *Cache) touch(key string) {
	now := time.Now()

	c.mu.Lock()
	e, ok := c.index[key]
	if ok {
		e.lastUsed = now
		c.index[key] = e
	}
	c.mu.Unlock()

	if ok {
		_ = os.Chtimes(e.path, now, now)
	}
}

type Writer struct {
	c    *Cache
	key  string
	f    *os.File
	enc  *encWriter
	done bool
}

func (c *Cache) Create(key string) (*Writer, error) {
	if c == nil {
		return nil, ErrDisabled
	}

	if key == "" || len(key) > maxKeyLen {
		return nil, fmt.Errorf("diskcache: invalid key length %d", len(key))
	}

	free, err := freeBytes(c.dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoSpace, err)
	}

	if free < c.minFree {
		return nil, ErrNoSpace
	}

	h, err := newHeader(key)
	if err != nil {
		return nil, err
	}

	aead, err := newAEAD(c.key, h.salt[:])
	if err != nil {
		return nil, err
	}

	f, err := os.CreateTemp(filepath.Join(c.dir, tmpDirName), "put-*")
	if err != nil {
		return nil, fmt.Errorf("diskcache: temp file: %w", err)
	}

	if _, err := f.Write(h.raw); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("diskcache: write header: %w", err)
	}

	return &Writer{c: c, key: key, f: f, enc: newEncWriter(f, aead, h.raw, chunkSize)}, nil
}

func (w *Writer) Write(p []byte) (int, error) {
	if w.done {
		return 0, errors.New("diskcache: write after close")
	}

	return w.enc.Write(p)
}

func (w *Writer) Abort() {
	if w.done {
		return
	}

	w.done = true
	w.f.Close()
	os.Remove(w.f.Name())
}

func (w *Writer) Close() error {
	if w.done {
		return nil
	}

	w.done = true
	tmp := w.f.Name()

	if err := w.enc.Close(); err != nil {
		w.f.Close()
		os.Remove(tmp)
		return fmt.Errorf("diskcache: seal: %w", err)
	}

	if err := w.f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("diskcache: close: %w", err)
	}

	fi, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("diskcache: stat: %w", err)
	}

	dst := w.c.pathFor(w.key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("diskcache: shard dir: %w", err)
	}

	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("diskcache: rename: %w", err)
	}

	w.c.commit(w.key, dst, fi.Size())
	return nil
}

func (c *Cache) commit(key, path string, size int64) {
	c.mu.Lock()
	if old, ok := c.index[key]; ok {
		c.total -= old.size
	}
	c.index[key] = entry{path: path, size: size, lastUsed: time.Now()}
	c.total += size
	victims := c.evictToBudgetLocked()
	c.mu.Unlock()

	unlinkAll(victims)
}

func (c *Cache) evictToBudgetLocked() []entry {
	if c.total <= c.budget {
		return nil
	}

	return c.evictLocked(c.budget - c.budget/10)
}

func (c *Cache) Put(key string, r io.Reader) error {
	w, err := c.Create(key)
	if err != nil {
		return err
	}

	if _, err := io.Copy(w, r); err != nil {
		w.Abort()
		return err
	}

	return w.Close()
}

func (c *Cache) Remove(key string) error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	e, ok := c.index[key]
	if ok {
		delete(c.index, key)
		c.total -= e.size
	}
	c.mu.Unlock()

	if !ok {
		return nil
	}

	if err := os.Remove(e.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("diskcache: remove: %w", err)
	}

	return nil
}

func (c *Cache) RemovePrefix(prefix string) (int, error) {
	if c == nil {
		return 0, nil
	}

	var victims []entry

	c.mu.Lock()
	for key, e := range c.index {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		delete(c.index, key)
		c.total -= e.size
		victims = append(victims, e)
	}
	c.mu.Unlock()

	return len(victims), unlinkAll(victims)
}

func (c *Cache) Sweep(maxAge time.Duration) (removed int, freed int64) {
	if c == nil {
		return 0, 0
	}

	var victims []entry

	c.mu.Lock()
	if maxAge > 0 {
		cutoff := time.Now().Add(-maxAge)
		for key, e := range c.index {
			if e.lastUsed.Before(cutoff) {
				delete(c.index, key)
				c.total -= e.size
				victims = append(victims, e)
			}
		}
	}
	victims = append(victims, c.evictToBudgetLocked()...)
	c.mu.Unlock()

	unlinkAll(victims)

	for {
		free, err := freeBytes(c.dir)
		if err != nil || free >= c.minFree {
			break
		}

		c.mu.Lock()
		more := c.evictLocked(c.total - (c.minFree - free))
		c.mu.Unlock()

		if len(more) == 0 {
			break
		}

		unlinkAll(more)
		victims = append(victims, more...)
	}

	for _, e := range victims {
		freed += e.size
	}

	return len(victims), freed
}

func (c *Cache) evictLocked(target int64) []entry {
	if c.total <= target {
		return nil
	}

	type keyed struct {
		key string
		entry
	}

	all := make([]keyed, 0, len(c.index))
	for key, e := range c.index {
		all = append(all, keyed{key: key, entry: e})
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].lastUsed.Before(all[j].lastUsed)
	})

	var victims []entry
	for _, k := range all {
		if c.total <= target {
			break
		}

		delete(c.index, k.key)
		c.total -= k.size
		victims = append(victims, k.entry)
	}

	return victims
}

func unlinkAll(victims []entry) error {
	var errs []error
	for _, e := range victims {
		if err := os.Remove(e.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
