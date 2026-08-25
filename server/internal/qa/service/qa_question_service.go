package service

import (
	"context"
	"errors"
	"fmt"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/qa/dto"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type refInfo struct {
	id       pgtype.UUID
	folderID pgtype.UUID
	name     string
}

func (s *QAService) SubmitQuestion(ctx context.Context, req dto.SubmitQuestionRequest, actor Actor) (dto.QuestionThreadResponse, error) {
	if actor.Role != permission.RoleGuest {
		return dto.QuestionThreadResponse{}, ErrOnlyGuestCanAsk
	}

	var wID, uID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.QuestionThreadResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return dto.QuestionThreadResponse{}, fmt.Errorf("parse user id: %w", err)
	}

	grp, err := s.repo.GetMemberGroupQA(ctx, qadb.GetMemberGroupQAParams{WorkspaceID: wID, UserID: uID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.QuestionThreadResponse{}, ErrQAForbidden
		}
		return dto.QuestionThreadResponse{}, fmt.Errorf("get member group: %w", err)
	}
	if !grp.QaEnabled {
		return dto.QuestionThreadResponse{}, ErrQADisabled
	}

	docRef, folderRef, err := s.resolveSubmitRefs(ctx, req, actor)
	if err != nil {
		return dto.QuestionThreadResponse{}, err
	}

	var created qadb.Question
	err = s.repo.ExecTxTx(ctx, func(q *qadb.Queries, tx pgx.Tx) error {
		lock, err := q.LockGroupQA(ctx, qadb.LockGroupQAParams{GroupID: grp.ID, WorkspaceID: wID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrQAForbidden
			}
			return fmt.Errorf("lock group: %w", err)
		}
		if !lock.QaEnabled {
			return ErrQADisabled
		}

		stats, err := q.GroupQuestionStats(ctx, grp.ID)
		if err != nil {
			return fmt.Errorf("group question stats: %w", err)
		}
		if lock.QaQuestionLimit != nil && stats.Used >= int64(*lock.QaQuestionLimit) {
			return ErrQuestionLimitReached
		}

		params := qadb.InsertQuestionParams{
			WorkspaceID: wID,
			GroupID:     grp.ID,
			GroupName:   lock.Name,
			Number:      stats.NextNumber,
			AuthorID:    uID,
			AuthorName:  actor.Name,
			Subject:     req.Subject,
			Body:        req.Body,
		}
		if docRef != nil {
			params.DocumentID = docRef.id
			params.DocumentName = docRef.name
		}
		if folderRef != nil {
			params.FolderID = folderRef.id
			params.FolderName = folderRef.name
		}

		created, err = q.InsertQuestion(ctx, params)
		if err != nil {
			return fmt.Errorf("insert question: %w", err)
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			activityservice.ActionQuestionSubmitted, activityservice.TargetQuestion,
			uuidString(created.ID), created.Subject,
			map[string]any{"group_name": lock.Name, "number": stats.NextNumber}))
	})
	if err != nil {
		return dto.QuestionThreadResponse{}, err
	}

	res := threadResponse(created, nil)
	if docRef != nil {
		res.DocumentRef = &dto.ReferenceChip{
			ID:       uuidString(docRef.id),
			FolderID: uuidString(docRef.folderID),
			Name:     docRef.name,
		}
	}
	if folderRef != nil {
		res.FolderRef = &dto.ReferenceChip{
			ID:   uuidString(folderRef.id),
			Name: folderRef.name,
		}
	}
	return res, nil
}

func (s *QAService) resolveSubmitRefs(ctx context.Context, req dto.SubmitQuestionRequest, actor Actor) (*refInfo, *refInfo, error) {
	var doc, folder *refInfo

	if req.DocumentID != "" {
		var dID pgtype.UUID
		if err := dID.Scan(req.DocumentID); err != nil {
			return nil, nil, ErrReferenceNotFound
		}
		d, err := s.repo.GetDocumentForRef(ctx, dID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrReferenceNotFound
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get document ref: %w", err)
		}
		if uuidString(d.WorkspaceID) != req.WorkspaceID {
			return nil, nil, ErrReferenceNotFound
		}
		ok, err := s.content.CanUserViewFolder(ctx, req.WorkspaceID, uuidString(d.FolderID), actor.UserID)
		if err != nil {
			return nil, nil, fmt.Errorf("check document ref access: %w", err)
		}
		if !ok {
			return nil, nil, ErrReferenceNotFound
		}
		doc = &refInfo{id: d.ID, folderID: d.FolderID, name: d.Name}
	}

	if req.FolderID != "" {
		var fID pgtype.UUID
		if err := fID.Scan(req.FolderID); err != nil {
			return nil, nil, ErrReferenceNotFound
		}
		f, err := s.repo.GetFolderForRef(ctx, fID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrReferenceNotFound
		}
		if err != nil {
			return nil, nil, fmt.Errorf("get folder ref: %w", err)
		}
		if uuidString(f.WorkspaceID) != req.WorkspaceID {
			return nil, nil, ErrReferenceNotFound
		}
		ok, err := s.content.CanUserViewFolder(ctx, req.WorkspaceID, req.FolderID, actor.UserID)
		if err != nil {
			return nil, nil, fmt.Errorf("check folder ref access: %w", err)
		}
		if !ok {
			return nil, nil, ErrReferenceNotFound
		}
		folder = &refInfo{id: f.ID, name: f.Name}
	}

	return doc, folder, nil
}

