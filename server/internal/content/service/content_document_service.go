package service

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/spool"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"golang.org/x/sync/errgroup"
)

func (s *ContentService) RequestUploadURL(ctx context.Context, workspaceID, folderID, reuseKey string) (dto.UploadURLResponse, error) {
	var fID pgtype.UUID
	if err := fID.Scan(folderID); err != nil {
		return dto.UploadURLResponse{}, fmt.Errorf("folder id parse: %w", err)
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.UploadURLResponse{}, ErrFolderNotFound
	}

	if err != nil {
		return dto.UploadURLResponse{}, fmt.Errorf("get folder: %w", err)
	}

	if uuidString(folder.WorkspaceID) != workspaceID {
		return dto.UploadURLResponse{}, ErrFolderNotFound
	}

	key := reuseKey
	if key == "" {
		key = storageKey(workspaceID, folderID)
	} else if err := validateStorageKey(key, workspaceID, folderID); err != nil {
		return dto.UploadURLResponse{}, err
	}
	url, err := s.store.PresignedPut(ctx, key, uploadURLTTL)
	if err != nil {
		return dto.UploadURLResponse{}, fmt.Errorf("presign put: %w", err)
	}

	return dto.UploadURLResponse{
		UploadURL:  url,
		StorageKey: key,
	}, nil
}

func (s *ContentService) CompletedUpload(ctx context.Context, req dto.CompleteUploadRequest, actor Actor) (dto.DocumentResponse, error) {
	var wID, fID, uID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("workspace id parse: %w", err)
	}

	if err := fID.Scan(req.FolderID); err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("folder id parse: %w", err)
	}

	if err := uID.Scan(req.UploadedBy); err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("user id parse: %w", err)
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.DocumentResponse{}, ErrFolderNotFound
	}

	if err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("get folder: %w", err)
	}

	if folder.WorkspaceID != wID {
		return dto.DocumentResponse{}, ErrFolderNotFound
	}

	if err := validateStorageKey(req.StorageKey, req.WorkspaceID, req.FolderID); err != nil {
		return dto.DocumentResponse{}, err
	}

	name, ok := validateNodeName(req.Name)
	if !ok {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, ErrDocumentNameInvalid
	}
	req.Name = name

	if err := assertUploadable(req.Name); err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, err
	}

	size, mime, err := s.store.Stat(ctx, req.StorageKey)
	if err != nil {
		return dto.DocumentResponse{}, ErrUploadNotFound
	}

	if err := assertUploadSize(size); err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, err
	}

	var doc contentdb.Document
	var ver contentdb.DocumentVersion

	err = s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		maxPos, err := q.GetMaxPosition(ctx, fID)
		if err != nil {
			return err
		}

		doc, err = q.CreateDocument(ctx, contentdb.CreateDocumentParams{
			WorkspaceID: wID,
			FolderID:    fID,
			Name:        req.Name,
			Position:    maxPos + 1,
			UploadedBy:  uID,
		})

		if err != nil {
			return err
		}

		ver, err = q.CreateDocumentVersion(ctx, contentdb.CreateDocumentVersionParams{
			DocumentID: doc.ID,
			VersionNo:  1,
			Mime:       mime,
			Size:       size,
			StorageKey: req.StorageKey,
			UploadedBy: uID,
		})

		if err != nil {
			return err
		}

		if err := q.SetCurrentVersion(ctx, contentdb.SetCurrentVersionParams{
			ID:               doc.ID,
			CurrentVersionID: ver.ID,
		}); err != nil {
			return err
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			activityservice.ActionDocumentUploaded, activityservice.TargetDocument,
			uuidString(doc.ID), doc.Name, nil))
	})

	if err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, fmt.Errorf("delete document: %w", err)
	}

	return dto.DocumentResponse{
		ID:               uuidString(doc.ID),
		FolderID:         uuidString(doc.FolderID),
		Name:             doc.Name,
		VersionNo:        ver.VersionNo,
		CurrentVersionID: uuidString(ver.ID),
		Mime:             ver.Mime,
		Size:             ver.Size,
		RenditionStatus:  renditionStatus(ver.RenditionKey, ver.RenditionFailedAt),
		VersionCount:     1,
		CreatedAt:        doc.CreatedAt.Time,
		UpdatedAt:        doc.UpdatedAt.Time,
	}, nil
}

