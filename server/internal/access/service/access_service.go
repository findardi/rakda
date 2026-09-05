package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/findardi/rakda/server/internal/platform/actor"
	"github.com/findardi/rakda/server/internal/platform/database"
	"github.com/findardi/rakda/server/internal/platform/ptr"
	"log"
	"strings"
	"time"

	"github.com/findardi/rakda/server/internal/access/dto"
	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/sender"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	MemberStatusActive = "active"
	// Derived only — never stored. See memberStatus.
	MemberStatusExpired = "expired"
)

const (
	DefaultGroupName = "Umum"
)

const (
	InviteStatusPending  = "pending"
	InviteStatusAccepted = "accepted"
	InviteStatusRejected = "rejected"
	InviteStatusRevoked  = "revoked"
	InviteStatusExpired  = "expired"

	inviteTTL = 7 * 24 * time.Hour

	OutcomeInvited = "invited"
	OutcomeSkipped = "skipped"

	ReasonAlreadyMember  = "already_member"
	ReasonAlreadyInvited = "already_invited"
)

var validInvitationStatuses = map[string]struct{}{
	InviteStatusPending:  {},
	InviteStatusAccepted: {},
	InviteStatusRejected: {},
	InviteStatusRevoked:  {},
	InviteStatusExpired:  {},
}

var (
	ErrRoleNotFound     = errors.New("role not found")
	ErrMemberAlreadyAdd = errors.New("user already a member of this workspace")
	ErrMemberNotFound   = errors.New("member not found")

	ErrCannotRemoveOwner     = errors.New("the workspace owner cannot be removed")
	ErrCannotAssignOwnerRole = errors.New("the owner role cannot be assigned")
	ErrCannotChangeOwnerRole = errors.New("the owner's role cannot be changed")
	ErrOnlyOwnerAssignsAdmin = errors.New("only the owner can assign the admin role")
	ErrCannotRemoveSelf      = errors.New("you cannot remove yourself")
	ErrCannotChangeSelfRole  = errors.New("you cannot change your own role")

	ErrInvitationNotFound      = errors.New("invitation not found")
	ErrInvitationNotResendable = errors.New("invitation can no longer be resent")
	ErrInvitationNotRevocable  = errors.New("invitation can no longer be revoked")
	ErrInvalidInvitationStatus = errors.New("invalid invitation status")

	ErrGroupNameTaken     = errors.New("group name already taken")
	ErrGroupNotFound      = errors.New("group not found")
	ErrGroupGuestOnly     = errors.New("groups can only be assigned to the guest role")
	ErrDeleteDefaultGroup = errors.New("group is default by system, cant deleted")

	ErrAssignMemberRole = errors.New("only can assign guest role")

	ErrExpiryGuestOnly = errors.New("access expiry can only be set for the guest role")
	ErrExpiryInvalid   = errors.New("access expiry must be an RFC 3339 timestamp")
	ErrExpiryInPast    = errors.New("access expiry must be in the future")
)

type AccessService struct {
	repo     AccessRepository
	mail     MailService
	asvc     AuthService
	token    Tokenizer
	webURL   string
	activity ActivityRecorder
}

func NewAccessService(repo AccessRepository, mail MailService, asvc AuthService, token Tokenizer, webURL string, activity ActivityRecorder) *AccessService {
	return &AccessService{
		repo:     repo,
		mail:     mail,
		asvc:     asvc,
		token:    token,
		webURL:   webURL,
		activity: activity,
	}
}

type Actor = actor.Actor

// parseAccessExpiry turns the optional expiry a manager typed into a column
// value. Only guests may carry one, and it must lie in the future: a date
// already past would mint a member who is expired on arrival, which is a
// removal wearing the wrong name. Empty means "never expires".
func parseAccessExpiry(raw, roleName string) (pgtype.Timestamptz, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Timestamptz{}, nil
	}
	if roleName != permission.RoleGuest {
		return pgtype.Timestamptz{}, ErrExpiryGuestOnly
	}

	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return pgtype.Timestamptz{}, ErrExpiryInvalid
	}
	if !t.After(time.Now()) {
		return pgtype.Timestamptz{}, ErrExpiryInPast
	}

	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}, nil
}

// memberStatus derives the status the API reports. The lapse is never
// stored — the invitation list derives its "expired" the same way in SQL — so
// a member reads as expired the moment the date passes and as active again
// the moment a manager moves it forward. Mirrors the guard in
// GetMembershipWithPermissions: expires_at <= now() is out.
func memberStatus(stored string, expiresAt pgtype.Timestamptz) string {
	if expiresAt.Valid && !expiresAt.Time.After(time.Now()) {
		return MemberStatusExpired
	}
	return stored
}

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// rfc3339OrNil formats an optional time for audit metadata; nil stays a JSON
// null so "cleared" is distinguishable from "unknown".
func rfc3339OrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// guardRoleAssignment enforces who may grant which privileged system role when
// inviting or changing a member. The owner role is never assignable via the API;
// the admin role may only be granted by the owner. Any other role (guest, custom)
// passes — the caller already holds the member:add/edit permission to reach here.
func guardRoleAssignment(actorRole, targetRole string) error {
	switch targetRole {
	case permission.RoleOwner:
		return ErrCannotAssignOwnerRole
	case permission.RoleAdmin:
		if actorRole != permission.RoleOwner {
			return ErrOnlyOwnerAssignsAdmin
		}
	}
	return nil
}

