package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"
	"github.com/findardi/Riksa-App/server/internal/content/dto"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxRestoreNameAttempts = 100

func resolveFolderRestoreName(ctx context.Context, q *contentdb.Queries, workspaceID, parentID pgtype.UUID, base string) (string, error) {
	name := base
	for i := 2; i < maxRestoreNameAttempts; i++ {
		if _, err := q.GetFolderByNameInParent(ctx, contentdb.GetFolderByNameInParentParams{
			WorkspaceID: workspaceID,
			ParentID:    parentID,
			Name:        name,
		}); errors.Is(err, pgx.ErrNoRows) {
			return name, nil
		} else if err != nil {
			return "", fmt.Errorf("check folder name: %w", err)
		}

		name = base + " (" + strconv.Itoa(i) + ")"
	}
	return "", ErrFolderNameTaken
}

func resolveDocumentRestoreName(ctx context.Context, q *contentdb.Queries, folderID pgtype.UUID, base string) (string, error) {
	name := base
	for i := 2; i < maxRestoreNameAttempts; i++ {
		if _, err := q.GetDocumentByNameInFolder(ctx, contentdb.GetDocumentByNameInFolderParams{
			FolderID: folderID,
			Name:     name,
		}); errors.Is(err, pgx.ErrNoRows) {
			return name, nil
		} else if err != nil {
			return "", fmt.Errorf("check document name: %w", err)
		}

		name = base + " (" + strconv.Itoa(i) + ")"
	}
	return "", ErrDocumentNameTaken
}

func (s *ContentService) ListTrash(ctx context.Context, workspaceID string, actor Actor) (dto.TrashListResponse, error) {
	if !actor.bypassesContentAccess() {
		return dto.TrashListResponse{}, ErrContentForbidden
	}

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return dto.TrashListResponse{}, fmt.Errorf("workspace id parse: %w", err)
	}

	folders, err := s.repo.ListTrashFolders(ctx, wID)
	if err != nil {
		return dto.TrashListResponse{}, fmt.Errorf("list trash folders: %w", err)
	}

	documents, err := s.repo.ListTrashDocuments(ctx, wID)
	if err != nil {
		return dto.TrashListResponse{}, fmt.Errorf("list trash documents: %w", err)
	}

	res := dto.TrashListResponse{
		Folders:        make([]dto.TrashFolderItem, 0, len(folders)),
		Documents:      make([]dto.TrashDocumentItem, 0, len(documents)),
		RetentionHours: int64(s.trashRetention.Hours()),
	}

	for _, f := range folders {
		res.Folders = append(res.Folders, dto.TrashFolderItem{
			ID:            uuidString(f.ID),
			Name:          f.Name,
			DeletedByName: f.DeletedByName,
			DeletedAt:     f.DeletedAt.Time,
			PurgeAfter:    f.DeletedAt.Time.Add(s.trashRetention),
			ParentName:    f.ParentName,
			ParentGone:    f.ParentGone,
			FolderCount:   f.FolderCount,
			DocumentCount: f.DocumentCount,
		})
	}

	for _, d := range documents {
		res.Documents = append(res.Documents, dto.TrashDocumentItem{
			ID:            uuidString(d.ID),
			Name:          d.Name,
			DeletedByName: d.DeletedByName,
			DeletedAt:     d.DeletedAt.Time,
			PurgeAfter:    d.DeletedAt.Time.Add(s.trashRetention),
			Mime:          deref(d.Mime),
			Size:          deref(d.Size),
			FolderName:    d.FolderName,
			FolderGone:    d.FolderGone,
		})
	}

	return res, nil
}

