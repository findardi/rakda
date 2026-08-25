package dto

import "time"

type SubmitQuestionRequest struct {
	WorkspaceID string `json:"-"`
	Subject     string `json:"subject" validate:"required,max=150"`
	Body        string `json:"body" validate:"required,max=5000"`
	DocumentID  string `json:"document_id" validate:"omitempty,uuid"`
	FolderID    string `json:"folder_id" validate:"omitempty,uuid"`
}

type ListQuestionsRequest struct {
	WorkspaceID string
	Limit       int
	Cursor      string
	Status      string
	GroupID     string
}

type ReplyQuestionRequest struct {
	WorkspaceID string `json:"-"`
	QuestionID  string `json:"-"`
	Body        string `json:"body" validate:"required,max=5000"`
}

type CreateFaqRequest struct {
	WorkspaceID      string `json:"-"`
	QuestionText     string `json:"question_text" validate:"required,max=150"`
	AnswerText       string `json:"answer_text" validate:"required,max=5000"`
	SourceQuestionID string `json:"source_question_id" validate:"omitempty,uuid"`
}

type QuestionListItem struct {
	ID         string    `json:"id"`
	Number     int32     `json:"number"`
	Subject    string    `json:"subject"`
	Status     string    `json:"status"`
	GroupID    string    `json:"group_id,omitempty"`
	GroupName  string    `json:"group_name"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	ReplyCount int64     `json:"reply_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type ListQuestionsResponse struct {
	Items          []QuestionListItem `json:"items"`
	NextCursor     string             `json:"next_cursor"`
	QuestionCount  int64              `json:"question_count"`
	FaqCount       int64              `json:"faq_count"`
	QAEnabled      bool               `json:"qa_enabled"`
	QuestionLimit  *int32             `json:"question_limit,omitempty"`
	QuotaRemaining *int32             `json:"quota_remaining,omitempty"`
	WaitingCount   *int64             `json:"waiting_count,omitempty"`
}

type ReferenceChip struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FolderID string `json:"folder_id,omitempty"`
}

type ReplyResponse struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	AuthorRole string    `json:"author_role"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type QuestionThreadResponse struct {
	ID          string          `json:"id"`
	Number      int32           `json:"number"`
	Subject     string          `json:"subject"`
	Body        string          `json:"body"`
	Status      string          `json:"status"`
	GroupID     string          `json:"group_id,omitempty"`
	GroupName   string          `json:"group_name"`
	AuthorID    string          `json:"author_id"`
	AuthorName  string          `json:"author_name"`
	CreatedAt   time.Time       `json:"created_at"`
	DocumentRef *ReferenceChip  `json:"document_ref,omitempty"`
	FolderRef   *ReferenceChip  `json:"folder_ref,omitempty"`
	Replies     []ReplyResponse `json:"replies"`
}

type ReplyResult struct {
	Reply          ReplyResponse `json:"reply"`
	QuestionStatus string        `json:"question_status"`
}

type WaitingCountResponse struct {
	WaitingCount int64 `json:"waiting_count"`
}

type FaqResponse struct {
	ID           string    `json:"id"`
	QuestionText string    `json:"question_text"`
	AnswerText   string    `json:"answer_text"`
	CreatedAt    time.Time `json:"created_at"`
}
