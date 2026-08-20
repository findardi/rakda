package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/render"
	"github.com/findardi/Riksa-App/server/internal/platform/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitPageTexts(t *testing.T) {
	t.Run("splits on form feed and drops trailing empty chunk", func(t *testing.T) {
		text := "page one\fpage two\fpage three\f"
		pages := splitPageTexts(text, 3)

		require.Len(t, pages, 3)
		assert.Equal(t, "page one", pages[0])
		assert.Equal(t, "page two", pages[1])
		assert.Equal(t, "page three", pages[2])
	})

	t.Run("pads with empty pages when output has fewer chunks", func(t *testing.T) {
		pages := splitPageTexts("only one\f", 3)

		require.Len(t, pages, 3)
		assert.Equal(t, "only one", pages[0])
		assert.Equal(t, "", pages[1])
		assert.Equal(t, "", pages[2])
	})

	t.Run("keeps empty middle pages", func(t *testing.T) {
		pages := splitPageTexts("text\f\fscanned\f", 3)

		require.Len(t, pages, 3)
		assert.Equal(t, "text", pages[0])
		assert.Equal(t, "", pages[1])
		assert.Equal(t, "scanned", pages[2])
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		pages := splitPageTexts("  leading and trailing  \f", 1)

		require.Len(t, pages, 1)
		assert.Equal(t, "leading and trailing", pages[0])
	})
}

func TestTruncatePageContent(t *testing.T) {
	t.Run("keeps content within the limit untouched", func(t *testing.T) {
		in := strings.Repeat("a", 1000)
		assert.Equal(t, in, truncatePageContent(in))
	})

	t.Run("cuts oversized content to the limit", func(t *testing.T) {
		in := strings.Repeat("a", maxPageTextBytes+1000)
		out := truncatePageContent(in)

		assert.LessOrEqual(t, len(out), maxPageTextBytes)
		assert.True(t, utf8.ValidString(out))
	})

	t.Run("never splits a multibyte rune", func(t *testing.T) {
		in := strings.Repeat("a", maxPageTextBytes-1) + "界界界"
		out := truncatePageContent(in)

		assert.LessOrEqual(t, len(out), maxPageTextBytes)
		assert.True(t, utf8.ValidString(out))
	})
}

type fakeTextExtractor struct {
	extractFn func(ctx context.Context, pdf io.Reader) (string, error)
}

func (f fakeTextExtractor) ExtractText(ctx context.Context, pdf io.Reader) (string, error) {
	return f.extractFn(ctx, pdf)
}

type fakeRenderer struct {
	pageCountFn func(ctx context.Context, pdf io.Reader) (int, error)
}

func (f fakeRenderer) PageCount(ctx context.Context, pdf io.Reader) (int, error) {
	return f.pageCountFn(ctx, pdf)
}

func (f fakeRenderer) RenderPage(ctx context.Context, pdf io.Reader, page int) ([]byte, error) {
	return nil, nil
}

type execCall struct {
	sql  string
	args []any
}

// recordingDB implements contentdb's (unexported) DBTX interface structurally
// so generated queries run against recorded calls instead of a live pool.
type recordingDB struct {
	execs []execCall
}

func (d *recordingDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	d.execs = append(d.execs, execCall{sql: sql, args: args})
	return pgconn.CommandTag{}, nil
}

func (d *recordingDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, errors.New("query not expected")
}