func (s *ContentService) RequestVersionUpload(ctx context.Context, workspaceID, documentID string) (dto.UploadURLResponse, error) {
	var dID pgtype.UUID
	if err := dID.Scan(documentID); err != nil {
		return dto.UploadURLResponse{}, fmt.Errorf("document id parse: %w", err)
	}

	doc, err := s.repo.GetDocumentByID(ctx, dID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.UploadURLResponse{}, ErrDocumentNotFound
	}

	if err != nil {
		return dto.UploadURLResponse{}, fmt.Errorf("get document: %w", err)
	}

	if uuidString(doc.WorkspaceID) != workspaceID {
		return dto.UploadURLResponse{}, ErrDocumentNotFound
	}

	key := storageKey(uuidString(doc.WorkspaceID), uuidString(doc.FolderID))
	url, err := s.store.PresignedPut(ctx, key, uploadURLTTL)
	if err != nil {
		return dto.UploadURLResponse{}, fmt.Errorf("presign put: %w", err)
	}

	return dto.UploadURLResponse{
		UploadURL:  url,
		StorageKey: key,
	}, nil
}

func (s *ContentService) CompletedVersion(ctx context.Context, req dto.CompleteVersionRequest, actor Actor) (dto.DocumentResponse, error) {
	var dID, uID pgtype.UUID
	if err := dID.Scan(req.DocumentID); err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("document id parse: %w", err)
	}

	if err := uID.Scan(req.UploadedBy); err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("user id parse: %w", err)
	}

	doc, err := s.repo.GetDocumentByID(ctx, dID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.DocumentResponse{}, ErrDocumentNotFound
	}

	if err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("get document: %w", err)
	}

	if uuidString(doc.WorkspaceID) != req.WorkspaceID {
		return dto.DocumentResponse{}, ErrDocumentNotFound
	}

	if err := assertVersionType(doc.Name, req.FileName); err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, err
	}

	size, mime, err := s.store.Stat(ctx, req.StorageKey)
	if err != nil {
		return dto.DocumentResponse{}, ErrUploadNotFound
	}

	if err := assertUploadSize(size); err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, err
	}

	var ver contentdb.DocumentVersion
	err = s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		next, err := q.GetNextVersionNo(ctx, dID)
		if err != nil {
			return err
		}

		ver, err = q.CreateDocumentVersion(ctx, contentdb.CreateDocumentVersionParams{
			DocumentID: dID,
			VersionNo:  next,
			Mime:       mime,
			Size:       size,
			StorageKey: req.StorageKey,
			UploadedBy: uID,
		})

		if err != nil {
			return err
		}

		// Staged, not current: the pointer moves only after the rendition proves
		// out (ensureRendition → PromoteStagedVersion). Until then every reader
		// keeps getting the old version, so a broken upload never breaks the room.
		if err := q.SetStagedVersion(ctx, contentdb.SetStagedVersionParams{
			ID:              dID,
			StagedVersionID: ver.ID,
		}); err != nil {
			return err
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			activityservice.ActionVersionUploaded, activityservice.TargetVersion,
			uuidString(ver.ID), doc.Name, map[string]any{"version_no": ver.VersionNo}))
	})

	if err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, fmt.Errorf("create version: %w", err)
	}

	s.convertVersionAsync(req.WorkspaceID, dID, ver.ID)

	cur, err := s.repo.GetCurrentVersion(ctx, dID)
	if err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("get current version: %w", err)
	}

	return dto.DocumentResponse{
		ID:                    uuidString(doc.ID),
		FolderID:              uuidString(doc.FolderID),
		Name:                  doc.Name,
		VersionNo:             cur.VersionNo,
		CurrentVersionID:      uuidString(cur.ID),
		Mime:                  cur.Mime,
		Size:                  cur.Size,
		RenditionStatus:       renditionStatus(cur.RenditionKey, cur.RenditionFailedAt),
		VersionCount:          ver.VersionNo,
		StagedVersionID:       uuidString(ver.ID),
		StagedVersionNo:       &ver.VersionNo,
		StagedRenditionStatus: dto.RenditionPending,
		CreatedAt:             doc.CreatedAt.Time,
		UpdatedAt:             ver.CreatedAt.Time,
	}, nil
}

// convertVersionAsync runs the rendition conversion detached from the request,
// so completing an upload or a retry answers immediately while gotenberg works.
// It re-reads both rows: the staged pointer was written after the caller's doc
// was loaded, and promotion in ensureRendition trusts the row it is handed.
// Every failure path only logs — the version stays pending/failed and the next
// open of it converts lazily, exactly as before this kick existed.
func (s *ContentService) convertVersionAsync(workspaceID string, documentID, versionID pgtype.UUID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), asyncConvertTimeout)
		defer cancel()

		doc, err := s.repo.GetDocumentByID(ctx, documentID)
		if err != nil {
			log.Printf("async convert: get document: %v", err)
			return
		}

		version, err := s.repo.GetVersionByID(ctx, versionID)
		if err != nil {
			log.Printf("async convert: get version: %v", err)
			return
		}

		if _, _, err := s.ensureRendition(ctx, workspaceID, doc, version); err != nil {
			log.Printf("async convert version %s: %v", uuidString(versionID), err)
		}
	}()
}

