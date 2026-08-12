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

type CheckEmailRequest struct {
	Email string `json:"email" validate:"required"`
}

type AddMembersRequest struct {
	WorkspaceId string   `json:"-"`
	Email       []string `json:"email" validate:"required,min=1,max=50,dive,email"`
	RoleId      string   `json:"role_id" validate:"required,uuid"`
}
