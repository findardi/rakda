package dto

type UploadURLRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	StorageKey  string `json:"storage_key"`
}

type CompleteUploadRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	UploadedBy  string `json:"-"`
	Name        string `json:"name" validate:"required"`
	StorageKey  string `json:"storage_key" validate:"required"`
}

type CompleteVersionRequest struct {
	WorkspaceID string `json:"-"`
	DocumentID  string `json:"-"`
	UploadedBy  string `json:"-"`
	StorageKey  string `json:"storage_key" validate:"required"`
}

type MoveDocumentRequest struct {
	WorkspaceID string `json:"-"`
	DocumentID  string `json:"-"`
	FolderID    string `json:"folder_id" validate:"required,uuid"`
	Position    *int   `json:"position"`
}

type ViewPageRequest struct {
	WorkspaceID   string
	DocumentID    string
	VersionID     string
	Page          int
	MarkPrimary   string
	MarkSecondary string
}

type MultipartPart struct {
	PartNumber int    `json:"part_number" validate:"required,gt=0"`
	ETag       string `json:"etag" validate:"required"`
}
type CompleteMultipartRequest struct {
	WorkspaceID string          `json:"-"`
	FolderID    string          `json:"-"`
	UploadedBy  string          `json:"-"`
	UploadID    string          `json:"upload_id" validate:"required"`
	Name        string          `json:"name" validate:"required"`
	StorageKey  string          `json:"storage_key" validate:"required"`
	ContentType string          `json:"content_type"`
	Parts       []MultipartPart `json:"parts" validate:"required,min=1,dive"`
}

type InitMultipartRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	Name        string `json:"name" validate:"required"`
	Size        int64  `json:"size" validate:"required,gt=0"`
}

type MultipartPartURLsRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	UploadID    string `json:"upload_id" validate:"required"`
	StorageKey  string `json:"storage_key" validate:"required"`
	PartNumbers []int  `json:"part_numbers" validate:"required,min=1"`
}

type ListPartsRequest struct {
	WorkspaceID string
	FolderID    string
	UploadID    string
	StorageKey  string
}

type AbortMultipartRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	UploadID    string `json:"upload_id" validate:"required"`
	StorageKey  string `json:"storage_key" validate:"required"`
}
