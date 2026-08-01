package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/findardi/Riksa-App/server/internal/content/dto"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

func (s *ContentService) CompletedUpload(ctx context.Context, req dto.CompleteUploadRequest) (dto.DocumentResponse, error) {
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

	size, mime, err := s.store.Stat(ctx, req.StorageKey)
	if err != nil {
		return dto.DocumentResponse{}, ErrUploadNotFound
	}

	var doc contentdb.Document
	var ver contentdb.DocumentVersion

	err = s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
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

		return q.SetCurrentVersion(ctx, contentdb.SetCurrentVersionParams{
			ID:               doc.ID,
			CurrentVersionID: ver.ID,
		})
	})

	if err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, fmt.Errorf("delete document: %w", err)
	}

	return dto.DocumentResponse{
		ID:        uuidString(doc.ID),
		FolderID:  uuidString(doc.FolderID),
		Name:      doc.Name,
		VersionNo: ver.VersionNo,
		Mime:      ver.Mime,
		Size:      ver.Size,
		CreatedAt: doc.CreatedAt.Time,
		UpdatedAt: doc.UpdatedAt.Time,
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

func (s *ContentService) CompletedVersion(ctx context.Context, req dto.CompleteVersionRequest) (dto.DocumentResponse, error) {
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

	size, mime, err := s.store.Stat(ctx, req.StorageKey)
	if err != nil {
		return dto.DocumentResponse{}, ErrUploadNotFound
	}

	var ver contentdb.DocumentVersion
	err = s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
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

		return q.SetCurrentVersion(ctx, contentdb.SetCurrentVersionParams{
			ID:               dID,
			CurrentVersionID: ver.ID,
		})
	})

	if err != nil {
		_ = s.store.Delete(ctx, req.StorageKey)
		return dto.DocumentResponse{}, fmt.Errorf("create version: %w", err)
	}

	return dto.DocumentResponse{
		ID:        uuidString(doc.ID),
		FolderID:  uuidString(doc.FolderID),
		Name:      doc.Name,
		VersionNo: ver.VersionNo,
		Mime:      ver.Mime,
		Size:      ver.Size,
		CreatedAt: doc.CreatedAt.Time,
		UpdatedAt: ver.CreatedAt.Time,
	}, nil
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
		docs = append(docs, dto.DocumentResponse{
			ID:        uuidString(r.ID),
			FolderID:  uuidString(r.FolderID),
			Name:      r.Name,
			VersionNo: r.VersionNo,
			Mime:      r.Mime,
			Size:      r.Size,
			CreatedAt: r.CreatedAt.Time,
			UpdatedAt: r.UpdatedAt.Time,
		})
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
			ID:             uuidString(r.ID),
			VersionNo:      r.VersionNo,
			Mime:           r.Mime,
			Size:           r.Size,
			UploadedBy:     uuidString(r.UploadedBy),
			UploadedByName: r.UploadedByName,
			IsCurrent:      r.IsCurrent,
			CreatedAt:      r.CreatedAt.Time,
		})
	}

	return vers, nil
}

func (s *ContentService) GetDownloadURL(ctx context.Context, workspaceID, documentID, versionID string, actor Actor) (dto.DownloadURLResponse, error) {
	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return dto.DownloadURLResponse{}, err
	}

	if err := s.requireFolderDownloadOriginal(ctx, workspaceID, uuidString(doc.FolderID), actor); err != nil {
		return dto.DownloadURLResponse{}, err
	}

	version, err := s.resolveRequestVersion(ctx, doc, versionID, actor)
	if err != nil {
		return dto.DownloadURLResponse{}, err
	}

	url, err := s.store.PresignedGet(ctx, version.StorageKey, doc.Name, downloadURLTTL)
	if err != nil {
		return dto.DownloadURLResponse{}, fmt.Errorf("presign get: %w", err)
	}

	return dto.DownloadURLResponse{
		DownloadURL: url,
	}, nil
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

	return nil
}

func (s *ContentService) MoveDocument(ctx context.Context, req dto.MoveDocumentRequest) error {
	var dID, fID pgtype.UUID
	if err := dID.Scan(req.DocumentID); err != nil {
		return fmt.Errorf("document id parse: %w", err)
	}
	if err := fID.Scan(req.FolderID); err != nil {
		return fmt.Errorf("folder id parse: %w", err)
	}

	return s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
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

		return nil
	})
}

func (s *ContentService) CompleteMultipart(ctx context.Context, req dto.CompleteMultipartRequest) (dto.DocumentResponse, error) {
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
	})
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
// `restoreBy` is validated but not stored: who restored what is an activity fact,
// and belongs in the audit log rather than in the version list.
func (s *ContentService) RestoreVersion(ctx context.Context, workspaceID, documentID, versionID, restoreBy string) (dto.DocumentResponse, error) {
	doc, err := s.getDocumentScoped(ctx, workspaceID, documentID)
	if err != nil {
		return dto.DocumentResponse{}, err
	}

	var vID, uID pgtype.UUID
	if err := vID.Scan(versionID); err != nil {
		return dto.DocumentResponse{}, ErrVersionNotFound
	}

	if err := uID.Scan(restoreBy); err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("restored by parse: %w", err)
	}

	var target contentdb.DocumentVersion
	err = s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
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

		return q.SetCurrentVersion(ctx, contentdb.SetCurrentVersionParams{
			ID:               doc.ID,
			CurrentVersionID: src.ID,
		})
	})

	if err != nil {
		return dto.DocumentResponse{}, err
	}

	// SetCurrentVersion stamps updated_at; the row read before the transaction
	// still carries the old one.
	fresh, err := s.repo.GetDocumentByID(ctx, doc.ID)
	if err != nil {
		return dto.DocumentResponse{}, fmt.Errorf("get document: %w", err)
	}

	return dto.DocumentResponse{
		ID:        uuidString(fresh.ID),
		FolderID:  uuidString(fresh.FolderID),
		Name:      fresh.Name,
		VersionNo: target.VersionNo,
		Mime:      target.Mime,
		Size:      target.Size,
		CreatedAt: fresh.CreatedAt.Time,
		UpdatedAt: fresh.UpdatedAt.Time,
	}, nil
}
