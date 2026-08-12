package dto

type CreateFolderRequest struct {
	WorkspaceID string `json:"-"`
	CreatedBy   string `json:"-"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name" validate:"required"`
}

type MoveFolderRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	ParentID    string `json:"parent_id"`
	Position    *int   `json:"position"`
}

type RenameFolderRequest struct {
	WorkspaceID string `json:"-"`
	FolderID    string `json:"-"`
	Name        string `json:"name" validate:"required"`
}

type SetFolderAccessRequest struct {
	WorkspaceID         string `json:"-"`
	FolderID            string `json:"-"`
	GroupID             string `json:"group_id" validate:"required,uuid"`
	CanView             bool   `json:"can_view"`
	CanDownload         bool   `json:"can_download"`
	CanWatermark        bool   `json:"can_watermark"`
	CanDownloadOriginal bool   `json:"can_download_original"`
}

type BulkFolderNode struct {
	Name     string           `json:"name"`
	Children []BulkFolderNode `json:"children"`
}

type BulkCreateFolderRequest struct {
	WorkspaceID string           `json:"-"`
	CreatedBy   string           `json:"-"`
	ParentID    string           `json:"parent_id"`
	Folders     []BulkFolderNode `json:"folders" validate:"required,min=1"`
}
