package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/findardi/rakda/server/internal/auth/dto"
	authdb "github.com/findardi/rakda/server/internal/auth/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/token"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailUnique            = errors.New("email already registered")
	ErrUsernameUnique         = errors.New("username already registered")
	ErrInvalidCredentials     = errors.New("invalid email/username or password")
	ErrInvalidCodeOTP         = errors.New("invalid or expired OTP code")
	ErrEmailAlreadyVerified   = errors.New("email already verified")
	ErrInvalidRefreshToken    = errors.New("invalid or expired refresh token")
	ErrEmailAlreadyRegistered = errors.New("email already registered, please login with password")
	ErrSSOEmailMissing        = errors.New("no verified email available from provider")
	ErrRefreshReuseDetected   = errors.New("refresh token reuse detected")
	ErrInvitationInvalid      = errors.New("invitation link is invalid or has expired")
	ErrInviteEmailRegistered  = errors.New("email already registered, please login to accept")
)

// tokenTTL maps a user_tokens.type to its lifetime.
var tokenTTL = map[string]time.Duration{
	"email_verification": 5 * time.Minute,
	"password_reset":     5 * time.Minute,
	"refresh":            24 * time.Hour,
}

type AuthService struct {
	repo   UserRepository
	otp    OTPService
	jwt    JWTService
	mail   MailService
	invite InvitationConsumer
}

func NewAuthService(repo UserRepository, otp OTPService, jwt JWTService, mail MailService, invite InvitationConsumer) *AuthService {
	return &AuthService{
		repo:   repo,
		otp:    otp,
		jwt:    jwt,
		mail:   mail,
		invite: invite,
	}
}

func (s *AuthService) UserExists(ctx context.Context, email string) (dto.UserResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.UserResponse{}, nil
	}
	if err != nil {
		return dto.UserResponse{}, err
	}

	return dto.UserResponse{
		ID:            uuidString(user.ID),
		Email:         user.Email,
		Username:      deref(user.Username),
		Status:        user.Status,
		EmailVerified: user.EmailVerifiedAt.Valid,
	}, nil
}

func (s *AuthService) RegisterUser(ctx context.Context, req dto.RegisterRequest) (dto.RegisterResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	username := strings.TrimSpace(req.Username)

	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return dto.RegisterResponse{}, ErrEmailUnique
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dto.RegisterResponse{}, fmt.Errorf("check email: %w", err)
	}

	if _, err := s.repo.GetUserByUsername(ctx, &username); err == nil {
		return dto.RegisterResponse{}, ErrUsernameUnique
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dto.RegisterResponse{}, fmt.Errorf("check username: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return dto.RegisterResponse{}, fmt.Errorf("hash password: %w", err)
	}

	hashPassword := string(hash)
	code := s.otp.Generate()

	// Transaction process
	var user authdb.User
	err = s.repo.ExecTx(ctx, func(q *authdb.Queries) error {
		u, err := q.CreateUser(ctx, authdb.CreateUserParams{
			Email:        email,
			Username:     &username,
			PasswordHash: &hashPassword,
			Status:       "pending",
		})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		if _, err := q.CreateUserToken(ctx, authdb.CreateUserTokenParams{
			UserID:   u.ID,
			Type:     "email_verification",
			CodeHash: s.otp.Hash(code),
			ExpiresAt: pgtype.Timestamptz{
				Time:  time.Now().Add(tokenTTL["email_verification"]),
				Valid: true,
			},
		}); err != nil {
			return fmt.Errorf("create otp: %w", err)
		}

		user = u
		return nil
	})
	if err != nil {
		return dto.RegisterResponse{}, fmt.Errorf("register tx: %w", err)
	}

	s.sendEmailAsync(email, "Verify your email",
		fmt.Sprintf("Your verification code is: %s (valid for 5 minutes)", code))

	return dto.RegisterResponse{
		ID:       uuidString(user.ID),
		Username: deref(user.Username),
	}, nil
}

// sendEmailAsync fires the email in the background; request ctx would be cancelled, so use Background.
func (s *AuthService) sendEmailAsync(to, subject, body string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, to, subject, body); err != nil {
			log.Printf("send email to %s failed: %v", to, err)
		}
	}()
}

