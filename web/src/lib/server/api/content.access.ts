import type { FolderAccessData, SetFolderAccessPayload } from '$lib/types/content';
import { del, get, put } from './client';

const base = (workspaceId: string) => `/content/workspaces/${workspaceId}`;

export const getFolderAccess = (token: string, workspaceId: string, folderId: string) =>
	get<FolderAccessData[]>(`${base(workspaceId)}/folders/${folderId}/access`, token);

export const setFolderAccess = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: SetFolderAccessPayload
) => put<null>(`${base(workspaceId)}/folders/${folderId}/access`, p, token);

export const removeFolderAccess = (
	token: string,
	workspaceId: string,
	folderId: string,
	groupId: string
) => del<null>(`${base(workspaceId)}/folders/${folderId}/access/${groupId}`, token);
