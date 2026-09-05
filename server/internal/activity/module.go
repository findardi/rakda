package activity

import (
	"context"

	accessrepo "github.com/findardi/rakda/server/internal/access/repository"
	"github.com/findardi/rakda/server/internal/activity/handler"
	activitydb "github.com/findardi/rakda/server/internal/activity/repository/sqlc"
	"github.com/findardi/rakda/server/internal/activity/service"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
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
	handler    *handler.ActivityHandler
	mw         *middleware.Middleware
	accessRepo *accessrepo.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier) *Module {
	s := service.NewActivityService(activitydb.New(pool))
	h := handler.NewActivityHandler(s)

	mw := middleware.New(verifier, userStatusReader{repo: auth.New(pool)}, nil)

	return &Module{
		handler:    h,
		mw:         mw,
		accessRepo: accessrepo.New(pool),
	}
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/activity", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.accessRepo.ResolveMembership))
			r.Use(m.mw.RequireRoomOpenForGuests)

			r.Group(func(r chi.Router) {
				r.Post("/documents/{documentID}/duration", m.handler.RecordDurations)
			})

			r.Group(func(r chi.Router) {
				r.Use(m.mw.RequireRoomWritable)

				r.Get("/", m.handler.ListActivity)
				r.Get("/export", m.handler.ExportActivity)

				r.Route("/documents/{documentID}", func(r chi.Router) {
					r.Get("/engagement", m.handler.GetDocumentReaders)
					r.Get("/engagement/readers/{actorID}", m.handler.GetReaderPages)
					r.Get("/engagement/export", m.handler.ExportDocumentEngagement)
				})
			})
		})
	})
}