func (s *AuthService) LoginUser(ctx context.Context, req dto.LoginRequest) (dto.LoginResponse, error) {
	var user authdb.User
	var err error

	switch {
	case req.Email != "":
		email := strings.ToLower(strings.TrimSpace(req.Email))
		user, err = s.repo.GetUserByEmail(ctx, email)
	case req.Username != "":
		username := strings.TrimSpace(req.Username)
		user, err = s.repo.GetUserByUsername(ctx, &username)
	default:
		return dto.LoginResponse{}, ErrInvalidCredentials
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return dto.LoginResponse{}, ErrInvalidCredentials
	}
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("get user: %w", err)
	}

	// User SSO without password
	if user.PasswordHash == nil {
		return dto.LoginResponse{}, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		return dto.LoginResponse{}, ErrInvalidCredentials
	}

	return s.issueToken(ctx, user)
}

func (s *AuthService) ResendOTP(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if user.Status != "pending" {
		return nil
	}

	code := s.otp.Generate()
	if _, err := s.repo.UpdateUserToken(ctx, authdb.UpdateUserTokenParams{
		UserID:   user.ID,
		Type:     "email_verification",
		CodeHash: s.otp.Hash(code),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(tokenTTL["email_verification"]),
			Valid: true,
		},
	}); err != nil {
		return fmt.Errorf("update otp: %w", err)
	}

	s.sendEmailAsync(email, "Verify your email",
		fmt.Sprintf("Your verification code is: %s (valid for 5 minutes)", code))

	return nil
}

