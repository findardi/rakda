package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/convert"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	renditionTestWorkspace = "99999999-9999-9999-9999-999999999999"
	renditionTestDocument  = "55555555-5555-5555-5555-555555555555"
	renditionTestVersion   = "66666666-6666-6666-6666-666666666666"
)

func renditionUUID(s string) pgtype.UUID {
	var id pgtype.UUID
	_ = id.Scan(s)
	return id
}

type renditionCalls struct {
	rendition  *contentdb.SetVersionRenditionParams
	renditions int
	failure    *contentdb.SetVersionRenditionFailureParams
	transient  *contentdb.SetVersionRenditionTransientFailureParams
	released   bool
	promoted   bool
	ctxErr     error
}

type renditionFakeRepo struct {
	ContentRepository

	claimFn       func(ctx context.Context, stale pgtype.Interval) (contentdb.DocumentVersion, error)
	getDocumentFn func(ctx context.Context, id pgtype.UUID) (contentdb.Document, error)
	calls         *renditionCalls
}

func (f *renditionFakeRepo) ClaimPendingRendition(ctx context.Context, stale pgtype.Interval) (contentdb.DocumentVersion, error) {
	return f.claimFn(ctx, stale)
}

func (f *renditionFakeRepo) GetDocumentByID(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) {
	return f.getDocumentFn(ctx, id)
}

func (f *renditionFakeRepo) SetVersionRendition(ctx context.Context, arg contentdb.SetVersionRenditionParams) error {
	f.calls.rendition, f.calls.ctxErr = &arg, ctx.Err()
	f.calls.renditions++
	return nil
}

func (f *renditionFakeRepo) SetVersionRenditionFailure(ctx context.Context, arg contentdb.SetVersionRenditionFailureParams) error {
	f.calls.failure, f.calls.ctxErr = &arg, ctx.Err()
	return nil
}

func (f *renditionFakeRepo) SetVersionRenditionTransientFailure(ctx context.Context, arg contentdb.SetVersionRenditionTransientFailureParams) error {
	f.calls.transient, f.calls.ctxErr = &arg, ctx.Err()
	return nil
}

func (f *renditionFakeRepo) ReleaseRenditionClaim(ctx context.Context, id pgtype.UUID) error {
	f.calls.released, f.calls.ctxErr = true, ctx.Err()
	return nil
}

func (f *renditionFakeRepo) PromoteStagedVersion(ctx context.Context, arg contentdb.PromoteStagedVersionParams) (int64, error) {
	f.calls.promoted = true
	return 1, nil
}

type fakeConverter struct {
	toPDFFn func(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error)
}

func (f fakeConverter) ToPDF(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error) {
	return f.toPDFFn(ctx, src, filename)
}

func renditionTestDoc(name string, staged bool) contentdb.Document {
	doc := contentdb.Document{
		ID:          renditionUUID(renditionTestDocument),
		WorkspaceID: renditionUUID(renditionTestWorkspace),
		Name:        name,
	}
	if staged {
		doc.StagedVersionID = renditionUUID(renditionTestVersion)
	}
	return doc
}

func renditionTestVersionRow(attempts int32) contentdb.DocumentVersion {
	return contentdb.DocumentVersion{
		ID:                renditionUUID(renditionTestVersion),
		DocumentID:        renditionUUID(renditionTestDocument),
		VersionNo:         1,
		StorageKey:        "w/f/" + renditionTestVersion + ".bin",
		RenditionAttempts: attempts,
	}
}

func newRenditionRepo(doc contentdb.Document) (*renditionFakeRepo, *renditionCalls) {
	calls := &renditionCalls{}
	return &renditionFakeRepo{
		getDocumentFn: func(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) { return doc, nil },
		calls:         calls,
	}, calls
}

func newRenditionTestService(t *testing.T, repo ContentRepository, store fakeStorage, conv convert.Converter, renderer render.Render) *ContentService {
	t.Helper()

	return NewContentService(repo, store, Viewer{Converter: conv, Renderer: renderer, DPI: 150},
		0, nil, StampDeps{}, ArchiveDeps{}, CacheDeps{}, RenditionDeps{})
}

