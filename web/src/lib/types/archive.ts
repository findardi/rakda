// Archive export contract — mirrors internal/content/dto/archive.go.

export type ArchiveStatus = 'pending' | 'ready' | 'failed';

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
}
