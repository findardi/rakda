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

	// Branding. Logo is a version token ("" = no logo) that keys the logo
	// endpoint's cache; HeroPreset is the chosen curated preset ("" = automatic);
	// HeroHue/HeroPhase are the resolved values the hero renders from either way,
	// so the web never recomputes an identity the server already owns.
	Logo       string `json:"logo"`
	HeroPreset string `json:"hero_preset"`
	HeroHue    int    `json:"hero_hue"`
	HeroPhase  int    `json:"hero_phase"`

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

// HeroPresetResponse is one curated hero identity the picker can preview.
type HeroPresetResponse struct {
	Key   string `json:"key"`
	Hue   int    `json:"hue"`
	Phase int    `json:"phase"`
}

type WorkspaceSummaryResponse struct {
	DocumentCount int64 `json:"document_count"`
	FolderCount   int64 `json:"folder_count"`
	GuestCount    int64 `json:"guest_count"`
}
