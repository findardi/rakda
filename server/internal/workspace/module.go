package workspace

import (
	"context"
	"errors"

	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/workspace/handler"
	"github.com/findardi/rakda/server/internal/workspace/repository"
	"github.com/findardi/rakda/server/internal/workspace/service"
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
	handler *handler.WorkspaceHandler
	mw      *middleware.Middleware
	repo    *repository.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, access service.AccessService, content service.ContentProvisioner, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewWorkspaceService(r, access, content, activity)
	h := handler.NewWorkspaceHandler(s)

	mw := middleware.New(verifier, userStatusReader{repo: auth.New(pool)}, nil)

	return &Module{
		handler: h,
		mw:      mw,
		repo:    r,
	}
}

func (m *Module) workspaceOwner(ctx context.Context, id string) (string, error) {
	var uid pgtype.UUID
	if err := uid.Scan(id); err != nil {
		return "", middleware.ErrResourceNotFound
	}

	ws, err := m.repo.GetWorkspaceByID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", middleware.ErrResourceNotFound
	}
	if err != nil {
		return "", err
	}

	v, _ := ws.OwnerID.Value()
	ownerID, _ := v.(string)
	return ownerID, nil
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/workspaces", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Post("/", m.handler.Create)
		r.Get("/", m.handler.GetWorkspaces)

		r.Group(func(r chi.Router) {
			r.Get("/{workspaceID}", m.handler.GetWorkspace)
			r.Get("/{workspaceID}/summary", m.handler.GetWorkspaceSummary)

			r.Group(func(r chi.Router) {
				r.Use(m.mw.RequireOwner("workspaceID", m.workspaceOwner))
				r.Put("/{workspaceID}", m.handler.UpdateWorkspace)
				r.Patch("/{workspaceID}/status", m.handler.UpdateStatusWorkspace)
				r.Delete("/{workspaceID}", m.handler.DeleteWorkspace)
			})
		})
	})
}
