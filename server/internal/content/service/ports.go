package service

import (
	"context"
	"io"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5"
)

type ContentRepository interface {
	contentdb.Querier
	ExecTx(ctx context.Context, fn func(*contentdb.Queries) error) error
	ExecTxTx(ctx context.Context, fn func(*contentdb.Queries, pgx.Tx) error) error
}

// ActivityExporter dan QAExporter dideklarasikan sebagai port, bukan impor
// langsung, karena qa/service sudah bergantung pada ContentService lewat
// ContentAccessChecker — impor balik akan jadi siklus.
type ActivityExporter interface {
	ExportActivityCSV(ctx context.Context, w io.Writer, workspaceID, role string) error
}

type QAExporter interface {
	ExportQuestionsCSV(ctx context.Context, w io.Writer, workspaceID, userID, name, email, role string) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
	RecordPageEvent(ctx context.Context, ev activityservice.PageEvent)
}
