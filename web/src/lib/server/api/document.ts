import type { ApiResult } from '$lib/types';
import type {
	AbortMultipartPayload,
	CompleteMultipartPayload,
	CompleteUploadPayload,
	CompleteVersionPayload,
	DocumentData,
	DownloadJobData,
	DownloadLimitsData,
	InitMultipartData,
	InitMultipartPayload,
	MoveDocumentPayload,
	MultipartPartsData,
	MultipartPartUrlsData,
	MultipartPartUrlsPayload,
	UploadUrlData,
	VersionData,
	ViewMetaData
} from '$lib/types/content';
import { API_URL, del, get, patch, post, upstreamHeaders } from './client';

const foldersBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/folders`;
const documentsBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/documents`;

export const listDocuments = (token: string, workspaceId: string, folderId: string) =>
	get<DocumentData[]>(`${foldersBase(workspaceId)}/${folderId}/documents`, token);

export const requestUploadUrl = (
	token: string,
	workspaceId: string,
	folderId: string,
	storageKey?: string
) =>
	post<UploadUrlData>(
		`${foldersBase(workspaceId)}/${folderId}/documents/upload-url`,
		storageKey ? { storage_key: storageKey } : undefined,
		token
	);

export const completeUpload = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: CompleteUploadPayload
) => post<DocumentData>(`${foldersBase(workspaceId)}/${folderId}/documents`, p, token);

// `?version=` is optional everywhere it appears: omitting it means the current
// version, which is what a guest is limited to. Passing a non-current version id
// is owner/admin only — the server answers 403 — and a malformed one is a 404,
// never a 500.
const versionQuery = (versionId?: string) =>
	versionId ? `?version=${encodeURIComponent(versionId)}` : '';

// Raw upstream response for the download proxy. This endpoint streams a PDF
// (Content-Type application/pdf, Content-Disposition attachment) — watermarked
// for `can_download`, clean for `can_download_original` — not a JSON envelope,
// so it bypasses the typed client entirely.
export const downloadDocument = (
	token: string,
	workspaceId: string,
	documentId: string,
	versionId?: string
) =>
	fetch(
		`${API_URL}${documentsBase(workspaceId)}/${documentId}/download${versionQuery(versionId)}`,
		{ headers: upstreamHeaders(token) }
	);

export const getDownloadLimits = (token: string, workspaceId: string) =>
	get<DownloadLimitsData>(`/content/workspaces/${workspaceId}/download-limits`, token);

export const getViewMeta = (
	token: string,
	workspaceId: string,
	documentId: string,
	versionId?: string
) =>
	get<ViewMetaData>(
		`${documentsBase(workspaceId)}/${documentId}/view${versionQuery(versionId)}`,
		token
	);

// Raw upstream response for the page-image proxy. This endpoint streams a
// watermarked PNG (Content-Type image/png), not a JSON envelope, so it bypasses
// the typed client entirely — the proxy route forwards the body and status.
export const fetchViewPage = (
	token: string,
	workspaceId: string,
	documentId: string,
	page: number | string,
	versionId?: string
) =>
	fetch(
		`${API_URL}${documentsBase(workspaceId)}/${documentId}/pages/${page}${versionQuery(versionId)}`,
		{ headers: upstreamHeaders(token) }
	);

// --- versions ----------------------------------------------------------
// Version uploads have no multipart path upstream: a new version is one
// presigned PUT, then a completion call. Large files therefore cannot resume
// the way a first upload can.

const versionsBase = (workspaceId: string, documentId: string) =>
	`${documentsBase(workspaceId)}/${documentId}/versions`;

export const listVersions = (token: string, workspaceId: string, documentId: string) =>
	get<VersionData[]>(versionsBase(workspaceId, documentId), token);

export const requestVersionUpload = (token: string, workspaceId: string, documentId: string) =>
	post<UploadUrlData>(`${versionsBase(workspaceId, documentId)}/upload-url`, undefined, token);

