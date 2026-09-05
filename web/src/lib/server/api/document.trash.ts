import type { RestoreData, TrashData } from '$lib/types/content';
import { get, post } from './client';

const trashBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/trash`;

export const getTrash = (token: string, workspaceId: string) =>
	get<TrashData>(trashBase(workspaceId), token);

export const restoreTrashFolder = (token: string, workspaceId: string, folderId: string) =>
	post<RestoreData>(`${trashBase(workspaceId)}/folders/${folderId}/restore`, undefined, token);

export const restoreTrashDocument = (token: string, workspaceId: string, documentId: string) =>
	post<RestoreData>(`${trashBase(workspaceId)}/documents/${documentId}/restore`, undefined, token);
