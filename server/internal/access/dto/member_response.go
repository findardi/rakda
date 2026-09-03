package dto

import "time"

type WorkspaceMemberResponse struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	RoleID      string    `json:"role_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type GetMemberResponse struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	RoleID      string    `json:"role_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	RoleName    string    `json:"role_name"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	GroupNames  []string  `json:"group_names"`
	// Nil when the membership never expires. Status reads "expired" once this
	// date has passed; the row itself is untouched.
	ExpiresAt *time.Time `json:"expires_at"`
}

// MyAccessResponse is the caller's own standing in a room: the middleware
// Membership plus the expiry only the caller needs to see.
type MyAccessResponse struct {
	Role            string     `json:"role"`
	Permissions     []string   `json:"permissions"`
	Status          string     `json:"status"`
	WorkspaceStatus string     `json:"workspace_status"`
	ExpiresAt       *time.Time `json:"expires_at"`
}

type AddMembersResponse struct {
	Email   string `json:"email"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

type InvitationResponse struct {
	ID                string    `json:"id"`
	WorkspaceID       string    `json:"workspace_id"`
	Email             string    `json:"email"`
	RoleID            string    `json:"role_id"`
	RoleName          string    `json:"role_name"`
	GroupID           string    `json:"group_id"`
	GroupName         string    `json:"group_name"`
	UserID            string    `json:"user_id"`
	InvitedBy         string    `json:"invited_by"`
	InvitedByUsername string    `json:"invited_by_username"`
	Status            string    `json:"status"`
	ExpiresAt         time.Time `json:"expires_at"`
	// The access window the member gets on acceptance; nil = unlimited.
	AccessExpiresAt *time.Time `json:"access_expires_at"`
	CreatedAt       time.Time  `json:"created_at"`
}
