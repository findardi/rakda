package dto

import "time"

type ArchiveResponse struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	RequestedBy     string     `json:"requested_by"`
	RequestedByName string     `json:"requested_by_name"`
	SizeBytes       int64      `json:"size_bytes"`
	ChecksumSHA256  string     `json:"checksum_sha256"`
	DocumentCount   int32      `json:"document_count"`
	MissingCount    int32      `json:"missing_count"`
	Error           string     `json:"error"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
}
