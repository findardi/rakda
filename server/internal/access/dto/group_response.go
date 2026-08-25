package dto

import "time"

type GroupResponse struct {
	ID              string    `json:"id"`
	WorkspaceID     string    `json:"workspace_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	IsDefault       bool      `json:"is_default"`
	QAEnabled       bool      `json:"qa_enabled"`
	QAQuestionLimit *int32    `json:"qa_question_limit"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type GroupMemberResponse struct {
	GroupID   string    `json:"group_id"`
	MemberID  string    `json:"member_id"`
	CreatedAt time.Time `json:"created_at"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	RoleName  string    `json:"role_name"`
	GroupName string    `json:"group_name"`
}
