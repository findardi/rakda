package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/diskcache"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var cacheTestKey = bytes.Repeat([]byte{3}, diskcache.MasterKeyLen)

type cacheFakeStorage struct {
	storage.Storage

	mu       sync.Mutex
	objects  map[string][]byte
	gets     map[string]int
	puts     map[string]int
	ranges   int
	statErr  error
	getErr   error
	rangeErr error
}

func newCacheFakeStorage() *cacheFakeStorage {
	return &cacheFakeStorage{objects: map[string][]byte{}, gets: map[string]int{}, puts: map[string]int{}}
}

func (f *cacheFakeStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gets[key]++
	if f.getErr != nil {
		return nil, f.getErr
	}

	b, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("get object %s: %w", key, storage.ErrNotFound)
	}

	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *cacheFakeStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.puts[key]++
	f.objects[key] = b
	return nil
}

func (f *cacheFakeStorage) Stat(_ context.Context, key string) (int64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.statErr != nil {
		return 0, "", f.statErr
	}

	b, ok := f.objects[key]
	if !ok {
		return 0, "", fmt.Errorf("stat object %s: %w", key, storage.ErrNotFound)
	}

	return int64(len(b)), "application/pdf", nil
}

func (f *cacheFakeStorage) GetRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.ranges++
	if f.rangeErr != nil {
		return nil, f.rangeErr
	}

	b, ok := f.objects[key]
	if !ok {
		return nil, fmt.Errorf("get object %s: %w", key, storage.ErrNotFound)
	}

	return io.NopCloser(bytes.NewReader(b[offset : offset+length])), nil
}

func (f *cacheFakeStorage) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.objects, key)
	return nil
}

func (f *cacheFakeStorage) getCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets[key]
}

func (f *cacheFakeStorage) putCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.puts[key]
}

type cacheFakeRenderer struct {
	mu      sync.Mutex
	renders int
}

func (r *cacheFakeRenderer) PageCount(context.Context, io.Reader) (int, error) { return 1, nil }

func (r *cacheFakeRenderer) RenderPage(_ context.Context, pdf io.Reader, page int) ([]byte, error) {
	if _, err := io.ReadAll(pdf); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.renders++
	return fmt.Appendf(nil, "png-page-%d", page), nil
}

func (r *cacheFakeRenderer) Open(pdf io.Reader) (render.Document, error) {
	if _, err := io.ReadAll(pdf); err != nil {
		return nil, err
	}
	return cacheFakeDocument{r: r}, nil
}

func (r *cacheFakeRenderer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.renders
}

type cacheFakeDocument struct{ r *cacheFakeRenderer }

func (d cacheFakeDocument) RenderPage(ctx context.Context, page int) ([]byte, error) {
	return d.r.RenderPage(ctx, bytes.NewReader(nil), page)
}

func (d cacheFakeDocument) Close() error { return nil }

func newTierCache(t *testing.T, minFree int64) *diskcache.Cache {
	t.Helper()
	c, err := diskcache.New(t.TempDir(), 1<<30, minFree, cacheTestKey)
	require.NoError(t, err)
	return c
}

func newCacheTestService(t *testing.T, repo ContentRepository, store storage.Storage, renderer render.Render, caches CacheDeps) *ContentService {
	t.Helper()
	return NewContentService(repo, store, Viewer{Renderer: renderer, DPI: 150}, 0, nil,
		StampDeps{Sync: 1, Async: 1}, ArchiveDeps{}, caches, RenditionDeps{})
}

func TestRenditionGetReadThrough(t *testing.T) {
	store := newCacheFakeStorage()
	store.objects["w/renditions/v/rendition.pdf"] = []byte("rendition-bytes")
	renditions := newTierCache(t, 0)
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{Renditions: renditions})

	for i := 1; i <= 2; i++ {
		rc, err := svc.renditionGet(context.Background(), "w/renditions/v/rendition.pdf")
		require.NoError(t, err)
		got, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())

		assert.Equal(t, []byte("rendition-bytes"), got, "pass %d", i)
		assert.Equal(t, 1, store.getCount("w/renditions/v/rendition.pdf"), "object storage fetched once, pass %d", i)
		assert.True(t, renditions.Has("w/renditions/v/rendition.pdf"))
	}
}

func TestRenditionGetPartialReadDoesNotCache(t *testing.T) {
	store := newCacheFakeStorage()
	store.objects["k"] = []byte("0123456789")
	renditions := newTierCache(t, 0)
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{Renditions: renditions})

	rc, err := svc.renditionGet(context.Background(), "k")
	require.NoError(t, err)
	head := make([]byte, 3)
	_, err = io.ReadFull(rc, head)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	assert.False(t, renditions.Has("k"))
	leftovers, err := os.ReadDir(filepath.Join(renditions.Dir(), "tmp"))
	require.NoError(t, err)
	assert.Empty(t, leftovers)
}

