package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"sync/atomic"
	"testing"
	"time"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	downloadTestWorkspaceID  = "11111111-1111-1111-1111-111111111111"
	downloadTestDocumentID   = "33333333-3333-3333-3333-333333333333"
	downloadTestVersionID    = "44444444-4444-4444-4444-444444444444"
	downloadTestRenditionKey = "renditions/rendition.pdf"
)

// markFailedCall merekam apa yang dilihat repo saat job ditandai gagal.
// ctx.Err() pada saat panggilan adalah satu-satunya bukti bahwa fase store
// berjalan di konteks hidup, bukan di konteks raster yang sudah kedaluwarsa.
type markFailedCall struct {
	ctxErr error
	params contentdb.MarkDownloadJobFailedParams
}

type downloadFakeRepo struct {
	ContentRepository

	doc     contentdb.Document
	version contentdb.DocumentVersion
	access  contentdb.ResolveFolderAccessRow
	created chan contentdb.CreateDownloadJobParams
	failed  chan markFailedCall
}

func (f *downloadFakeRepo) GetDocumentByID(context.Context, pgtype.UUID) (contentdb.Document, error) {
	return f.doc, nil
}

func (f *downloadFakeRepo) ResolveFolderAccess(context.Context, contentdb.ResolveFolderAccessParams) (contentdb.ResolveFolderAccessRow, error) {
	return f.access, nil
}

func (f *downloadFakeRepo) GetCurrentVersion(context.Context, pgtype.UUID) (contentdb.DocumentVersion, error) {
	return f.version, nil
}

func (f *downloadFakeRepo) GetPendingDownloadJob(context.Context, contentdb.GetPendingDownloadJobParams) (contentdb.DocumentDownloadJob, error) {
	return contentdb.DocumentDownloadJob{}, pgx.ErrNoRows
}

func (f *downloadFakeRepo) CreateDownloadJob(_ context.Context, arg contentdb.CreateDownloadJobParams) (contentdb.DocumentDownloadJob, error) {
	f.created <- arg
	return contentdb.DocumentDownloadJob{ID: downloadTestJobID(), PageCount: arg.PageCount}, nil
}

func (f *downloadFakeRepo) MarkDownloadJobFailed(ctx context.Context, arg contentdb.MarkDownloadJobFailedParams) error {
	f.failed <- markFailedCall{ctxErr: ctx.Err(), params: arg}
	return nil
}

// countingRenderer menghitung Open. Embed nil sengaja: PageCount/RenderPage
// pada renderer tidak boleh tersentuh di jalur unduhan (halaman dirender lewat
// Document), jadi panggilan ke sana panic dan tes langsung gagal.
type countingRenderer struct {
	render.Render

	calls atomic.Int32
	open  func(io.Reader) (render.Document, error)
}

func (r *countingRenderer) Open(pdf io.Reader) (render.Document, error) {
	r.calls.Add(1)
	return r.open(pdf)
}

func failingOpen(io.Reader) (render.Document, error) {
	return nil, errors.New("renderer unavailable in test")
}

func blockingOpen(io.Reader) (render.Document, error) {
	return blockingDocument{}, nil
}

// blockingDocument menahan RenderPage sampai ctx mati: cara mendorong tenggat
// raster kedaluwarsa tanpa menunggu 45 menit.
type blockingDocument struct{}

func (blockingDocument) RenderPage(ctx context.Context, _ int) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingDocument) Close() error { return nil }

// unexpectedWatermarker menggantikan nil: bila jalur tes salah dan BurnImage
// tercapai, hasilnya asersi gagal, bukan panic nil interface.
type unexpectedWatermarker struct{}

func (unexpectedWatermarker) Burn([]byte, watermark.Mark) ([]byte, error) {
	return nil, errors.New("unexpected Burn")
}

func (unexpectedWatermarker) BurnImage([]byte, watermark.Mark) (*image.RGBA, error) {
	return nil, errors.New("unexpected BurnImage")
}

func downloadTestJobID() pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	return id
}

// renditionOnlyStore mengembalikan byte hanya untuk kunci rendition. Kunci
// page-cache harus miss: pageForDownload memakai hasil Get tanpa menyentuh
// renderer, jadi hit palsu akan melewati RenderPage dan mencapai BurnImage.
func renditionOnlyStore() fakeStorage {
	return fakeStorage{
		getFn: func(_ context.Context, key string) (io.ReadCloser, error) {
			if key != downloadTestRenditionKey {
				return nil, errors.New("miss")
			}
			return io.NopCloser(bytes.NewReader([]byte("rendition"))), nil
		},
	}
}

