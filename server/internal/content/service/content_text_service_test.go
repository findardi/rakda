package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/storage"
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

func (f fakeRenderer) Open(pdf io.Reader) (render.Document, error) {
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

	listPendingFn         func(ctx context.Context, limit int32) ([]contentdb.ListPendingTextExtractionRow, error)
	listPendingOCRFn      func(ctx context.Context, limit int32) ([]contentdb.ListPendingOCRPagesRow, error)
	listPendingBBoxFn     func(ctx context.Context, limit int32) ([]contentdb.ListPendingWordBoxesRow, error)
	getVersionFn          func(ctx context.Context, id pgtype.UUID) (contentdb.DocumentVersion, error)
	getDocumentFn         func(ctx context.Context, id pgtype.UUID) (contentdb.Document, error)
	setFailureFn          func(ctx context.Context, arg contentdb.SetVersionTextFailureParams) error
	setRenditionFn        func(ctx context.Context, arg contentdb.SetVersionRenditionParams) error
	setPageOCRResultFn    func(ctx context.Context, arg contentdb.SetPageOCRResultParams) error
	setPageOCRFailureFn   func(ctx context.Context, arg contentdb.SetPageOCRFailureParams) error
	setPageWordBoxesFn    func(ctx context.Context, arg contentdb.SetPageWordBoxesParams) error
	resolveFolderAccessFn func(ctx context.Context, arg contentdb.ResolveFolderAccessParams) (contentdb.ResolveFolderAccessRow, error)
	searchPendingBoxFn    func(ctx context.Context, arg contentdb.SearchPendingBoxPagesParams) ([]int32, error)
	searchWordBoxesFn     func(ctx context.Context, arg contentdb.SearchWordBoxesParams) ([]contentdb.SearchWordBoxesRow, error)
	execTxFn              func(ctx context.Context, fn func(*contentdb.Queries) error) error
}

func (f *textFakeRepo) ListPendingTextExtraction(ctx context.Context, limit int32) ([]contentdb.ListPendingTextExtractionRow, error) {
	return f.listPendingFn(ctx, limit)
}

func (f *textFakeRepo) ListPendingOCRPages(ctx context.Context, limit int32) ([]contentdb.ListPendingOCRPagesRow, error) {
	return f.listPendingOCRFn(ctx, limit)
}

func (f *textFakeRepo) SetPageOCRResult(ctx context.Context, arg contentdb.SetPageOCRResultParams) error {
	return f.setPageOCRResultFn(ctx, arg)
}

func (f *textFakeRepo) SetPageOCRFailure(ctx context.Context, arg contentdb.SetPageOCRFailureParams) error {
	return f.setPageOCRFailureFn(ctx, arg)
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
	putFn func(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
}

func (f fakeStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return f.getFn(ctx, key)
}

func (f fakeStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	return f.putFn(ctx, key, r, size, contentType)
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
	}, 0, nil, StampDeps{Sync: 2, Async: 2}, ArchiveDeps{}, CacheDeps{}, RenditionDeps{})
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
	db := &recordingDB{}

	repo := &textFakeRepo{execTxFn: db.execTx}

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

	svc := newTextTestService(t, repo, store, fakeRenderer{}, extractor)
	err := svc.extractVersionText(context.Background(), pendingRow(versionID, &over))
	require.NoError(t, err)

	assert.Equal(t, int(over), db.countSQL("insert into document_page_texts"))
	assert.Equal(t, 1, db.countSQL("text_extracted_at = now()"))
}

func TestExtractVersionTextWithoutRendition(t *testing.T) {
	versionID := "44444444-4444-4444-4444-444444444444"
	pageCount := int32(1)

	extracted := false
	extractor := fakeTextExtractor{
		extractFn: func(ctx context.Context, pdf io.Reader) (string, error) {
			extracted = true
			return "", nil
		},
	}

	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			t.Errorf("store.Get(%q) must not be called without a rendition", key)
			return nil, errors.New("unexpected get")
		},
	}

	svc := newTextTestService(t, &textFakeRepo{}, store, fakeRenderer{}, extractor)

	row := pendingRow(versionID, &pageCount)
	row.RenditionKey = nil

	err := svc.extractVersionText(context.Background(), row)
	require.ErrorIs(t, err, ErrRenditionPending)
	assert.False(t, extracted)
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

