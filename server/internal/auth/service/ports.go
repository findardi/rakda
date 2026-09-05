package service

import (
	"context"

	authdb "github.com/findardi/rakda/server/internal/auth/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/token"
	"github.com/jackc/pgx/v5"
)

type UserRepository interface {
	authdb.Querier
	ExecTx(ctx context.Context, fn func(q *authdb.Queries) error) error
	ExecTxTx(ctx context.Context, fn func(*authdb.Queries, pgx.Tx) error) error
}

type OTPService interface {
	Generate() string
	Hash(code string) string
	Compare(hash, code string) bool
	GenerateRefreshToken() (string, error)
}

type JWTService interface {
	CreateToken(claims token.JwtClaims, tokenType token.TokenType) (string, error)
	VerifyToken(tokenString string) (*token.JwtClaims, error)
}

type MailService interface {
	Send(ctx context.Context, to, subject, textBody, htmlBody string) error
}

type InvitePreview struct {
	Email         string `json:"email"`
	WorkspaceName string `json:"workspace_name"`
	RoleName      string `json:"role_name"`
}

type InvitationConsumer interface {
	PreviewInvitation(ctx context.Context, token string) (InvitePreview, error)
	ConsumeInvitation(ctx context.Context, tx pgx.Tx, token, newUserID string) error
}