func (s *ContentService) ListDocuments(ctx context.Context, workspaceID, folderID string, actor Actor) ([]dto.DocumentResponse, error) {
	var fID pgtype.UUID
	if err := fID.Scan(folderID); err != nil {
		return []dto.DocumentResponse{}, fmt.Errorf("folder id parse: %w", err)
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return []dto.DocumentResponse{}, ErrFolderNotFound
	}

	if err != nil {
		return []dto.DocumentResponse{}, fmt.Errorf("get folder: %w", err)
	}

	if uuidString(folder.WorkspaceID) != workspaceID {
		return []dto.DocumentResponse{}, ErrFolderNotFound
	}

	if err := s.requireFolderView(ctx, workspaceID, folderID, actor); err != nil {
		return []dto.DocumentResponse{}, err
	}

	rows, err := s.repo.ListDocumentsByFolder(ctx, fID)
	if err != nil {
		return []dto.DocumentResponse{}, fmt.Errorf("list documents: %w", err)
	}

	docs := make([]dto.DocumentResponse, 0, len(rows))
	for _, r := range rows {
		d := dto.DocumentResponse{
			ID:               uuidString(r.ID),
			FolderID:         uuidString(r.FolderID),
			Name:             r.Name,
			VersionNo:        r.VersionNo,
			CurrentVersionID: uuidString(r.CurrentVersionID),
			Mime:             r.Mime,
			Size:             r.Size,
			RenditionStatus:  renditionStatus(r.RenditionKey, r.RenditionFailedAt),
			VersionCount:     r.VersionCount,
			CreatedAt:        r.CreatedAt.Time,
			UpdatedAt:        r.UpdatedAt.Time,
		}

		// Staged state is manager knowledge, like the history it belongs to:
		// a guest only ever knows the served version.
		if actor.bypassesContentAccess() && r.StagedVersionID.Valid {
			d.StagedVersionID = uuidString(r.StagedVersionID)
			d.StagedVersionNo = r.StagedVersionNo
			d.StagedRenditionStatus = renditionStatus(r.StagedRenditionKey, r.StagedRenditionFailedAt)
		}

		docs = append(docs, d)
	}

	return docs, nil
}

func (s *ContentService) ListVersions(ctx context.Context, workspaceID, documentID string, actor Actor) ([]dto.VersionResponse, error) {
	if !actor.bypassesContentAccess() {
		return nil, ErrContentForbidden
	}

	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return nil, err
	}

	rows, err := s.repo.ListVersionsWithUploader(ctx, doc.ID)
	if err != nil {
		return []dto.VersionResponse{}, fmt.Errorf("list versions: %w", err)
	}

	vers := make([]dto.VersionResponse, 0, len(rows))
	for _, r := range rows {
		vers = append(vers, dto.VersionResponse{
			ID:              uuidString(r.ID),
			VersionNo:       r.VersionNo,
			Mime:            r.Mime,
			Size:            r.Size,
			UploadedBy:      uuidString(r.UploadedBy),
			UploadedByName:  r.UploadedByName,
			IsCurrent:       r.IsCurrent,
			IsStaged:        r.IsStaged,
			RenditionStatus: renditionStatus(r.RenditionKey, r.RenditionFailedAt),
			CreatedAt:       r.CreatedAt.Time,
		})
	}

	return vers, nil
}

type DownloadResult struct {
	Body     io.ReadCloser
	FileName string
	JobID    string
}

