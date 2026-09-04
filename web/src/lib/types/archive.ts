// Archive export contract — mirrors internal/content/dto/archive.go.

export type ArchiveStatus = 'pending' | 'ready' | 'failed';

// `room` = the whole room; `folders` = the subtrees of the listed folders. The
// names are a snapshot taken at creation, so the list still reads correctly
// after a folder is renamed or deleted.
export type ArchiveScope = 'room' | 'folders';

export interface ArchiveData {
	id: string;
	status: ArchiveStatus;
	requested_by: string;
	requested_by_name: string;
	size_bytes: number;
	checksum_sha256: string;
	document_count: number;
	missing_count: number;
	error: string;
	created_at: string;
	completed_at: string | null;
	expires_at: string;
	scope: ArchiveScope;
	scope_folder_ids: string[] | null;
	scope_folder_names: string[] | null;
}
