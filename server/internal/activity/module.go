package activity

import (
	"context"
	"errors"

	accessrepo "github.com/findardi/Riksa-App/server/internal/access/repository"
	accessdb "github.com/findardi/Riksa-App/server/internal/access/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/activity/handler"
	"github.com/findardi/Riksa-App/server/internal/activity/repository"
	"github.com/findardi/Riksa-App/server/internal/activity/service"
	auth "github.com/findardi/Riksa-App/server/internal/auth/repository"
	"github.com/findardi/Riksa-App/server/internal/platform/middleware"
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
	handler    *handler.ActivityHandler
	mw         *middleware.Middleware
	accessRepo *accessrepo.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier) *Module {
	r := repository.New(pool)
	s := service.NewActivityService(r)
	h := handler.NewActivityHandler(s)

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
	r.Route("/activity", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.workspaceMember))

			r.Get("/", m.handler.ListActivity)
			r.Get("/export", m.handler.ExportActivity)

			r.Route("/documents/{documentID}", func(r chi.Router) {
				r.Post("/duration", m.handler.RecordDurations)
				r.Get("/engagement", m.handler.GetDocumentEngagement)
				r.Get("/engagement/export", m.handler.ExportDocumentEngagement)
			})
		})
	})
}