func (s *AuthService) VerifyEmail(ctx context.Context, req dto.VerifyOtpRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCodeOTP
	}
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if user.Status != "pending" {
		return ErrEmailAlreadyVerified
	}

	otp, err := s.repo.GetValidUserToken(ctx, authdb.GetValidUserTokenParams{
		UserID: user.ID,
		Type:   "email_verification",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCodeOTP
	}
	if err != nil {
		return fmt.Errorf("get otp: %w", err)
	}

	if !s.otp.Compare(otp.CodeHash, req.Code) {
		return ErrInvalidCodeOTP
	}

	err = s.repo.ExecTx(ctx, func(q *authdb.Queries) error {
		if _, err := q.UpdateStatus(ctx, authdb.UpdateStatusParams{
			ID:     user.ID,
			Status: "active",
		}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		if err := q.DeleteUserToken(ctx, authdb.DeleteUserTokenParams{
			CodeHash: otp.CodeHash,
			UserID:   user.ID,
			Type:     "email_verification",
		}); err != nil {
			return fmt.Errorf("delete token: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("verify tx: %w", err)
	}

	return nil
}

func (s *AuthService) LogoutUser(ctx context.Context, req dto.LogoutRequest) error {
	var uid pgtype.UUID
	if err := uid.Scan(req.UserID); err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	if err := s.repo.DeleteUserToken(ctx, authdb.DeleteUserTokenParams{
		CodeHash: s.otp.Hash(req.RefreshToken),
		UserID:   uid,
		Type:     "refresh",
	}); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}

	return nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req dto.RefreshTokenRequest) (dto.LoginResponse, error) {
	old, err := s.repo.GetRefreshToken(ctx, s.otp.Hash(req.RefreshToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.LoginResponse{}, ErrInvalidRefreshToken
	}
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("get refresh token: %w", err)
	}

	// reuse detection: a consumed token replayed → revoke all refresh tokens (per-user)
	if old.UsedAt.Valid {
		if err := s.repo.DeleteTokensByType(ctx, authdb.DeleteTokensByTypeParams{
			UserID: old.UserID,
			Type:   "refresh",
		}); err != nil {
			return dto.LoginResponse{}, fmt.Errorf("revoke refresh tokens: %w", err)
		}
		return dto.LoginResponse{}, ErrRefreshReuseDetected
	}

	// never used but past expiry → just invalid
	if !old.ExpiresAt.Valid || old.ExpiresAt.Time.Before(time.Now()) {
		return dto.LoginResponse{}, ErrInvalidRefreshToken
	}

	user, err := s.repo.GetUserById(ctx, old.UserID)
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("get user: %w", err)
	}

	accessToken, err := s.jwt.CreateToken(token.JwtClaims{
		ID:       uuidString(user.ID),
		Username: deref(user.Username),
		Email:    user.Email,
		Status:   user.Status,
	}, token.TokenType("token_login"))
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("create access token: %w", err)
	}

	newRefresh, err := s.otp.GenerateRefreshToken()
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("generate refresh token: %w", err)
	}

	err = s.repo.ExecTx(ctx, func(q *authdb.Queries) error {
		// soft-consume: keep the row (marked used) so a replay is detectable until expiry
		if err := q.MarkRefreshTokenUsed(ctx, old.ID); err != nil {
			return fmt.Errorf("mark old refresh used: %w", err)
		}

		if _, err := q.CreateUserToken(ctx, authdb.CreateUserTokenParams{
			UserID:   user.ID,
			Type:     "refresh",
			CodeHash: s.otp.Hash(newRefresh),
			ExpiresAt: pgtype.Timestamptz{
				Time:  time.Now().Add(tokenTTL["refresh"]),
				Valid: true,
			},
		}); err != nil {
			return fmt.Errorf("create new refresh: %w", err)
		}
		return nil
	})
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("rotate refresh tx: %w", err)
	}

	return dto.LoginResponse{
		Token:        accessToken,
		RefreshToken: newRefresh,
	}, nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, req dto.ForgotPasswordRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // anti-enumeration: do not reveal missing email
	}
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	code := s.otp.Generate()
	err = s.repo.ExecTx(ctx, func(q *authdb.Queries) error {
		if err := q.DeleteTokensByType(ctx, authdb.DeleteTokensByTypeParams{
			UserID: user.ID,
			Type:   "password_reset",
		}); err != nil {
			return fmt.Errorf("clear old reset token: %w", err)
		}

		if _, err := q.CreateUserToken(ctx, authdb.CreateUserTokenParams{
			UserID:   user.ID,
			Type:     "password_reset",
			CodeHash: s.otp.Hash(code),
			ExpiresAt: pgtype.Timestamptz{
				Time:  time.Now().Add(tokenTTL["password_reset"]),
				Valid: true,
			},
		}); err != nil {
			return fmt.Errorf("create reset token: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("forgot password tx: %w", err)
	}

	s.sendEmailAsync(email, "Reset Password",
		fmt.Sprintf("Your verification code is: %s (valid for 5 minutes)", code))

	return nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req dto.ResetPasswordRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCodeOTP
	}
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	otp, err := s.repo.GetValidUserToken(ctx, authdb.GetValidUserTokenParams{
		UserID: user.ID,
		Type:   "password_reset",
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCodeOTP
	}
	if err != nil {
		return fmt.Errorf("get reset token: %w", err)
	}

	if !s.otp.Compare(otp.CodeHash, req.Code) {
		return ErrInvalidCodeOTP
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	hashPassword := string(hash)

	err = s.repo.ExecTx(ctx, func(q *authdb.Queries) error {
		if err := q.UpdatePassword(ctx, authdb.UpdatePasswordParams{
			ID:           user.ID,
			PasswordHash: &hashPassword,
		}); err != nil {
			return fmt.Errorf("update password: %w", err)
		}

		if err := q.DeleteTokensByType(ctx, authdb.DeleteTokensByTypeParams{
			UserID: user.ID,
			Type:   "password_reset",
		}); err != nil {
			return fmt.Errorf("delete reset token: %w", err)
		}

		// revoke all sessions on password change
		if err := q.DeleteTokensByType(ctx, authdb.DeleteTokensByTypeParams{
			UserID: user.ID,
			Type:   "refresh",
		}); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reset password tx: %w", err)
	}

	return nil
}

func (s *AuthService) ValidateOTP(ctx context.Context, req dto.ValidateOtpRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	user, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCodeOTP
	}
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if _, err := s.repo.GetTokenByCodeAndUser(ctx, authdb.GetTokenByCodeAndUserParams{
		CodeHash: s.otp.Hash(req.Code),
		UserID:   user.ID,
		Type:     "password_reset",
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCodeOTP
		}
		return fmt.Errorf("get reset token: %w", err)
	}

	return nil
}

func (s *AuthService) CheckEmail(ctx context.Context, req dto.ForgotPasswordRequest) error {
	email := strings.ToLower(strings.TrimSpace(req.Email))

	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return ErrEmailUnique
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("check email: %w", err)
	}

	return nil
}

func (s *AuthService) GetMe(ctx context.Context, userID string) (dto.UserResponse, error) {
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return dto.UserResponse{}, fmt.Errorf("parse user id: %w", err)
	}

	user, err := s.repo.GetUserById(ctx, uid)
	if err != nil {
		return dto.UserResponse{}, fmt.Errorf("get user: %w", err)
	}

	return dto.UserResponse{
		ID:            uuidString(user.ID),
		Email:         user.Email,
		Username:      deref(user.Username),
		Status:        user.Status,
		EmailVerified: user.EmailVerifiedAt.Valid,
	}, nil
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type SSOIdentity struct {
	ProviderUID   string
	Email         string
	EmailVerified bool
	Username      string
}

func (s *AuthService) issueToken(ctx context.Context, user authdb.User) (dto.LoginResponse, error) {
	claims := token.JwtClaims{
		ID:       uuidString(user.ID),
		Username: deref(user.Username),
		Email:    user.Email,
		Status:   user.Status,
	}

	accessToken, err := s.jwt.CreateToken(claims, token.TokenType("token_login"))
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("create access token :%w", err)
	}

	if err := s.repo.DeleteExpiredUserTokens(ctx, user.ID); err != nil {
		return dto.LoginResponse{}, fmt.Errorf("cleanup expired token :%w", err)
	}

	refreshToken, err := s.otp.GenerateRefreshToken()
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("generate refresh token :%w", err)
	}

	if _, err := s.repo.CreateUserToken(ctx, authdb.CreateUserTokenParams{
		UserID:   user.ID,
		Type:     "refresh",
		CodeHash: s.otp.Hash(refreshToken),
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(tokenTTL["refresh"]),
			Valid: true,
		},
	}); err != nil {
		return dto.LoginResponse{}, fmt.Errorf("create refresh token :%w", err)
	}

	return dto.LoginResponse{
		Token:        accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) SSOLogin(ctx context.Context, provider string, identity SSOIdentity) (dto.LoginResponse, error) {
	ident, err := s.repo.GetUserIdentity(ctx, authdb.GetUserIdentityParams{
		Provider:    provider,
		ProviderUid: identity.ProviderUID,
	})

	if err == nil {
		user, err := s.repo.GetUserById(ctx, ident.UserID)
		if err != nil {
			return dto.LoginResponse{}, fmt.Errorf("get user :%w", err)
		}
		return s.issueToken(ctx, user)
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return dto.LoginResponse{}, fmt.Errorf("get identity :%w", err)
	}

	if identity.Email == "" || !identity.EmailVerified {
		return dto.LoginResponse{}, ErrSSOEmailMissing
	}
	email := strings.ToLower(strings.TrimSpace(identity.Email))

	_, err = s.repo.GetUserByEmail(ctx, email)
	if err == nil {
		return dto.LoginResponse{}, ErrEmailAlreadyRegistered
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return dto.LoginResponse{}, fmt.Errorf("get user by email :%w", err)
	}

	var user authdb.User
	if err := s.repo.ExecTx(ctx, func(q *authdb.Queries) error {
		u, err := q.CreateUser(ctx, authdb.CreateUserParams{
			Email:        email,
			Username:     nil,
			PasswordHash: nil,
			Status:       "active",
		})

		if err != nil {
			return fmt.Errorf("create user :%w", err)
		}

		if _, err := q.CreateUserIdentity(ctx, authdb.CreateUserIdentityParams{
			UserID:      u.ID,
			Provider:    provider,
			ProviderUid: identity.ProviderUID,
			Email:       &email,
		}); err != nil {
			return fmt.Errorf("create identity :%w", err)
		}

		user = u
		return nil
	}); err != nil {
		return dto.LoginResponse{}, err
	}

	return s.issueToken(ctx, user)

}