func newDownloadTestService(t *testing.T, repo *downloadFakeRepo, request, downloadJob render.Render) *ContentService {
	t.Helper()

	return NewContentService(repo, renditionOnlyStore(), Viewer{
		Renderer:            request,
		DownloadJobRenderer: downloadJob,
		Watermark:           unexpectedWatermarker{},
		DPI:                 150,
	}, 0, nil, StampDeps{Sync: 1, Async: 1}, ArchiveDeps{}, CacheDeps{})
}

func downloadFixture(pageCount int32) *downloadFakeRepo {
	var wID, fID, dID, vID pgtype.UUID
	_ = wID.Scan(downloadTestWorkspaceID)
	_ = fID.Scan("22222222-2222-2222-2222-222222222222")
	_ = dID.Scan(downloadTestDocumentID)
	_ = vID.Scan(downloadTestVersionID)

	key := downloadTestRenditionKey

	return &downloadFakeRepo{
		created: make(chan contentdb.CreateDownloadJobParams, 1),
		failed:  make(chan markFailedCall, 1),
		doc:     contentdb.Document{ID: dID, WorkspaceID: wID, FolderID: fID, Name: "laporan.docx"},
		version: contentdb.DocumentVersion{
			ID:           vID,
			DocumentID:   dID,
			VersionNo:    1,
			StorageKey:   "raw",
			RenditionKey: &key,
			PageCount:    &pageCount,
		},
		access: contentdb.ResolveFolderAccessRow{CanView: true, CanDownload: true, CanWatermark: true},
	}
}

func guestActor() Actor {
	return Actor{
		UserID:     "55555555-5555-5555-5555-555555555555",
		Role:       permission.RoleGuest,
		Email:      "tamu@contoh.id",
		RoomStatus: permission.RoomActive,
	}
}

func downloadAsGuest(svc *ContentService) (DownloadResult, error) {
	return svc.DownloadDocument(context.Background(), downloadTestWorkspaceID, downloadTestDocumentID,
		"", guestActor(), watermark.Mark{Primary: "tamu@contoh.id"})
}

