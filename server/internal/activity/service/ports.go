package service

import (
	"context"

	activitydb "github.com/findardi/Riksa-App/server/internal/activity/repository/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

type ActivityRepository interface {
	InsertActivityLog(ctx context.Context, arg activitydb.InsertActivityLogParams) error
	InsertContentEvent(ctx context.Context, arg activitydb.InsertContentEventParams) error
	ListActivityLogs(ctx context.Context, arg activitydb.ListActivityLogsParams) ([]activitydb.ListActivityLogsRow, error)

	ListDocumentReaders(ctx context.Context, arg activitydb.ListDocumentReadersParams) ([]activitydb.ListDocumentReadersRow, error)
	ListReaderPages(ctx context.Context, arg activitydb.ListReaderPagesParams) ([]activitydb.ListReaderPagesRow, error)
	ListEngagementBreakdown(ctx context.Context, arg activitydb.ListEngagementBreakdownParams) ([]activitydb.ListEngagementBreakdownRow, error)
	GetDocumentForEvent(ctx context.Context, id pgtype.UUID) (activitydb.GetDocumentForEventRow, error)
}
