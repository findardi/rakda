package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"
	"github.com/findardi/Riksa-App/server/internal/content/dto"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/convert"
	"github.com/findardi/Riksa-App/server/internal/platform/render"
	"github.com/findardi/Riksa-App/server/internal/platform/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Viewer struct {
	Converter convert.Converter
	Renderer  render.Render
	Watermark watermark.Watermarker
	PDFStamp  watermark.PDFStamper
	DPI       int
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

func renditionPageKey(workspaceID, versionID string, page, dpi int) string {
	return fmt.Sprintf("%s/renditions/%s/pages/%d@%d.png", workspaceID, versionID, page, dpi)
}

func renditionPrefix(workspaceID, versionID string) string {
	return fmt.Sprintf("%s/renditions/%s/", workspaceID, versionID)
}

func (s *ContentService) resolveViewAccess(ctx context.Context, workspaceID, folderID string, actor Actor) (viewAccess, error) {
	if actor.bypassesContentAccess() {
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

	if v.ID != doc.CurrentVersionID && !actor.bypassesContentAccess() {
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

	if uuidString(doc.WorkspaceID) != workspaceID {
		return contentdb.Document{}, ErrDocumentNotFound
	}

	return doc, nil
}

func (s *ContentService) ensureRendition(ctx context.Context, workspaceID string, doc contentdb.Document, version contentdb.DocumentVersion) (string, int, error) {
	if version.RenditionFailedAt.Valid {
		return "", 0, ErrRenditionFailed
	}

	if version.RenditionKey != nil && version.PageCount != nil {
		if int(*version.PageCount) > maxRenditionPages {
			return "", 0, fmt.Errorf("%w: %d, max %d", ErrTooManyPages, *version.PageCount, maxRenditionPages)
		}

		return *version.RenditionKey, int(*version.PageCount), nil
	}

	versionID := uuidString(version.ID)

	var (
		renditionKey string
		pageCount    int
	)

	if convert.IsPDF(doc.Name) {
		renditionKey = version.StorageKey

		pdf, err := s.store.Get(ctx, renditionKey)
		if err != nil {
			return "", 0, fmt.Errorf("get original pdf: %w", err)
		}
		defer pdf.Close()

		pageCount, err = s.viewer.Renderer.PageCount(ctx, pdf)
		if err != nil {
			return "", 0, s.markRenditionFailed(ctx, version.ID, err)
		}
	} else {
		renditionKey = renditionPDFKey(workspaceID, versionID)

		src, err := s.store.Get(ctx, version.StorageKey)
		if err != nil {
			return "", 0, fmt.Errorf("get original: %w", err)
		}
		defer src.Close()

		pdf, err := s.viewer.Converter.ToPDF(ctx, src, doc.Name)
		if err != nil {
			if errors.Is(err, convert.ErrUnsupportedFile) {
				return "", 0, ErrNotViewable
			}

			return "", 0, s.markRenditionFailed(ctx, version.ID, err)
		}
		defer pdf.Close()

		buf, err := io.ReadAll(pdf)
		if err != nil {
			return "", 0, fmt.Errorf("read pdf: %w", err)
		}

		if err := s.store.Put(ctx, renditionKey, bytes.NewReader(buf), int64(len(buf)), "application/pdf"); err != nil {
			return "", 0, fmt.Errorf("store rendition: %w", err)
		}

		pageCount, err = s.viewer.Renderer.PageCount(ctx, bytes.NewReader(buf))
		if err != nil {
			return "", 0, s.markRenditionFailed(ctx, version.ID, err)
		}
	}

	pc := int32(pageCount)
	if err := s.repo.SetVersionRendition(ctx, contentdb.SetVersionRenditionParams{
		RenditionKey: &renditionKey,
		PageCount:    &pc,
		ID:           version.ID,
	}); err != nil {
		return "", 0, fmt.Errorf("set rendition: %w", err)
	}

	if pageCount > maxRenditionPages {
		return "", 0, fmt.Errorf("%w: %d, max %d", ErrTooManyPages, pageCount, maxRenditionPages)
	}

	return renditionKey, pageCount, nil
}

func (s *ContentService) markRenditionFailed(ctx context.Context, versionID pgtype.UUID, cause error) error {
	msg := cause.Error()
	if err := s.repo.SetVersionRenditionFailure(ctx, contentdb.SetVersionRenditionFailureParams{
		RenditionError: &msg,
		ID:             versionID,
	}); err != nil {
		log.Printf("record rendition failure: %v", err)
	}

	return ErrRenditionFailed
}

func (s *ContentService) GetViewMeta(ctx context.Context, workspaceID, documentID, versionID string, actor Actor) (dto.ViewMetaResponse, error) {
	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return dto.ViewMetaResponse{}, err
	}

	access, err := s.resolveViewAccess(ctx, workspaceID, uuidString(doc.FolderID), actor)
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

	_, pageCount, err := s.ensureRendition(ctx, workspaceID, doc, version)
	if err != nil {
		return dto.ViewMetaResponse{}, err
	}

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionDocumentViewed, activityservice.TargetDocument,
		documentID, doc.Name, map[string]any{"version_no": version.VersionNo}))

	return dto.ViewMetaResponse{
		DocumentID:          uuidString(doc.ID),
		Name:                doc.Name,
		Mime:                version.Mime,
		VersionID:           uuidString(version.ID),
		VersionNo:           version.VersionNo,
		PageCount:           pageCount,
		CanDownload:         access.canDownload,
		CanDownloadOriginal: access.canDownloadOriginal,
	}, nil
}

func (s *ContentService) GetPageImage(ctx context.Context, req dto.ViewPageRequest, actor Actor) ([]byte, error) {
	doc, err := s.getDocumentScoped(ctx, req.WorkspaceID, req.DocumentID)
	if err != nil {
		return nil, err
	}

	access, err := s.resolveViewAccess(ctx, req.WorkspaceID, uuidString(doc.FolderID), actor)
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

	renditionKey, pageCount, err := s.ensureRendition(ctx, req.WorkspaceID, doc, version)
	if err != nil {
		return nil, err
	}

	if req.Page < 1 || req.Page > pageCount {
		return nil, ErrPageOutOfRange
	}

	pageBytes, err := s.loadOrRenderPage(ctx, req.WorkspaceID, uuidString(version.ID), renditionKey, req.Page)
	if err != nil {
		return nil, err
	}

	if !actor.managesRoom() {
		s.activity.RecordPageEvent(ctx, activityservice.PageEvent{
			WorkspaceID:  req.WorkspaceID,
			DocumentID:   uuidString(doc.ID),
			DocumentName: doc.Name,
			VersionID:    uuidString(version.ID),
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
	key := renditionPageKey(workspaceID, versionID, page, s.viewer.DPI)

	if r, err := s.store.Get(ctx, key); err == nil {
		b, rerr := io.ReadAll(r)
		r.Close()
		if rerr == nil && len(b) > 0 {
			return b, nil
		}
	}

	pdf, err := s.store.Get(ctx, renditionKey)
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

	if err := s.store.Put(ctx, key, bytes.NewReader(img), int64(len(img)), "image/png"); err != nil {
		return nil, fmt.Errorf("cache page: %w", err)
	}

	return img, nil
}
