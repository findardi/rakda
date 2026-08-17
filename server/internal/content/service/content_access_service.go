package service

import (
	"context"
	"errors"
	"fmt"

	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"
	"github.com/findardi/Riksa-App/server/internal/content/dto"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Actor struct {
	UserID string
	Role   string
	Name   string
	Email  string
}

func (a Actor) managesRoom() bool {
	return a.Role == permission.RoleOwner || a.Role == permission.RoleAdmin
}

func (a Actor) bypassesContentAccess() bool {
	return a.managesRoom()
}

func (s *ContentService) resolveFolderAccess(ctx context.Context, workspaceID, folderID string, actor Actor) (contentdb.ResolveFolderAccessRow, error) {
	var wID, fID, uID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return contentdb.ResolveFolderAccessRow{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := fID.Scan(folderID); err != nil {
		return contentdb.ResolveFolderAccessRow{}, ErrFolderNotFound
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return contentdb.ResolveFolderAccessRow{}, ErrContentForbidden
	}

	row, err := s.repo.ResolveFolderAccess(ctx, contentdb.ResolveFolderAccessParams{
		WorkspaceID: wID,
		UserID:      uID,
		FolderID:    fID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contentdb.ResolveFolderAccessRow{}, ErrContentForbidden
	}
	if err != nil {
		return contentdb.ResolveFolderAccessRow{}, fmt.Errorf("resolve folder access: %w", err)
	}

	return row, nil
}

func (s *ContentService) requireFolderView(ctx context.Context, workspaceID, folderID string, actor Actor) error {
	if actor.bypassesContentAccess() {
		return nil
	}

	row, err := s.resolveFolderAccess(ctx, workspaceID, folderID, actor)
	if err != nil {
		return err
	}
	if !row.CanView {
		return ErrContentForbidden
	}

	return nil
}

func (s *ContentService) requireFolderDownloadOriginal(ctx context.Context, workspaceID, folderID string, actor Actor) error {
	if actor.bypassesContentAccess() {
		return nil
	}

	row, err := s.resolveFolderAccess(ctx, workspaceID, folderID, actor)
	if err != nil {
		return err
	}

	if !row.CanDownloadOriginal {
		return ErrContentForbidden
	}

	return nil
}

func (s *ContentService) SetFolderAccess(ctx context.Context, req dto.SetFolderAccessRequest, actor Actor) error {
	var wID, fID, gID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return fmt.Errorf("workspace id parse: %w", err)
	}

	if err := fID.Scan(req.FolderID); err != nil {
		return ErrFolderNotFound
	}

	if err := gID.Scan(req.GroupID); err != nil {
		return ErrAccessTargetInvalid
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFolderNotFound
	}

	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}

	if folder.WorkspaceID != wID {
		return ErrFolderNotFound
	}

	canDownload := req.CanDownload || req.CanDownloadOriginal
	canView := req.CanView || canDownload || req.CanWatermark

	if _, err := s.repo.SetFolderAccess(ctx, contentdb.SetFolderAccessParams{
		GroupID:             gID,
		WorkspaceID:         wID,
		FolderID:            fID,
		CanView:             canView,
		CanDownload:         canDownload,
		CanWatermark:        req.CanWatermark,
		CanDownloadOriginal: req.CanDownloadOriginal,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccessTargetInvalid
		}
		return fmt.Errorf("set folder access: %w", err)
	}

	s.activity.Record(ctx, s.activityEntry(req.WorkspaceID, actor,
		activityservice.ActionFolderAccessChanged, activityservice.TargetFolderAccess,
		req.FolderID, folder.Name, map[string]any{
			"group_id":              req.GroupID,
			"can_view":              canView,
			"can_download":          canDownload,
			"can_watermark":         req.CanWatermark,
			"can_download_original": req.CanDownloadOriginal,
		}))

	return nil
}

func (s *ContentService) RemoveFolderAccess(ctx context.Context, workspaceID, groupID, folderID string, actor Actor) error {
	var wID, fID, gID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}
	if err := fID.Scan(folderID); err != nil {
		return ErrFolderNotFound
	}
	if err := gID.Scan(groupID); err != nil {
		return ErrAccessTargetInvalid
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFolderNotFound
	}

	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}

	if folder.WorkspaceID != wID {
		return ErrFolderNotFound
	}

	if err := s.repo.RemoveFolderAccess(ctx, contentdb.RemoveFolderAccessParams{
		FolderID:    fID,
		GroupID:     gID,
		WorkspaceID: wID,
	}); err != nil {
		return fmt.Errorf("remove folder access: %w", err)
	}

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionFolderAccessRemoved, activityservice.TargetFolderAccess,
		folderID, folder.Name, map[string]any{"group_id": groupID}))

	return nil
}

func (s *ContentService) ListFolderAccess(ctx context.Context, workspaceID, folderID string) ([]dto.FolderAccessResponse, error) {
	var wID, fID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return nil, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := fID.Scan(folderID); err != nil {
		return nil, ErrFolderNotFound
	}

	rows, err := s.repo.ListFolderAccess(ctx, contentdb.ListFolderAccessParams{
		FolderID:    fID,
		WorkspaceID: wID,
	})
	if err != nil {
		return nil, fmt.Errorf("list folder access: %w", err)
	}

	res := make([]dto.FolderAccessResponse, 0, len(rows))
	for _, r := range rows {
		res = append(res, dto.FolderAccessResponse{
			FolderID:            uuidString(r.FolderID),
			GroupID:             uuidString(r.GroupID),
			GroupName:           r.GroupName,
			CanView:             r.CanView,
			CanDownload:         r.CanDownload,
			CanWatermark:        r.CanWatermark,
			CanDownloadOriginal: r.CanDownloadOriginal,
		})
	}

	return res, nil
}