// recvOrFatal membatasi penantian pada kanal fake supaya tes yang salah
// gagal dengan pesan, bukan menggantung sampai timeout paket.
func recvOrFatal[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// requireSlotFree membuktikan goroutine job sudah melepas slotnya: kirim
// non-blocking hanya berhasil bila kanal tidak penuh. Tanpa ini tes kembali
// saat goroutine masih hidup, dan -race/goleak akan melaporkannya.
func requireSlotFree(t *testing.T, sem chan struct{}) {
	t.Helper()

	require.Eventually(t, func() bool {
		select {
		case sem <- struct{}{}:
			<-sem
			return true
		default:
			return false
		}
	}, 5*time.Second, 5*time.Millisecond, "slot semaphore belum dilepas")
}

func TestDownloadDocumentQueuesJobAboveThreshold(t *testing.T) {
	repo := downloadFixture(int32(asyncDownloadPageThreshold + 1))
	svc := newDownloadTestService(t, repo, &countingRenderer{open: failingOpen}, &countingRenderer{open: failingOpen})

	res, err := downloadAsGuest(svc)

	require.NoError(t, err)
	assert.NotEmpty(t, res.JobID)
	assert.Nil(t, res.Body)

	created := recvOrFatal(t, repo.created, "job row")
	assert.Equal(t, int32(asyncDownloadPageThreshold+1), created.PageCount)
	assert.Equal(t, "laporan.docx", created.DocumentName)

	recvOrFatal(t, repo.failed, "job failure")
	requireSlotFree(t, svc.stampAsyncSem)
}

func TestDownloadDocumentRefusedInArchivedRoom(t *testing.T) {
	repo := downloadFixture(int32(asyncDownloadPageThreshold + 1))
	svc := newDownloadTestService(t, repo, &countingRenderer{open: failingOpen}, &countingRenderer{open: failingOpen})

	actor := guestActor()
	actor.RoomStatus = permission.RoomArchive

	_, err := svc.DownloadDocument(context.Background(), downloadTestWorkspaceID, downloadTestDocumentID,
		"", actor, watermark.Mark{})

	require.ErrorIs(t, err, ErrContentForbidden)
	assert.Empty(t, repo.created)
}

func TestDownloadDocumentQueuesAboveOldPageCap(t *testing.T) {
	repo := downloadFixture(200)
	svc := newDownloadTestService(t, repo, &countingRenderer{open: failingOpen}, &countingRenderer{open: failingOpen})

	res, err := downloadAsGuest(svc)

	require.NoError(t, err)
	require.NotErrorIs(t, err, ErrWatermarkDownloadTooLarge)
	assert.NotEmpty(t, res.JobID)

	created := recvOrFatal(t, repo.created, "job row")
	assert.Equal(t, int32(200), created.PageCount)

	recvOrFatal(t, repo.failed, "job failure")
	requireSlotFree(t, svc.stampAsyncSem)
}

func TestDownloadDocumentAsyncJobUsesDownloadJobRenderer(t *testing.T) {
	repo := downloadFixture(int32(asyncDownloadPageThreshold + 1))
	request := &countingRenderer{open: failingOpen}
	downloadJob := &countingRenderer{open: failingOpen}
	svc := newDownloadTestService(t, repo, request, downloadJob)

	res, err := downloadAsGuest(svc)

	require.NoError(t, err)
	require.NotEmpty(t, res.JobID)

	recvOrFatal(t, repo.created, "job row")
	call := recvOrFatal(t, repo.failed, "job failure")
	requireSlotFree(t, svc.stampAsyncSem)

	assert.NoError(t, call.ctxErr)
	assert.Equal(t, int32(1), downloadJob.calls.Load(), "job latar harus merender di kolam unduhan")
	assert.Equal(t, int32(0), request.calls.Load(), "kolam request tidak boleh tersentuh job latar")
}

func TestDownloadDocumentSyncUsesRequestRenderer(t *testing.T) {
	repo := downloadFixture(1)
	request := &countingRenderer{open: failingOpen}
	downloadJob := &countingRenderer{open: failingOpen}
	svc := newDownloadTestService(t, repo, request, downloadJob)

	_, err := downloadAsGuest(svc)

	require.ErrorIs(t, err, ErrStampFailed)
	requireSlotFree(t, svc.stampSem)

	assert.Equal(t, int32(1), request.calls.Load(), "jalur sinkron harus merender di kolam request")
	assert.Equal(t, int32(0), downloadJob.calls.Load(), "kolam unduhan tidak boleh tersentuh jalur sinkron")
	assert.Empty(t, repo.created)
}

// Regresi cacat "satu konteks untuk raster dan store": bila tenggat raster
// habis, penandaan gagal harus tetap berjalan di konteks hidup. Dengan konteks
// bersama, repo melihat ctx.Err() == DeadlineExceeded dan baris tinggal
// pending sampai sweeper stale.
func TestRunDownloadJobStoresOnFreshContextAfterRasterDeadline(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	repo := downloadFixture(2)
	downloadJob := &countingRenderer{open: blockingOpen}
	svc := newDownloadTestService(t, repo, &countingRenderer{open: failingOpen}, downloadJob)

	job := contentdb.DocumentDownloadJob{ID: downloadTestJobID(), PageCount: 2}

	svc.runDownloadJob(t.Context(), downloadTestWorkspaceID, job, downloadTestVersionID, downloadTestRenditionKey, 2,
		watermark.Mark{Primary: "tamu@contoh.id"}, 20*time.Millisecond, time.Minute)

	call := recvOrFatal(t, repo.failed, "job failure")
	require.NoError(t, call.ctxErr, "fase store harus berjalan di konteks hidup, bukan konteks raster yang kedaluwarsa")
	assert.Equal(t, job.ID, call.params.ID)
	assert.Contains(t, call.params.Error, context.DeadlineExceeded.Error())
	assert.Equal(t, int32(1), downloadJob.calls.Load())
}

func TestDownloadJobSweeperNeverOutlivesARunningJob(t *testing.T) {
	assert.Greater(t, downloadJobStaleAge, downloadJobTimeout+downloadJobStoreTimeout,
		"sweeper akan menandai failed job yang masih berjalan")
	assert.LessOrEqual(t, maxWatermarkDownloadPages, maxRenditionPages,
		"plafon unduhan tidak boleh melewati plafon rendition")
}