func (s *AccessService) ProvisionWorkspace(ctx context.Context, tx pgx.Tx, workspaceID, ownerID pgtype.UUID) error {
	q := accessdb.New(tx)

	var ownerRoleID pgtype.UUID
	for _, r := range permission.DefaultSystemRoles() {
		role, err := q.InsertRole(ctx, accessdb.InsertRoleParams{
			WorkspaceID: workspaceID,
			Name:        r.Name,
			Permissions: r.Permissions,
			IsSystem:    true,
		})
		if err != nil {
			return fmt.Errorf("seed role %s: %w", r.Name, err)
		}
		if r.Name == permission.RoleOwner {
			ownerRoleID = role.ID
		}
	}

	if _, err := q.AddMember(ctx, accessdb.AddMemberParams{
		WorkspaceID: workspaceID,
		UserID:      ownerID,
		RoleID:      ownerRoleID,
		Status:      MemberStatusActive,
	}); err != nil {
		return fmt.Errorf("add owner member: %w", err)
	}

	g, err := q.CreateDefaultGroup(ctx, accessdb.CreateDefaultGroupParams{
		WorkspaceID: workspaceID,
		Name:        DefaultGroupName,
	})
	if err != nil {
		return fmt.Errorf("seed default group: %w", err)
	}

	if err := q.GrantDefaultFolderAccess(ctx, accessdb.GrantDefaultFolderAccessParams{
		GroupID:     g.ID,
		WorkspaceID: workspaceID,
	}); err != nil {
		return fmt.Errorf("grant default folder access: %w", err)
	}

	return nil
}

