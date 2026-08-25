package dto

type CreateGroupRequest struct {
	WorkspaceID string `json:"-"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateGroupRequest struct {
	GroupID     string `json:"-"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateGroupQARequest struct {
	WorkspaceID   string `json:"-"`
	GroupID       string `json:"-"`
	QAEnabled     *bool  `json:"qa_enabled" validate:"required"`
	QuestionLimit *int32 `json:"question_limit" validate:"omitempty,min=0"`
}

type GroupMemberRequest struct {
	MemberID []string `json:"member_id" validate:"required"`
	GroupID  string   `json:"-"`
}
