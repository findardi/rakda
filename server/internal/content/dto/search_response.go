package dto

type SearchFolderItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ParentID   string `json:"parent_id"`
	Breadcrumb string `json:"breadcrumb"`
}

type SearchDocumentItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FolderID   string `json:"folder_id"`
	Breadcrumb string `json:"breadcrumb"`
	Mime       string `json:"mime"`
}

type SearchResponse struct {
	Folders   []SearchFolderItem   `json:"folders"`
	Documents []SearchDocumentItem `json:"documents"`
}

type SearchLogRequest struct {
	Query string `json:"query"`
}
