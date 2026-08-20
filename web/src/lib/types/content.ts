export interface CreateFolderPayload {
	name: string;
	parent_id: string;
}

// `position` is "insert before whatever currently sits at index N"; omitting it
// appends. The server clamps out-of-range values instead of erroring.
export interface MoveFolderPayload {
	parent_id: string;
	position?: number;
}

export interface RenameFolderPayload {
	name: string;
}

export interface FolderData {
	id: string;
	workspace_id: string;
	parent_id: string;
	name: string;
	position: number;
	is_default: boolean;
	created_by: string;
	created_at: string;
	updated_at: string;
}

export interface FolderTreeNode {
	id: string;
	name: string;
	number: string;
	position: number;
	is_default: boolean;
	children: FolderTreeNode[];
}

export interface DocumentData {
	id: string;
	folder_id: string;
	name: string;
	version_no: number;
	mime: string;
	size: number;
	created_at: string;
	updated_at: string;
}

export interface UploadUrlData {
	upload_url: string;
	storage_key: string;
}

export interface ViewMetaData {
	document_id: string;
	name: string;
	mime: string;
	version_id: string;
	version_no: number;
	page_count: number;
	can_download: boolean;
	can_download_original: boolean;
}

export interface CompleteUploadPayload {
	name: string;
	storage_key: string;
}

// --- versions ---
// History is owner/admin only: the server answers 403 for a guest even though
// the route sits behind `document:view`. Rows arrive newest-first.
//
// `is_current` is the served version and is NOT necessarily the highest
// version_no: restore repoints `documents.current_version_id` at an existing
// version instead of copying it forward, so v2 can be current while v3 exists.
// Never infer the current version from the number.

export interface VersionData {
	id: string;
	version_no: number;
	mime: string;
	size: number;
	uploaded_by: string;
	uploaded_by_name: string;
	is_current: boolean;
	created_at: string;
}

// A new version keeps the document's name; only the bytes change, so the
// completion payload carries just the storage key it was uploaded under.
export interface CompleteVersionPayload {
	storage_key: string;
}

// --- bulk folder tree ---
// Server caps a request at 500 nodes total and 32 levels, reuses folders that
// already exist (`created: false`), and does the whole thing in one transaction.

export interface BulkFolderNode {
	name: string;
	children: BulkFolderNode[];
}

export interface BulkCreateFolderPayload {
	parent_id: string;
	folders: BulkFolderNode[];
}

export interface BulkFolderResult {
	path: string;
	id: string;
	created: boolean;
}

export interface BulkCreateFolderData {
	folders: BulkFolderResult[];
}

// --- multipart / resumable upload ---
// `upload_id` + `storage_key` are the whole resume handle: the server keeps no
// upload-session row, so losing this pair strands the upload in object storage.

export interface InitMultipartPayload {
	name: string;
	size: number;
}

export interface InitMultipartData {
	upload_id: string;
	storage_key: string;
	part_size: number;
	part_count: number;
}

export interface MultipartPartUrlsPayload {
	upload_id: string;
	storage_key: string;
	part_numbers: number[];
}

export interface MultipartPartUrl {
	part_number: number;
	url: string;
}

export interface MultipartPartUrlsData {
	urls: MultipartPartUrl[];
}

export interface UploadedPart {
	part_number: number;
	etag: string;
	size: number;
}

export interface MultipartPartsData {
	parts: UploadedPart[];
}

export interface CompletedPart {
	part_number: number;
	etag: string;
}

export interface CompleteMultipartPayload {
	upload_id: string;
	name: string;
	storage_key: string;
	content_type: string;
	parts: CompletedPart[];
}

export interface AbortMultipartPayload {
	upload_id: string;
	storage_key: string;
}

export interface MoveDocumentPayload {
	folder_id: string;
	position?: number;
}

export interface FolderAccessData {
	folder_id: string;
	group_id: string;
	group_name: string;
	can_view: boolean;
	can_download: boolean;
	can_watermark: boolean;
	can_download_original: boolean;
}

export interface SetFolderAccessPayload {
	group_id: string;
	can_view: boolean;
	can_download: boolean;
	can_watermark: boolean;
	can_download_original: boolean;
}

export interface InheritedFolderAccess extends FolderAccessData {
	source_folder_id: string;
	source_folder_name: string;
}

// --- trash ---

export interface TrashFolder {
	id: string;
	name: string;
	deleted_by_name: string;
	deleted_at: string;
	purge_after: string;
}

export interface TrashDocument extends TrashFolder {
	mime: string;
	size: number;
}

export interface TrashData {
	folders: TrashFolder[];
	documents: TrashDocument[];
}

export interface DirectFolderAccess extends FolderAccessData {
	shadows: InheritedFolderAccess | null;
}

export interface FolderAccessPanel {
	folder_id: string;
	direct: DirectFolderAccess[];
	inherited: InheritedFolderAccess[];
}
