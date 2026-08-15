package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/findardi/Riksa-App/server/internal/activity/dto"
	activitydb "github.com/findardi/Riksa-App/server/internal/activity/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/permission"
	"github.com/jackc/pgx/v5"
)

const maxPageDurationMs = 3_600_000

var ErrDocumentNotFound = errors.New("document not found")

func (s *ActivityService) RecordPageDurations(ctx context.Context, req dto.RecordDurationsRequest, actorID, actorEmail string) error {
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

	if uuidString(doc.WorkspaceID) != req.WorkspaceID {
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

func (s *ActivityService) GetDocumentEngagement(ctx context.Context, workspaceID, documentID, actorRole string) (dto.DocumentEngagementResponse, error) {
	if actorRole != permission.RoleOwner && actorRole != permission.RoleAdmin {
		return dto.DocumentEngagementResponse{}, ErrActivityForbidden
	}

	docID, err := pgUUID(documentID)
	if err != nil {
		return dto.DocumentEngagementResponse{}, ErrDocumentNotFound
	}

	doc, err := s.repo.GetDocumentForEvent(ctx, docID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.DocumentEngagementResponse{}, ErrDocumentNotFound
	}

	if err != nil {
		return dto.DocumentEngagementResponse{}, fmt.Errorf("get document: %w", err)
	}

	if uuidString(doc.WorkspaceID) != workspaceID {
		return dto.DocumentEngagementResponse{}, ErrDocumentNotFound
	}

	wID, err := pgUUID(workspaceID)
	if err != nil {
		return dto.DocumentEngagementResponse{}, fmt.Errorf("workspace id: %w", err)
	}

	rows, err := s.repo.GetDocumentEngagement(ctx, activitydb.GetDocumentEngagementParams{
		WorkspaceID: wID,
		DocumentID:  docID,
	})
	if err != nil {
		return dto.DocumentEngagementResponse{}, fmt.Errorf("get engagement: %w", err)
	}

	res := dto.DocumentEngagementResponse{
		DocumentID:   documentID,
		DocumentName: doc.Name,
		Pages:        make([]dto.PageEngagement, 0, len(rows)),
	}

	for _, r := range rows {
		var pageNo int32
		if r.PageNo != nil {
			pageNo = *r.PageNo
		}

		res.Pages = append(res.Pages, dto.PageEngagement{
			PageNo:        pageNo,
			Opens:         r.Opens,
			RawHits:       r.RawHits,
			UniqueViewers: r.UniqueViewers,
			ReadMs:        r.ReadMs,
		})

		res.TotalOpens += r.Opens
		res.TotalReadMs += r.ReadMs
	}

	return res, nil
}
