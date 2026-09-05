package service

import (
	"context"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	invitationdb "github.com/findardi/rakda/server/internal/invitation/repository/sqlc"
	"github.com/jackc/pgx/v5"
)

type InvitationRepo interface {
	invitationdb.Querier
	ExecTxTx(ctx context.Context, fn func(*invitationdb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
}