func (s *ContentService) DownloadDocument(ctx context.Context, workspaceID, documentID, versionID string, actor Actor, mark watermark.Mark) (DownloadResult, error) {
	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return DownloadResult{}, err
	}

	access, err := s.resolveViewAccess(ctx, workspaceID, uuidString(doc.FolderID), actor)
	if err != nil {
		return DownloadResult{}, err
	}

	clean := access.canDownloadOriginal
	if !clean && !access.canDownload {
		return DownloadResult{}, ErrContentForbidden
	}

	version, err := s.resolveRequestVersion(ctx, doc, versionID, actor)
	if err != nil {
		return DownloadResult{}, err
	}

	renditionKey, pageCount, err := s.ensureRendition(ctx, workspaceID, doc, version)
	if err != nil {
		return DownloadResult{}, err
	}

	if clean {
		src, err := s.renditionGet(ctx, renditionKey)
		if err != nil {
			return DownloadResult{}, fmt.Errorf("get rendition: %w", err)
		}

		s.recordDownload(ctx, workspaceID, doc, version, actor, "clean")

		return DownloadResult{Body: src, FileName: downloadName(doc.Name)}, nil
	}

	if pageCount > maxWatermarkDownloadPages {
		return DownloadResult{}, fmt.Errorf("%w: %d pages, max %d", ErrWatermarkDownloadTooLarge, pageCount, maxWatermarkDownloadPages)
	}

	if pageCount > asyncDownloadPageThreshold {
		job, err := s.startDownloadJob(ctx, workspaceID, doc, version, pageCount, renditionKey, actor, mark)
		if err != nil {
			return DownloadResult{}, err
		}

		return DownloadResult{JobID: uuidString(job.ID)}, nil
	}

	select {
	case s.stampSem <- struct{}{}:
	default:
		return DownloadResult{}, ErrDownloadBusy
	}

	stampCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), downloadJobTimeout)
	result := make(chan stampResult, 1)

	// Jalur sinkron memakai kolam request tanpa nice — sengaja: pengguna
	// menunggu di bawah syncDownloadBudget, dan kolam ter-nice hanya akan
	// mendorong lebih banyak permintaan lewat anggaran itu ke eskalasi. Job
	// eskalasi ikut di kolam ini karena Document-nya sudah terbuka di sini.
	go func() {
		body, err := s.rasterWatermarkPDF(stampCtx, s.viewer.Renderer, workspaceID, uuidString(version.ID), renditionKey, pageCount, mark)
		result <- stampResult{body: body, err: err}
	}()

	budget := time.NewTimer(syncDownloadBudget)
	defer budget.Stop()

	select {
	case res := <-result:
		cancel()
		<-s.stampSem

		if res.err != nil {
			return DownloadResult{}, fmt.Errorf("%w: %v", ErrStampFailed, res.err)
		}

		s.recordDownload(ctx, workspaceID, doc, version, actor, "watermarked")

		return DownloadResult{Body: res.body, FileName: downloadName(doc.Name)}, nil

	case <-budget.C:
		job, err := s.escalateDownload(ctx, workspaceID, doc, version, pageCount, actor, result, cancel)
		if err != nil {
			return DownloadResult{}, err
		}

		return DownloadResult{JobID: uuidString(job.ID)}, nil
	}
}

func (s *ContentService) recordDownload(ctx context.Context, workspaceID string, doc contentdb.Document,
	version contentdb.DocumentVersion, actor Actor, variant string) {
	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionDocumentDownloaded, activityservice.TargetDocument,
		uuidString(doc.ID), doc.Name, map[string]any{"version_no": version.VersionNo, "variant": variant}))
}

func (s *ContentService) RetryRendition(ctx context.Context, workspaceID, documentID, versionID string, actor Actor) error {
	if !actor.managesRoom() {
		return ErrContentForbidden
	}

	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return err
	}

	version, err := s.resolveRequestVersion(ctx, doc, versionID, actor)
	if err != nil {
		return err
	}

	if err := s.repo.ClearVersionRenditionFailure(ctx, version.ID); err != nil {
		return fmt.Errorf("clear rendition failure: %w", err)
	}

	// The retry does the work, not just permits it: without this kick the toast
	// would promise a conversion that only the next reader's open triggers.
	s.convertVersionAsync(workspaceID, doc.ID, version.ID)

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionRenditionRetried, activityservice.TargetVersion,
		versionID, doc.Name, map[string]any{"version_no": version.VersionNo}))

	return nil
}

type spooledReadCloser struct {
	file *os.File
	dir  string
}

func (r *spooledReadCloser) Read(p []byte) (int, error) {
	return r.file.Read(p)
}

func (r *spooledReadCloser) Close() error {
	err := r.file.Close()
	if rmErr := os.RemoveAll(r.dir); rmErr != nil && err == nil {
		err = rmErr
	}
	return err
}

func (r *spooledReadCloser) size() (int64, error) {
	fi, err := r.file.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat watermarked pdf: %w", err)
	}

	return fi.Size(), nil
}

func (r *spooledReadCloser) rewind() error {
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind watermarked pdf: %w", err)
	}

	return nil
}

