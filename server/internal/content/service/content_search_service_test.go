package service

import (
	"context"
	"testing"

	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinBreadcrumb(t *testing.T) {
	t.Run("full path when every ancestor is visible", func(t *testing.T) {
		nodes := []breadcrumbNode{
			{name: "Root", visible: true},
			{name: "Mid", visible: true},
			{name: "target", visible: true},
		}
		assert.Equal(t, "Root / Mid / target", joinBreadcrumb(nodes))
	})

	t.Run("cuts at the first invisible ancestor, no ellipsis", func(t *testing.T) {
		nodes := []breadcrumbNode{
			{name: "Root", visible: true},
			{name: "Hidden", visible: false},
			{name: "target", visible: true},
		}
		assert.Equal(t, "Root", joinBreadcrumb(nodes))
	})

	t.Run("empty when the root itself is invisible", func(t *testing.T) {
		nodes := []breadcrumbNode{
			{name: "Root", visible: false},
			{name: "target", visible: true},
		}
		assert.Equal(t, "", joinBreadcrumb(nodes))
	})

	t.Run("empty for no rows", func(t *testing.T) {
		assert.Equal(t, "", joinBreadcrumb(nil))
	})

	t.Run("single visible node", func(t *testing.T) {
		assert.Equal(t, "pitch deck", joinBreadcrumb([]breadcrumbNode{{name: "pitch deck", visible: true}}))
	})
}

func TestBuildSearchQuery(t *testing.T) {
	t.Run("keeps meaningful tokens space-separated", func(t *testing.T) {
		q, tokens := buildSearchQuery("Laporan Keuangan")
		assert.Equal(t, "laporan keuangan", q)
		assert.Equal(t, []string{"laporan", "keuangan"}, tokens)
	})

	t.Run("drops Indonesian stopwords", func(t *testing.T) {
		q, tokens := buildSearchQuery("yang dan laporan keuangan")
		assert.Equal(t, "laporan keuangan", q)
		assert.Equal(t, []string{"laporan", "keuangan"}, tokens)
	})

	t.Run("drops punctuation and single letters", func(t *testing.T) {
		q, tokens := buildSearchQuery("pitch-deck, final!")
		assert.Equal(t, "pitch deck final", q)
		assert.Equal(t, []string{"pitch", "deck", "final"}, tokens)
	})

	t.Run("empty when everything is a stopword", func(t *testing.T) {
		q, tokens := buildSearchQuery("yang dan di ke")
		assert.Equal(t, "", q)
		assert.Empty(t, tokens)
	})
}

func TestStripHeadlineMarkup(t *testing.T) {
	assert.Equal(t, "laporan tahunan", stripHeadlineMarkup("laporan <b>tahunan</b>"))
	assert.Equal(t, "a b c", stripHeadlineMarkup("<b>a</b> b c"))
	assert.Equal(t, "no markup", stripHeadlineMarkup("no markup"))
}

func TestSearchBoxes(t *testing.T) {
	docID := "88888888-8888-8888-8888-888888888888"
	var dID pgtype.UUID
	_ = dID.Scan(docID)
	var fID pgtype.UUID
	_ = fID.Scan("99999999-9999-9999-9999-999999999999")

	repo := &textFakeRepo{
		getDocumentFn: func(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) {
			var wID pgtype.UUID
			_ = wID.Scan("99999999-9999-9999-9999-999999999999")
			return contentdb.Document{ID: dID, WorkspaceID: wID, FolderID: fID, Name: "laporan.pdf"}, nil
		},
		searchPendingBoxFn: func(ctx context.Context, arg contentdb.SearchPendingBoxPagesParams) ([]int32, error) {
			return []int32{3, 5}, nil
		},
		searchWordBoxesFn: func(ctx context.Context, arg contentdb.SearchWordBoxesParams) ([]contentdb.SearchWordBoxesRow, error) {
			return []contentdb.SearchWordBoxesRow{
				{PageNo: 2, X: 0.1, Y: 0.2, W: 0.3, H: 0.1},
				{PageNo: 2, X: 0.5, Y: 0.2, W: 0.2, H: 0.1},
			}, nil
		},
	}

	svc := NewContentService(repo, fakeStorage{}, Viewer{}, 0, nil, 2)
	res, err := svc.SearchBoxes(context.Background(), "99999999-9999-9999-9999-999999999999", docID, "laporan", Actor{UserID: "u1", Role: "owner"})
	require.NoError(t, err)

	require.Len(t, res.Matches, 1)
	assert.Equal(t, int32(2), res.Matches[0].PageNo)
	require.Len(t, res.Matches[0].Boxes, 2)
	assert.Equal(t, 0.1, res.Matches[0].Boxes[0].X)

	// Halaman 3 & 5 kena secara semantik tapi belum punya koordinat → pending.
	assert.Equal(t, []int32{3, 5}, res.Pending)
}

func TestSearchBoxesStopwordOnly(t *testing.T) {
	svc := NewContentService(&textFakeRepo{}, fakeStorage{}, Viewer{}, 0, nil, 2)
	res, err := svc.SearchBoxes(context.Background(), "99999999-9999-9999-9999-999999999999", "doc", "yang dan", Actor{UserID: "u1", Role: "owner"})
	require.NoError(t, err)
	assert.Empty(t, res.Matches)
	assert.Empty(t, res.Pending)
}

func TestSearchBoxesForbidden(t *testing.T) {
	docID := "88888888-8888-8888-8888-888888888888"
	var dID pgtype.UUID
	_ = dID.Scan(docID)
	var fID pgtype.UUID
	_ = fID.Scan("99999999-9999-9999-9999-999999999999")

	repo := &textFakeRepo{
		getDocumentFn: func(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) {
			var wID pgtype.UUID
			_ = wID.Scan("99999999-9999-9999-9999-999999999999")
			return contentdb.Document{ID: dID, WorkspaceID: wID, FolderID: fID, Name: "laporan.pdf"}, nil
		},
		resolveFolderAccessFn: func(ctx context.Context, arg contentdb.ResolveFolderAccessParams) (contentdb.ResolveFolderAccessRow, error) {
			return contentdb.ResolveFolderAccessRow{CanView: false}, nil
		},
	}

	svc := NewContentService(repo, fakeStorage{}, Viewer{}, 0, nil, 2)
	_, err := svc.SearchBoxes(context.Background(), "99999999-9999-9999-9999-999999999999", docID, "laporan", Actor{UserID: "u1", Role: "guest"})
	require.ErrorIs(t, err, ErrContentForbidden)
}
