package service

import (
	"context"
	"errors"
	"fmt"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func scanUniqueUUIDs(raw []string) ([]pgtype.UUID, error) {
	seen := make(map[string]bool, len(raw))
	out := make([]pgtype.UUID, 0, len(raw))
	for _, r := range raw {
		if seen[r] {
			continue
		}
		seen[r] = true

		var id pgtype.UUID
		if err := id.Scan(r); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *ContentService) BulkDeleteFolders(ctx context.Context, req dto.BulkDeleteFoldersRequest, actor Actor) error {
	var wID, uID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return fmt.Errorf("workspace id parse: %w", err)
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return fmt.Errorf("user id parse: %w", err)
	}

	ids, err := scanUniqueUUIDs(req.FolderIDs)
	if err != nil {
		return ErrFolderNotFound
	}

	return s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		if err := q.LockWorkspaceStructure(ctx, wID); err != nil {
			return fmt.Errorf("lock workspace structure: %w", err)
		}

		for _, id := range ids {
			folder, err := q.GetFolderByID(ctx, id)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrFolderNotFound
			}
			if err != nil {
				return fmt.Errorf("get folder: %w", err)
			}
			if folder.WorkspaceID != wID {
				return ErrFolderNotFound
			}
			if folder.IsDefault {
				return ErrDeleteDefault
			}
		}

		for _, id := range ids {
			if err := q.SoftDeleteFolderSubtree(ctx, contentdb.SoftDeleteFolderSubtreeParams{
				DeletedBy: uID,
				FolderID:  id,
			}); err != nil {
				return fmt.Errorf("soft delete folder tree: %w", err)
			}

			if err := q.SoftDeleteDocumentsForFolderRoot(ctx, contentdb.SoftDeleteDocumentsForFolderRootParams{
				DeletedBy: uID,
				FolderID:  id,
			}); err != nil {
				return fmt.Errorf("soft delete documents: %w", err)
			}
		}

		return s.activity.RecordTx(ctx, tx, activityservice.NewEntry(req.WorkspaceID, actor.UserID, actor.Name, actor.Role,
			activityservice.ActionFolderDeleted, activityservice.TargetFolder,
			"", "", map[string]any{"bulk": true, "count": len(ids)}))
	})
}

func (s *ContentService) BulkDeleteDocuments(ctx context.Context, req dto.BulkDeleteDocumentsRequest, actor Actor) error {
	var wID, uID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return fmt.Errorf("workspace id parse: %w", err)
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return fmt.Errorf("user id parse: %w", err)
	}

	ids, err := scanUniqueUUIDs(req.DocumentIDs)
	if err != nil {
		return ErrDocumentNotFound
	}

	return s.repo.ExecTxTx(ctx, func(q *contentdb.Queries, tx pgx.Tx) error {
		for _, id := range ids {
			doc, err := q.GetDocumentByID(ctx, id)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrDocumentNotFound
			}
			if err != nil {
				return fmt.Errorf("get document: %w", err)
			}
			if doc.WorkspaceID != wID {
				return ErrDocumentNotFound
			}
		}

		for _, id := range ids {
			if err := q.SoftDeleteDocument(ctx, contentdb.SoftDeleteDocumentParams{
				ID:        id,
				DeletedBy: uID,
			}); err != nil {
				return fmt.Errorf("soft delete document: %w", err)
			}
		}

		return s.activity.RecordTx(ctx, tx, activityservice.NewEntry(req.WorkspaceID, actor.UserID, actor.Name, actor.Role,
			activityservice.ActionDocumentDeleted, activityservice.TargetDocument,
			"", "", map[string]any{"bulk": true, "count": len(ids)}))
	})
}