// rasterWatermarkPDF merakit varian unduhan ber-watermark sebagai raster
// ter-flatten (keputusan 9-g): tiap halaman dirender/ambil-cache → `BurnImage`
// yang sama dengan viewer → ditulis JPEG → rakit ulang jadi PDF. Tandanya
// adalah piksel, jadi `pdfcpu watermark remove` tidak lagi bisa mencabutnya.
//
// Halaman ditulis JPEG, bukan PNG (16-h): ImportImages menempelkan byte JPEG
// apa adanya sebagai DCTDecode, sedangkan PNG ia dekode lalu deflate ulang
// sebagai RGB mentah tanpa predictor — itulah asal berkas 40× lipat.
//
// Geometri halaman dijaga lewat pengelompokan run: `ImportImages` memaksa satu
// `PageDim` per panggilan (default A4), jadi halaman dikelompokkan jadi runs
// berurutan berdimensi sama dan digabung berurutan dengan `MergeRaw`. Run juga
// dipotong tiap `stampPagesPerRun` halaman (9.5-f); angka 25 lahir saat
// ImportImages masih menahan ~10 MB piksel per halaman dan dipertahankan
// sampai byte/halaman pasca-JPEG diukur (U-62). Import berjalan bergantian,
// bukan paralel.
//
// renderer adalah parameter karena dua kolam poppler melayani fungsi ini:
// jalur sinkron dan eskalasi memakai Viewer.Renderer, job latar memakai
// Viewer.DownloadJobRenderer yang ter-nice.
func (s *ContentService) rasterWatermarkPDF(ctx context.Context, renderer render.Render, workspaceID, versionID, renditionKey string, pageCount int, mark watermark.Mark) (*spooledReadCloser, error) {
	dir, err := os.MkdirTemp("", spool.Prefix+"wm-*")
	if err != nil {
		return nil, fmt.Errorf("temp dir: %w", err)
	}

	removeAll := func() { os.RemoveAll(dir) }

	pagesPath := filepath.Join(dir, "pages")
	if err := os.Mkdir(pagesPath, 0o700); err != nil {
		removeAll()
		return nil, fmt.Errorf("pages dir: %w", err)
	}

	pdf, err := s.renditionGet(ctx, renditionKey)
	if err != nil {
		removeAll()
		return nil, fmt.Errorf("get rendition: %w", err)
	}

	doc, err := renderer.Open(pdf)
	pdf.Close()
	if err != nil {
		removeAll()
		return nil, fmt.Errorf("open rendition: %w", err)
	}
	defer doc.Close()

	pages, err := s.burnPages(ctx, workspaceID, versionID, doc, pagesPath, pageCount, mark)
	if err != nil {
		removeAll()
		return nil, err
	}

	runFiles, err := importPageRuns(dir, pages, s.viewer.DPI, stampPagesPerRun)
	if err != nil {
		removeAll()
		return nil, err
	}

	outPath, err := mergeRuns(dir, runFiles)
	if err != nil {
		removeAll()
		return nil, err
	}

	f, err := os.Open(outPath)
	if err != nil {
		removeAll()
		return nil, fmt.Errorf("open watermarked pdf: %w", err)
	}

	return &spooledReadCloser{file: f, dir: dir}, nil
}

type burnedPage struct {
	path string
	w, h float64
}

func (s *ContentService) burnPages(ctx context.Context, workspaceID, versionID string, doc render.Document, pagesPath string, pageCount int, mark watermark.Mark) ([]burnedPage, error) {
	pages := make([]burnedPage, pageCount)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(stampWorkers)

	for page := 1; page <= pageCount && gctx.Err() == nil; page++ {
		g.Go(func() error {
			src, err := s.pageForDownload(gctx, workspaceID, versionID, doc, page)
			if err != nil {
				return fmt.Errorf("page %d: %w", page, err)
			}

			img, err := s.viewer.Watermark.BurnImage(src, mark)
			if err != nil {
				return fmt.Errorf("burn page %d: %w", page, err)
			}

			path := filepath.Join(pagesPath, fmt.Sprintf("p%04d.jpg", page))
			if err := writeJPEG(path, img); err != nil {
				return fmt.Errorf("write page %d: %w", page, err)
			}

			b := img.Bounds()
			pages[page-1] = burnedPage{path: path, w: float64(b.Dx()), h: float64(b.Dy())}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return pages, nil
}

func writeJPEG(path string, img image.Image) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: downloadJPEGQuality}); err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

type pageRun struct {
	w, h   float64
	images []string
}

func groupPageRuns(pages []burnedPage, maxPerRun int) []pageRun {
	var runs []pageRun
	for _, pg := range pages {
		last := len(runs) - 1
		if last < 0 || runs[last].w != pg.w || runs[last].h != pg.h || len(runs[last].images) >= maxPerRun {
			runs = append(runs, pageRun{w: pg.w, h: pg.h})
			last++
		}
		runs[last].images = append(runs[last].images, pg.path)
	}
	return runs
}

func importPageRuns(dir string, pages []burnedPage, dpi, maxPerRun int) ([]string, error) {
	runs := groupPageRuns(pages, maxPerRun)
	conf := model.NewDefaultConfiguration()
	runFiles := make([]string, 0, len(runs))

	for i, run := range runs {
		runPath := filepath.Join(dir, fmt.Sprintf("run-%03d.pdf", i))
		if err := importRun(runPath, run, dpi, conf); err != nil {
			return nil, fmt.Errorf("import run %d: %w", i, err)
		}
		for _, img := range run.images {
			os.Remove(img)
		}
		runFiles = append(runFiles, runPath)
	}

	return runFiles, nil
}

func importRun(runPath string, run pageRun, dpi int, conf *model.Configuration) error {
	out, err := os.Create(runPath)
	if err != nil {
		return err
	}
	defer out.Close()

	readers := make([]io.Reader, 0, len(run.images))
	for _, imgPath := range run.images {
		f, err := os.Open(imgPath)
		if err != nil {
			return err
		}
		defer f.Close()
		readers = append(readers, f)
	}

	// `Pos: types.Full` memakai dimensi piksel gambar sebagai mediabox
	// (PageDim diabaikan), jadi posisi non-Full + DPI render dipakai:
	// gambar dikonversi px→pt (px/DPI*72) di dalam mediabox PageDim.
	d := float64(dpi)
	imp := &pdfcpu.Import{
		PageDim:  &types.Dim{Width: run.w / d * 72, Height: run.h / d * 72},
		DPI:      dpi,
		UserDim:  true,
		Pos:      types.Center,
		Scale:    1,
		ScaleAbs: true,
		InpUnit:  types.POINTS,
	}

	if err := api.ImportImages(nil, out, readers, imp, conf); err != nil {
		return err
	}

	return out.Close()
}

func mergeRuns(dir string, runFiles []string) (string, error) {
	if len(runFiles) == 1 {
		return runFiles[0], nil
	}

	outPath := filepath.Join(dir, "out.pdf")
	out, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create output: %w", err)
	}
	defer out.Close()

	seekers := make([]io.ReadSeeker, 0, len(runFiles))
	for _, rf := range runFiles {
		f, err := os.Open(rf)
		if err != nil {
			return "", fmt.Errorf("open run %s: %w", rf, err)
		}
		defer f.Close()
		seekers = append(seekers, f)
	}

	if err := api.MergeRaw(seekers, out, false, model.NewDefaultConfiguration()); err != nil {
		return "", fmt.Errorf("merge runs: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close output: %w", err)
	}

	for _, rf := range runFiles {
		os.Remove(rf)
	}

	return outPath, nil
}

