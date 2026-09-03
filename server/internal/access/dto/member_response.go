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
	CreatedAt         time.Time `json:"created_at"`
}
