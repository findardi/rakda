package service

import (
	"context"
	"testing"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingActivity struct {
	entries []activityservice.Entry
}

func (r *recordingActivity) Record(ctx context.Context, e activityservice.Entry) {
	r.entries = append(r.entries, e)
}

func (r *recordingActivity) RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error {
	r.entries = append(r.entries, e)
	return nil
}

func (r *recordingActivity) RecordPageEvent(ctx context.Context, ev activityservice.PageEvent) {}

type viewFakeRepo struct {
	ContentRepository

	doc       contentdb.Document
	version   contentdb.DocumentVersion
	requested []pgtype.UUID
	cleared   int64
	clearedID pgtype.UUID
}

func (f *viewFakeRepo) GetDocumentByID(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) {
	return f.doc, nil
}

func (f *viewFakeRepo) GetCurrentVersion(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error) {
	return f.version, nil
}

func (f *viewFakeRepo) GetVersionByID(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error) {
	return f.version, nil
}

func (f *viewFakeRepo) RequestRendition(ctx context.Context, id pgtype.UUID) error {
	f.requested = append(f.requested, id)
	return nil
}

func (f *viewFakeRepo) ClearVersionRenditionFailure(ctx context.Context, id pgtype.UUID) (int64, error) {
	f.clearedID = id
	return f.cleared, nil
}

func (f *viewFakeRepo) PromoteStagedVersion(ctx context.Context, arg contentdb.PromoteStagedVersionParams) (int64, error) {
	return 1, nil
}

func managerActor() Actor {
	return Actor{UserID: "u1", Role: permission.RoleOwner, Name: "Owner", Email: "owner@example.test"}
}

func viewTestVersion(key *string, pages *int32, failed bool) contentdb.DocumentVersion {
	v := contentdb.DocumentVersion{
		ID:           renditionUUID(renditionTestVersion),
		DocumentID:   renditionUUID(renditionTestDocument),
		VersionNo:    2,
		Mime:         "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		RenditionKey: key,
		PageCount:    pages,
	}
	if failed {
		v.RenditionFailedAt = pgtype.Timestamptz{Valid: true}
	}
	return v
}

func newViewTestService(repo ContentRepository, activity ActivityRecorder) *ContentService {
	return NewContentService(repo, fakeStorage{}, Viewer{}, 0, activity, StampDeps{}, ArchiveDeps{}, CacheDeps{}, RenditionDeps{})
}

func TestGetViewMetaRenditionStates(t *testing.T) {
	key := "w/renditions/v/rendition.pdf"
	pages := int32(4)

	tests := []struct {
		name       string
		version    contentdb.DocumentVersion
		wantStatus string
		wantPages  int
		wantWake   bool
		wantViewed bool
	}{
		{"pending wakes the worker and records nothing", viewTestVersion(nil, nil, false), dto.RenditionPending, 0, true, false},
		{"failed records nothing", viewTestVersion(nil, nil, true), dto.RenditionFailed, 0, false, false},
		{"ready records the view", viewTestVersion(&key, &pages, false), dto.RenditionReady, 4, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := renditionTestDoc("kontrak.docx", false)
			doc.CurrentVersionID = tt.version.ID
			repo := &viewFakeRepo{doc: doc, version: tt.version}
			activity := &recordingActivity{}
			svc := newViewTestService(repo, activity)

			res, err := svc.GetViewMeta(context.Background(), renditionTestWorkspace, renditionTestDocument, "", managerActor())
			require.NoError(t, err)

			assert.Equal(t, tt.wantStatus, res.RenditionStatus)
			assert.Equal(t, tt.wantPages, res.PageCount)
			assert.Equal(t, tt.wantWake, len(svc.rendition.Wake) == 1, "wake token")
			assert.Equal(t, tt.wantViewed, len(activity.entries) == 1, "document_viewed")
			assert.Empty(t, repo.requested, "a current version is claimable on its own")
		})
	}
}

func TestGetViewMetaRequestsRenditionForOldVersion(t *testing.T) {
	version := viewTestVersion(nil, nil, false)
	doc := renditionTestDoc("kontrak.docx", false)
	doc.CurrentVersionID = renditionUUID("77777777-7777-7777-7777-777777777777")
	repo := &viewFakeRepo{doc: doc, version: version}
	svc := newViewTestService(repo, &recordingActivity{})

	res, err := svc.GetViewMeta(context.Background(), renditionTestWorkspace, renditionTestDocument, renditionTestVersion, managerActor())
	require.NoError(t, err)

	assert.Equal(t, dto.RenditionPending, res.RenditionStatus)
	assert.Equal(t, []pgtype.UUID{version.ID}, repo.requested)
	assert.Len(t, svc.rendition.Wake, 1)
}

func TestGetViewMetaAboveCapIsStillAnError(t *testing.T) {
	key := "w/renditions/v/rendition.pdf"
	over := int32(maxRenditionPages + 1)
	version := viewTestVersion(&key, &over, false)
	doc := renditionTestDoc("kontrak.docx", false)
	doc.CurrentVersionID = version.ID
	svc := newViewTestService(&viewFakeRepo{doc: doc, version: version}, &recordingActivity{})

	_, err := svc.GetViewMeta(context.Background(), renditionTestWorkspace, renditionTestDocument, "", managerActor())
	require.ErrorIs(t, err, ErrTooManyPages)
}

func TestRetryRenditionWakesOnlyWhenSomethingWasCleared(t *testing.T) {
	tests := []struct {
		name      string
		cleared   int64
		wantWake  bool
		wantAudit int
	}{
		{"failed version restarts", 1, true, 1},
		{"not failed is a no-op", 0, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version := viewTestVersion(nil, nil, tt.cleared == 1)
			doc := renditionTestDoc("kontrak.docx", false)
			doc.CurrentVersionID = version.ID
			repo := &viewFakeRepo{doc: doc, version: version, cleared: tt.cleared}
			activity := &recordingActivity{}
			svc := newViewTestService(repo, activity)

			err := svc.RetryRendition(context.Background(), renditionTestWorkspace, renditionTestDocument, renditionTestVersion, managerActor())
			require.NoError(t, err)

			assert.Equal(t, version.ID, repo.clearedID)
			assert.Equal(t, tt.wantWake, len(svc.rendition.Wake) == 1, "wake token")
			assert.Len(t, activity.entries, tt.wantAudit)
		})
	}
}
