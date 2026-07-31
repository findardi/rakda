package dto

import "time"

type TrashFolderItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DeletedByName string    `json:"deleted_by_name"`
	DeletedAt     time.Time `json:"deleted_at"`
	PurgeAfter    time.Time `json:"purge_after"`
}

type TrashDocumentItem struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DeletedByName string    `json:"deleted_by_name"`
	DeletedAt     time.Time `json:"deleted_at"`
	PurgeAfter    time.Time `json:"purge_after"`
	Mime          string    `json:"mime"`
	Size          int64     `json:"size"`
}

type TrashListResponse struct {
	Folders   []TrashFolderItem   `json:"folders"`
	Documents []TrashDocumentItem `json:"documents"`
}