func (d *recordingDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (d *recordingDB) countSQL(fragment string) int {
	n := 0
	for _, c := range d.execs {
		if strings.Contains(c.sql, fragment) {
			n++
		}
	}
	return n
}

func (d *recordingDB) execTx(ctx context.Context, fn func(*contentdb.Queries) error) error {
	return fn(contentdb.New(d))
}

type textFakeRepo struct {
	ContentRepository

	listPendingFn  func(ctx context.Context, limit int32) ([]contentdb.ListPendingTextExtractionRow, error)
	getVersionFn   func(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error)
	setFailureFn   func(ctx context.Context, arg contentdb.SetVersionTextFailureParams) error
	setRenditionFn func(ctx context.Context, arg contentdb.SetVersionRenditionParams) error
	execTxFn       func(ctx context.Context, fn func(*contentdb.Queries) error) error
}

func (f *textFakeRepo) ListPendingTextExtraction(ctx context.Context, limit int32) ([]contentdb.ListPendingTextExtractionRow, error) {
	return f.listPendingFn(ctx, limit)
}

func (f *textFakeRepo) GetVersionByID(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error) {
	return f.getVersionFn(ctx, id)
}

func (f *textFakeRepo) SetVersionTextFailure(ctx context.Context, arg contentdb.SetVersionTextFailureParams) error {
	return f.setFailureFn(ctx, arg)
}

func (f *textFakeRepo) SetVersionRendition(ctx context.Context, arg contentdb.SetVersionRenditionParams) error {
	return f.setRenditionFn(ctx, arg)
}

func (f *textFakeRepo) ExecTx(ctx context.Context, fn func(*contentdb.Queries) error) error {
	return f.execTxFn(ctx, fn)
}

type fakeStorage struct {
	storage.Storage
	getFn func(ctx context.Context, key string) (io.ReadCloser, error)
}

func (f fakeStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return f.getFn(ctx, key)
}

func pendingRow(versionID string, pageCount *int32) contentdb.ListPendingTextExtractionRow {
	var id pgtype.UUID
	_ = id.Scan(versionID)

	var wID pgtype.UUID
	_ = wID.Scan("99999999-9999-9999-9999-999999999999")

	renditionKey := "w/renditions/" + versionID + "/rendition.pdf"

	return contentdb.ListPendingTextExtractionRow{
		ID:           id,
		DocumentID:   id,
		VersionNo:    1,
		Mime:         "application/pdf",
		Size:         100,
		StorageKey:   "w/v/" + versionID + ".pdf",
		RenditionKey: &renditionKey,
		PageCount:    pageCount,
		DocumentName: "laporan.pdf",
		WorkspaceID:  wID,
	}
}

func newTextTestService(t *testing.T, repo *textFakeRepo, store fakeStorage, renderer render.Render, extractor render.TextExtractor) *ContentService {
	t.Helper()

	return NewContentService(repo, store, Viewer{
		Renderer:      renderer,
		TextExtractor: extractor,
		DPI:           150,
	}, 0, nil)
}

func TestExtractVersionText(t *testing.T) {
	versionID := "11111111-1111-1111-1111-111111111111"
	pageCount := int32(2)
	db := &recordingDB{}

	repo := &textFakeRepo{
		execTxFn: db.execTx,
	}

	extractor := fakeTextExtractor{
		extractFn: func(ctx context.Context, pdf io.Reader) (string, error) {
			b, _ := io.ReadAll(pdf)
			if string(b) != "rendition bytes" {
				return "", errors.New("unexpected pdf bytes")
			}
			return "halaman satu\fhalaman dua\f", nil
		},
	}

	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			assert.Equal(t, "w/renditions/"+versionID+"/rendition.pdf", key)
			return io.NopCloser(bytes.NewReader([]byte("rendition bytes"))), nil
		},
	}

	renderer := fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) {
		return int(pageCount), nil
	}}

	svc := newTextTestService(t, repo, store, renderer, extractor)
	err := svc.extractVersionText(context.Background(), pendingRow(versionID, &pageCount))
	require.NoError(t, err)

	assert.Equal(t, 1, db.countSQL("delete from document_page_texts"))
	assert.Equal(t, 2, db.countSQL("insert into document_page_texts"))
	assert.Equal(t, 1, db.countSQL("text_extracted_at = now()"))

	var wantID pgtype.UUID
	_ = wantID.Scan(versionID)

	for _, c := range db.execs {
		if strings.Contains(c.sql, "insert into document_page_texts") {
			require.Len(t, c.args, 3)
			assert.Equal(t, wantID, c.args[0])
			assert.Equal(t, int32(1), c.args[1])
			assert.Equal(t, "halaman satu", c.args[2])
			break
		}
	}
}