func (s *AccessService) AddMember(ctx context.Context, req dto.CreateWorkspaceMemberRequest) (dto.WorkspaceMemberResponse, error) {
	var wsID, userID, roleID pgtype.UUID
	if err := wsID.Scan(req.WorkspaceId); err != nil {
		return dto.WorkspaceMemberResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := userID.Scan(req.UserId); err != nil {
		return dto.WorkspaceMemberResponse{}, fmt.Errorf("parse user id: %w", err)
	}
	if err := roleID.Scan(req.RoleId); err != nil {
		return dto.WorkspaceMemberResponse{}, fmt.Errorf("parse role id: %w", err)
	}

	role, err := s.repo.GetRole(ctx, roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.WorkspaceMemberResponse{}, ErrRoleNotFound
	}
	if err != nil {
		return dto.WorkspaceMemberResponse{}, fmt.Errorf("get role: %w", err)
	}
	if err := guardRoleAssignment(req.ActorRole, role.Name); err != nil {
		return dto.WorkspaceMemberResponse{}, err
	}

	status := req.Status
	if status == "" {
		status = MemberStatusActive
	}

	member, err := s.repo.AddMember(ctx, accessdb.AddMemberParams{
		WorkspaceID: wsID,
		UserID:      userID,
		RoleID:      roleID,
		Status:      status,
	})
	if database.IsUniqueViolation(err, "workspace_members_user_key") {
		return dto.WorkspaceMemberResponse{}, ErrMemberAlreadyAdd
	}
	if err != nil {
		return dto.WorkspaceMemberResponse{}, fmt.Errorf("add member: %w", err)
	}

	return dto.WorkspaceMemberResponse{
		ID:          member.ID.String(),
		WorkspaceID: member.WorkspaceID.String(),
		UserID:      member.UserID.String(),
		RoleID:      member.RoleID.String(),
		Status:      member.Status,
		CreatedAt:   member.CreatedAt.Time,
		UpdatedAt:   member.UpdatedAt.Time,
	}, nil
}

func (s *AccessService) AddMembers(ctx context.Context, req dto.AddMembersRequest, actor Actor) ([]dto.AddMembersResponse, error) {
	outcome := []dto.AddMembersResponse{}

	var wsID, roleID, inID pgtype.UUID
	if err := wsID.Scan(req.WorkspaceId); err != nil {
		return outcome, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := roleID.Scan(req.RoleId); err != nil {
		return outcome, fmt.Errorf("parse role id: %w", err)
	}
	if err := inID.Scan(actor.UserID); err != nil {
		return outcome, fmt.Errorf("parse invited by id: %w", err)
	}

	invitedRole, err := s.repo.GetRole(ctx, roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return outcome, ErrRoleNotFound
	}
	if err != nil {
		return outcome, fmt.Errorf("get role: %w", err)
	}
	// The actor's authority over the role is checked before the request's
	// shape: a request the actor may not make at all must not be answered
	// with a validation error.
	if err := guardRoleAssignment(actor.Role, invitedRole.Name); err != nil {
		return outcome, err
	}

	// A group may be chosen for guest invitations only. When one is given it
	// must exist in THIS workspace — group membership drives folder access and
	// Q&A siloing, so a foreign group id must never be accepted. Empty means
	// "default group at acceptance time".
	var gID pgtype.UUID
	groupName := ""
	if req.GroupId != "" {
		if err := gID.Scan(req.GroupId); err != nil {
			return outcome, fmt.Errorf("parse group id: %w", err)
		}
		if invitedRole.Name != permission.RoleGuest {
			return outcome, ErrGroupGuestOnly
		}

		g, err := s.repo.GetGroup(ctx, gID)
		if errors.Is(err, pgx.ErrNoRows) {
			return outcome, ErrGroupNotFound
		}
		if err != nil {
			return outcome, fmt.Errorf("get group: %w", err)
		}
		if g.WorkspaceID.String() != req.WorkspaceId {
			return outcome, ErrGroupNotFound
		}
		groupName = g.Name
	}

	accessExpiresAt, err := parseAccessExpiry(req.AccessExpiresAt, invitedRole.Name)
	if err != nil {
		return outcome, err
	}

	inviteMeta := map[string]any{"role": invitedRole.Name}
	if groupName != "" {
		inviteMeta["group"] = groupName
	}
	if accessExpiresAt.Valid {
		inviteMeta["access_expires_at"] = accessExpiresAt.Time.Format(time.RFC3339)
	}

	seen := make(map[string]struct{})
	for _, raw := range req.Email {
		email := strings.ToLower(strings.TrimSpace(raw))
		if email == "" {
			continue
		}
		if _, dup := seen[email]; dup {
			continue
		}
		seen[email] = struct{}{}

		u, err := s.asvc.UserExists(ctx, email)
		if err != nil {
			return outcome, fmt.Errorf("check user %s: %w", email, err)
		}

		registered := u.ID != ""

		// uID stays null when the email has no account yet (invite a future user)
		var uID pgtype.UUID
		if registered {
			if err := uID.Scan(u.ID); err != nil {
				return outcome, fmt.Errorf("parse user id: %w", err)
			}

			_, err := s.repo.GetMemberByWorkspaceUser(ctx, accessdb.GetMemberByWorkspaceUserParams{
				WorkspaceID: wsID,
				UserID:      uID,
			})
			if err == nil {
				outcome = append(outcome, dto.AddMembersResponse{
					Email:   email,
					Outcome: OutcomeSkipped,
					Reason:  ReasonAlreadyMember,
				})
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return outcome, fmt.Errorf("check member %s: %w", email, err)
			}
		}

		rawToken := s.token.Generate()

		codeHash := s.token.Hash(rawToken)
		expiresAt := pgtype.Timestamptz{Time: time.Now().Add(inviteTTL), Valid: true}

		// Revive a previously revoked/rejected invitation — or a pending one
		// whose expiry has lapsed — for this email instead of leaving a stale
		// row and inserting a duplicate that the pending unique index rejects.
		revived, err := s.repo.ReinviteWorkspaceInvitation(ctx, accessdb.ReinviteWorkspaceInvitationParams{
			WorkspaceID:     wsID,
			Email:           email,
			RoleID:          roleID,
			GroupID:         gID,
			UserID:          uID,
			InvitedBy:       inID,
			CodeHash:        codeHash,
			ExpiresAt:       expiresAt,
			AccessExpiresAt: accessExpiresAt,
		})
		if err == nil {
			s.sendInviteEmail(email, rawToken, registered,
				revived.WorkspaceName, ptr.Deref(revived.InvitedByUsername),
				revived.ExpiresAt.Time, revived.AccessExpiresAt.Time)
			s.activity.Record(ctx, activityservice.NewEntry(req.WorkspaceId, actor.UserID, actor.Name, actor.Role,
				activityservice.ActionInviteSent, activityservice.TargetInvitation,
				revived.ID.String(), email, inviteMeta))
			outcome = append(outcome, dto.AddMembersResponse{
				Email:   email,
				Outcome: OutcomeInvited,
			})
			continue
		}
		if database.IsUniqueViolation(err, "workspace_invitations_pending_key") {
			outcome = append(outcome, dto.AddMembersResponse{
				Email:   email,
				Outcome: OutcomeSkipped,
				Reason:  ReasonAlreadyInvited,
			})
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return outcome, fmt.Errorf("reinvite %s: %w", email, err)
		}

		// No revivable invitation: create a fresh one.
		fresh, err := s.repo.InsertWorkspaceInvitation(ctx, accessdb.InsertWorkspaceInvitationParams{
			WorkspaceID:     wsID,
			Email:           email,
			RoleID:          roleID,
			UserID:          uID,
			GroupID:         gID,
			InvitedBy:       inID,
			CodeHash:        codeHash,
			Status:          InviteStatusPending,
			ExpiresAt:       expiresAt,
			AccessExpiresAt: accessExpiresAt,
		})
		if database.IsUniqueViolation(err, "workspace_invitations_pending_key") {
			outcome = append(outcome, dto.AddMembersResponse{
				Email:   email,
				Outcome: OutcomeSkipped,
				Reason:  ReasonAlreadyInvited,
			})
			continue
		}
		if err != nil {
			return outcome, fmt.Errorf("insert invitation %s: %w", email, err)
		}

		s.sendInviteEmail(email, rawToken, registered,
			fresh.WorkspaceName, ptr.Deref(fresh.InvitedByUsername),
			fresh.ExpiresAt.Time, fresh.AccessExpiresAt.Time)
		s.activity.Record(ctx, activityservice.NewEntry(req.WorkspaceId, actor.UserID, actor.Name, actor.Role,
			activityservice.ActionInviteSent, activityservice.TargetInvitation,
			fresh.ID.String(), email, inviteMeta))
		outcome = append(outcome, dto.AddMembersResponse{
			Email:   email,
			Outcome: OutcomeInvited,
		})
	}

	return outcome, nil
}

func (s *AccessService) ListInvitations(ctx context.Context, workspaceID, status string) ([]dto.InvitationResponse, error) {
	invitations := []dto.InvitationResponse{}

	status = strings.ToLower(strings.TrimSpace(status))

	// empty status => return all statuses; otherwise filter by the given one
	var statusFilter *string
	if status != "" {
		if _, ok := validInvitationStatuses[status]; !ok {
			return invitations, ErrInvalidInvitationStatus
		}
		statusFilter = &status
	}

	var wsID pgtype.UUID
	if err := wsID.Scan(workspaceID); err != nil {
		return invitations, fmt.Errorf("parse workspace id: %w", err)
	}

	rows, err := s.repo.ListWorkspaceInvitations(ctx, accessdb.ListWorkspaceInvitationsParams{
		WorkspaceID: wsID,
		Status:      statusFilter,
	})
	if err != nil {
		return invitations, fmt.Errorf("list invitations: %w", err)
	}

	for _, r := range rows {
		invitations = append(invitations, dto.InvitationResponse{
			ID:                r.ID.String(),
			WorkspaceID:       r.WorkspaceID.String(),
			Email:             r.Email,
			RoleID:            r.RoleID.String(),
			RoleName:          ptr.Deref(r.RoleName),
			GroupID:           r.GroupID.String(),
			GroupName:         ptr.Deref(r.GroupName),
			UserID:            r.UserID.String(),
			InvitedBy:         r.InvitedBy.String(),
			InvitedByUsername: ptr.Deref(r.InvitedByUsername),
			Status:            r.Status,
			ExpiresAt:         r.ExpiresAt.Time,
			AccessExpiresAt:   timePtr(r.AccessExpiresAt),
			CreatedAt:         r.CreatedAt.Time,
		})
	}

	return invitations, nil
}

// sendInviteEmail fires the invite email in the background; the request ctx
// would be cancelled, so use a fresh one. Failure is logged, not fatal.
func (s *AccessService) sendInviteEmail(to, token string, registered bool, workspaceName, invitedBy string, expiresAt, accessExpiresAt time.Time) {
	em := sender.BuildInviteEmail(s.webURL, invitedBy, workspaceName, token, registered, expiresAt, accessExpiresAt)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.mail.Send(ctx, to, em.Subject, em.Text, em.HTML); err != nil {
			log.Printf("send invite email to %s failed: %v", to, err)
		}
	}()
}

func (s *AccessService) GetRoles(ctx context.Context, workspaceID string) ([]dto.WorkspaceRoleResponse, error) {
	var roles []dto.WorkspaceRoleResponse

	var wsID pgtype.UUID
	if err := wsID.Scan(workspaceID); err != nil {
		return roles, fmt.Errorf("parse workspace id: %w", err)
	}

	wsRoles, err := s.repo.GetRoles(ctx, wsID)
	if err != nil {
		return roles, fmt.Errorf("get roles: %w", err)
	}

	for _, r := range wsRoles {
		role := dto.WorkspaceRoleResponse{
			ID:          r.ID.String(),
			WorkspaceID: r.WorkspaceID.String(),
			Name:        r.Name,
			Permissions: r.Permissions,
			IsSystem:    r.IsSystem,
			CreatedAt:   r.CreatedAt.Time,
			UpdatedAt:   r.UpdatedAt.Time,
		}

		roles = append(roles, role)
	}

	return roles, nil
}

func (s *AccessService) GetRole(ctx context.Context, roleId string) (dto.WorkspaceRoleResponse, error) {
	var roleID pgtype.UUID
	if err := roleID.Scan(roleId); err != nil {
		return dto.WorkspaceRoleResponse{}, fmt.Errorf("parse role id: %w", err)
	}

	role, err := s.repo.GetRole(ctx, roleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.WorkspaceRoleResponse{}, ErrRoleNotFound
	}
	if err != nil {
		return dto.WorkspaceRoleResponse{}, fmt.Errorf("get role: %w", err)
	}

	return dto.WorkspaceRoleResponse{
		ID:          role.ID.String(),
		WorkspaceID: role.WorkspaceID.String(),
		Name:        role.Name,
		Permissions: role.Permissions,
		IsSystem:    role.IsSystem,
		CreatedAt:   role.CreatedAt.Time,
		UpdatedAt:   role.UpdatedAt.Time,
	}, nil
}

func (s *AccessService) GetMembers(ctx context.Context, workspaceID string) ([]dto.GetMemberResponse, error) {
	var members []dto.GetMemberResponse
	var wsID pgtype.UUID
	if err := wsID.Scan(workspaceID); err != nil {
		return members, fmt.Errorf("parse workspace id: %w", err)
	}

	wsMembers, err := s.repo.GetMembers(ctx, wsID)
	if err != nil {
		return members, fmt.Errorf("get members: %w", err)
	}

	for _, w := range wsMembers {
		member := dto.GetMemberResponse{
			ID:          w.ID.String(),
			WorkspaceID: w.WorkspaceID.String(),
			UserID:      w.UserID.String(),
			RoleID:      w.RoleID.String(),
			Status:      memberStatus(w.Status, w.ExpiresAt),
			CreatedAt:   w.CreatedAt.Time,
			UpdatedAt:   w.UpdatedAt.Time,
			RoleName:    ptr.Deref(w.RoleName),
			Username:    ptr.Deref(w.Username),
			Email:       ptr.Deref(w.Email),
			GroupNames:  w.GroupNames,
			ExpiresAt:   timePtr(w.ExpiresAt),
		}

		members = append(members, member)
	}

	return members, nil
}

func (s *AccessService) GetMember(ctx context.Context, memberID string) (dto.GetMemberResponse, error) {
	var mID pgtype.UUID
	if err := mID.Scan(memberID); err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("parse member id: %w", err)
	}

	w, err := s.repo.GetMember(ctx, mID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.GetMemberResponse{}, ErrMemberNotFound
	}
	if err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("get member: %w", err)
	}

	return dto.GetMemberResponse{
		ID:          w.ID.String(),
		WorkspaceID: w.WorkspaceID.String(),
		UserID:      w.UserID.String(),
		RoleID:      w.RoleID.String(),
		Status:      memberStatus(w.Status, w.ExpiresAt),
		CreatedAt:   w.CreatedAt.Time,
		UpdatedAt:   w.UpdatedAt.Time,
		RoleName:    ptr.Deref(w.RoleName),
		Username:    ptr.Deref(w.Username),
		Email:       ptr.Deref(w.Email),
		GroupNames:  w.GroupNames,
		ExpiresAt:   timePtr(w.ExpiresAt),
	}, nil
}

