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
	ActorID    string `json:"actor_id"`
	ActorName  string `json:"actor_name"`
	ActorEmail string `json:"actor_email"`
	// The reader's current group. Both empty when the reader is no longer a
	// member of the room: content_events snapshots the actor, never the group.
	GroupID    string    `json:"group_id"`
	GroupName  string    `json:"group_name"`
	Opens      int64     `json:"opens"`
	PagesSeen  int64     `json:"pages_seen"`
	ReadMs     int64     `json:"read_ms"`
	LastReadAt time.Time `json:"last_read_at"`
}

type DocumentReadersResponse struct {
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name"`
	// Pages of the served version; 0 until a rendition exists. Events recorded
	// against an older version may name pages beyond it.
	PageCount   int32              `json:"page_count"`
	Readers     []ReaderEngagement `json:"readers"`
	TotalReadMs int64              `json:"total_read_ms"`
}

type ReaderPageEngagement struct {
	PageNo int32 `json:"page_no"`
	Opens  int64 `json:"opens"`
	ReadMs int64 `json:"read_ms"`
}

type ReaderDetailResponse struct {
	DocumentID   string                 `json:"document_id"`
	DocumentName string                 `json:"document_name"`
	PageCount    int32                  `json:"page_count"`
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
	GroupName  string
	PageNo     int32
	Opens      int64
	ReadMs     int64
}