func (s *QAService) ListQuestions(ctx context.Context, req dto.ListQuestionsRequest, actor Actor) (dto.ListQuestionsResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = defaultQuestionPageSize
	}
	if limit > maxQuestionPageSize {
		limit = maxQuestionPageSize
	}

	var wID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.ListQuestionsResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	params := qadb.ListQuestionsParams{
		WorkspaceID: wID,
		PageSize:    int32(limit),
	}

	if req.Status != "" {
		if !validStatusFilter(req.Status) {
			return dto.ListQuestionsResponse{}, ErrInvalidFilter
		}
		status := req.Status
		params.Status = &status
	}
	if req.Cursor != "" {
		cursorCreatedAt, cursorID, err := parseQuestionCursor(req.Cursor)
		if err != nil {
			return dto.ListQuestionsResponse{}, err
		}
		params.CursorCreatedAt = cursorCreatedAt
		params.CursorID = cursorID
	}

	res := dto.ListQuestionsResponse{
		Items:     []dto.QuestionListItem{},
		QAEnabled: true,
	}
	var scopeGroup pgtype.UUID

	if actor.managesRoom() {
		if req.GroupID != "" {
			var gID pgtype.UUID
			if err := gID.Scan(req.GroupID); err != nil {
				return dto.ListQuestionsResponse{}, ErrInvalidFilter
			}
			params.GroupID = gID
		}

		waiting, err := s.repo.CountWaitingQuestions(ctx, wID)
		if err != nil {
			return dto.ListQuestionsResponse{}, fmt.Errorf("count waiting questions: %w", err)
		}
		res.WaitingCount = &waiting
	} else {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return dto.ListQuestionsResponse{}, fmt.Errorf("parse user id: %w", err)
		}
		grp, err := s.repo.GetMemberGroupQA(ctx, qadb.GetMemberGroupQAParams{WorkspaceID: wID, UserID: uID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return dto.ListQuestionsResponse{}, ErrQAForbidden
			}
			return dto.ListQuestionsResponse{}, fmt.Errorf("get member group: %w", err)
		}
		params.GroupID = grp.ID
		scopeGroup = grp.ID
		res.QAEnabled = grp.QaEnabled
		res.QuestionLimit = grp.QaQuestionLimit
	}

	rows, err := s.repo.ListQuestions(ctx, params)
	if err != nil {
		return dto.ListQuestionsResponse{}, fmt.Errorf("list questions: %w", err)
	}
	for _, row := range rows {
		res.Items = append(res.Items, dto.QuestionListItem{
			ID:         uuidString(row.ID),
			Number:     row.Number,
			Subject:    row.Subject,
			Status:     row.Status,
			GroupID:    uuidString(row.GroupID),
			GroupName:  row.GroupName,
			AuthorID:   uuidString(row.AuthorID),
			AuthorName: row.AuthorName,
			ReplyCount: row.ReplyCount,
			CreatedAt:  row.CreatedAt.Time,
		})
	}
	if len(rows) == limit {
		last := rows[len(rows)-1]
		res.NextCursor = questionCursor(last.CreatedAt.Time, uuidString(last.ID))
	}

	total, err := s.repo.CountQuestions(ctx, qadb.CountQuestionsParams{WorkspaceID: wID, GroupID: scopeGroup})
	if err != nil {
		return dto.ListQuestionsResponse{}, fmt.Errorf("count questions: %w", err)
	}
	res.QuestionCount = total
	if !actor.managesRoom() && res.QuestionLimit != nil {
		remaining := int32(0)
		if d := int64(*res.QuestionLimit) - total; d > 0 {
			remaining = int32(d)
		}
		res.QuotaRemaining = &remaining
	}

	faqCount, err := s.repo.CountFaqs(ctx, wID)
	if err != nil {
		return dto.ListQuestionsResponse{}, fmt.Errorf("count faqs: %w", err)
	}
	res.FaqCount = faqCount

	return res, nil
}

