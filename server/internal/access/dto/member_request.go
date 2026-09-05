package dto

type CreateWorkspaceMemberRequest struct {
	WorkspaceId string `json:"-"`
	UserId      string `json:"user_id" validate:"required"`
	RoleId      string `json:"role_id" validate:"required"`
	Status      string `json:"status"`
	ActorRole   string `json:"-"`
}

type UpdateMemberRoleRequest struct {
	MemberID string `json:"-"`
	RoleId   string `json:"role_id" validate:"required"`
}

// UpdateMemberExpiryRequest sets or clears a guest's access expiry. A null
// expires_at clears it; a value must be an RFC 3339 timestamp in the future.
// Format and ordering are checked in the service, not by tag, so both the
// invite path and this one share one rule.
type UpdateMemberExpiryRequest struct {
	MemberID  string  `json:"-"`
	ExpiresAt *string `json:"expires_at"`
}

type AddMembersRequest struct {
	WorkspaceId string   `json:"-"`
	Email       []string `json:"email" validate:"required,min=1,max=50,dive,email"`
	RoleId      string   `json:"role_id" validate:"required,uuid"`
	GroupId     string   `json:"group_id" validate:"omitempty,uuid"`
	// Optional access expiry for guest invitations, RFC 3339. Empty means the
	// member never expires.
	AccessExpiresAt string `json:"access_expires_at"`
}
