package service

import (
	"context"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
)

type QARepository interface {
	qadb.Querier
	ExecTxTx(ctx context.Context, fn func(*qadb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
}

type ContentAccessChecker interface {
	CanUserViewFolder(ctx context.Context, workspaceID, folderID, userID string) (bool, error)
}
