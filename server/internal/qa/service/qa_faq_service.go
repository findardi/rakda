package service

import (
	"context"
	"errors"
	"fmt"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/qa/dto"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *QAService) ListFaqs(ctx context.Context, workspaceID string) ([]dto.FaqResponse, error) {
	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return nil, fmt.Errorf("parse workspace id: %w", err)
	}

	faqs, err := s.repo.ListFaqs(ctx, wID)
	if err != nil {
		return nil, fmt.Errorf("list faqs: %w", err)
	}

	res := []dto.FaqResponse{}
	for _, faq := range faqs {
		res = append(res, faqResponse(faq))
	}
	return res, nil
}

func (s *QAService) CreateFaq(ctx context.Context, req dto.CreateFaqRequest, actor Actor) (dto.FaqResponse, error) {
	if !actor.managesRoom() {
		return dto.FaqResponse{}, ErrQAForbidden
	}

	var wID, uID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.FaqResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return dto.FaqResponse{}, fmt.Errorf("parse user id: %w", err)
	}

	var sourceID pgtype.UUID
	if req.SourceQuestionID != "" {
		var qID pgtype.UUID
		if err := qID.Scan(req.SourceQuestionID); err != nil {
			return dto.FaqResponse{}, ErrReferenceNotFound
		}
		source, err := s.repo.GetQuestion(ctx, qadb.GetQuestionParams{ID: qID, WorkspaceID: wID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return dto.FaqResponse{}, ErrReferenceNotFound
			}
			return dto.FaqResponse{}, fmt.Errorf("get source question: %w", err)
		}
		sourceID = source.ID
	}

	var created qadb.Faq
	err := s.repo.ExecTxTx(ctx, func(q *qadb.Queries, tx pgx.Tx) error {
		var err error
		created, err = q.InsertFaq(ctx, qadb.InsertFaqParams{
			WorkspaceID:      wID,
			QuestionText:     req.QuestionText,
			AnswerText:       req.AnswerText,
			SourceQuestionID: sourceID,
			CreatedBy:        uID,
			CreatorName:      actor.Name,
		})
		if err != nil {
			return fmt.Errorf("insert faq: %w", err)
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			activityservice.ActionFaqPublished, activityservice.TargetFaq,
			uuidString(created.ID), created.QuestionText, nil))
	})
	if err != nil {
		return dto.FaqResponse{}, err
	}

	return faqResponse(created), nil
}

func faqResponse(faq qadb.Faq) dto.FaqResponse {
	return dto.FaqResponse{
		ID:           uuidString(faq.ID),
		QuestionText: faq.QuestionText,
		AnswerText:   faq.AnswerText,
		CreatedAt:    faq.CreatedAt.Time,
	}
}
