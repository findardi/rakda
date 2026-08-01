import type { ApiResult } from '$lib/types';
import type {
	AbortMultipartPayload,
	CompleteMultipartPayload,
	CompleteUploadPayload,
	CompleteVersionPayload,
	DocumentData,
	DownloadUrlData,
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
import { API_URL, del, get, patch, post } from './client';

const foldersBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/folders`;
const documentsBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/documents`;

export function listDocuments(
	token: string,
	workspaceId: string,
	folderId: string
): Promise<ApiResult<DocumentData[]>> {
	return get<DocumentData[]>(`${foldersBase(workspaceId)}/${folderId}/documents`, token);
}

export function requestUploadUrl(
	token: string,
	workspaceId: string,
	folderId: string,
	storageKey?: string
): Promise<ApiResult<UploadUrlData>> {
	return post<UploadUrlData>(
		`${foldersBase(workspaceId)}/${folderId}/documents/upload-url`,
		storageKey ? { storage_key: storageKey } : undefined,
		token
	);
}

export function completeUpload(
	token: string,
	workspaceId: string,
	folderId: string,
	p: CompleteUploadPayload
): Promise<ApiResult<DocumentData>> {
	return post<DocumentData>(`${foldersBase(workspaceId)}/${folderId}/documents`, p, token);
}

// `?version=` is optional everywhere it appears: omitting it means the current
// version, which is what a guest is limited to. Passing a non-current version id
// is owner/admin only — the server answers 403 — and a malformed one is a 404,
// never a 500.
const versionQuery = (versionId?: string) =>
	versionId ? `?version=${encodeURIComponent(versionId)}` : '';

export function getDownloadUrl(
	token: string,
	workspaceId: string,
	documentId: string,
	versionId?: string
): Promise<ApiResult<DownloadUrlData>> {
	return get<DownloadUrlData>(
		`${documentsBase(workspaceId)}/${documentId}/download${versionQuery(versionId)}`,
		token
	);
}

export function getViewMeta(
	token: string,
	workspaceId: string,
	documentId: string,
	versionId?: string
): Promise<ApiResult<ViewMetaData>> {
	return get<ViewMetaData>(
		`${documentsBase(workspaceId)}/${documentId}/view${versionQuery(versionId)}`,
		token
	);
}

// Raw upstream response for the page-image proxy. This endpoint streams a
// watermarked PNG (Content-Type image/png), not a JSON envelope, so it bypasses
// the typed client entirely — the proxy route forwards the body and status.
export function fetchViewPage(
	token: string,
	workspaceId: string,
	documentId: string,
	page: number | string,
	versionId?: string
): Promise<Response> {
	return fetch(
		`${API_URL}${documentsBase(workspaceId)}/${documentId}/pages/${page}${versionQuery(versionId)}`,
		{ headers: { authorization: `Bearer ${token}` } }
	);
}

// --- versions ----------------------------------------------------------
// Version uploads have no multipart path upstream: a new version is one
// presigned PUT, then a completion call. Large files therefore cannot resume
// the way a first upload can.

const versionsBase = (workspaceId: string, documentId: string) =>
	`${documentsBase(workspaceId)}/${documentId}/versions`;

export function listVersions(
	token: string,
	workspaceId: string,
	documentId: string
): Promise<ApiResult<VersionData[]>> {
	return get<VersionData[]>(versionsBase(workspaceId, documentId), token);
}

export function requestVersionUpload(
	token: string,
	workspaceId: string,
	documentId: string
): Promise<ApiResult<UploadUrlData>> {
	return post<UploadUrlData>(
		`${versionsBase(workspaceId, documentId)}/upload-url`,
		undefined,
		token
	);
}

export function completeVersion(
	token: string,
	workspaceId: string,
	documentId: string,
	p: CompleteVersionPayload
): Promise<ApiResult<DocumentData>> {
	return post<DocumentData>(versionsBase(workspaceId, documentId), p, token);
}

// Restore copies the chosen version forward as a new current one, so nothing is
// overwritten and the act is itself undoable. Restoring the version that is
// already current is a 409.
export function restoreVersion(
	token: string,
	workspaceId: string,
	documentId: string,
	versionId: string
): Promise<ApiResult<DocumentData>> {
	return post<DocumentData>(
		`${versionsBase(workspaceId, documentId)}/${versionId}/restore`,
		undefined,
		token
	);
}

const multipartBase = (workspaceId: string, folderId: string) =>
	`${foldersBase(workspaceId)}/${folderId}/documents/multipart`;

export function initMultipart(
	token: string,
	workspaceId: string,
	folderId: string,
	p: InitMultipartPayload
): Promise<ApiResult<InitMultipartData>> {
	return post<InitMultipartData>(`${multipartBase(workspaceId, folderId)}/init`, p, token);
}

// Upstream caps a batch at 100 part numbers and the presigned URLs expire in
// 15 minutes, so callers request them in waves rather than all up front.
export function multipartPartUrls(
	token: string,
	workspaceId: string,
	folderId: string,
	p: MultipartPartUrlsPayload
): Promise<ApiResult<MultipartPartUrlsData>> {
	return post<MultipartPartUrlsData>(`${multipartBase(workspaceId, folderId)}/part-urls`, p, token);
}

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

export function completeMultipart(
	token: string,
	workspaceId: string,
	folderId: string,
	p: CompleteMultipartPayload
): Promise<ApiResult<DocumentData>> {
	return post<DocumentData>(`${multipartBase(workspaceId, folderId)}/complete`, p, token);
}

export function abortMultipart(
	token: string,
	workspaceId: string,
	folderId: string,
	p: AbortMultipartPayload
): Promise<ApiResult<null>> {
	return del<null>(multipartBase(workspaceId, folderId), token, p);
}

export function moveDocument(
	token: string,
	workspaceId: string,
	documentId: string,
	p: MoveDocumentPayload
): Promise<ApiResult<null>> {
	return patch<null>(`${documentsBase(workspaceId)}/${documentId}/move`, p, token);
}

export function deleteDocument(
	token: string,
	workspaceId: string,
	documentId: string
): Promise<ApiResult<null>> {
	return del<null>(`${documentsBase(workspaceId)}/${documentId}`, token);
}