type fakeOCR struct {
	ocrFn func(ctx context.Context, pdf io.Reader, page int) (render.OCRResult, error)
}

func (f fakeOCR) OpenOCR(pdf io.Reader) (render.OCRDocument, error) {
	return fakeOCRDocument{fn: f.ocrFn, pdf: pdf}, nil
}

type fakeOCRDocument struct {
	fn  func(ctx context.Context, pdf io.Reader, page int) (render.OCRResult, error)
	pdf io.Reader
}

func (d fakeOCRDocument) OCRPage(ctx context.Context, page int) (render.OCRResult, error) {
	return d.fn(ctx, d.pdf, page)
}

func (d fakeOCRDocument) Close() error {
	return nil
}

func TestOCRSweeperOpensRenditionOncePerVersion(t *testing.T) {
	versionID := "88888888-8888-8888-8888-888888888888"

	repo := &textFakeRepo{
		listPendingOCRFn: func(ctx context.Context, limit int32) ([]contentdb.ListPendingOCRPagesRow, error) {
			return []contentdb.ListPendingOCRPagesRow{
				ocrPendingRow(versionID, 1),
				ocrPendingRow(versionID, 2),
				ocrPendingRow(versionID, 3),
			}, nil
		},
		setPageOCRResultFn:  func(ctx context.Context, arg contentdb.SetPageOCRResultParams) error { return nil },
		setPageOCRFailureFn: func(ctx context.Context, arg contentdb.SetPageOCRFailureParams) error { return nil },
	}

	gets := 0
	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			gets++
			return io.NopCloser(bytes.NewReader([]byte("rendition bytes"))), nil
		},
	}

	pages := 0
	ocr := fakeOCR{
		ocrFn: func(ctx context.Context, pdf io.Reader, page int) (render.OCRResult, error) {
			pages++
			return render.OCRResult{Text: "halaman"}, nil
		},
	}

	svc := NewContentService(repo, store, Viewer{
		Renderer:      fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) { return 10, nil }},
		TextExtractor: fakeTextExtractor{extractFn: func(ctx context.Context, pdf io.Reader) (string, error) { return "", nil }},
		OCR:           ocr,
		DPI:           150,
	}, 0, nil, StampDeps{Sync: 2, Async: 2}, ArchiveDeps{}, CacheDeps{}, RenditionDeps{})

	svc.sweepOCROnce(context.Background(), 10)

	assert.Equal(t, 1, gets)
	assert.Equal(t, 3, pages)
}

func ocrPendingRow(versionID string, pageNo int32) contentdb.ListPendingOCRPagesRow {
	var id pgtype.UUID
	_ = id.Scan(versionID)
	var wID pgtype.UUID
	_ = wID.Scan("99999999-9999-9999-9999-999999999999")
	key := "w/renditions/" + versionID + "/rendition.pdf"

	return contentdb.ListPendingOCRPagesRow{
		WorkspaceID:  wID,
		VersionID:    id,
		PageNo:       pageNo,
		RenditionKey: &key,
	}
}

