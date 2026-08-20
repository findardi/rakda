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
	Content   []SearchContentHit   `json:"content"`
}

// SearchContentHit — level 1 dari hasil isi: satu baris per dokumen.
type SearchContentHit struct {
	DocumentID   string `json:"document_id"`
	DocumentName string `json:"document_name"`
	FolderID     string `json:"folder_id"`
	Breadcrumb   string `json:"breadcrumb"`
	PageCount    int32  `json:"page_count"`
	HitCount     int64  `json:"hit_count"`
}

// SearchContentPage — level 2: halaman-halaman kena di dalam satu dokumen.
type SearchContentPage struct {
	PageNo  int32  `json:"page_no"`
	Snippet string `json:"snippet"`
}

type SearchContentPagesResponse struct {
	Pages []SearchContentPage `json:"pages"`
}

type SearchLogRequest struct {
	Query string `json:"query"`
}
