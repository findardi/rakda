package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type downloadFakeRepo struct {
	ContentRepository

	doc     contentdb.Document
	version contentdb.DocumentVersion
	access  contentdb.ResolveFolderAccessRow
	created chan contentdb.CreateDownloadJobParams
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

	var id pgtype.UUID
	if err := id.Scan("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"); err != nil {
		return contentdb.DocumentDownloadJob{}, err
	}

	return contentdb.DocumentDownloadJob{ID: id, PageCount: arg.PageCount}, nil
}

func (f *downloadFakeRepo) MarkDownloadJobFailed(context.Context, contentdb.MarkDownloadJobFailedParams) error {
	return nil
}

type failingRenderer struct {
	render.Render
}

func (failingRenderer) Open(io.Reader) (render.Document, error) {
	return nil, errors.New("renderer unavailable in test")
}

func newDownloadTestService(t *testing.T, repo *downloadFakeRepo) *ContentService {
	t.Helper()

	store := fakeStorage{
		getFn: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("rendition"))), nil
		},
	}

	return NewContentService(repo, store, Viewer{
		Renderer: failingRenderer{},
		DPI:      150,
	}, 0, nil, 1, ArchiveDeps{})
}

func downloadFixture(pageCount int32) *downloadFakeRepo {
	var wID, fID, dID, vID pgtype.UUID
	_ = wID.Scan("11111111-1111-1111-1111-111111111111")
	_ = fID.Scan("22222222-2222-2222-2222-222222222222")
	_ = dID.Scan("33333333-3333-3333-3333-333333333333")
	_ = vID.Scan("44444444-4444-4444-4444-444444444444")

	key := "renditions/rendition.pdf"

	return &downloadFakeRepo{
		created: make(chan contentdb.CreateDownloadJobParams, 1),
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

func TestDownloadDocumentQueuesJobAboveThreshold(t *testing.T) {
	repo := downloadFixture(int32(asyncDownloadPageThreshold + 1))
	svc := newDownloadTestService(t, repo)

	res, err := svc.DownloadDocument(context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"33333333-3333-3333-3333-333333333333",
		"", guestActor(), watermark.Mark{Primary: "tamu@contoh.id"})

	require.NoError(t, err)
	assert.NotEmpty(t, res.JobID)
	assert.Nil(t, res.Body)

	created := <-repo.created
	assert.Equal(t, int32(asyncDownloadPageThreshold+1), created.PageCount)
	assert.Equal(t, "laporan.docx", created.DocumentName)
}

func TestDownloadDocumentRefusedInArchivedRoom(t *testing.T) {
	repo := downloadFixture(int32(asyncDownloadPageThreshold + 1))
	svc := newDownloadTestService(t, repo)

	actor := guestActor()
	actor.RoomStatus = permission.RoomArchive

	_, err := svc.DownloadDocument(context.Background(),
		"11111111-1111-1111-1111-111111111111",
		"33333333-3333-3333-3333-333333333333",
		"", actor, watermark.Mark{})

	require.ErrorIs(t, err, ErrContentForbidden)
	assert.Len(t, repo.created, 0)
}
