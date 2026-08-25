package dto

type TemplateNodeResponse struct {
	NameID   string                 `json:"name_id"`
	NameEN   string                 `json:"name_en"`
	Children []TemplateNodeResponse `json:"children,omitempty"`
}

type FolderTemplateResponse struct {
	Key         string                 `json:"key"`
	NameID      string                 `json:"name_id"`
	NameEN      string                 `json:"name_en"`
	DescID      string                 `json:"desc_id"`
	DescEN      string                 `json:"desc_en"`
	FolderCount int                    `json:"folder_count"`
	Folders     []TemplateNodeResponse `json:"folders"`
}

type ApplyTemplateRequest struct {
	WorkspaceID string `json:"-"`
	TemplateKey string `json:"-"`
	Locale      string `json:"locale" validate:"required,oneof=id en"`
}

type ApplyTemplateResponse struct {
	Folders      []BulkFolderResult `json:"folders"`
	CreatedCount int                `json:"created_count"`
	SkippedCount int                `json:"skipped_count"`
	Template     string             `json:"template"`
}
