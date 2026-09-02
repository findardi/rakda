package service

import (
	"context"
	"encoding/csv"
	"io"
	"time"

	"github.com/findardi/rakda/server/internal/activity/dto"
)

const exportBatchSize = 100

var activityCSVHeader = []string{
	"id", "created_at", "actor_id", "actor_name", "actor_role",
	"action", "target_type", "target_id", "target_name", "metadata",
}

func (s *ActivityService) WriteActivityCSV(ctx context.Context, w io.Writer, req dto.ListActivityRequest, role string) error {
	if req.Limit <= 0 {
		req.Limit = exportBatchSize
	}

	page, err := s.ListActivity(ctx, req, role)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(activityCSVHeader); err != nil {
		return err
	}

	for {
		for _, it := range page.Items {
			row := []string{
				it.ID,
				it.CreatedAt.Format(time.RFC3339Nano),
				it.ActorID,
				it.ActorName,
				it.ActorRole,
				it.Action,
				it.TargetType,
				it.TargetID,
				it.TargetName,
				string(it.Metadata),
			}
			if err := cw.Write(row); err != nil {
				return err
			}
		}

		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}

		if page.NextCursor == "" {
			return nil
		}

		req.Cursor = page.NextCursor
		page, err = s.ListActivity(ctx, req, role)
		if err != nil {
			return err
		}
	}
}

func (s *ActivityService) ExportActivityCSV(ctx context.Context, w io.Writer, workspaceID, role string) error {
	return s.WriteActivityCSV(ctx, w, dto.ListActivityRequest{
		WorkspaceID: workspaceID,
		Limit:       exportBatchSize,
	}, role)
}