export const completeVersion = (
	token: string,
	workspaceId: string,
	documentId: string,
	p: CompleteVersionPayload
) => post<DocumentData>(versionsBase(workspaceId, documentId), p, token);

// Restore repoints `current_version_id` at the chosen version — no row is
// copied, nothing is overwritten, so the act is itself undoable. Restoring the
// version that is already current is a 409.
export const restoreVersion = (
	token: string,
	workspaceId: string,
	documentId: string,
	versionId: string
) =>
	post<DocumentData>(
		`${versionsBase(workspaceId, documentId)}/${versionId}/restore`,
		undefined,
		token
	);

// Clears a recorded rendition failure and restarts the conversion server-side.
// Owner/admin only upstream; the UI never offers it to guests.
export const retryRendition = (
	token: string,
	workspaceId: string,
	documentId: string,
	versionId: string
) =>
	post<null>(
		`${versionsBase(workspaceId, documentId)}/${versionId}/retry-rendition`,
		undefined,
		token
	);

const multipartBase = (workspaceId: string, folderId: string) =>
	`${foldersBase(workspaceId)}/${folderId}/documents/multipart`;

export const initMultipart = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: InitMultipartPayload
) => post<InitMultipartData>(`${multipartBase(workspaceId, folderId)}/init`, p, token);

// Upstream caps a batch at 100 part numbers and the presigned URLs expire in
// 15 minutes, so callers request them in waves rather than all up front.
export const multipartPartUrls = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: MultipartPartUrlsPayload
) => post<MultipartPartUrlsData>(`${multipartBase(workspaceId, folderId)}/part-urls`, p, token);

// The resume read: which parts object storage already holds. Query string here,
// unlike abort, which takes the same pair as a JSON body.
export function multipartParts(
	token: string,
	workspaceId: string,
	folderId: string,
	uploadId: string,
	storageKey: string
): Promise<ApiResult<MultipartPartsData>> {
	const q = new URLSearchParams({ upload_id: uploadId, storage_key: storageKey });
	return get<MultipartPartsData>(`${multipartBase(workspaceId, folderId)}/parts?${q}`, token);
}

export const completeMultipart = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: CompleteMultipartPayload
) => post<DocumentData>(`${multipartBase(workspaceId, folderId)}/complete`, p, token);

export const abortMultipart = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: AbortMultipartPayload
) => del<null>(multipartBase(workspaceId, folderId), token, p);

export const moveDocument = (
	token: string,
	workspaceId: string,
	documentId: string,
	p: MoveDocumentPayload
) => patch<null>(`${documentsBase(workspaceId)}/${documentId}/move`, p, token);

export const deleteDocument = (token: string, workspaceId: string, documentId: string) =>
	del<null>(`${documentsBase(workspaceId)}/${documentId}`, token);

// Atomic: one unknown/foreign id fails the whole batch with a 404 — nothing
// is half-deleted. Soft-delete to trash, same as the single delete.
export const bulkDeleteDocuments = (token: string, workspaceId: string, documentIds: string[]) =>
	post<null>(`${documentsBase(workspaceId)}/bulk-delete`, { document_ids: documentIds }, token);

export const listDownloadJobs = (token: string, workspaceId: string) =>
	get<DownloadJobData[]>(`/content/workspaces/${workspaceId}/download-jobs`, token);

export const getDownloadJob = (token: string, workspaceId: string, jobId: string) =>
	get<DownloadJobData>(`/content/workspaces/${workspaceId}/download-jobs/${jobId}`, token);

export function downloadJobArtifact(
	token: string,
	workspaceId: string,
	jobId: string,
	range?: string | null
): Promise<Response> {
	const headers = upstreamHeaders(token);
	if (range) headers.range = range;

	return fetch(`${API_URL}/content/workspaces/${workspaceId}/download-jobs/${jobId}/download`, {
		headers
	});
}