func TestExtractVersionTextAbovePageCap(t *testing.T) {
	versionID := "22222222-2222-2222-2222-222222222222"
	over := int32(maxRenditionPages + 1)

	fresh := contentdb.DocumentVersion{}
	{
		var id pgtype.UUID
		_ = id.Scan(versionID)
		key := "w/renditions/" + versionID + "/rendition.pdf"
		pc := over
		fresh = contentdb.DocumentVersion{
			ID:           id,
			RenditionKey: &key,
			PageCount:    &pc,
		}
	}

	db := &recordingDB{}

	repo := &textFakeRepo{
		getVersionFn: func(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error) {
			return fresh, nil
		},
		setRenditionFn: func(ctx context.Context, arg contentdb.SetVersionRenditionParams) error {
			return nil
		},
		execTxFn: db.execTx,
	}

	extractor := fakeTextExtractor{
		extractFn: func(ctx context.Context, pdf io.Reader) (string, error) {
			return strings.Repeat("x", 20) + "\f", nil
		},
	}

	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("rendition bytes"))), nil
		},
	}

	renderer := fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) {
		return int(over), nil
	}}

	svc := newTextTestService(t, repo, store, renderer, extractor)
	row := pendingRow(versionID, nil)
	row.RenditionKey = nil
	err := svc.extractVersionText(context.Background(), row)
	require.NoError(t, err)

	assert.Equal(t, int(over), db.countSQL("insert into document_page_texts"))
	assert.Equal(t, 1, db.countSQL("text_extracted_at = now()"))
}

func TestExtractVersionTextExtractorFailure(t *testing.T) {
	versionID := "33333333-3333-3333-3333-333333333333"
	pageCount := int32(1)

	var recorded *contentdb.SetVersionTextFailureParams

	repo := &textFakeRepo{
		setFailureFn: func(ctx context.Context, arg contentdb.SetVersionTextFailureParams) error {
			recorded = &arg
			return nil
		},
	}

	extractor := fakeTextExtractor{
		extractFn: func(ctx context.Context, pdf io.Reader) (string, error) {
			return "", errors.New("incorrect password")
		},
	}

	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("rendition bytes"))), nil
		},
	}

	renderer := fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) {
		return int(pageCount), nil
	}}

	svc := newTextTestService(t, repo, store, renderer, extractor)
	err := svc.extractVersionText(context.Background(), pendingRow(versionID, &pageCount))
	require.ErrorIs(t, err, ErrTextExtractionFailed)

	require.NotNil(t, recorded)
	var wantID pgtype.UUID
	_ = wantID.Scan(versionID)
	assert.Equal(t, wantID, recorded.ID)
	require.NotNil(t, recorded.TextError)
	assert.Contains(t, *recorded.TextError, "incorrect password")
}

func TestSweepTextOnceContinuesPastFailures(t *testing.T) {
	okID := "44444444-4444-4444-4444-444444444444"
	badID := "55555555-5555-5555-5555-555555555555"
	pageCount := int32(1)

	var failed bool

	repo := &textFakeRepo{
		listPendingFn: func(ctx context.Context, limit int32) ([]contentdb.ListPendingTextExtractionRow, error) {
			return []contentdb.ListPendingTextExtractionRow{
				pendingRow(badID, &pageCount),
				pendingRow(okID, &pageCount),
			}, nil
		},
		setFailureFn: func(ctx context.Context, arg contentdb.SetVersionTextFailureParams) error {
			failed = true
			return nil
		},
		execTxFn: (&recordingDB{}).execTx,
	}

	calls := 0
	extractor := fakeTextExtractor{
		extractFn: func(ctx context.Context, pdf io.Reader) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("incorrect password")
			}
			return "teks\f", nil
		},
	}

	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("rendition bytes"))), nil
		},
	}

	renderer := fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) {
		return int(pageCount), nil
	}}

	svc := newTextTestService(t, repo, store, renderer, extractor)
	svc.sweepTextOnce(context.Background(), 10)

	assert.True(t, failed)
}
