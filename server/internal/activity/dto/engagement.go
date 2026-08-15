package dto

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

type PageEngagement struct {
	PageNo        int32 `json:"page_no"`
	Opens         int64 `json:"opens"`
	RawHits       int64 `json:"raw_hits"`
	UniqueViewers int64 `json:"unique_viewers"`
	ReadMs        int64 `json:"read_ms"`
}

type DocumentEngagementResponse struct {
	DocumentID   string           `json:"document_id"`
	DocumentName string           `json:"document_name"`
	Pages        []PageEngagement `json:"pages"`
	TotalOpens   int64            `json:"total_opens"`
	TotalReadMs  int64            `json:"total_read_ms"`
}
