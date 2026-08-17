package dto

import "time"

type PageDurationEntry struct {
	PageNo     int32 `json:"page_no" validate:"required,min=1"`
	DurationMs int64 `json:"duration_ms" validate:"required,min=1"`
}

type RecordDurationsRequest struct {
	WorkspaceID string              `json:"-"`
	DocumentID  string              `json:"-"`
	VersionID   string              `json:"version_id"`
	Durations   []PageDurationEntry `json:"durations" validate:"required,min=1,max=200,dive"`
}

type ReaderEngagement struct {
	ActorID    string    `json:"actor_id"`
	ActorName  string    `json:"actor_name"`
	ActorEmail string    `json:"actor_email"`
	Opens      int64     `json:"opens"`
	PagesSeen  int64     `json:"pages_seen"`
	ReadMs     int64     `json:"read_ms"`
	LastReadAt time.Time `json:"last_read_at"`
}

type DocumentReadersResponse struct {
	DocumentID   string             `json:"document_id"`
	DocumentName string             `json:"document_name"`
	Readers      []ReaderEngagement `json:"readers"`
	TotalReadMs  int64              `json:"total_read_ms"`
}

type ReaderPageEngagement struct {
	PageNo int32 `json:"page_no"`
	Opens  int64 `json:"opens"`
	ReadMs int64 `json:"read_ms"`
}

type ReaderDetailResponse struct {
	DocumentID   string                 `json:"document_id"`
	DocumentName string                 `json:"document_name"`
	ActorID      string                 `json:"actor_id"`
	Pages        []ReaderPageEngagement `json:"pages"`
	TotalReadMs  int64                  `json:"total_read_ms"`
}

type EngagementBreakdown struct {
	DocumentName string
	Rows         []EngagementBreakdownRow
}

type EngagementBreakdownRow struct {
	ActorID    string
	ActorName  string
	ActorEmail string
	PageNo     int32
	Opens      int64
	ReadMs     int64
}