func TestOCRSweeperWritesResultAndFailure(t *testing.T) {
	okID := "66666666-6666-6666-6666-666666666666"
	badID := "77777777-7777-7777-7777-777777777777"

	var (
		written  []contentdb.SetPageOCRResultParams
		failures []contentdb.SetPageOCRFailureParams
	)

	repo := &textFakeRepo{
		listPendingOCRFn: func(ctx context.Context, limit int32) ([]contentdb.ListPendingOCRPagesRow, error) {
			return []contentdb.ListPendingOCRPagesRow{
				ocrPendingRow(badID, 1),
				ocrPendingRow(okID, 2),
			}, nil
		},
		setPageOCRResultFn: func(ctx context.Context, arg contentdb.SetPageOCRResultParams) error {
			written = append(written, arg)
			return nil
		},
		setPageOCRFailureFn: func(ctx context.Context, arg contentdb.SetPageOCRFailureParams) error {
			failures = append(failures, arg)
			return nil
		},
	}

	calls := 0
	ocr := fakeOCR{
		ocrFn: func(ctx context.Context, pdf io.Reader, page int) (render.OCRResult, error) {
			calls++
			if calls == 1 {
				return render.OCRResult{}, errors.New("empty page image")
			}
			return render.OCRResult{
				Text: "Laporan Keuangan",
				Words: []render.Word{
					{Text: "Laporan", X: 0.1, Y: 0.2, W: 0.3, H: 0.1, Conf: 92.5},
				},
			}, nil
		},
	}

	store := fakeStorage{
		getFn: func(ctx context.Context, key string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("rendition bytes"))), nil
		},
	}

	renderer := fakeRenderer{pageCountFn: func(ctx context.Context, pdf io.Reader) (int, error) {
		return 10, nil
	}}

	svc := NewContentService(repo, store, Viewer{
		Renderer:      renderer,
		TextExtractor: fakeTextExtractor{extractFn: func(ctx context.Context, pdf io.Reader) (string, error) { return "", nil }},
		OCR:           ocr,
		DPI:           150,
	}, 0, nil, StampDeps{Sync: 2, Async: 2}, ArchiveDeps{}, CacheDeps{}, RenditionDeps{})

	svc.sweepOCROnce(context.Background(), 10)

	require.Len(t, failures, 1)
	var wantBad pgtype.UUID
	_ = wantBad.Scan(badID)
	assert.Equal(t, wantBad, failures[0].VersionID)
	assert.Equal(t, int32(1), failures[0].PageNo)
	require.NotNil(t, failures[0].OcrError)
	assert.Contains(t, *failures[0].OcrError, "empty page image")

	require.Len(t, written, 1)
	var wantOK pgtype.UUID
	_ = wantOK.Scan(okID)
	assert.Equal(t, wantOK, written[0].VersionID)
	assert.Equal(t, int32(2), written[0].PageNo)
	assert.Equal(t, "Laporan Keuangan", written[0].Content)
	require.NotNil(t, written[0].Words)
	assert.Contains(t, string(written[0].Words), "Laporan")
}

func (f *textFakeRepo) GetDocumentByID(ctx context.Context, id pgtype.UUID) (contentdb.Document, error) {
	return f.getDocumentFn(ctx, id)
}

func (f *textFakeRepo) ResolveFolderAccess(ctx context.Context, arg contentdb.ResolveFolderAccessParams) (contentdb.ResolveFolderAccessRow, error) {
	return f.resolveFolderAccessFn(ctx, arg)
}

func (f *textFakeRepo) ListPendingWordBoxes(ctx context.Context, limit int32) ([]contentdb.ListPendingWordBoxesRow, error) {
	return f.listPendingBBoxFn(ctx, limit)
}

func (f *textFakeRepo) SetPageWordBoxes(ctx context.Context, arg contentdb.SetPageWordBoxesParams) error {
	return f.setPageWordBoxesFn(ctx, arg)
}

func (f *textFakeRepo) SearchPendingBoxPages(ctx context.Context, arg contentdb.SearchPendingBoxPagesParams) ([]int32, error) {
	return f.searchPendingBoxFn(ctx, arg)
}

func (f *textFakeRepo) SearchWordBoxes(ctx context.Context, arg contentdb.SearchWordBoxesParams) ([]contentdb.SearchWordBoxesRow, error) {
	return f.searchWordBoxesFn(ctx, arg)
}
