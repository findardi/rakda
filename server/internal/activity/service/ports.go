package service

import (
	"context"

	activitydb "github.com/findardi/Riksa-App/server/internal/activity/repository/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

type ActivityRepository interface {
	InsertActivityLog(ctx context.Context, arg activitydb.InsertActivityLogParams) error
	InsertContentEvent(ctx context.Context, arg activitydb.InsertContentEventParams) error
	ListActivityLogs(ctx context.Context, arg activitydb.ListActivityLogsParams) ([]activitydb.ActivityLog, error)

	GetDocumentEngagement(ctx context.Context, arg activitydb.GetDocumentEngagementParams) ([]activitydb.GetDocumentEngagementRow, error)
	GetDocumentForEvent(ctx context.Context, id pgtype.UUID) (activitydb.GetDocumentForEventRow, error)
}
