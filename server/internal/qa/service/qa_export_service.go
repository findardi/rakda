package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/qa/dto"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *QAService) ExportQuestions(ctx context.Context, req dto.ExportQuestionsRequest, actor Actor) (dto.ExportQuestionsPage, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultQuestionPageSize
	}
	if limit > maxQuestionPageSize {
		limit = maxQuestionPageSize
	}

	var wID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.ExportQuestionsPage{}, fmt.Errorf("parse workspace id: %w", err)
	}

	params := qadb.ListQuestionsParams{
		WorkspaceID: wID,
		PageSize:    int32(limit),
	}

	if req.Status != "" {
		if !validStatusFilter(req.Status) {
			return dto.ExportQuestionsPage{}, ErrInvalidFilter
		}
		status := req.Status
		params.Status = &status
	}
	if req.Cursor != "" {
		cursorCreatedAt, cursorID, err := parseQuestionCursor(req.Cursor)
		if err != nil {
			return dto.ExportQuestionsPage{}, err
		}
		params.CursorCreatedAt = cursorCreatedAt
		params.CursorID = cursorID
	}

	if actor.managesRoom() {
		if req.GroupID != "" {
			var gID pgtype.UUID
			if err := gID.Scan(req.GroupID); err != nil {
				return dto.ExportQuestionsPage{}, ErrInvalidFilter
			}
			params.GroupID = gID
		}
	} else {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return dto.ExportQuestionsPage{}, fmt.Errorf("parse user id: %w", err)
		}
		grp, err := s.repo.GetMemberGroupQA(ctx, qadb.GetMemberGroupQAParams{WorkspaceID: wID, UserID: uID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return dto.ExportQuestionsPage{}, ErrQAForbidden
			}
			return dto.ExportQuestionsPage{}, fmt.Errorf("get member group: %w", err)
		}
		if !grp.QaEnabled {
			return dto.ExportQuestionsPage{}, ErrQADisabled
		}
		params.GroupID = grp.ID
	}

	rows, err := s.repo.ListQuestions(ctx, params)
	if err != nil {
		return dto.ExportQuestionsPage{}, fmt.Errorf("list questions: %w", err)
	}

	page := dto.ExportQuestionsPage{Rows: []dto.QuestionExportRow{}}
	for _, question := range rows {
		page.Rows = append(page.Rows, dto.QuestionExportRow{
			Number:    question.Number,
			Group:     question.GroupName,
			Subject:   question.Subject,
			Status:    question.Status,
			Type:      "question",
			Author:    question.AuthorName,
			Role:      permission.RoleGuest,
			Body:      question.Body,
			Document:  question.DocumentName,
			Folder:    question.FolderName,
			CreatedAt: question.CreatedAt.Time,
		})

		replies, err := s.repo.ListQuestionReplies(ctx, question.ID)
		if err != nil {
			return dto.ExportQuestionsPage{}, fmt.Errorf("list question replies: %w", err)
		}
		for _, reply := range replies {
			page.Rows = append(page.Rows, dto.QuestionExportRow{
				Number:    question.Number,
				Group:     question.GroupName,
				Subject:   question.Subject,
				Status:    question.Status,
				Type:      "reply",
				Author:    reply.AuthorName,
				Role:      reply.AuthorRole,
				Body:      reply.Body,
				CreatedAt: reply.CreatedAt.Time,
			})
		}
	}

	if len(rows) == limit {
		last := rows[len(rows)-1]
		page.NextCursor = questionCursor(last.CreatedAt.Time, last.ID.String())
	}

	return page, nil
}