func (s *AccessService) UpdateMemberRole(ctx context.Context, req dto.UpdateMemberRoleRequest, actor Actor) (dto.GetMemberResponse, error) {
	var mID, rID pgtype.UUID
	if err := mID.Scan(req.MemberID); err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("parse member id: %w", err)
	}
	if err := rID.Scan(req.RoleId); err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("parse role id: %w", err)
	}

	target, err := s.GetMember(ctx, req.MemberID)
	if err != nil {
		return dto.GetMemberResponse{}, err
	}
	if target.UserID == actor.UserID {
		return dto.GetMemberResponse{}, ErrCannotChangeSelfRole
	}
	if target.RoleName == permission.RoleOwner {
		return dto.GetMemberResponse{}, ErrCannotChangeOwnerRole
	}

	newRole, err := s.repo.GetRole(ctx, rID)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.GetMemberResponse{}, ErrRoleNotFound
	}
	if err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("get role: %w", err)
	}
	if err := guardRoleAssignment(actor.Role, newRole.Name); err != nil {
		return dto.GetMemberResponse{}, err
	}

	_, err = s.repo.UpdateRole(ctx, accessdb.UpdateRoleParams{
		ID:     mID,
		RoleID: rID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.GetMemberResponse{}, ErrMemberNotFound
	}
	if err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("update member role: %w", err)
	}

	s.activity.Record(ctx, activityservice.NewEntry(target.WorkspaceID, actor.UserID, actor.Name, actor.Role,
		activityservice.ActionRoleChanged, activityservice.TargetMember,
		req.MemberID, target.Email, map[string]any{"from": target.RoleName, "to": newRole.Name}))

	return s.GetMember(ctx, req.MemberID)
}

