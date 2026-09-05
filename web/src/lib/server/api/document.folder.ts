import type {
	ApplyTemplateData,
	BulkCreateFolderData,
	BulkCreateFolderPayload,
	CreateFolderPayload,
	FolderData,
	FolderTemplateData,
	FolderTreeNode,
	MoveFolderPayload,
	RenameFolderPayload
} from '$lib/types/content';
import { del, get, patch, post, put } from './client';

const foldersBase = (workspaceId: string) => `/content/workspaces/${workspaceId}/folders`;

export const getFoldersTree = (token: string, workspaceId: string) =>
	get<FolderTreeNode[]>(foldersBase(workspaceId), token);

export const createFolder = (token: string, workspaceId: string, p: CreateFolderPayload) =>
	post<FolderData>(foldersBase(workspaceId), p, token);

export const bulkCreateFolders = (token: string, workspaceId: string, p: BulkCreateFolderPayload) =>
	post<BulkCreateFolderData>(`${foldersBase(workspaceId)}/bulk`, p, token);

// Atomic: one unknown/foreign id fails the whole batch with a 404 — nothing
// is half-deleted. Soft-delete to trash, same as the single delete.
export const bulkDeleteFolders = (token: string, workspaceId: string, folderIds: string[]) =>
	post<null>(`${foldersBase(workspaceId)}/bulk-delete`, { folder_ids: folderIds }, token);

export const listFolderTemplates = (token: string, workspaceId: string) =>
	get<FolderTemplateData[]>(`/content/workspaces/${workspaceId}/folder-templates`, token);

export const applyFolderTemplate = (
	token: string,
	workspaceId: string,
	templateKey: string,
	locale: string
) =>
	post<ApplyTemplateData>(
		`/content/workspaces/${workspaceId}/folder-templates/${templateKey}/apply`,
		{ locale },
		token
	);

export const renameFolder = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: RenameFolderPayload
) => put<FolderData>(`${foldersBase(workspaceId)}/${folderId}`, p, token);

export const moveFolder = (
	token: string,
	workspaceId: string,
	folderId: string,
	p: MoveFolderPayload
) => patch<null>(`${foldersBase(workspaceId)}/${folderId}/move`, p, token);

export const deleteFolder = (token: string, workspaceId: string, folderId: string) =>
	del<null>(`${foldersBase(workspaceId)}/${folderId}`, token);