func (s *AuthService) PreviewInvitationSignup(ctx context.Context, token string) (InvitePreview, error) {
	return s.invite.PreviewInvitation(ctx, token)
}

func (s *AuthService) AcceptInvitationSignup(ctx context.Context, req dto.AcceptInvitationRequest) (dto.LoginResponse, error) {
	preview, err := s.invite.PreviewInvitation(ctx, req.Token)
	if err != nil {
		return dto.LoginResponse{}, err
	}

	email := strings.ToLower(strings.TrimSpace(preview.Email))
	username := strings.TrimSpace(req.Username)

	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return dto.LoginResponse{}, ErrInviteEmailRegistered
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dto.LoginResponse{}, fmt.Errorf("check email: %w", err)
	}

	if _, err := s.repo.GetUserByUsername(ctx, &username); err == nil {
		return dto.LoginResponse{}, ErrUsernameUnique
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dto.LoginResponse{}, fmt.Errorf("check username: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return dto.LoginResponse{}, fmt.Errorf("hash password: %w", err)
	}
	hashpassword := string(hash)

	var user authdb.User
	err = s.repo.ExecTxTx(ctx, func(q *authdb.Queries, tx pgx.Tx) error {
		u, err := q.CreateUserVerified(ctx, authdb.CreateUserVerifiedParams{
			Email:        email,
			Username:     &username,
			PasswordHash: &hashpassword,
		})

		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		if err := s.invite.ConsumeInvitation(ctx, tx, req.Token, uuidString(u.ID)); err != nil {
			return err
		}

		user = u
		return nil
	})

	if err != nil {
		return dto.LoginResponse{}, err
	}

	return s.issueToken(ctx, user)
}
