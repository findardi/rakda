import type { ApiResult } from '$lib/types';
import type { ArchiveData } from '$lib/types/archive';
import { API_URL, del, get, post, upstreamHeaders } from './client';

const base = (workspaceId: string) => `/content/workspaces/${workspaceId}/archives`;

export function listArchives(
	token: string,
	workspaceId: string
): Promise<ApiResult<ArchiveData[]>> {
	return get<ArchiveData[]>(base(workspaceId), token);
}

// No ids = the whole room; each id includes that folder's subtree. The overview
// sends root ids, the folder rail one id at any depth.
export function createArchive(
	token: string,
	workspaceId: string,
	folderIds: string[] = []
): Promise<ApiResult<ArchiveData>> {
	return post<ArchiveData>(
		base(workspaceId),
		folderIds.length ? { folder_ids: folderIds } : {},
		token
	);
}

export function deleteArchive(
	token: string,
	workspaceId: string,
	archiveId: string
): Promise<ApiResult<null>> {
	return del<null>(`${base(workspaceId)}/${archiveId}`, token);
}

// Raw passthrough: the archive is room-sized, so the bytes must never be
// buffered on this tier. `range` is forwarded so a resumed download reaches the
// backend as a 206 instead of restarting at zero.
export function downloadArchive(
	token: string,
	workspaceId: string,
	archiveId: string,
	range?: string
): Promise<Response> {
	const headers: Record<string, string> = upstreamHeaders(token);
	if (range) headers.range = range;

	return fetch(`${API_URL}${base(workspaceId)}/${archiveId}/download`, { headers });
}