func TestRenditionGetFailsOpenWhenCacheRefusesWrites(t *testing.T) {
	store := newCacheFakeStorage()
	store.objects["k"] = []byte("payload")
	renditions := newTierCache(t, math.MaxInt64)
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{Renditions: renditions})

	rc, err := svc.renditionGet(context.Background(), "k")
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	assert.Equal(t, []byte("payload"), got)
	assert.False(t, renditions.Has("k"))
}

func TestRenditionGetPropagatesStoreError(t *testing.T) {
	store := newCacheFakeStorage()
	store.getErr = errors.New("neo unreachable")
	renditions := newTierCache(t, 0)
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{Renditions: renditions})

	_, err := svc.renditionGet(context.Background(), "k")
	assert.ErrorContains(t, err, "neo unreachable")
	assert.False(t, renditions.Has("k"))
}

func TestRenditionGetWithoutCacheAlwaysHitsStore(t *testing.T) {
	store := newCacheFakeStorage()
	store.objects["k"] = []byte("payload")
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{})

	for range 2 {
		rc, err := svc.renditionGet(context.Background(), "k")
		require.NoError(t, err)
		_, err = io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
	}

	assert.Equal(t, 2, store.getCount("k"))
}

func TestPageCacheLocalTierNeverTouchesObjectStore(t *testing.T) {
	store := newCacheFakeStorage()
	store.objects["rk"] = []byte("%PDF")
	renderer := &cacheFakeRenderer{}
	pages := newTierCache(t, 0)
	svc := newCacheTestService(t, nil, store, renderer, CacheDeps{Pages: pages})

	pageKey := pageCacheKey("w", "v", 1, 150)

	first, err := svc.loadOrRenderPage(context.Background(), "w", "v", "rk", 1)
	require.NoError(t, err)
	second, err := svc.loadOrRenderPage(context.Background(), "w", "v", "rk", 1)
	require.NoError(t, err)

	assert.Equal(t, []byte("png-page-1"), first)
	assert.Equal(t, first, second)
	assert.Equal(t, 1, renderer.count(), "second read is a local hit")
	assert.True(t, pages.Has(pageKey))
	assert.Zero(t, store.getCount(pageKey))
	assert.Zero(t, store.putCount(pageKey))

	doc, err := renderer.Open(bytes.NewReader(nil))
	require.NoError(t, err)
	fromDownload, err := svc.pageForDownload(context.Background(), "w", "v", doc, 1)
	require.NoError(t, err)
	assert.Equal(t, first, fromDownload)
	assert.Equal(t, 1, renderer.count(), "download path reads the local page too")

	_, err = svc.pageForDownload(context.Background(), "w", "v", doc, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, renderer.count())
	assert.False(t, pages.Has(pageCacheKey("w", "v", 2, 150)), "download path never fills the cache")
}

func TestPageCacheFallsBackToObjectStoreWhenDisabled(t *testing.T) {
	store := newCacheFakeStorage()
	store.objects["rk"] = []byte("%PDF")
	renderer := &cacheFakeRenderer{}
	svc := newCacheTestService(t, nil, store, renderer, CacheDeps{})

	pageKey := pageCacheKey("w", "v", 1, 150)

	for range 2 {
		got, err := svc.loadOrRenderPage(context.Background(), "w", "v", "rk", 1)
		require.NoError(t, err)
		assert.Equal(t, []byte("png-page-1"), got)
	}

	assert.Equal(t, 1, renderer.count())
	assert.Equal(t, 1, store.putCount(pageKey))
	assert.Equal(t, 2, store.getCount(pageKey))
}

func spooledBody(t *testing.T, content []byte) *spooledReadCloser {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pdf")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	f, err := os.Open(path)
	require.NoError(t, err)
	return &spooledReadCloser{file: f, dir: dir}
}

func TestStoreDownloadArtifactPrefersLocalTier(t *testing.T) {
	store := newCacheFakeStorage()
	downloads := newTierCache(t, 0)
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{Downloads: downloads})

	content := bytes.Repeat([]byte("pdf"), 1000)
	body := spooledBody(t, content)
	defer body.Close()

	require.NoError(t, svc.storeDownloadArtifact(context.Background(), "downloads/w/j.pdf", body, int64(len(content))))
	assert.True(t, downloads.Has("downloads/w/j.pdf"))
	assert.Zero(t, store.putCount("downloads/w/j.pdf"))
}