// UpdateMemberExpiry sets, moves, or clears a guest's access expiry. Nothing
// but the date is checked at the gate, so a lapsed member is revived by a new
// future date or by clearing it — no re-invitation, group and history intact.
func (s *AccessService) UpdateMemberExpiry(ctx context.Context, req dto.UpdateMemberExpiryRequest, actor Actor) (dto.GetMemberResponse, error) {
	var mID pgtype.UUID
	if err := mID.Scan(req.MemberID); err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("parse member id: %w", err)
	}

	target, err := s.GetMember(ctx, req.MemberID)
	if err != nil {
		return dto.GetMemberResponse{}, err
	}
	// Checked here as well as in parseAccessExpiry so that clearing on a
	// non-guest is refused too, rather than silently succeeding as a no-op.
	if target.RoleName != permission.RoleGuest {
		return dto.GetMemberResponse{}, ErrExpiryGuestOnly
	}

	var expiresAt pgtype.Timestamptz
	if req.ExpiresAt != nil {
		expiresAt, err = parseAccessExpiry(*req.ExpiresAt, target.RoleName)
		if err != nil {
			return dto.GetMemberResponse{}, err
		}
	}

	_, err = s.repo.UpdateMemberExpiry(ctx, accessdb.UpdateMemberExpiryParams{
		ID:        mID,
		ExpiresAt: expiresAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.GetMemberResponse{}, ErrMemberNotFound
	}
	if err != nil {
		return dto.GetMemberResponse{}, fmt.Errorf("update member expiry: %w", err)
	}

	s.activity.Record(ctx, activityservice.NewEntry(target.WorkspaceID, actor.UserID, actor.Name, actor.Role,
		activityservice.ActionMemberExpiryChanged, activityservice.TargetMember,
		req.MemberID, target.Email, map[string]any{
			"from": rfc3339OrNil(target.ExpiresAt),
			"to":   rfc3339OrNil(timePtr(expiresAt)),
		}))

	return s.GetMember(ctx, req.MemberID)
}

