package dto

type WorkspaceCreateRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
	OwnerID     string `json:"-"`
}

type WorkspaceUpdateStatusRequest struct {
	ID     string `json:"-"`
	Status string `json:"status" validate:"required"`
}

// WorkspaceHeroRequest picks a curated hero preset; empty means "automatic
// from the slug", the identity every room is born with.
type WorkspaceHeroRequest struct {
	ID     string `json:"-"`
	Preset string `json:"preset"`
}

type WorkspaceUpdateRequest struct {
	ID          string `json:"-"`
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}
