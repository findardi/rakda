package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/convert"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/findardi/rakda/server/internal/platform/spool"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Viewer struct {
	Converter convert.Converter
	Renderer  render.Render

	// DownloadJobRenderer melayani job unduhan ber-watermark latar saja —
	// kolam poppler ter-nice supaya pembaca viewer menang saat CPU berebut.
	// Jalur sinkron (≤ asyncDownloadPageThreshold, pengguna menunggu di bawah
	// syncDownloadBudget) dan job eskalasi (Document-nya sudah terbuka) tetap
	// memakai Renderer. Nil → Renderer (lihat NewContentService).
	DownloadJobRenderer render.Render

	Watermark     watermark.Watermarker
	TextExtractor render.TextExtractor
	WordBoxes     render.WordBoxExtractor
	OCR           render.OCR
	DPI           int
}

type viewAccess struct {
	canView             bool
	canDownload         bool
	canDownloadOriginal bool
	canWatermark        bool
}

func renditionPDFKey(workspaceID, versionID string) string {
	return fmt.Sprintf("%s/renditions/%s/rendition.pdf", workspaceID, versionID)
}

// pageCacheKey dan pageCachePrefix tinggal di prefix puncak sendiri,
// page-cache/, supaya PNG halaman bisa kedaluwarsa tanpa menyentuh
// rendition.pdf yang berbagi prefix renditions/ (keputusan 9.5-c).
func pageCacheKey(workspaceID, versionID string, page, dpi int) string {
	return fmt.Sprintf("page-cache/%s/%s/%d@%d.png", workspaceID, versionID, page, dpi)
}

func pageCachePrefix(workspaceID, versionID string) string {
	return fmt.Sprintf("page-cache/%s/%s/", workspaceID, versionID)
}

func renditionPrefix(workspaceID, versionID string) string {
	return fmt.Sprintf("%s/renditions/%s/", workspaceID, versionID)
}

func (s *ContentService) resolveViewAccess(ctx context.Context, workspaceID, folderID string, actor Actor) (viewAccess, error) {
	if actor.managesRoom() {
		return viewAccess{
			canView:             true,
			canDownload:         true,
			canDownloadOriginal: true,
			canWatermark:        false,
		}, nil
	}

	row, err := s.resolveFolderAccess(ctx, workspaceID, folderID, actor)
	if err != nil {
		return viewAccess{}, err
	}

	if actor.RoomStatus == permission.RoomArchive {
		return viewAccess{
			canView:             row.CanView,
			canDownload:         false,
			canDownloadOriginal: false,
			canWatermark:        true,
		}, nil
	}

	return viewAccess{
		canView:             row.CanView,
		canDownload:         row.CanDownload,
		canDownloadOriginal: row.CanDownloadOriginal,
		canWatermark:        row.CanWatermark,
	}, nil
}

func (s *ContentService) resolveRequestVersion(ctx context.Context, doc contentdb.Document, versionID string, actor Actor) (contentdb.DocumentVersion, error) {
	if versionID == "" {
		v, err := s.repo.GetCurrentVersion(ctx, doc.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return contentdb.DocumentVersion{}, ErrDocumentNotFound
		}

		if err != nil {
			return contentdb.DocumentVersion{}, fmt.Errorf("get document: %w", err)
		}

		return v, nil
	}

	var vID pgtype.UUID
	if err := vID.Scan(versionID); err != nil {
		return contentdb.DocumentVersion{}, ErrVersionNotFound
	}

	v, err := s.repo.GetVersionByID(ctx, vID)
	if errors.Is(err, pgx.ErrNoRows) {
		return contentdb.DocumentVersion{}, ErrVersionNotFound
	}

	if err != nil {
		return contentdb.DocumentVersion{}, fmt.Errorf("get version: %w", err)
	}

	if v.DocumentID != doc.ID {
		return contentdb.DocumentVersion{}, ErrVersionNotFound
	}

	if v.ID != doc.CurrentVersionID && !actor.managesRoom() {
		return contentdb.DocumentVersion{}, ErrContentForbidden
	}

	return v, nil
}

func (s *ContentService) getDocumentScoped(ctx context.Context, workspaceID, documentID string) (contentdb.Document, error) {
	var dID pgtype.UUID
	if err := dID.Scan(documentID); err != nil {
		return contentdb.Document{}, ErrDocumentNotFound
	}

	doc, err := s.repo.GetDocumentByID(ctx, dID)
	if errors.Is(err, pgx.ErrNoRows) {
		return contentdb.Document{}, ErrDocumentNotFound
	}

	if err != nil {
		return contentdb.Document{}, fmt.Errorf("get document: %w", err)
	}

	if doc.WorkspaceID.String() != workspaceID {
		return contentdb.Document{}, ErrDocumentNotFound
	}

	return doc, nil
}