func (s *ContentService) RestoreFolders(ctx context.Context, workspaceID, folderID string, actor Actor) (dto.RestoreResponse, error) {
	if !actor.bypassesContentAccess() {
		return dto.RestoreResponse{}, ErrContentForbidden
	}

	var fID, wID pgtype.UUID
	if err := fID.Scan(folderID); err != nil {
		return dto.RestoreResponse{}, fmt.Errorf("folder id parse: %w", err)
	}

	if err := wID.Scan(workspaceID); err != nil {
		return dto.RestoreResponse{}, fmt.Errorf("workspace id parse: %w", err)
	}

	var res dto.RestoreResponse
	err := s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		if err := q.LockWorkspaceStructure(ctx, wID); err != nil {
			return fmt.Errorf("lock workspace: %w", err)
		}

		folder, err := q.GetTrashedFolderByID(ctx, fID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotInTrash
		} else if err != nil {
			return fmt.Errorf("get trash folder: %w", err)
		}

		if uuidString(folder.WorkspaceID) != workspaceID {
			return ErrNotInTrash
		}

		if folder.DeletedRootFolderID.Valid {
			return ErrNotInTrash
		}

		parentID := folder.ParentID
		parentName := ""
		if parentID.Valid {
			parent, err := q.GetFolderByID(ctx, parentID)
			if errors.Is(err, pgx.ErrNoRows) {
				parentID = pgtype.UUID{}
			} else if err != nil {
				return fmt.Errorf("get parent folder: %w", err)
			} else {
				parentName = parent.Name
			}
		}

		name, err := resolveFolderRestoreName(ctx, q, wID, parentID, folder.Name)
		if err != nil {
			return err
		}

		maxPos, err := q.GetMaxPositionInParent(ctx, contentdb.GetMaxPositionInParentParams{
			WorkspaceID: wID,
			ParentID:    parentID,
		})

		if err != nil {
			return fmt.Errorf("get max position: %w", err)
		}

		if err := q.RestoreFolderRoot(ctx, contentdb.RestoreFolderRootParams{
			ID:       fID,
			Name:     name,
			ParentID: parentID,
			Position: maxPos + 1,
		}); err != nil {
			return fmt.Errorf("restore folder: %w", err)
		}

		if err := q.RestoreFoldersSweptBy(ctx, fID); err != nil {
			return fmt.Errorf("restore swept folders: %w", err)
		}

		if err := q.RestoreDocumentsSweptBy(ctx, fID); err != nil {
			return fmt.Errorf("restore swept documents: %w", err)
		}

		res = dto.RestoreResponse{
			ID:         folderID,
			Name:       name,
			Renamed:    name != folder.Name,
			FolderID:   uuidString(parentID),
			FolderName: parentName,
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(workspaceID, actor,
			activityservice.ActionFolderRestored, activityservice.TargetFolder,
			folderID, name, nil))
	})
	if err != nil {
		return dto.RestoreResponse{}, err
	}

	return res, nil
}

func (s *ContentService) RestoreDocument(ctx context.Context, workspaceID, documentID string, actor Actor) (dto.RestoreResponse, error) {
	if !actor.bypassesContentAccess() {
		return dto.RestoreResponse{}, ErrContentForbidden
	}

	var dID, wID pgtype.UUID
	if err := dID.Scan(documentID); err != nil {
		return dto.RestoreResponse{}, fmt.Errorf("document id parse: %w", err)
	}

	if err := wID.Scan(workspaceID); err != nil {
		return dto.RestoreResponse{}, fmt.Errorf("workspace id parse: %w", err)
	}

	var res dto.RestoreResponse
	err := s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		if err := q.LockWorkspaceStructure(ctx, wID); err != nil {
			return fmt.Errorf("lock workspace: %w", err)
		}

		doc, err := q.GetTrashedDocumentByID(ctx, dID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotInTrash
		} else if err != nil {
			return fmt.Errorf("get trashed document: %w", err)
		}

		if uuidString(doc.WorkspaceID) != workspaceID {
			return ErrNotInTrash
		}

		if doc.DeletedRootFolderID.Valid {
			return ErrNotInTrash
		}

		folderID := doc.FolderID
		folderName := ""
		folder, err := q.GetFolderByID(ctx, folderID)
		if errors.Is(err, pgx.ErrNoRows) {
			def, err := q.GetDefaultFolder(ctx, wID)
			if err != nil {
				return fmt.Errorf("get default folder: %w", err)
			}
			folderID = def.ID
			folderName = def.Name
		} else if err != nil {
			return fmt.Errorf("get folder: %w", err)
		} else {
			folderName = folder.Name
		}

		name, err := resolveDocumentRestoreName(ctx, q, folderID, doc.Name)
		if err != nil {
			return err
		}

		maxPos, err := q.GetMaxPosition(ctx, folderID)
		if err != nil {
			return fmt.Errorf("get max position: %w", err)
		}

		if err := q.RestoreDocument(ctx, contentdb.RestoreDocumentParams{
			ID:       dID,
			FolderID: folderID,
			Position: maxPos + 1,
			Name:     name,
		}); err != nil {
			return fmt.Errorf("restore document: %w", err)
		}

		res = dto.RestoreResponse{
			ID:         documentID,
			Name:       name,
			Renamed:    name != doc.Name,
			FolderID:   uuidString(folderID),
			FolderName: folderName,
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(workspaceID, actor,
			activityservice.ActionDocumentRestored, activityservice.TargetDocument,
			documentID, name, nil))
	})
	if err != nil {
		return dto.RestoreResponse{}, err
	}

	return res, nil
}
