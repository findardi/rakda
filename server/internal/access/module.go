package access

import (
	"context"
	"errors"

	"github.com/findardi/rakda/server/internal/access/handler"
	"github.com/findardi/rakda/server/internal/access/repository"
	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	"github.com/findardi/rakda/server/internal/access/service"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/permission"
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
	handler *handler.AccessHandler
	mw      *middleware.Middleware
	repo    *repository.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, mail service.MailService, asvc service.AuthService, token service.Tokenizer, webURL string, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewAccessService(r, mail, asvc, token, webURL, activity)
	h := handler.NewAccessHandler(s)

	mw := middleware.New(verifier, userStatusReader{repo: auth.New(pool)}, nil)
	return &Module{
		handler: h,
		mw:      mw,
		repo:    r,
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

	row, err := m.repo.GetMembershipWithPermissions(ctx, accessdb.GetMembershipWithPermissionsParams{
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
	r.Route("/access", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)
		// Read-only permission catalog — feeds the per-role permission view.
		r.Get("/permissions", m.handler.GetPermissions)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.workspaceMember))
			r.Get("/me", m.handler.GetMyAccess)
			// Roles are fixed system roles (owner/admin/guest); read-only via API.
			r.Route("/roles", func(r chi.Router) {
				r.With(m.mw.RequirePermission(permission.PermRoleView)).Get("/", m.handler.GetRoles)
				r.With(m.mw.RequirePermission(permission.PermRoleView)).Get("/{roleID}", m.handler.GetRole)
			})

			r.Route("/members", func(r chi.Router) {
				r.With(m.mw.RequirePermission(permission.PermMemberAdd)).Post("/", m.handler.AddMember)
				r.With(m.mw.RequirePermission(permission.PermMemberView)).Get("/", m.handler.GetMembers)
				r.With(m.mw.RequirePermission(permission.PermMemberView)).Get("/{memberID}", m.handler.GetMember)
				r.With(m.mw.RequirePermission(permission.PermMemberEdit)).Put("/{memberID}", m.handler.UpdateMember)
				r.With(m.mw.RequirePermission(permission.PermMemberDelete)).Delete("/{memberID}", m.handler.DeleteMember)
			})

			r.Route("/invitations", func(r chi.Router) {
				r.With(m.mw.RequirePermission(permission.PermMemberAdd)).Post("/", m.handler.AddMembers)
				r.With(m.mw.RequirePermission(permission.PermMemberView)).Get("/", m.handler.GetInvitations)
				r.With(m.mw.RequirePermission(permission.PermMemberAdd)).Post("/{invitationID}/resend", m.handler.ResendInvitation)
				r.With(m.mw.RequirePermission(permission.PermMemberDelete)).Post("/{invitationID}/revoke", m.handler.RevokeInvitation)
			})

			r.Route("/groups", func(r chi.Router) {
				r.With(m.mw.RequirePermission(permission.PermGroupCreate)).Post("/", m.handler.CreateGroup)
				r.With(m.mw.RequirePermission(permission.PermGroupView)).Get("/", m.handler.GetGroups)
				r.With(m.mw.RequirePermission(permission.PermGroupView)).Get("/{groupID}", m.handler.GetGroup)
				r.With(m.mw.RequirePermission(permission.PermGroupEdit)).Put("/{groupID}", m.handler.UpdateGroup)
				r.With(m.mw.RequirePermission(permission.PermGroupDelete)).Delete("/{groupID}", m.handler.DeleteGroup)
				r.With(m.mw.RequirePermission(permission.PermGroupAssign)).Post("/{groupID}/assign", m.handler.AssignMember)
				r.With(m.mw.RequirePermission(permission.PermGroupAssign)).Delete("/{groupID}/unassign/{memberID}", m.handler.UnassignMember)
			})
		})
	})
}