func (s *ContentService) DeleteDocument(ctx context.Context, workspaceID, documentID string, actor Actor) error {
	var dID pgtype.UUID
	if err := dID.Scan(documentID); err != nil {
		return fmt.Errorf("document id parse: %w", err)
	}

	doc, err := s.repo.GetDocumentByID(ctx, dID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDocumentNotFound
	} else if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	if uuidString(doc.WorkspaceID) != workspaceID {
		return ErrDocumentNotFound
	}

	var uID pgtype.UUID
	if err := uID.Scan(actor.UserID); err != nil {
		return fmt.Errorf("user id parse: %w", err)
	}

	if err := s.repo.SoftDeleteDocument(ctx, contentdb.SoftDeleteDocumentParams{
		ID:        dID,
		DeletedBy: uID,
	}); err != nil {
		return fmt.Errorf("soft delete document: %w", err)
	}

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionDocumentDeleted, activityservice.TargetDocument,
		documentID, doc.Name, nil))

	return nil
}

func (s *ContentService) MoveDocument(ctx context.Context, req dto.MoveDocumentRequest, actor Actor) error {
	var dID, fID pgtype.UUID
	if err := dID.Scan(req.DocumentID); err != nil {
		return fmt.Errorf("document id parse: %w", err)
	}
	if err := fID.Scan(req.FolderID); err != nil {
		return fmt.Errorf("folder id parse: %w", err)
	}

	return s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		doc, err := q.GetDocumentByID(ctx, dID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDocumentNotFound
		}
		if err != nil {
			return fmt.Errorf("get document: %w", err)
		}
		if uuidString(doc.WorkspaceID) != req.WorkspaceID {
			return ErrDocumentNotFound
		}

		if err := q.LockWorkspaceStructure(ctx, doc.WorkspaceID); err != nil {
			return fmt.Errorf("lock workspace structure: %w", err)
		}

		folder, err := q.GetFolderByID(ctx, fID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFolderNotFound
		}
		if err != nil {
			return fmt.Errorf("get target folder: %w", err)
		}
		if uuidString(folder.WorkspaceID) != req.WorkspaceID {
			return ErrParentCrossWorkspace
		}

		oldFolder := doc.FolderID

		maxPos, err := q.GetMaxPosition(ctx, fID)
		if err != nil {
			return fmt.Errorf("check max position: %w", err)
		}

		pos := maxPos + 1
		if req.Position != nil {
			pos = clampPosition(int32(*req.Position), maxPos+1)
		}

		if err := q.MoveDocument(ctx, contentdb.MoveDocumentParams{
			ID:       dID,
			FolderID: fID,
			Position: pos,
		}); err != nil {
			return fmt.Errorf("move document: %w", err)
		}

		if err := q.ReindexDocumentSiblings(ctx, contentdb.ReindexDocumentSiblingsParams{
			FolderID: fID,
			MovedID:  dID,
		}); err != nil {
			return fmt.Errorf("reindex target siblings: %w", err)
		}

		if oldFolder != fID {
			if err := q.ReindexDocumentSiblings(ctx, contentdb.ReindexDocumentSiblingsParams{
				FolderID: oldFolder,
				MovedID:  dID,
			}); err != nil {
				return fmt.Errorf("reindex source siblings: %w", err)
			}
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			activityservice.ActionDocumentMoved, activityservice.TargetDocument,
			req.DocumentID, doc.Name, map[string]any{"to_folder_id": req.FolderID}))
	})
}

