package dto

import "time"

type TrashFolderItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DeletedByName string    `json:"deleted_by_name"`
	DeletedAt     time.Time `json:"deleted_at"`
	PurgeAfter    time.Time `json:"purge_after"`
	ParentName    string    `json:"parent_name"`
	ParentGone    bool      `json:"parent_gone"`
	FolderCount   int64     `json:"folder_count"`
	DocumentCount int64     `json:"document_count"`
}

type TrashDocumentItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DeletedByName string    `json:"deleted_by_name"`
	DeletedAt     time.Time `json:"deleted_at"`
	PurgeAfter    time.Time `json:"purge_after"`
	Mime          string    `json:"mime"`
	Size          int64     `json:"size"`
	FolderName    string    `json:"folder_name"`
	FolderGone    bool      `json:"folder_gone"`
}

type TrashListResponse struct {
	Folders        []TrashFolderItem   `json:"folders"`
	Documents      []TrashDocumentItem `json:"documents"`
	RetentionHours int64               `json:"retention_hours"`
}

type RestoreResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Renamed    bool   `json:"renamed"`
	FolderID   string `json:"folder_id"`
	FolderName string `json:"folder_name"`
}