func (s *ContentService) renditionState(ctx context.Context, doc contentdb.Document, version contentdb.DocumentVersion) (string, int, error) {
	if version.RenditionFailedAt.Valid {
		return "", 0, ErrRenditionFailed
	}

	if version.RenditionKey != nil && version.PageCount != nil {
		if int(*version.PageCount) > maxRenditionPages {
			return "", 0, fmt.Errorf("%w: %d, max %d", ErrTooManyPages, *version.PageCount, maxRenditionPages)
		}

		s.promoteStaged(ctx, doc, version)
		return *version.RenditionKey, int(*version.PageCount), nil
	}

	if version.ID != doc.CurrentVersionID && version.ID != doc.StagedVersionID {
		if err := s.repo.RequestRendition(ctx, version.ID); err != nil {
			return "", 0, fmt.Errorf("request rendition: %w", err)
		}
	}

	s.wakeRenditionWorker()
	return "", 0, ErrRenditionPending
}

// promoteStaged serves a staged version the moment its rendition proves out —
// and only then: promotion never fires from CompletedVersion itself. The
// in-memory comparison is a cheap gate (most renditions have nothing staged);
// truth lives in PromoteStagedVersion's where-guard, which no-ops when a
// restore or newer upload moved the pointers while this conversion ran.
func (s *ContentService) promoteStaged(ctx context.Context, doc contentdb.Document, version contentdb.DocumentVersion) {
	if !doc.StagedVersionID.Valid || doc.StagedVersionID != version.ID {
		return
	}

	if _, err := s.repo.PromoteStagedVersion(ctx, contentdb.PromoteStagedVersionParams{
		ID:        doc.ID,
		VersionID: version.ID,
	}); err != nil {
		log.Printf("promote staged version %s: %v", version.ID.String(), err)
	}
}