// MemberExpiry reports the caller's own access expiry in a room, nil when
// none is set. Read on its own rather than through the membership guard so
// the guard's Membership shape — copied per module — stays untouched.
func (s *AccessService) MemberExpiry(ctx context.Context, workspaceID, userID string) (*time.Time, error) {
	var wsID, uID pgtype.UUID
	if err := wsID.Scan(workspaceID); err != nil {
		return nil, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := uID.Scan(userID); err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	m, err := s.repo.GetMemberByWorkspaceUser(ctx, accessdb.GetMemberByWorkspaceUserParams{
		WorkspaceID: wsID,
		UserID:      uID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get member: %w", err)
	}

	return timePtr(m.ExpiresAt), nil
}

func (s *AccessService) DeleteMember(ctx context.Context, memberID string, actor Actor) error {
	var mID pgtype.UUID
	if err := mID.Scan(memberID); err != nil {
		return fmt.Errorf("parse member id: %w", err)
	}

	target, err := s.GetMember(ctx, memberID)
	if err != nil {
		return err
	}
	if target.UserID == actor.UserID {
		return ErrCannotRemoveSelf
	}
	if target.RoleName == permission.RoleOwner {
		return ErrCannotRemoveOwner
	}

	if err := s.repo.DeleteMember(ctx, mID); err != nil {
		return fmt.Errorf("delete member: %w", err)
	}

	s.activity.Record(ctx, activityservice.NewEntry(target.WorkspaceID, actor.UserID, actor.Name, actor.Role,
		activityservice.ActionMemberRemoved, activityservice.TargetMember,
		memberID, target.Email, nil))

	return nil
}

func (s *AccessService) ResendInvitation(ctx context.Context, invitationID string, actor Actor) error {
	var invID pgtype.UUID
	if err := invID.Scan(invitationID); err != nil {
		return fmt.Errorf("parse invitation id: %w", err)
	}

	inv, err := s.repo.GetWorkspaceInvitation(ctx, invID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvitationNotFound
	}
	if err != nil {
		return fmt.Errorf("get invitation: %w", err)
	}

	rawToken := s.token.Generate()

	updated, err := s.repo.ResendInvitation(ctx, accessdb.ResendInvitationParams{
		ID:        invID,
		CodeHash:  s.token.Hash(rawToken),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(inviteTTL), Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvitationNotResendable
		}
		return fmt.Errorf("resend invitation: %w", err)
	}

	// The email must carry the renewed expiry, not the one on the row fetched
	// before the update — for a lapsed invitation that date is in the past.
	s.sendInviteEmail(inv.Email, rawToken, inv.UserID.Valid,
		inv.WorkspaceName, ptr.Deref(inv.InvitedByUsername),
		updated.ExpiresAt.Time, updated.AccessExpiresAt.Time)
	s.activity.Record(ctx, activityservice.NewEntry(inv.WorkspaceID.String(), actor.UserID, actor.Name, actor.Role,
		activityservice.ActionInviteResent, activityservice.TargetInvitation,
		invitationID, inv.Email, nil))
	return nil
}

func (s *AccessService) RevokeInvitation(ctx context.Context, invitationID string, actor Actor) error {
	var invID pgtype.UUID
	if err := invID.Scan(invitationID); err != nil {
		return fmt.Errorf("parse invitation id: %w", err)
	}

	inv, err := s.repo.RevokeWorkspaceInvitation(ctx, invID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvitationNotRevocable
		}
		return fmt.Errorf("revoke invitation: %w", err)
	}

	s.activity.Record(ctx, activityservice.NewEntry(inv.WorkspaceID.String(), actor.UserID, actor.Name, actor.Role,
		activityservice.ActionInviteRevoked, activityservice.TargetInvitation,
		invitationID, inv.Email, nil))

	return nil
}

func (s *AccessService) CreateGroup(ctx context.Context, req dto.CreateGroupRequest, actor Actor) (dto.GroupResponse, error) {
	var wID pgtype.UUID
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.GroupResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	var g accessdb.WorkspaceGroup
	err := s.repo.ExecTxTx(ctx, func(q *accessdb.Queries, tx pgx.Tx) error {
		created, err := q.CreateGroup(ctx, accessdb.CreateGroupParams{
			WorkspaceID: wID,
			Name:        req.Name,
			Description: &req.Description,
		})
		if database.IsUniqueViolation(err, "workspace_groups_name_key") {
			return ErrGroupNameTaken
		}
		if err != nil {
			return fmt.Errorf("create group: %w", err)
		}

		if err := q.GrantDefaultFolderAccess(ctx, accessdb.GrantDefaultFolderAccessParams{
			GroupID:     created.ID,
			WorkspaceID: wID,
		}); err != nil {
			return fmt.Errorf("grant default folder access: %w", err)
		}

		g = created
		return s.activity.RecordTx(ctx, tx, activityservice.NewEntry(req.WorkspaceID, actor.UserID, actor.Name, actor.Role,
			activityservice.ActionGroupCreated, activityservice.TargetGroup,
			created.ID.String(), created.Name, nil))
	})
	if err != nil {
		return dto.GroupResponse{}, err
	}

	return groupResponse(g), nil
}

func groupResponse(g accessdb.WorkspaceGroup) dto.GroupResponse {
	return dto.GroupResponse{
		ID:              g.ID.String(),
		WorkspaceID:     g.WorkspaceID.String(),
		Name:            g.Name,
		Description:     ptr.Deref(g.Description),
		IsDefault:       g.IsDefault,
		QAEnabled:       g.QaEnabled,
		QAQuestionLimit: g.QaQuestionLimit,
		CreatedAt:       g.CreatedAt.Time,
		UpdatedAt:       g.UpdatedAt.Time,
	}
}

func (s *AccessService) GetGroups(ctx context.Context, workspaceID string) ([]dto.GroupResponse, error) {
	var groups []dto.GroupResponse
	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return groups, fmt.Errorf("parse workspace id: %w", err)
	}

	gps, err := s.repo.GetGroups(ctx, wID)
	if err != nil {
		return groups, fmt.Errorf("get groups: %w", err)
	}

	for _, g := range gps {
		groups = append(groups, groupResponse(g))
	}

	return groups, nil
}