func sourceStore(t *testing.T, puts *[]string) fakeStorage {
	t.Helper()

	return fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("source bytes"))), nil
		},
		putFn: func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			assert.Equal(t, "application/pdf", contentType)
			assert.Equal(t, int64(len(b)), size)
			*puts = append(*puts, key)
			return nil
		},
	}
}

func pdfBytesConverter(t *testing.T) fakeConverter {
	t.Helper()

	return fakeConverter{toPDFFn: func(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error) {
		assert.Equal(t, "kontrak.docx", filename)
		return io.NopCloser(strings.NewReader("%PDF-fake")), nil
	}}
}

func failingConverter(err error) fakeConverter {
	return fakeConverter{toPDFFn: func(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error) {
		return nil, err
	}}
}

func blockingConverter() fakeConverter {
	return fakeConverter{toPDFFn: func(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error) {
		<-ctx.Done()
		return nil, fmt.Errorf("gotenberg call: %w", ctx.Err())
	}}
}

func pagesRenderer(n int, err error) fakeRenderer {
	return fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) { return n, err }}
}

func TestRunRenditionJobConvertsStoresAndPromotes(t *testing.T) {
	repo, calls := newRenditionRepo(renditionTestDoc("kontrak.docx", true))
	var puts []string
	svc := newRenditionTestService(t, repo, sourceStore(t, &puts), pdfBytesConverter(t), pagesRenderer(3, nil))

	svc.runRenditionJob(context.Background(), renditionTestVersionRow(0), time.Second, time.Second)

	wantKey := renditionTestWorkspace + "/renditions/" + renditionTestVersion + "/rendition.pdf"
	assert.Equal(t, []string{wantKey}, puts)
	require.NotNil(t, calls.rendition)
	assert.Equal(t, wantKey, *calls.rendition.RenditionKey)
	assert.Equal(t, int32(3), *calls.rendition.PageCount)
	assert.True(t, calls.promoted)
	assert.Nil(t, calls.failure)
	assert.Nil(t, calls.transient)
	assert.False(t, calls.released)
}

func TestRunRenditionJobPDFCountsWithoutConverting(t *testing.T) {
	repo, calls := newRenditionRepo(renditionTestDoc("laporan.pdf", false))
	var puts []string
	conv := failingConverter(errors.New("converter must not run for a pdf upload"))
	svc := newRenditionTestService(t, repo, sourceStore(t, &puts), conv, pagesRenderer(2, nil))

	version := renditionTestVersionRow(0)
	svc.runRenditionJob(context.Background(), version, time.Second, time.Second)

	assert.Empty(t, puts)
	require.NotNil(t, calls.rendition)
	assert.Equal(t, version.StorageKey, *calls.rendition.RenditionKey)
	assert.Equal(t, int32(2), *calls.rendition.PageCount)
	assert.False(t, calls.promoted)
}

func TestRunRenditionJobAboveCapIsStoredButNotPromoted(t *testing.T) {
	repo, calls := newRenditionRepo(renditionTestDoc("kontrak.docx", true))
	var puts []string
	svc := newRenditionTestService(t, repo, sourceStore(t, &puts), pdfBytesConverter(t), pagesRenderer(maxRenditionPages+1, nil))

	svc.runRenditionJob(context.Background(), renditionTestVersionRow(0), time.Second, time.Second)

	require.NotNil(t, calls.rendition)
	assert.Equal(t, int32(maxRenditionPages+1), *calls.rendition.PageCount)
	assert.False(t, calls.promoted)
}

func TestRunRenditionJobOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		attempts    int32
		convertErr  error
		pageErr     error
		wantFailure string        // substring of the permanent message; "" = not permanent
		wantBackoff time.Duration // transient: the wait recorded on the row
	}{
		{name: "transport error is transient", convertErr: errors.New("gotenberg call: dial tcp: connection refused"), wantBackoff: 30 * time.Second},
		{name: "503 is transient", convertErr: &convert.StatusError{Code: 503, Body: "busy"}, wantBackoff: 30 * time.Second},
		{name: "third transient failure waits longer", attempts: 2, convertErr: errors.New("gotenberg call: eof"), wantBackoff: 10 * time.Minute},
		{name: "400 is permanent", convertErr: &convert.StatusError{Code: 400, Body: "bad file"}, wantFailure: "status 400"},
		{name: "unsupported type is permanent", convertErr: convert.ErrUnsupportedFile, wantFailure: "unsupported file type"},
		{name: "unreadable output is permanent", pageErr: fmt.Errorf("%w: pdfinfo: garbage", render.ErrRenderFailed), wantFailure: "render failed"},
		{name: "fifth transient failure gives up", attempts: 4, convertErr: errors.New("gotenberg call: eof"), wantFailure: "gave up after 5 attempts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, calls := newRenditionRepo(renditionTestDoc("kontrak.docx", false))
			var puts []string
			conv := pdfBytesConverter(t)
			if tt.convertErr != nil {
				conv = failingConverter(tt.convertErr)
			}
			svc := newRenditionTestService(t, repo, sourceStore(t, &puts), conv, pagesRenderer(1, tt.pageErr))

			svc.runRenditionJob(context.Background(), renditionTestVersionRow(tt.attempts), time.Second, time.Second)

			assert.Nil(t, calls.rendition)
			assert.False(t, calls.released)
			assert.Empty(t, puts, "a failed conversion must not store an object")

			if tt.wantFailure != "" {
				require.NotNil(t, calls.failure, "expected a permanent failure")
				assert.Contains(t, *calls.failure.RenditionError, tt.wantFailure)
				assert.Nil(t, calls.transient)
				return
			}

			require.NotNil(t, calls.transient, "expected a transient failure")
			assert.Equal(t, pgInterval(tt.wantBackoff), calls.transient.Backoff)
			assert.NotEmpty(t, *calls.transient.RenditionError)
			assert.Nil(t, calls.failure)
		})
	}
}

func TestRunRenditionJobShutdownReleasesWithoutCounting(t *testing.T) {
	repo, calls := newRenditionRepo(renditionTestDoc("kontrak.docx", false))
	var puts []string

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conv := fakeConverter{toPDFFn: func(ctx context.Context, src io.Reader, filename string) (io.ReadCloser, error) {
		cancel()
		<-ctx.Done()
		return nil, fmt.Errorf("gotenberg call: %w", ctx.Err())
	}}
	svc := newRenditionTestService(t, repo, sourceStore(t, &puts), conv, pagesRenderer(1, nil))

	svc.runRenditionJob(ctx, renditionTestVersionRow(3), time.Second, time.Second)

	assert.True(t, calls.released)
	assert.NoError(t, calls.ctxErr, "release must run on a live bookkeeping context")
	assert.Nil(t, calls.transient)
	assert.Nil(t, calls.failure)
}

func TestRunRenditionJobTimeoutIsTransient(t *testing.T) {
	repo, calls := newRenditionRepo(renditionTestDoc("kontrak.docx", false))
	var puts []string
	svc := newRenditionTestService(t, repo, sourceStore(t, &puts), blockingConverter(), pagesRenderer(1, nil))

	svc.runRenditionJob(context.Background(), renditionTestVersionRow(0), 10*time.Millisecond, time.Second)

	require.NotNil(t, calls.transient)
	assert.NoError(t, calls.ctxErr, "the outcome must be written on the bookkeeping context")
	assert.Contains(t, *calls.transient.RenditionError, "deadline exceeded")
	assert.False(t, calls.released)
}

func TestRunRenditionJobReleasesWhenDocumentIsGone(t *testing.T) {
	repo, calls := newRenditionRepo(contentdb.Document{})
	repo.getDocumentFn = func(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) {
		return contentdb.Document{}, pgx.ErrNoRows
	}
	conv := failingConverter(errors.New("converter must not run without a document"))
	svc := newRenditionTestService(t, repo, fakeStorage{}, conv, fakeRenderer{})

	svc.runRenditionJob(context.Background(), renditionTestVersionRow(0), time.Second, time.Second)

	assert.True(t, calls.released)
	assert.Nil(t, calls.transient)
	assert.Nil(t, calls.failure)
}