func (s *ContentService) CompleteMultipart(ctx context.Context, req dto.CompleteMultipartRequest, actor Actor) (dto.DocumentResponse, error) {
	if err := validateStorageKey(req.StorageKey, req.WorkspaceID, req.FolderID); err != nil {
		return dto.DocumentResponse{}, err
	}

	parts := make([]storage.Part, 0, len(req.Parts))
	for _, p := range req.Parts {
		parts = append(parts, storage.Part{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})

	if err := s.store.CompleteMultipart(ctx, req.StorageKey, req.UploadID, req.ContentType, parts); err != nil {
		_ = s.store.AbortMultipart(ctx, req.StorageKey, req.UploadID)
		return dto.DocumentResponse{}, fmt.Errorf("complete multipart: %w", err)
	}

	return s.CompletedUpload(ctx, dto.CompleteUploadRequest{
		WorkspaceID: req.WorkspaceID,
		FolderID:    req.FolderID,
		UploadedBy:  req.UploadedBy,
		Name:        req.Name,
		StorageKey:  req.StorageKey,
	}, actor)
}

func (s *ContentService) assertFolderInWorkspace(ctx context.Context, workspaceID, folderID string) error {
	var fID pgtype.UUID
	if err := fID.Scan(folderID); err != nil {
		return fmt.Errorf("folder id parse: %w", err)
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFolderNotFound
	}

	if err != nil {
		return fmt.Errorf("get filder: %w", err)
	}

	if uuidString(folder.WorkspaceID) != workspaceID {
		return ErrFolderNotFound
	}

	return nil
}

func (s *ContentService) InitMultipart(ctx context.Context, req dto.InitMultipartRequest) (dto.InitMultipartResponse, error) {
	if err := s.assertFolderInWorkspace(ctx, req.WorkspaceID, req.FolderID); err != nil {
		return dto.InitMultipartResponse{}, err
	}

	name, ok := validateNodeName(req.Name)
	if !ok {
		return dto.InitMultipartResponse{}, ErrDocumentNameInvalid
	}
	req.Name = name

	if err := assertUploadable(req.Name); err != nil {
		return dto.InitMultipartResponse{}, err
	}

	if err := assertUploadSize(req.Size); err != nil {
		return dto.InitMultipartResponse{}, err
	}

	var fID pgtype.UUID
	if err := fID.Scan(req.FolderID); err != nil {
		return dto.InitMultipartResponse{}, fmt.Errorf("folder id parse: %w", err)
	}

	if _, err := s.repo.GetDocumentByNameInFolder(ctx, contentdb.GetDocumentByNameInFolderParams{
		FolderID: fID,
		Name:     req.Name,
	}); err == nil {
		return dto.InitMultipartResponse{}, ErrDocumentNameTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dto.InitMultipartResponse{}, fmt.Errorf("check document name: %w", err)
	}

	partCount := int((req.Size + multipartPartSize - 1) / multipartPartSize)
	if partCount > maxMultipartParts {
		return dto.InitMultipartResponse{}, ErrUploadTooLarge
	}

	key := storageKey(req.WorkspaceID, req.FolderID)
	uploadID, err := s.store.InitMultipart(ctx, key)
	if err != nil {
		return dto.InitMultipartResponse{}, fmt.Errorf("init multipart: %w", err)
	}

	return dto.InitMultipartResponse{
		UploadID:   uploadID,
		StorageKey: key,
		PartSize:   multipartPartSize,
		PartCount:  partCount,
	}, nil
}

func (s *ContentService) MultipartPartURLs(ctx context.Context, req dto.MultipartPartURLsRequest) (dto.MultipartPartURLsResponse, error) {
	if err := validateStorageKey(req.StorageKey, req.WorkspaceID, req.FolderID); err != nil {
		return dto.MultipartPartURLsResponse{}, err
	}

	if len(req.PartNumbers) > maxPartURLsPerCall {
		return dto.MultipartPartURLsResponse{}, ErrTooManyParts
	}

	urls := make([]dto.MultipartPartURL, 0, len(req.PartNumbers))
	for _, n := range req.PartNumbers {
		if n < 1 || n > maxMultipartParts {
			return dto.MultipartPartURLsResponse{}, ErrInvalidPartNumber
		}

		u, err := s.store.PresignPart(ctx, req.StorageKey, req.UploadID, n, uploadURLTTL)
		if err != nil {
			return dto.MultipartPartURLsResponse{}, fmt.Errorf("presign part %d: %w", n, err)
		}

		urls = append(urls, dto.MultipartPartURL{
			PartNumber: n,
			URL:        u,
		})
	}

	return dto.MultipartPartURLsResponse{
		URLs: urls,
	}, nil
}

func (s *ContentService) MultipartParts(ctx context.Context, req dto.ListPartsRequest) (dto.MultipartPartsResponse, error) {
	if err := validateStorageKey(req.StorageKey, req.WorkspaceID, req.FolderID); err != nil {
		return dto.MultipartPartsResponse{}, err
	}

	parts, err := s.store.ListParts(ctx, req.StorageKey, req.UploadID)
	if err != nil {
		return dto.MultipartPartsResponse{}, fmt.Errorf("list parts: %w", err)
	}

	out := make([]dto.UploadedPart, 0, len(parts))
	for _, p := range parts {
		out = append(out, dto.UploadedPart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
			Size:       p.Size,
		})
	}

	return dto.MultipartPartsResponse{Parts: out}, nil
}

