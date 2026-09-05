package workspace

import (
	"context"
	"errors"

	accessrepo "github.com/findardi/rakda/server/internal/access/repository"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/storage"
	"github.com/findardi/rakda/server/internal/workspace/handler"
	"github.com/findardi/rakda/server/internal/workspace/repository"
	"github.com/findardi/rakda/server/internal/workspace/service"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *handler.WorkspaceHandler
	mw      *middleware.Middleware
	repo    *repository.Repository
	access  *accessrepo.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, access, content service.Provisioner, activity service.ActivityRecorder, store storage.Storage) *Module {
	r := repository.New(pool)
	s := service.NewWorkspaceService(r, access, content, activity, store)
	h := handler.NewWorkspaceHandler(s)

	mw := middleware.New(verifier, auth.New(pool), nil)

	return &Module{
		handler: h,
		mw:      mw,
		repo:    r,
		access:  accessrepo.New(pool),
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
		// The curated hero identities; static, so it sits before the id routes.
		r.Get("/hero-presets", m.handler.GetHeroPresets)

		// The same lifecycle gate the other four {workspaceID} modules carry:
		// membership, then prepare-room closure for guests, then the archive
		// freeze on every non-GET.
		r.Route("/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.access.ResolveMembership))
			r.Use(m.mw.RequireRoomOpenForGuests)

			// The one mutation an archived room must keep accepting is the
			// transition out of archive, so status sits beside the writable
			// group, not inside it — the same shape as the four content-side
			// exceptions. The service still refuses every other illegal move.
			r.With(m.mw.RequireOwner("workspaceID", m.workspaceOwner)).
				Patch("/status", m.handler.UpdateStatusWorkspace)

			r.Group(func(r chi.Router) {
				r.Use(m.mw.RequireRoomWritable)

				r.Get("/", m.handler.GetWorkspace)
				r.Get("/summary", m.handler.GetWorkspaceSummary)
				r.Get("/branding/logo", m.handler.GetLogo)

				r.Group(func(r chi.Router) {
					r.Use(m.mw.RequireOwner("workspaceID", m.workspaceOwner))
					r.Put("/", m.handler.UpdateWorkspace)
					r.Delete("/", m.handler.DeleteWorkspace)
					r.Put("/branding/logo", m.handler.SetLogo)
					r.Delete("/branding/logo", m.handler.RemoveLogo)
					r.Put("/branding/hero", m.handler.SetHeroPreset)
				})
			})
		})
	})
}