func TestStoreDownloadArtifactFallsBackToObjectStore(t *testing.T) {
	content := bytes.Repeat([]byte("pdf"), 1000)

	tests := []struct {
		name   string
		caches CacheDeps
	}{
		{name: "cache disabled", caches: CacheDeps{}},
		{name: "cache refuses writes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "cache refuses writes" {
				tt.caches = CacheDeps{Downloads: newTierCache(t, math.MaxInt64)}
			}

			store := newCacheFakeStorage()
			svc := newCacheTestService(t, nil, store, nil, tt.caches)
			body := spooledBody(t, content)
			defer body.Close()

			require.NoError(t, svc.storeDownloadArtifact(context.Background(), "k", body, int64(len(content))))
			assert.Equal(t, 1, store.putCount("k"))
			assert.Equal(t, content, store.objects["k"])
			assert.False(t, tt.caches.Downloads.Has("k"))
		})
	}
}

func TestOpenDownloadJobRangeServesLocalSlice(t *testing.T) {
	store := newCacheFakeStorage()
	downloads := newTierCache(t, 0)
	svc := newCacheTestService(t, nil, store, nil, CacheDeps{Downloads: downloads})

	content := []byte("0123456789abcdefghij")
	require.NoError(t, downloads.Put("k", bytes.NewReader(content)))
	obj := DownloadJobObject{Key: "k", Size: int64(len(content))}

	rc, err := svc.OpenDownloadJobRange(context.Background(), obj, 5, 7)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	assert.Equal(t, content[5:12], got)
	assert.Zero(t, store.ranges)

	store.objects["k"] = content
	rc, err = svc.OpenDownloadJobRange(context.Background(), DownloadJobObject{Key: "k", Size: 999}, 0, 4)
	require.NoError(t, err)
	got, err = io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	assert.Equal(t, content[:4], got)
	assert.Equal(t, 1, store.ranges, "size mismatch drops the local copy and falls back")
	assert.False(t, downloads.Has("k"))
}

type lostJobFakeRepo struct {
	*downloadFakeRepo

	job  contentdb.DocumentDownloadJob
	lost []contentdb.MarkReadyDownloadJobLostParams
}

func (f *lostJobFakeRepo) GetDownloadJob(context.Context, pgtype.UUID) (contentdb.DocumentDownloadJob, error) {
	return f.job, nil
}

func (f *lostJobFakeRepo) MarkReadyDownloadJobLost(_ context.Context, arg contentdb.MarkReadyDownloadJobLostParams) error {
	f.lost = append(f.lost, arg)
	return nil
}

func readyJobFixture(t *testing.T) *lostJobFakeRepo {
	t.Helper()
	base := downloadFixture(10)

	var jobID, requester pgtype.UUID
	require.NoError(t, jobID.Scan("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
	require.NoError(t, requester.Scan(guestActor().UserID))

	return &lostJobFakeRepo{
		downloadFakeRepo: base,
		job: contentdb.DocumentDownloadJob{
			ID:           jobID,
			WorkspaceID:  base.doc.WorkspaceID,
			DocumentID:   base.doc.ID,
			VersionID:    base.version.ID,
			RequestedBy:  requester,
			DocumentName: base.doc.Name,
			Status:       DownloadJobStatusReady,
			ObjectKey:    "downloads/w/job.pdf",
			SizeBytes:    42,
			ExpiresAt:    pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		},
	}
}

func TestGetDownloadJobObjectArtifactLocation(t *testing.T) {
	const wsID = "11111111-1111-1111-1111-111111111111"
	const jobID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

	tests := []struct {
		name     string
		local    bool
		remote   bool
		statErr  error
		wantErr  error
		wantLost int
	}{
		{name: "local artifact served without stat", local: true},
		{name: "remote artifact served", remote: true},
		{name: "missing everywhere is lost", wantErr: ErrDownloadJobLost, wantLost: 1},
		{name: "transient stat error is not lost", statErr: errors.New("neo timeout"), wantErr: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := readyJobFixture(t)
			store := newCacheFakeStorage()
			store.statErr = tt.statErr
			downloads := newTierCache(t, 0)
			svc := newCacheTestService(t, repo, store, nil, CacheDeps{Downloads: downloads})

			if tt.local {
				require.NoError(t, downloads.Put(repo.job.ObjectKey, bytes.NewReader([]byte("x"))))
			}
			if tt.remote {
				store.objects[repo.job.ObjectKey] = []byte("x")
			}

			obj, err := svc.GetDownloadJobObject(context.Background(), wsID, jobID, guestActor())

			switch {
			case tt.wantErr != nil:
				assert.ErrorIs(t, err, tt.wantErr)
			case tt.statErr != nil:
				assert.ErrorContains(t, err, "neo timeout")
			default:
				require.NoError(t, err)
				assert.Equal(t, repo.job.ObjectKey, obj.Key)
				assert.Equal(t, int64(42), obj.Size)
			}

			assert.Len(t, repo.lost, tt.wantLost)
		})
	}
}
