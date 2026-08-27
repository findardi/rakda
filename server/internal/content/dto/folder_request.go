package dto

type CreateFolderRequest struct {
	WorkspaceID string `json:"-"`
	CreatedBy   string `json:"-"`
	ParentID    string `json:"parent_id"`
	Name        string `json:"name" validate:"required,max=200"`
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
	Name        string `json:"name" validate:"required,max=200"`
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

type BulkDeleteFoldersRequest struct {
	WorkspaceID string   `json:"-"`
	FolderIDs   []string `json:"folder_ids" validate:"required,min=1,max=100,dive,uuid"`
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
