package qa

import (
	accessrepo "github.com/findardi/rakda/server/internal/access/repository"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/qa/handler"
	"github.com/findardi/rakda/server/internal/qa/repository"
	"github.com/findardi/rakda/server/internal/qa/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler    *handler.QAHandler
	mw         *middleware.Middleware
	accessRepo *accessrepo.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, content service.ContentAccessChecker, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewQAService(r, content, activity)
	h := handler.NewQAHandler(s)

	mw := middleware.New(verifier, auth.New(pool), nil)

	return &Module{
		handler:    h,
		mw:         mw,
		accessRepo: accessrepo.New(pool),
	}
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/qa", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.accessRepo.ResolveMembership))
			r.Use(m.mw.RequireRoomOpenForGuests)
			r.Use(m.mw.RequireRoomWritable)

			r.Route("/questions", func(r chi.Router) {
				r.Get("/", m.handler.ListQuestions)
				r.Post("/", m.handler.SubmitQuestion)
				r.Get("/count", m.handler.CountWaiting)
				r.Get("/export", m.handler.ExportQuestions)

				r.Route("/{questionID}", func(r chi.Router) {
					r.Get("/", m.handler.GetQuestion)
					r.Post("/replies", m.handler.ReplyQuestion)
					r.Post("/close", m.handler.CloseQuestion)
					r.Post("/reopen", m.handler.ReopenQuestion)
				})
			})

			r.Route("/faqs", func(r chi.Router) {
				r.Get("/", m.handler.ListFaqs)
				r.Post("/", m.handler.CreateFaq)
			})
		})
	})
}
