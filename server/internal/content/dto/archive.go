package dto

import "time"

const (
	ArchiveScopeRoom    = "room"
	ArchiveScopeFolders = "folders"
)

// CreateArchiveRequest memilih cakupan paket. folder_ids yang tidak dikirim
// (atau null) berarti seluruh ruang; setiap id yang dikirim menyertakan
// subtree folder itu. `[]` ditolak oleh min=1 — omitempty pada validator
// hanya melewati slice nil, jadi daftar kosong yang eksplisit adalah kesalahan
// klien, bukan sinonim "seluruh ruang".
type CreateArchiveRequest struct {
	FolderIDs []string `json:"folder_ids" validate:"omitempty,min=1,max=100,dive,uuid"`
}

type ArchiveResponse struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	RequestedBy     string     `json:"requested_by"`
	RequestedByName string     `json:"requested_by_name"`
	SizeBytes       int64      `json:"size_bytes"`
	ChecksumSHA256  string     `json:"checksum_sha256"`
	DocumentCount   int32      `json:"document_count"`
	MissingCount    int32      `json:"missing_count"`
	Error           string     `json:"error"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	ExpiresAt       time.Time  `json:"expires_at"`

	// Scope adalah "room" atau "folders". Kedua slice di bawah nil untuk
	// "room"; untuk "folders" keduanya sejajar, urutan sesuai permintaan, dan
	// nama adalah potret saat pembuatan (folder bisa diganti nama sesudahnya).
	Scope            string   `json:"scope"`
	ScopeFolderIDs   []string `json:"scope_folder_ids"`
	ScopeFolderNames []string `json:"scope_folder_names"`
}