func (s *QAService) CountWaiting(ctx context.Context, workspaceID string, actor Actor) (dto.WaitingCountResponse, error) {
	if !actor.managesRoom() {
		return dto.WaitingCountResponse{}, ErrQAForbidden
	}

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return dto.WaitingCountResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	waiting, err := s.repo.CountWaitingQuestions(ctx, wID)
	if err != nil {
		return dto.WaitingCountResponse{}, fmt.Errorf("count waiting questions: %w", err)
	}

	return dto.WaitingCountResponse{WaitingCount: waiting}, nil
}

func (s *QAService) GetQuestion(ctx context.Context, workspaceID, questionID string, actor Actor) (dto.QuestionThreadResponse, error) {
	question, _, err := s.loadVisibleQuestion(ctx, workspaceID, questionID, actor)
	if err != nil {
		return dto.QuestionThreadResponse{}, err
	}

	replies, err := s.repo.ListQuestionReplies(ctx, question.ID)
	if err != nil {
		return dto.QuestionThreadResponse{}, fmt.Errorf("list question replies: %w", err)
	}

	res := threadResponse(question, replies)
	if err := s.attachRefs(ctx, &res, question, actor); err != nil {
		return dto.QuestionThreadResponse{}, err
	}

	return res, nil
}

func (s *QAService) ReplyQuestion(ctx context.Context, req dto.ReplyQuestionRequest, actor Actor) (dto.ReplyResult, error) {
	question, grp, err := s.loadVisibleQuestion(ctx, req.WorkspaceID, req.QuestionID, actor)
	if err != nil {
		return dto.ReplyResult{}, err
	}
	if question.Status == StatusClosed {
		return dto.ReplyResult{}, ErrQuestionClosed
	}
	if !actor.managesRoom() && !grp.QaEnabled {
		return dto.ReplyResult{}, ErrQADisabled
	}

	var uID pgtype.UUID
	if err := uID.Scan(actor.UserID); err != nil {
		return dto.ReplyResult{}, fmt.Errorf("parse user id: %w", err)
	}

	newStatus := nextStatusAfterReply(actor.Role)
	var reply qadb.QuestionReply
	err = s.repo.ExecTxTx(ctx, func(q *qadb.Queries, tx pgx.Tx) error {
		locked, err := q.GetQuestionLocked(ctx, question.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrQuestionNotFound
			}
			return fmt.Errorf("lock question: %w", err)
		}
		if locked.Status == StatusClosed {
			return ErrQuestionClosed
		}

		reply, err = q.InsertReply(ctx, qadb.InsertReplyParams{
			QuestionID: question.ID,
			AuthorID:   uID,
			AuthorName: actor.Name,
			AuthorRole: actor.Role,
			Body:       req.Body,
		})
		if err != nil {
			return fmt.Errorf("insert reply: %w", err)
		}

		if newStatus != locked.Status {
			if err := q.UpdateQuestionStatus(ctx, qadb.UpdateQuestionStatusParams{Status: newStatus, ID: question.ID}); err != nil {
				return fmt.Errorf("update question status: %w", err)
			}
		}

		action := activityservice.ActionQuestionReplied
		if actor.managesRoom() {
			action = activityservice.ActionQuestionAnswered
		}
		return s.activity.RecordTx(ctx, tx, s.activityEntry(req.WorkspaceID, actor,
			action, activityservice.TargetQuestion,
			uuidString(question.ID), question.Subject, nil))
	})
	if err != nil {
		return dto.ReplyResult{}, err
	}

	return dto.ReplyResult{Reply: replyResponse(reply), QuestionStatus: newStatus}, nil
}

func (s *QAService) CloseQuestion(ctx context.Context, workspaceID, questionID string, actor Actor) error {
	question, _, err := s.loadVisibleQuestion(ctx, workspaceID, questionID, actor)
	if err != nil {
		return err
	}
	if !actor.managesRoom() && uuidString(question.AuthorID) != actor.UserID {
		return ErrCloseNotAllowed
	}

	return s.repo.ExecTxTx(ctx, func(q *qadb.Queries, tx pgx.Tx) error {
		locked, err := q.GetQuestionLocked(ctx, question.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrQuestionNotFound
			}
			return fmt.Errorf("lock question: %w", err)
		}
		if locked.Status == StatusClosed {
			return ErrQuestionClosed
		}

		if err := q.UpdateQuestionStatus(ctx, qadb.UpdateQuestionStatusParams{Status: StatusClosed, ID: question.ID}); err != nil {
			return fmt.Errorf("update question status: %w", err)
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(workspaceID, actor,
			activityservice.ActionQuestionClosed, activityservice.TargetQuestion,
			uuidString(question.ID), question.Subject, nil))
	})
}