func (s *ContentService) AbortMultipart(ctx context.Context, req dto.AbortMultipartRequest) error {
	if err := validateStorageKey(req.StorageKey, req.WorkspaceID, req.FolderID); err != nil {
		return err
	}

	if err := s.store.AbortMultipart(ctx, req.StorageKey, req.UploadID); err != nil {
		return fmt.Errorf("abort multipart: %w", err)
	}

	return nil
}

// RestoreVersion repoints the document at a version it already has rather than
// copying that version forward. `current_version_id` is the switch, so a new row
// would carry no new content — it would only spend a gotenberg conversion and a
// full page render rebuilding a rendition the target version already has cached.
//
// The consequence: the current version is no longer necessarily the highest
// version_no. `is_current` on the version list is the authority, not the number.
//
// Who restored what is an activity fact: it is recorded in the activity log,
// not in the version list.
func (s *ContentService) RestoreVersion(ctx context.Context, workspaceID, documentID, versionID string, actor Actor) (dto.DocumentResponse, error) {
	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return dto.DocumentResponse{}, err
	}

	var vID pgtype.UUID
	if err := vID.Scan(versionID); err != nil {
		return dto.DocumentResponse{}, ErrVersionNotFound
	}

	var target contentdb.DocumentVersion
	err = s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		src, err := q.GetVersionByID(ctx, vID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrVersionNotFound
		}

		if err != nil {
			return fmt.Errorf("get version: %w", err)
		}

		if src.DocumentID != doc.ID {
			return ErrVersionNotFound
		}

		cur, err := q.GetCurrentVersion(ctx, doc.ID)
		if err != nil {
			return fmt.Errorf("get current version: %w", err)
		}

		if src.ID == cur.ID {
			return ErrAlreadyCurrent
		}

		target = src

		if err := q.SetCurrentVersion(ctx, contentdb.SetCurrentVersionParams{
			ID:               doc.ID,
			CurrentVersionID: src.ID,
		}); err != nil {
			return err
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(workspaceID, actor,
			activityservice.ActionVersionRestored, activityservice.TargetVersion,
			versionID, doc.Name, map[string]any{"version_no": src.VersionNo}))
	})

	if err != nil {
		return dto.DocumentResponse{}, err
	}

	// SetCurrentVersion stamps updated_at; the row read before the transaction
	// still carries the old one. It also cleared any staged version — the
	// explicit choice wins — so the staged fields stay empty here.
	fresh, err := s.repo.GetDocumentByID(ctx, doc.ID)
	if err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("get document: %w", err)
	}

	// Versions are never deleted individually, so numbering is dense and the
	// next number minus one is the count.
	next, err := s.repo.GetNextVersionNo(ctx, doc.ID)
	if err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("count versions: %w", err)
	}

	return dto.DocumentResponse{
		ID:               uuidString(fresh.ID),
		FolderID:         uuidString(fresh.FolderID),
		Name:             fresh.Name,
		VersionNo:        target.VersionNo,
		CurrentVersionID: uuidString(target.ID),
		Mime:             target.Mime,
		Size:             target.Size,
		RenditionStatus:  renditionStatus(target.RenditionKey, target.RenditionFailedAt),
		VersionCount:     next - 1,
		CreatedAt:        fresh.CreatedAt.Time,
		UpdatedAt:        fresh.UpdatedAt.Time,
	}, nil
}

func renditionStatus(key *string, failedAt pgtype.Timestamptz) string {
	if failedAt.Valid {
		return dto.RenditionFailed
	}
	if key != nil {
		return dto.RenditionReady
	}
	return dto.RenditionPending
}
