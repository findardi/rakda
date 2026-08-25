package qa

import (
	"context"
	"errors"

	accessrepo "github.com/findardi/rakda/server/internal/access/repository"
	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/qa/handler"
	"github.com/findardi/rakda/server/internal/qa/repository"
	"github.com/findardi/rakda/server/internal/qa/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
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
	handler    *handler.QAHandler
	mw         *middleware.Middleware
	accessRepo *accessrepo.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, content service.ContentAccessChecker, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewQAService(r, content, activity)
	h := handler.NewQAHandler(s)

	mw := middleware.New(verifier, userStatusReader{repo: auth.New(pool)}, nil)

	return &Module{
		handler:    h,
		mw:         mw,
		accessRepo: accessrepo.New(pool),
	}
}

func (m *Module) workspaceMember(ctx context.Context, workspaceID, userID string) (*middleware.Membership, error) {
	var wID, uID pgtype.UUID

	if err := wID.Scan(workspaceID); err != nil {
		return nil, middleware.ErrResourceNotFound
	}
	if err := uID.Scan(userID); err != nil {
		return nil, middleware.ErrResourceNotFound
	}

	row, err := m.accessRepo.GetMembershipWithPermissions(ctx, accessdb.GetMembershipWithPermissionsParams{
		WorkspaceID: wID,
		UserID:      uID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, middleware.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}

	return &middleware.Membership{
		Role:        row.RoleName,
		Permissions: row.Permissions,
		Status:      row.Status,
	}, nil
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/qa", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.workspaceMember))

			r.Route("/questions", func(r chi.Router) {
				r.Get("/", m.handler.ListQuestions)
				r.Post("/", m.handler.SubmitQuestion)
				r.Get("/count", m.handler.CountWaiting)

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