func (s *QAService) ReopenQuestion(ctx context.Context, workspaceID, questionID string, actor Actor) error {
	if !actor.managesRoom() {
		return ErrReopenNotAllowed
	}

	question, _, err := s.loadVisibleQuestion(ctx, workspaceID, questionID, actor)
	if err != nil {
		return err
	}

	return s.repo.ExecTxTx(ctx, func(q *qadb.Queries, tx pgx.Tx) error {
		locked, err := q.GetQuestionLocked(ctx, question.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrQuestionNotFound
			}
			return fmt.Errorf("lock question: %w", err)
		}
		if locked.Status != StatusClosed {
			return ErrQuestionNotClosed
		}

		if err := q.UpdateQuestionStatus(ctx, qadb.UpdateQuestionStatusParams{Status: StatusWaiting, ID: question.ID}); err != nil {
			return fmt.Errorf("update question status: %w", err)
		}

		return s.activity.RecordTx(ctx, tx, s.activityEntry(workspaceID, actor,
			activityservice.ActionQuestionReopened, activityservice.TargetQuestion,
			uuidString(question.ID), question.Subject, nil))
	})
}

func (s *QAService) loadVisibleQuestion(ctx context.Context, workspaceID, questionID string, actor Actor) (qadb.Question, qadb.GetMemberGroupQARow, error) {
	var grp qadb.GetMemberGroupQARow

	var wID, qID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return qadb.Question{}, grp, ErrQuestionNotFound
	}
	if err := qID.Scan(questionID); err != nil {
		return qadb.Question{}, grp, ErrQuestionNotFound
	}

	question, err := s.repo.GetQuestion(ctx, qadb.GetQuestionParams{ID: qID, WorkspaceID: wID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return qadb.Question{}, grp, ErrQuestionNotFound
		}
		return qadb.Question{}, grp, fmt.Errorf("get question: %w", err)
	}

	if !actor.managesRoom() {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return qadb.Question{}, grp, fmt.Errorf("parse user id: %w", err)
		}
		grp, err = s.repo.GetMemberGroupQA(ctx, qadb.GetMemberGroupQAParams{WorkspaceID: wID, UserID: uID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return qadb.Question{}, grp, ErrQuestionNotFound
			}
			return qadb.Question{}, grp, fmt.Errorf("get member group: %w", err)
		}
		if !questionVisibleTo(question, actor, grp.ID) {
			return qadb.Question{}, grp, ErrQuestionNotFound
		}
	}

	return question, grp, nil
}

func (s *QAService) attachRefs(ctx context.Context, res *dto.QuestionThreadResponse, question qadb.Question, actor Actor) error {
	if question.DocumentID.Valid {
		doc, err := s.repo.GetDocumentForRef(ctx, question.DocumentID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("resolve document ref: %w", err)
		default:
			show := actor.managesRoom()
			if !show {
				ok, err := s.content.CanUserViewFolder(ctx, uuidString(question.WorkspaceID), uuidString(doc.FolderID), actor.UserID)
				if err != nil {
					return fmt.Errorf("check document ref access: %w", err)
				}
				show = ok
			}
			if show {
				res.DocumentRef = &dto.ReferenceChip{
					ID:       uuidString(doc.ID),
					FolderID: uuidString(doc.FolderID),
					Name:     doc.Name,
				}
			}
		}
	}

	if question.FolderID.Valid {
		folder, err := s.repo.GetFolderForRef(ctx, question.FolderID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return fmt.Errorf("resolve folder ref: %w", err)
		default:
			show := actor.managesRoom()
			if !show {
				ok, err := s.content.CanUserViewFolder(ctx, uuidString(question.WorkspaceID), uuidString(folder.ID), actor.UserID)
				if err != nil {
					return fmt.Errorf("check folder ref access: %w", err)
				}
				show = ok
			}
			if show {
				res.FolderRef = &dto.ReferenceChip{
					ID:   uuidString(folder.ID),
					Name: folder.Name,
				}
			}
		}
	}

	return nil
}

func threadResponse(question qadb.Question, replies []qadb.QuestionReply) dto.QuestionThreadResponse {
	res := dto.QuestionThreadResponse{
		ID:         uuidString(question.ID),
		Number:     question.Number,
		Subject:    question.Subject,
		Body:       question.Body,
		Status:     question.Status,
		GroupID:    uuidString(question.GroupID),
		GroupName:  question.GroupName,
		AuthorID:   uuidString(question.AuthorID),
		AuthorName: question.AuthorName,
		CreatedAt:  question.CreatedAt.Time,
		Replies:    []dto.ReplyResponse{},
	}
	for _, reply := range replies {
		res.Replies = append(res.Replies, replyResponse(reply))
	}
	return res
}

func replyResponse(reply qadb.QuestionReply) dto.ReplyResponse {
	return dto.ReplyResponse{
		ID:         uuidString(reply.ID),
		AuthorID:   uuidString(reply.AuthorID),
		AuthorName: reply.AuthorName,
		AuthorRole: reply.AuthorRole,
		Body:       reply.Body,
		CreatedAt:  reply.CreatedAt.Time,
	}
}
