package dto

import "time"

type UploadURLResponse struct {
	UploadURL  string `json:"upload_url"`
	StorageKey string `json:"storage_key"`
}

type DocumentResponse struct {
	ID        string    `json:"id"`
	FolderID  string    `json:"folder_id"`
	Name      string    `json:"name"`
	VersionNo int32     `json:"version_no"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type VersionResponse struct {
	ID             string    `json:"id"`
	VersionNo      int32     `json:"version_no"`
	Mime           string    `json:"mime"`
	Size           int64     `json:"size"`
	UploadedBy     string    `json:"uploaded_by"`
	UploadedByName string    `json:"uploaded_by_name"`
	IsCurrent      bool      `json:"is_current"`
	CreatedAt      time.Time `json:"created_at"`
}

type DownloadURLResponse struct {
	DownloadURL string `json:"download_url"`
}

type ViewMetaResponse struct {
	DocumentID          string `json:"document_id"`
	Name                string `json:"name"`
	Mime                string `json:"mime"`
	VersionID           string `json:"version_id"`
	VersionNo           int32  `json:"version_no"`
	PageCount           int    `json:"page_count"`
	CanDownloadOriginal bool   `json:"can_download_original"`
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