func TestDrainRenditionsStopsWhenQueueIsEmpty(t *testing.T) {
	repo, calls := newRenditionRepo(renditionTestDoc("laporan.pdf", false))
	claims := 0
	repo.claimFn = func(ctx context.Context, stale pgtype.Interval) (contentdb.DocumentVersion, error) {
		assert.Equal(t, pgInterval(renditionClaimStaleAge), stale)
		claims++
		if claims > 2 {
			return contentdb.DocumentVersion{}, pgx.ErrNoRows
		}
		return renditionTestVersionRow(0), nil
	}
	var puts []string
	svc := newRenditionTestService(t, repo, sourceStore(t, &puts), fakeConverter{}, pagesRenderer(1, nil))

	svc.drainRenditions(context.Background())

	assert.Equal(t, 3, claims)
	assert.Equal(t, 2, calls.renditions)
}

func TestDrainRenditionsSkipsClaimOnDeadContext(t *testing.T) {
	repo, _ := newRenditionRepo(renditionTestDoc("laporan.pdf", false))
	repo.claimFn = func(ctx context.Context, stale pgtype.Interval) (contentdb.DocumentVersion, error) {
		t.Error("must not claim on a dead context")
		return contentdb.DocumentVersion{}, pgx.ErrNoRows
	}
	svc := newRenditionTestService(t, repo, fakeStorage{}, fakeConverter{}, fakeRenderer{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.drainRenditions(ctx)
}

func TestWakeRenditionWorkerNeverBlocks(t *testing.T) {
	svc := newRenditionTestService(t, &renditionFakeRepo{}, fakeStorage{}, fakeConverter{}, fakeRenderer{})

	for range 3 {
		svc.wakeRenditionWorker()
	}

	assert.Len(t, svc.rendition.Wake, 1)
}

func TestRunRenditionWorkerDrainsOnWake(t *testing.T) {
	repo, _ := newRenditionRepo(renditionTestDoc("laporan.pdf", false))
	claimed := make(chan struct{}, 8)
	repo.claimFn = func(ctx context.Context, stale pgtype.Interval) (contentdb.DocumentVersion, error) {
		claimed <- struct{}{}
		return contentdb.DocumentVersion{}, pgx.ErrNoRows
	}
	svc := newRenditionTestService(t, repo, fakeStorage{}, fakeConverter{}, fakeRenderer{})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.RunRenditionWorker(ctx, time.Hour)
	}()

	<-claimed // the drain on start
	svc.wakeRenditionWorker()

	select {
	case <-claimed:
	case <-time.After(2 * time.Second):
		t.Fatal("wake did not trigger a drain")
	}

	cancel()
	<-done
}

func TestClassifyRenditionError(t *testing.T) {
	dead, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want renditionOutcome
	}{
		{"dead worker context", dead, errors.New("anything"), renditionShutdown},
		{"job deadline", context.Background(), fmt.Errorf("convert: %w", context.DeadlineExceeded), renditionTransient},
		{"storage error", context.Background(), errors.New("get original: connection reset"), renditionTransient},
		{"gotenberg 5xx", context.Background(), fmt.Errorf("convert: %w", &convert.StatusError{Code: 504}), renditionTransient},
		{"gotenberg 4xx", context.Background(), fmt.Errorf("convert: %w", &convert.StatusError{Code: 422}), renditionPermanent},
		{"unsupported", context.Background(), fmt.Errorf("convert: %w", convert.ErrUnsupportedFile), renditionPermanent},
		{"poppler cannot read", context.Background(), fmt.Errorf("page count: %w", render.ErrRenderFailed), renditionPermanent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyRenditionError(tt.ctx, tt.err))
		})
	}
}
