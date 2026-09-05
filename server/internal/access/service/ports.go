package service

import (
	"context"

	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	authdto "github.com/findardi/rakda/server/internal/auth/dto"
	"github.com/jackc/pgx/v5"
)

type AccessRepository interface {
	accessdb.Querier
	ExecTxTx(ctx context.Context, fn func(*accessdb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
}

type MailService interface {
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

type Tokenizer interface {
	Generate() string
	Hash(code string) string
}

type AuthService interface {
	UserExists(ctx context.Context, email string) (authdto.UserResponse, error)
}
