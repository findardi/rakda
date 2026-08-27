package service

import (
	"context"
	"encoding/csv"
	"io"
	"strconv"
	"time"

	"github.com/findardi/rakda/server/internal/qa/dto"
)

const exportBatchSize = 100

var questionsCSVHeader = []string{
	"number", "group", "subject", "status", "type",
	"author", "role", "body", "document", "folder", "created_at",
}

func (s *QAService) WriteQuestionsCSV(ctx context.Context, w io.Writer, req dto.ExportQuestionsRequest, actor Actor) error {
	if req.Limit <= 0 {
		req.Limit = exportBatchSize
	}

	page, err := s.ExportQuestions(ctx, req, actor)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(questionsCSVHeader); err != nil {
		return err
	}

	for {
		for _, row := range page.Rows {
			out := []string{
				strconv.FormatInt(int64(row.Number), 10),
				row.Group,
				row.Subject,
				row.Status,
				row.Type,
				row.Author,
				row.Role,
				row.Body,
				row.Document,
				row.Folder,
				row.CreatedAt.Format(time.RFC3339Nano),
			}
			if err := cw.Write(out); err != nil {
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
		page, err = s.ExportQuestions(ctx, req, actor)
		if err != nil {
			return err
		}
	}
}

func (s *QAService) ExportQuestionsCSV(ctx context.Context, w io.Writer, workspaceID, userID, name, email, role string) error {
	return s.WriteQuestionsCSV(ctx, w, dto.ExportQuestionsRequest{
		WorkspaceID: workspaceID,
		Limit:       exportBatchSize,
	}, Actor{UserID: userID, Name: name, Email: email, Role: role})
}
