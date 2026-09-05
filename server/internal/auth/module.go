package auth

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"time"

	"github.com/findardi/rakda/server/internal/auth/handler"
	"github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/auth/service"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/oauth"
	"github.com/go-chi/chi/v5"
)

type Module struct {
	handler *handler.AuthHandler
	mw      *middleware.Middleware
}

func NewModule(pool *pgxpool.Pool, otp service.OTPService, jwt service.JWTService, mail service.MailService, limiter middleware.RateStore, providers map[string]oauth.Provider, invite service.InvitationConsumer) *Module {
	r := repository.New(pool)
	s := service.NewAuthService(r, otp, jwt, mail, invite)
	h := handler.NewAuthHandler(s, providers)
	mw := middleware.New(jwt, r, limiter)

	return &Module{
		handler: h,
		mw:      mw,
	}
}

func (m *Module) RegisterRoutes(r chi.Router) {
	bruteForce := func(name string, key middleware.KeyFunc) middleware.RateConfig {
		return middleware.RateConfig{
			Name:   name,
			Limit:  5,
			Window: 15 * time.Minute,
			Key:    key,
		}
	}

	cooldown := func(name string, key middleware.KeyFunc) middleware.RateConfig {
		return middleware.RateConfig{
			Name:   name,
			Limit:  1,
			Window: time.Minute,
			Key:    key,
		}
	}

	r.Route("/auth", func(r chi.Router) {
		// public, no limit
		r.Post("/register", m.handler.Register)
		r.Post("/check-email", m.handler.CheckEmail)
		r.Post("/refresh", m.handler.RefreshToken)

		// public, brute-force guard (per ip+email)
		r.With(m.mw.RateLimit(bruteForce("login", middleware.KeyFromJSONField("email")))).
			Post("/login", m.handler.Login)
		r.With(m.mw.RateLimit(bruteForce("validation-otp", middleware.KeyFromJSONField("email")))).
			Post("/validation-otp", m.handler.CheckOTP)
		r.With(m.mw.RateLimit(bruteForce("reset-password", middleware.KeyFromJSONField("email")))).
			Post("/reset-password", m.handler.ResetPassword)

		// public, cooldown (throttle email send)
		r.With(m.mw.RateLimit(cooldown("forgot-password", middleware.KeyFromJSONField("email")))).
			Post("/forgot-password", m.handler.ForgotPassword)

		r.Get("/sso/{provider}/url", m.handler.SSOAuthUrl)
		r.Post("/sso/{provider}/exchange", m.handler.SSOExchange)

		r.With(m.mw.RateLimit(bruteForce("invite-preview", nil))).
			Get("/invitations/{token}", m.handler.PreviewInvitation)
		r.With(m.mw.RateLimit(bruteForce("invite-accept", nil))).
			Post("/invitations/{token}/accept", m.handler.AcceptInvitation)
		// protected
		r.Group(func(r chi.Router) {
			r.Use(m.mw.RequireAuth)

			r.With(m.mw.RateLimit(cooldown("resend-otp", middleware.KeyFromClaims))).
				Post("/resend-otp", m.handler.ResendOTP)
			r.With(m.mw.RateLimit(bruteForce("verify-email", middleware.KeyFromClaims))).
				Post("/verify-email", m.handler.VerifyAccount)

			r.Post("/logout", m.handler.Logout)
			r.Get("/me", m.handler.GetMe)
		})
	})
}
