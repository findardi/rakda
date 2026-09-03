package dto

import "time"

type UploadURLResponse struct {
	UploadURL  string `json:"upload_url"`
	StorageKey string `json:"storage_key"`
}

const (
	RenditionPending = "pending"
	RenditionReady   = "ready"
	RenditionFailed  = "failed"
)

type DocumentResponse struct {
	ID               string `json:"id"`
	FolderID         string `json:"folder_id"`
	Name             string `json:"name"`
	VersionNo        int32  `json:"version_no"`
	CurrentVersionID string `json:"current_version_id"`
	Mime             string `json:"mime"`
	Size             int64  `json:"size"`
	RenditionStatus  string `json:"rendition_status"`
	VersionCount     int32  `json:"version_count"`
	// Staged fields describe an uploaded version that is not served yet: it
	// becomes current only once its rendition succeeds. Manager-only — the
	// service leaves them empty for guests.
	StagedVersionID       string    `json:"staged_version_id,omitempty"`
	StagedVersionNo       *int32    `json:"staged_version_no,omitempty"`
	StagedRenditionStatus string    `json:"staged_rendition_status,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type VersionResponse struct {
	ID              string    `json:"id"`
	VersionNo       int32     `json:"version_no"`
	Mime            string    `json:"mime"`
	Size            int64     `json:"size"`
	UploadedBy      string    `json:"uploaded_by"`
	UploadedByName  string    `json:"uploaded_by_name"`
	IsCurrent       bool      `json:"is_current"`
	IsStaged        bool      `json:"is_staged"`
	RenditionStatus string    `json:"rendition_status"`
	CreatedAt       time.Time `json:"created_at"`
}

type ViewMetaResponse struct {
	DocumentID                string `json:"document_id"`
	Name                      string `json:"name"`
	Mime                      string `json:"mime"`
	VersionID                 string `json:"version_id"`
	VersionNo                 int32  `json:"version_no"`
	PageCount                 int    `json:"page_count"`
	RenditionStatus           string `json:"rendition_status"`
	CanDownload               bool   `json:"can_download"`
	CanDownloadOriginal       bool   `json:"can_download_original"`
	WatermarkDownloadMaxPages int    `json:"watermark_download_max_pages"`
}

type DownloadLimitsResponse struct {
	WatermarkDownloadMaxPages int `json:"watermark_download_max_pages"`
}

type InitMultipartResponse struct {
	UploadID   string `json:"upload_id"`
	StorageKey string `json:"storage_key"`
	PartSize   int64  `json:"part_size"`
	PartCount  int    `json:"part_count"`
}

type MultipartPartURL struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

type MultipartPartURLsResponse struct {
	URLs []MultipartPartURL `json:"urls"`
}

type UploadedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

type MultipartPartsResponse struct {
	Parts []UploadedPart `json:"parts"`
}
