package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/findardi/rakda/server/internal/activity/dto"
	activitydb "github.com/findardi/rakda/server/internal/activity/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxPageDurationMs = 3_600_000

var ErrDocumentNotFound = errors.New("document not found")

type engagementScope struct {
	doc         activitydb.GetDocumentForEventRow
	workspaceID pgtype.UUID
	documentID  pgtype.UUID
}

// pageCount is 0 until the served version has a rendition; the web tier then
// draws its page axis from the recorded events instead.
func (s engagementScope) pageCount() int32 {
	if s.doc.PageCount == nil {
		return 0
	}
	return *s.doc.PageCount
}

func isRoomManager(role string) bool {
	return role == permission.RoleOwner || role == permission.RoleAdmin
}

func (s *ActivityService) RecordPageDurations(ctx context.Context, req dto.RecordDurationsRequest, actorID, actorEmail, actorRole string) error {
	if isRoomManager(actorRole) {
		return nil
	}

	docID, err := pgUUID(req.DocumentID)
	if err != nil {
		return ErrDocumentNotFound
	}

	doc, err := s.repo.GetDocumentForEvent(ctx, docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDocumentNotFound
	}

	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}

	if doc.WorkspaceID.String() != req.WorkspaceID {
		return ErrDocumentNotFound
	}

	workspaceID, err := pgUUID(req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("workspace id: %w", err)
	}

	versionID, err := pgUUID(req.VersionID)
	if err != nil {
		return ErrInvalidFilter
	}

	aID, err := pgUUID(actorID)
	if err != nil {
		return fmt.Errorf("actor id: %w", err)
	}

	for _, d := range req.Durations {
		durationMs := int32(min(d.DurationMs, maxPageDurationMs))
		pageNo := d.PageNo

		if err := s.repo.InsertContentEvent(ctx, activitydb.InsertContentEventParams{
			WorkspaceID:  workspaceID,
			DocumentID:   docID,
			DocumentName: doc.Name,
			VersionID:    versionID,
			PageNo:       &pageNo,
			EventType:    EventPageDuration,
			DurationMs:   &durationMs,
			ActorID:      aID,
			ActorEmail:   actorEmail,
		}); err != nil {
			return fmt.Errorf("insert page durations: %w", err)
		}
	}

	return nil
}

func (s *ActivityService) resolveEngagementScope(ctx context.Context, workspaceID, documentID, actorRole string) (engagementScope, error) {
	if !isRoomManager(actorRole) {
		return engagementScope{}, ErrActivityForbidden
	}

	docID, err := pgUUID(documentID)
	if err != nil {
		return engagementScope{}, ErrDocumentNotFound
	}

	doc, err := s.repo.GetDocumentForEvent(ctx, docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return engagementScope{}, ErrDocumentNotFound
	}

	if err != nil {
		return engagementScope{}, fmt.Errorf("get document: %w", err)
	}

	if doc.WorkspaceID.String() != workspaceID {
		return engagementScope{}, ErrDocumentNotFound
	}

	wID, err := pgUUID(workspaceID)
	if err != nil {
		return engagementScope{}, fmt.Errorf("workspace id: %w", err)
	}

	return engagementScope{doc: doc, workspaceID: wID, documentID: docID}, nil
}

func (s *ActivityService) GetDocumentReaders(ctx context.Context, workspaceID, documentID, actorRole string) (dto.DocumentReadersResponse, error) {
	scope, err := s.resolveEngagementScope(ctx, workspaceID, documentID, actorRole)
	if err != nil {
		return dto.DocumentReadersResponse{}, err
	}

	rows, err := s.repo.ListDocumentReaders(ctx, activitydb.ListDocumentReadersParams{
		WorkspaceID: scope.workspaceID,
		DocumentID:  scope.documentID,
	})
	if err != nil {
		return dto.DocumentReadersResponse{}, fmt.Errorf("list readers: %w", err)
	}

	res := dto.DocumentReadersResponse{
		DocumentID:   documentID,
		DocumentName: scope.doc.Name,
		PageCount:    scope.pageCount(),
		Readers:      make([]dto.ReaderEngagement, 0, len(rows)),
	}

	for _, r := range rows {
		res.Readers = append(res.Readers, dto.ReaderEngagement{
			ActorID:    r.ActorID.String(),
			ActorName:  r.ActorName,
			ActorEmail: r.ActorEmail,
			GroupID:    r.GroupID.String(),
			GroupName:  r.GroupName,
			Opens:      r.Opens,
			PagesSeen:  r.PagesSeen,
			ReadMs:     r.ReadMs,
			LastReadAt: r.LastReadAt.Time,
		})

		res.TotalReadMs += r.ReadMs
	}

	return res, nil
}

func (s *ActivityService) GetReaderPages(ctx context.Context, workspaceID, documentID, readerID, actorRole string) (dto.ReaderDetailResponse, error) {
	scope, err := s.resolveEngagementScope(ctx, workspaceID, documentID, actorRole)
	if err != nil {
		return dto.ReaderDetailResponse{}, err
	}

	actorID, err := pgUUID(readerID)
	if err != nil || !actorID.Valid {
		return dto.ReaderDetailResponse{}, ErrInvalidFilter
	}

	rows, err := s.repo.ListReaderPages(ctx, activitydb.ListReaderPagesParams{
		WorkspaceID: scope.workspaceID,
		DocumentID:  scope.documentID,
		ActorID:     actorID,
	})
	if err != nil {
		return dto.ReaderDetailResponse{}, fmt.Errorf("list reader pages: %w", err)
	}

	res := dto.ReaderDetailResponse{
		DocumentID:   documentID,
		DocumentName: scope.doc.Name,
		PageCount:    scope.pageCount(),
		ActorID:      readerID,
		Pages:        make([]dto.ReaderPageEngagement, 0, len(rows)),
	}

	for _, r := range rows {
		var pageNo int32
		if r.PageNo != nil {
			pageNo = *r.PageNo
		}

		res.Pages = append(res.Pages, dto.ReaderPageEngagement{
			PageNo: pageNo,
			Opens:  r.Opens,
			ReadMs: r.ReadMs,
		})

		res.TotalReadMs += r.ReadMs
	}

	return res, nil
}

func (s *ActivityService) GetEngagementBreakdown(ctx context.Context, workspaceID, documentID, actorRole string) (dto.EngagementBreakdown, error) {
	scope, err := s.resolveEngagementScope(ctx, workspaceID, documentID, actorRole)
	if err != nil {
		return dto.EngagementBreakdown{}, err
	}

	rows, err := s.repo.ListEngagementBreakdown(ctx, activitydb.ListEngagementBreakdownParams{
		WorkspaceID: scope.workspaceID,
		DocumentID:  scope.documentID,
	})
	if err != nil {
		return dto.EngagementBreakdown{}, fmt.Errorf("list engagement breakdown: %w", err)
	}

	out := dto.EngagementBreakdown{
		DocumentName: scope.doc.Name,
		Rows:         make([]dto.EngagementBreakdownRow, 0, len(rows)),
	}

	for _, r := range rows {
		var pageNo int32
		if r.PageNo != nil {
			pageNo = *r.PageNo
		}

		out.Rows = append(out.Rows, dto.EngagementBreakdownRow{
			ActorID:    r.ActorID.String(),
			ActorName:  r.ActorName,
			ActorEmail: r.ActorEmail,
			GroupName:  r.GroupName,
			PageNo:     pageNo,
			Opens:      r.Opens,
			ReadMs:     r.ReadMs,
		})
	}

	return out, nil
}
