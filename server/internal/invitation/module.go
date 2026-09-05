package invitation

import (
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/invitation/handler"
	"github.com/findardi/rakda/server/internal/invitation/repository"
	"github.com/findardi/rakda/server/internal/invitation/service"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *handler.InvitationHandler
	mw      *middleware.Middleware
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewInvitationService(r, activity)
	h := handler.NewInvitationHandler(s)

	mw := middleware.New(verifier, auth.New(pool), nil)
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
