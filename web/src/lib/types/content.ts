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

export type RenditionStatus = 'pending' | 'ready' | 'failed';

export interface DocumentData {
	id: string;
	folder_id: string;
	name: string;
	version_no: number;
	current_version_id: string;
	mime: string;
	size: number;
	rendition_status: RenditionStatus;
	version_count: number;
	// A staged version was uploaded but is not served yet: the pointer flips
	// only once its rendition succeeds. Manager-only — absent for guests.
	staged_version_id?: string;
	staged_version_no?: number;
	staged_rendition_status?: RenditionStatus;
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
	rendition_status: RenditionStatus;
	can_download: boolean;
	can_download_original: boolean;
	watermark_download_max_pages: number;
}

export interface DownloadLimitsData {
	watermark_download_max_pages: number;
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
	// Uploaded but not served yet: it becomes current once its rendition
	// succeeds. At most one version per document.
	is_staged: boolean;
	rendition_status: RenditionStatus;
	created_at: string;
}

// A new version keeps the document's name; only the bytes change. `file_name`
// is the picked file's name, sent for the server's type gate only.
export interface CompleteVersionPayload {
	storage_key: string;
	file_name: string;
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

// --- folder templates ---
// Curated server-side constants; both languages arrive together and the web
// picks per active locale. Applying rides the bulk engine: additive, existing
// folders are reused and completed (merge-down).

export interface TemplateNodeData {
	name_id: string;
	name_en: string;
	children?: TemplateNodeData[];
}

export interface FolderTemplateData {
	key: string;
	name_id: string;
	name_en: string;
	desc_id: string;
	desc_en: string;
	folder_count: number;
	folders: TemplateNodeData[];
}

export interface ApplyTemplateData {
	folders: BulkFolderResult[];
	created_count: number;
	skipped_count: number;
	template: string;
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

export interface TrashItem {
	id: string;
	name: string;
	deleted_by_name: string;
	deleted_at: string;
	purge_after: string;
}

export interface TrashFolder extends TrashItem {
	parent_name: string;
	parent_gone: boolean;
	folder_count: number;
	document_count: number;
}

export interface TrashDocument extends TrashItem {
	mime: string;
	size: number;
	folder_name: string;
	folder_gone: boolean;
}

export interface TrashData {
	folders: TrashFolder[];
	documents: TrashDocument[];
	retention_hours: number;
}

export interface RestoreData {
	id: string;
	name: string;
	renamed: boolean;
	folder_id: string;
	folder_name: string;
}

export interface DirectFolderAccess extends FolderAccessData {
	shadows: InheritedFolderAccess | null;
}

export interface FolderAccessPanel {
	folder_id: string;
	direct: DirectFolderAccess[];
	inherited: InheritedFolderAccess[];
}

export interface SearchFolderItem {
	id: string;
	name: string;
	parent_id: string;
	breadcrumb: string;
}

export interface SearchDocumentItem {
	id: string;
	name: string;
	folder_id: string;
	breadcrumb: string;
	mime: string;
}

export interface SearchData {
	folders: SearchFolderItem[];
	documents: SearchDocumentItem[];
	content: SearchContentHit[];
}

// Level 1 of content results: one row per document.
export interface SearchContentHit {
	document_id: string;
	document_name: string;
	folder_id: string;
	breadcrumb: string;
	page_count: number;
	hit_count: number;
}

// Level 2: the pages that matched inside one document.
export interface SearchContentPage {
	page_no: number;
	snippet: string;
}

export interface SearchContentPagesData {
	pages: SearchContentPage[];
}

// Search box for the viewer overlay (9-f): normalized 0..1, no text.
export interface SearchBox {
	x: number;
	y: number;
	w: number;
	h: number;
}

export interface SearchBoxPage {
	page_no: number;
	boxes: SearchBox[];
}

export interface SearchBoxesData {
	matches: SearchBoxPage[];
	pending: number[];
}

export interface DownloadJobData {
	id: string;
	document_id: string;
	version_id: string;
	document_name: string;
	file_name: string;
	version_no: number;
	page_count: number;
	status: 'pending' | 'ready' | 'failed';
	size_bytes: number;
	error?: string;
	created_at: string;
	completed_at?: string;
	expires_at: string;
}