func (s *AccessService) DeleteGroup(ctx context.Context, groupID string, actor Actor) error {
	var gID pgtype.UUID
	if err := gID.Scan(groupID); err != nil {
		return fmt.Errorf("parse group id: %w", err)
	}

	return s.repo.ExecTxTx(ctx, func(q *accessdb.Queries, tx pgx.Tx) error {
		g, err := q.GetGroup(ctx, gID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrGroupNotFound
			}
			return fmt.Errorf("get group: %w", err)
		}

		if g.IsDefault {
			return ErrDeleteDefaultGroup
		}

		if _, err := q.MoveGroupMembersToDefaultGroup(ctx, gID); err != nil {
			return fmt.Errorf("move group members to default group: %w", err)
		}

		if err := q.DeleteGroup(ctx, gID); err != nil {
			return fmt.Errorf("delete group: %w", err)
		}

		return s.activity.RecordTx(ctx, tx, activityservice.NewEntry(g.WorkspaceID.String(), actor.UserID, actor.Name, actor.Role,
			activityservice.ActionGroupDeleted, activityservice.TargetGroup,
			groupID, g.Name, nil))
	})
}

func (s *AccessService) UpdateGroup(ctx context.Context, req dto.UpdateGroupRequest, actor Actor) (dto.GroupResponse, error) {
	var gID pgtype.UUID
	if err := gID.Scan(req.GroupID); err != nil {
		return dto.GroupResponse{}, fmt.Errorf("parse group id: %w", err)
	}

	g, err := s.repo.UpdateGroup(ctx, accessdb.UpdateGroupParams{
		ID:          gID,
		Name:        req.Name,
		Description: &req.Description,
	})

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dto.GroupResponse{}, ErrGroupNotFound
	case database.IsUniqueViolation(err, "workspace_groups_name_key"):
		return dto.GroupResponse{}, ErrGroupNameTaken
	case err != nil:
		return dto.GroupResponse{}, fmt.Errorf("update group: %w", err)
	}

	s.activity.Record(ctx, activityservice.NewEntry(g.WorkspaceID.String(), actor.UserID, actor.Name, actor.Role,
		activityservice.ActionGroupUpdated, activityservice.TargetGroup,
		req.GroupID, g.Name, nil))

	return groupResponse(g), nil
}

