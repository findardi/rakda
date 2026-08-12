package service

import (
	"context"

	activitydb "github.com/findardi/Riksa-App/server/internal/activity/repository/sqlc"
)

type ActivityRepository interface {
	InsertActivityLog(ctx context.Context, arg activitydb.InsertActivityLogParams) error
	InsertContentEvent(ctx context.Context, arg activitydb.InsertContentEventParams) error
	ListActivityLogs(ctx context.Context, arg activitydb.ListActivityLogsParams) ([]activitydb.ActivityLog, error)
}