func (s *ContentService) buildRendition(ctx context.Context, workspaceID string, doc contentdb.Document, version contentdb.DocumentVersion) (string, int, error) {
	if convert.IsPDF(doc.Name) {
		pdf, err := s.renditionGet(ctx, version.StorageKey)
		if err != nil {
			return "", 0, fmt.Errorf("get original pdf: %w", err)
		}
		defer pdf.Close()

		pageCount, err := s.viewer.Renderer.PageCount(ctx, pdf)
		if err != nil {
			return "", 0, fmt.Errorf("page count: %w", err)
		}

		return version.StorageKey, pageCount, nil
	}

	renditionKey := renditionPDFKey(workspaceID, version.ID.String())

	src, err := s.store.Get(ctx, version.StorageKey)
	if err != nil {
		return "", 0, fmt.Errorf("get original: %w", err)
	}
	defer src.Close()

	pdf, err := s.viewer.Converter.ToPDF(ctx, src, doc.Name)
	if err != nil {
		return "", 0, fmt.Errorf("convert: %w", err)
	}
	defer pdf.Close()

	// Tumpahkan sekali ke berkas sementara (9.5-b): poppler sudah men-spool
	// ke disk untuk PageCount, jadi menahan seluruh PDF di RAM hanyalah
	// membayar dua kali. Berkas yang sama dipakai untuk PageCount lalu Put.
	f, err := os.CreateTemp("", spool.Prefix+"rendition-*.pdf")
	if err != nil {
		return "", 0, fmt.Errorf("temp rendition: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	n, err := io.Copy(f, pdf)
	if err != nil {
		return "", 0, fmt.Errorf("spool rendition: %w", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, fmt.Errorf("rewind rendition: %w", err)
	}

	pageCount, err := s.viewer.Renderer.PageCount(ctx, f)
	if err != nil {
		return "", 0, fmt.Errorf("page count: %w", err)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", 0, fmt.Errorf("rewind rendition: %w", err)
	}

	if err := s.store.Put(ctx, renditionKey, f, n, "application/pdf"); err != nil {
		return "", 0, fmt.Errorf("store rendition: %w", err)
	}

	if pageCount <= maxRenditionPages {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			s.renditionPut(renditionKey, f)
		}
	}

	return renditionKey, pageCount, nil
}

func (s *ContentService) GetViewMeta(ctx context.Context, workspaceID, documentID, versionID string, actor Actor) (dto.ViewMetaResponse, error) {
	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return dto.ViewMetaResponse{}, err
	}

	access, err := s.resolveViewAccess(ctx, workspaceID, doc.FolderID.String(), actor)
	if err != nil {
		return dto.ViewMetaResponse{}, err
	}
	if !access.canView {
		return dto.ViewMetaResponse{}, ErrContentForbidden
	}

	if !convert.Viewable(doc.Name) {
		return dto.ViewMetaResponse{}, ErrNotViewable
	}

	version, err := s.resolveRequestVersion(ctx, doc, versionID, actor)
	if err != nil {
		return dto.ViewMetaResponse{}, fmt.Errorf("get current version: %w", err)
	}

	_, pageCount, err := s.renditionState(ctx, doc, version)

	status := dto.RenditionReady
	switch {
	case errors.Is(err, ErrRenditionPending):
		status, pageCount = dto.RenditionPending, 0
	case errors.Is(err, ErrRenditionFailed):
		status, pageCount = dto.RenditionFailed, 0
	case err != nil:
		return dto.ViewMetaResponse{}, err
	}

	if status == dto.RenditionReady {
		s.activity.Record(ctx, activityservice.NewEntry(workspaceID, actor.UserID, actor.Name, actor.Role,
			activityservice.ActionDocumentViewed, activityservice.TargetDocument,
			documentID, doc.Name, map[string]any{"version_no": version.VersionNo}))
	}

	return dto.ViewMetaResponse{
		DocumentID:                doc.ID.String(),
		Name:                      doc.Name,
		Mime:                      version.Mime,
		VersionID:                 version.ID.String(),
		VersionNo:                 version.VersionNo,
		PageCount:                 pageCount,
		RenditionStatus:           status,
		CanDownload:               access.canDownload,
		CanDownloadOriginal:       access.canDownloadOriginal,
		WatermarkDownloadMaxPages: maxWatermarkDownloadPages,
	}, nil
}

func (s *ContentService) GetPageImage(ctx context.Context, req dto.ViewPageRequest, actor Actor) ([]byte, error) {
	doc, err := s.getDocumentScoped(ctx, req.WorkspaceID, req.DocumentID)
	if err != nil {
		return nil, err
	}

	access, err := s.resolveViewAccess(ctx, req.WorkspaceID, doc.FolderID.String(), actor)
	if err != nil {
		return nil, err
	}
	if !access.canView {
		return nil, ErrContentForbidden
	}

	if !convert.Viewable(doc.Name) {
		return nil, ErrNotViewable
	}

	version, err := s.resolveRequestVersion(ctx, doc, req.VersionID, actor)
	if err != nil {
		return nil, fmt.Errorf("get current version: %w", err)
	}

	renditionKey, pageCount, err := s.renditionState(ctx, doc, version)
	if err != nil {
		return nil, err
	}

	if req.Page < 1 || req.Page > pageCount {
		return nil, ErrPageOutOfRange
	}

	pageBytes, err := s.loadOrRenderPage(ctx, req.WorkspaceID, version.ID.String(), renditionKey, req.Page)
	if err != nil {
		return nil, err
	}

	if !actor.managesRoom() {
		s.activity.RecordPageEvent(ctx, activityservice.PageEvent{
			WorkspaceID:  req.WorkspaceID,
			DocumentID:   doc.ID.String(),
			DocumentName: doc.Name,
			VersionID:    version.ID.String(),
			PageNo:       int32(req.Page),
			EventType:    activityservice.EventViewPage,
			ActorID:      actor.UserID,
			ActorEmail:   actor.Email,
		})
	}

	if !access.canWatermark {
		return pageBytes, nil
	}

	marked, err := s.viewer.Watermark.Burn(pageBytes, watermark.Mark{
		Primary:   req.MarkPrimary,
		Secondary: req.MarkSecondary,
	})
	if err != nil {
		return nil, fmt.Errorf("watermark: %w", err)
	}

	return marked, nil
}

func (s *ContentService) loadOrRenderPage(ctx context.Context, workspaceID, versionID, renditionKey string, page int) ([]byte, error) {
	key := pageCacheKey(workspaceID, versionID, page, s.viewer.DPI)

	if b, ok := s.cachedPage(ctx, key); ok {
		return b, nil
	}

	img, err := s.renderPage(ctx, renditionKey, page)
	if err != nil {
		return nil, err
	}

	if err := s.storePage(ctx, key, img); err != nil {
		return nil, fmt.Errorf("cache page: %w", err)
	}

	return img, nil
}

// renderPage merender satu halaman tanpa menyentuh cache PNG.
func (s *ContentService) renderPage(ctx context.Context, renditionKey string, page int) ([]byte, error) {
	pdf, err := s.renditionGet(ctx, renditionKey)
	if err != nil {
		return nil, fmt.Errorf("get rendition: %w", err)
	}
	defer pdf.Close()

	img, err := s.viewer.Renderer.RenderPage(ctx, pdf, page)
	if errors.Is(err, render.ErrPageOutOfRange) {
		return nil, ErrPageOutOfRange
	}
	if err != nil {
		return nil, fmt.Errorf("render page: %w", err)
	}

	return img, nil
}

// pageForDownload memakai cache PNG bila ada dan merender ke berkas
// sementara bila tidak — unduhan menyentuh semua halaman termasuk yang tak
// pernah dibaca, jadi mengisi cache dari sini hanya men-churn cache dengan
// halaman yang tidak dilihat siapa pun; jalur ini tidak pernah mengisi cache
// (keputusan 9-g, dipertahankan oleh 9.5-c).
func (s *ContentService) pageForDownload(ctx context.Context, workspaceID, versionID string, doc render.Document, page int) ([]byte, error) {
	key := pageCacheKey(workspaceID, versionID, page, s.viewer.DPI)

	if b, ok := s.cachedPage(ctx, key); ok {
		return b, nil
	}

	img, err := doc.RenderPage(ctx, page)
	if errors.Is(err, render.ErrPageOutOfRange) {
		return nil, ErrPageOutOfRange
	}
	if err != nil {
		return nil, fmt.Errorf("render page: %w", err)
	}

	return img, nil
}

func (s *ContentService) DownloadLimits() dto.DownloadLimitsResponse {
	return dto.DownloadLimitsResponse{WatermarkDownloadMaxPages: maxWatermarkDownloadPages}
}
