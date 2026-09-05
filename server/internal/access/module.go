package access

import (
	"github.com/findardi/rakda/server/internal/access/handler"
	"github.com/findardi/rakda/server/internal/access/repository"
	"github.com/findardi/rakda/server/internal/access/service"
	auth "github.com/findardi/rakda/server/internal/auth/repository"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	handler *handler.AccessHandler
	mw      *middleware.Middleware
	repo    *repository.Repository
}

func NewModule(pool *pgxpool.Pool, verifier middleware.TokenVerifier, mail service.MailService, asvc service.AuthService, token service.Tokenizer, webURL string, activity service.ActivityRecorder) *Module {
	r := repository.New(pool)
	s := service.NewAccessService(r, mail, asvc, token, webURL, activity)
	h := handler.NewAccessHandler(s)

	mw := middleware.New(verifier, auth.New(pool), nil)
	return &Module{
		handler: h,
		mw:      mw,
		repo:    r,
	}
}

func (m *Module) RegisterRoutes(r chi.Router) {
	r.Route("/access", func(r chi.Router) {
		r.Use(m.mw.RequireAuth)
		r.Use(m.mw.RequireActive)
		// Read-only permission catalog — feeds the per-role permission view.
		r.Get("/permissions", m.handler.GetPermissions)

		r.Route("/workspaces/{workspaceID}", func(r chi.Router) {
			r.Use(m.mw.RequireMember("workspaceID", m.repo.ResolveMembership))

			r.Get("/me", m.handler.GetMyAccess)

			r.Group(func(r chi.Router) {
				r.Use(m.mw.RequireRoomOpenForGuests)
				r.Use(m.mw.RequireRoomWritable)

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
					r.With(m.mw.RequirePermission(permission.PermMemberEdit)).Patch("/{memberID}/expiry", m.handler.UpdateMemberExpiry)
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
					r.With(m.mw.RequirePermission(permission.PermGroupEdit)).Patch("/{groupID}/qa", m.handler.UpdateGroupQA)
					r.With(m.mw.RequirePermission(permission.PermGroupDelete)).Delete("/{groupID}", m.handler.DeleteGroup)
					r.With(m.mw.RequirePermission(permission.PermGroupAssign)).Post("/{groupID}/assign", m.handler.AssignMember)
					r.With(m.mw.RequirePermission(permission.PermGroupAssign)).Delete("/{groupID}/unassign/{memberID}", m.handler.UnassignMember)
				})
			})
		})
	})
}
