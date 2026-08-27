package dto

import "time"

type WorkspaceResponse struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Diisi hanya oleh daftar ruangan: peran pemanggil di ruangan itu, dan
	// stempel aktivitas terakhir yang memberi daftar urutan bermakna.
	Role           string     `json:"role,omitempty"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
}

// WorkspaceListResponse membawa kuota bersama daftarnya: keduanya lahir dari
// query yang sama, jadi klien tidak perlu request kedua — dan tidak perlu
// menebak angka batasnya sendiri.
type WorkspaceListResponse struct {
	Workspaces []WorkspaceResponse `json:"workspaces"`
	OwnedCount int                 `json:"owned_count"`
	OwnedLimit int                 `json:"owned_limit"`
}
