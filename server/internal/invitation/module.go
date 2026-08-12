package invitation

import (
	"context"

	auth "github.com/findardi/Riksa-App/server/internal/auth/repository"
	"github.com/findardi/Riksa-App/server/internal/invitation/handler"
	"github.com/findardi/Riksa-App/server/internal/invitation/repository"
	"github.com/findardi/Riksa-App/server/internal/invitation/service"
	"github.com/findardi/Riksa-App/server/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userStatusReader struct {
	repo *auth.Repository
}

func (s userStatusReader) UserStatus(ctx context.Context, userID string) (string, error) {
	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return "", err
	}

	user, err := s.repo.GetUserById(ctx, uid)
	if err != nil {
		return "", err
	}
	return user.Status, nil
}

type Module struct {
	handler *handler.InvitationHandler
	mw      *middleware.Middleware
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewInvitationService(r, activity)
	h := handler.NewInvitationHandler(s)

	mw := middleware.New(verifier, userStatusReader{repo: auth.New(pool)}, nil)
	return &Module{
		handler: h,
		mw:      mw,
	}
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/invitations", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Get("/", m.handler.GetListInvitations)
		r.Post("/{invitationID}/accept", m.handler.AcceptInvitation)
		r.Post("/{invitationID}/reject", m.handler.RejectInvitation)
	})
}