func (s *AccessService) UpdateGroupQA(ctx context.Context, req dto.UpdateGroupQARequest, actor Actor) (dto.GroupResponse, error) {
	var gID, wID pgtype.UUID
	if err := gID.Scan(req.GroupID); err != nil {
		return dto.GroupResponse{}, fmt.Errorf("parse group id: %w", err)
	}
	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.GroupResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	g, err := s.repo.UpdateGroupQA(ctx, accessdb.UpdateGroupQAParams{
		QaEnabled:       *req.QAEnabled,
		QaQuestionLimit: req.QuestionLimit,
		ID:              gID,
		WorkspaceID:     wID,
	})

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dto.GroupResponse{}, ErrGroupNotFound
	case err != nil:
		return dto.GroupResponse{}, fmt.Errorf("update group qa: %w", err)
	}

	s.activity.Record(ctx, activityservice.NewEntry(req.WorkspaceID, actor.UserID, actor.Name, actor.Role,
		activityservice.ActionQaSettingsChanged, activityservice.TargetGroup,
		req.GroupID, g.Name, map[string]any{
			"qa_enabled":     g.QaEnabled,
			"question_limit": g.QaQuestionLimit,
		}))

	return groupResponse(g), nil
}

func (s *AccessService) GetGroupDetail(ctx context.Context, groupID string) ([]dto.GroupMemberResponse, error) {
	var members []dto.GroupMemberResponse
	var gID pgtype.UUID
	if err := gID.Scan(groupID); err != nil {
		return members, fmt.Errorf("parse group id: %w", err)
	}

	gm, err := s.repo.GetGroupMembers(ctx, gID)
	if err != nil {
		return members, fmt.Errorf("get group members: %w", err)
	}

	for _, m := range gm {
		member := dto.GroupMemberResponse{
			GroupID:   m.GroupID.String(),
			MemberID:  m.MemberID.String(),
			CreatedAt: m.CreatedAt.Time,
			Username:  ptr.Deref(m.Username),
			Email:     ptr.Deref(m.Email),
			RoleName:  ptr.Deref(m.RoleName),
			GroupName: ptr.Deref(m.GroupName),
		}

		members = append(members, member)
	}

	return members, nil
}

func (s *AccessService) AssignToGroup(ctx context.Context, req dto.GroupMemberRequest, actor Actor) ([]dto.GroupMemberResponse, error) {

	var gID pgtype.UUID
	if err := gID.Scan(req.GroupID); err != nil {
		return []dto.GroupMemberResponse{}, fmt.Errorf("parse group id: %w", err)
	}

	for _, m := range req.MemberID {
		var mID pgtype.UUID
		if err := mID.Scan(m); err != nil {
			return []dto.GroupMemberResponse{}, fmt.Errorf("parse member id: %w", err)
		}

		mem, err := s.repo.GetMember(ctx, mID)
		if err != nil {
			return []dto.GroupMemberResponse{}, fmt.Errorf("get member: %w", err)
		}

		if ptr.Deref(mem.RoleName) != "guest" {
			return []dto.GroupMemberResponse{}, ErrAssignMemberRole
		}

		_, err = s.repo.InsertGroupMember(ctx, accessdb.InsertGroupMemberParams{
			GroupID:  gID,
			MemberID: mID,
		})
		// if database.IsUniqueViolation(err, "workspace_group_members_pkey") {
		// 	continue
		// }
		if err != nil {
			return []dto.GroupMemberResponse{}, fmt.Errorf("assign member to group: %w", err)
		}

		s.activity.Record(ctx, activityservice.NewEntry(mem.WorkspaceID.String(), actor.UserID, actor.Name, actor.Role,
			activityservice.ActionGroupAssigned, activityservice.TargetMember,
			m, ptr.Deref(mem.Email), map[string]any{"group_id": req.GroupID}))
	}

	return s.GetGroupDetail(ctx, req.GroupID)
}

func (s *AccessService) UnassignFromGroup(ctx context.Context, groupID, memberID string, actor Actor) error {
	var gID, mID pgtype.UUID
	if err := gID.Scan(groupID); err != nil {
		return fmt.Errorf("parse group id: %w", err)
	}
	if err := mID.Scan(memberID); err != nil {
		return fmt.Errorf("parse member id: %w", err)
	}

	mem, err := s.repo.GetMember(ctx, mID)
	if err != nil {
		return fmt.Errorf("get member: %w", err)
	}

	moved, err := s.repo.MoveMemberToDefaultGroup(ctx, mID)
	if err != nil {
		return fmt.Errorf("move member to default group: %w", err)
	}

	if moved == 0 {
		if err := s.repo.DeleteGroupMember(ctx, accessdb.DeleteGroupMemberParams{
			GroupID:  gID,
			MemberID: mID,
		}); err != nil {
			return fmt.Errorf("unassign member from group: %w", err)
		}
	}

	s.activity.Record(ctx, activityservice.NewEntry(mem.WorkspaceID.String(), actor.UserID, actor.Name, actor.Role,
		activityservice.ActionGroupUnassigned, activityservice.TargetMember,
		memberID, ptr.Deref(mem.Email), map[string]any{"group_id": groupID}))

	return nil
}
