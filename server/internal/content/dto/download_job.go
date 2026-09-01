package dto

import "time"

type DownloadJobResponse struct {
	ID           string     `json:"id"`
	DocumentID   string     `json:"document_id"`
	VersionID    string     `json:"version_id"`
	DocumentName string     `json:"document_name"`
	FileName     string     `json:"file_name"`
	VersionNo    int32      `json:"version_no"`
	PageCount    int32      `json:"page_count"`
	Status       string     `json:"status"`
	SizeBytes    int64      `json:"size_bytes"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
}
