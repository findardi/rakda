import type { ApiResult } from '$lib/types';
import type { TrashData } from '$lib/types/content';
import { get, post } from './client';

const trashBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/trash`;

export function getTrash(token: string, workspaceId: string): Promise<ApiResult<TrashData>> {
	return get<TrashData>(trashBase(workspaceId), token);
}

export function restoreTrashFolder(
	token: string,
	workspaceId: string,
	folderId: string
): Promise<ApiResult<null>> {
	return post<null>(`${trashBase(workspaceId)}/folders/${folderId}/restore`, undefined, token);
}

export function restoreTrashDocument(
	token: string,
	workspaceId: string,
	documentId: string
): Promise<ApiResult<null>> {
	return post<null>(`${trashBase(workspaceId)}/documents/${documentId}/restore`, undefined, token);
}
