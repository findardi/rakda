package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/findardi/Riksa-App/server/internal/activity/dto"
	activitydb "github.com/findardi/Riksa-App/server/internal/activity/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/permission"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	defaultActivityPageSize = 50
	maxActivityPageSize     = 100
)

var (
	ErrActivityForbidden = errors.New("no access to activity log")
	ErrInvalidCursor     = errors.New("invalid cursor")
	ErrInvalidFilter     = errors.New("invalid filter")
)

func (s *ActivityService) ListActivity(ctx context.Context, req dto.ListActivityRequest, actorRole string) (dto.ListActivityResponse, error) {
	if actorRole != permission.RoleOwner && actorRole != permission.RoleAdmin {
		return dto.ListActivityResponse{}, ErrActivityForbidden
	}

	workspaceID, err := pgUUID(req.WorkspaceID)
	if err != nil {
		return dto.ListActivityResponse{}, fmt.Errorf("workspace id: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultActivityPageSize
	}
	if limit > maxActivityPageSize {
		limit = maxActivityPageSize
	}

	params := activitydb.ListActivityLogsParams{
		WorkspaceID: workspaceID,
		PageSize:    int32(limit),
	}

	if req.Cursor != "" {
		createdAt, id, err := parseActivityCursor(req.Cursor)
		if err != nil {
			return dto.ListActivityResponse{}, ErrInvalidCursor
		}
		params.CursorCreatedAt = createdAt
		params.CursorID = id
	}

	if req.From != "" {
		t, err := parseTimeFilter(req.From, false)
		if err != nil {
			return dto.ListActivityResponse{}, ErrInvalidFilter
		}
		params.FromTime = t
	}

	if req.To != "" {
		t, err := parseTimeFilter(req.To, true)
		if err != nil {
			return dto.ListActivityResponse{}, ErrInvalidFilter
		}
		params.ToTime = t
	}

	if req.ActorID != "" {
		id, err := pgUUID(req.ActorID)
		if err != nil {
			return dto.ListActivityResponse{}, ErrInvalidFilter
		}
		params.ActorID = id
	}

	if req.Action != "" {
		params.Action = &req.Action
	}

	rows, err := s.repo.ListActivityLogs(ctx, params)
	if err != nil {
		return dto.ListActivityResponse{}, fmt.Errorf("list activity: %w", err)
	}

	items := make([]dto.ActivityLogResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.ActivityLogResponse{
			ID:         uuidString(r.ID),
			ActorID:    uuidString(r.ActorID),
			ActorName:  r.ActorName,
			ActorRole:  r.ActorRole,
			Action:     r.Action,
			TargetType: r.TargetType,
			TargetID:   uuidString(r.TargetID),
			TargetName: r.TargetName,
			Metadata:   json.RawMessage(r.Metadata),
			CreatedAt:  r.CreatedAt.Time,
		})
	}

	next := ""
	if len(rows) == limit {
		last := rows[len(rows)-1]
		next = activityCursor(last.CreatedAt.Time, uuidString(last.ID))
	}

	return dto.ListActivityResponse{Items: items, NextCursor: next}, nil
}

func activityCursor(createdAt time.Time, id string) string {
	return fmt.Sprintf("%d_%s", createdAt.UnixMicro(), id)
}

func parseActivityCursor(cursor string) (pgtype.Timestamptz, pgtype.UUID, error) {
	var createdAt pgtype.Timestamptz
	var id pgtype.UUID

	micros, rawID, found := strings.Cut(cursor, "_")
	if !found {
		return createdAt, id, ErrInvalidCursor
	}

	v, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return createdAt, id, ErrInvalidCursor
	}

	if err := id.Scan(rawID); err != nil {
		return createdAt, id, ErrInvalidCursor
	}

	createdAt = pgtype.Timestamptz{Time: time.UnixMicro(v), Valid: true}
	return createdAt, id, nil
}

// parseTimeFilter accepts RFC3339 or a bare date; a bare date used as the upper
// bound covers that whole day.
func parseTimeFilter(raw string, endOfDay bool) (pgtype.Timestamptz, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return pgtype.Timestamptz{Time: t, Valid: true}, nil
	}

	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return pgtype.Timestamptz{}, ErrInvalidFilter
	}

	if endOfDay {
		t = t.Add(24*time.Hour - time.Microsecond)
	}

	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
