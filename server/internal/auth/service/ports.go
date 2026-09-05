package service

import (
	"context"

	authdb "github.com/findardi/rakda/server/internal/auth/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/token"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type UserRepository interface {
	GetUserByEmail(ctx context.Context, email string) (authdb.User, error)
	GetUserByUsername(ctx context.Context, username *string) (authdb.User, error)
	GetUserById(ctx context.Context, id pgtype.UUID) (authdb.User, error)
	GetValidUserToken(ctx context.Context, arg authdb.GetValidUserTokenParams) (authdb.UserToken, error)
	GetRefreshToken(ctx context.Context, codeHash string) (authdb.UserToken, error)
	GetTokenByCodeAndUser(ctx context.Context, arg authdb.GetTokenByCodeAndUserParams) (authdb.UserToken, error)
	GetUserIdentity(ctx context.Context, arg authdb.GetUserIdentityParams) (authdb.UserIdentity, error)

	CreateUser(ctx context.Context, arg authdb.CreateUserParams) (authdb.User, error)
	CreateUserToken(ctx context.Context, arg authdb.CreateUserTokenParams) (authdb.UserToken, error)
	CreateUserIdentity(ctx context.Context, arg authdb.CreateUserIdentityParams) (authdb.UserIdentity, error)
	CreateUserVerified(ctx context.Context, arg authdb.CreateUserVerifiedParams) (authdb.User, error)

	UpdateStatus(ctx context.Context, arg authdb.UpdateStatusParams) (authdb.User, error)
	UpdateUserToken(ctx context.Context, arg authdb.UpdateUserTokenParams) (authdb.UserToken, error)

	DeleteUserToken(ctx context.Context, arg authdb.DeleteUserTokenParams) error
	DeleteTokensByType(ctx context.Context, arg authdb.DeleteTokensByTypeParams) error
	MarkRefreshTokenUsed(ctx context.Context, id pgtype.UUID) error
	DeleteExpiredUserTokens(ctx context.Context, userID pgtype.UUID) error

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
