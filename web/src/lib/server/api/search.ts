import type { SearchBoxesData, SearchContentPagesData, SearchData } from '$lib/types/content';
import { get, post } from './client';

export const searchContent = (token: string, workspaceId: string, query: string) =>
	get<SearchData>(
		`/content/workspaces/${workspaceId}/search?q=${encodeURIComponent(query)}`,
		token
	);

export const searchWordBoxes = (
	token: string,
	workspaceId: string,
	documentId: string,
	query: string
) =>
	get<SearchBoxesData>(
		`/content/workspaces/${workspaceId}/documents/${encodeURIComponent(documentId)}/search-boxes?q=${encodeURIComponent(query)}`,
		token
	);

export const searchContentPages = (
	token: string,
	workspaceId: string,
	documentId: string,
	query: string
) =>
	get<SearchContentPagesData>(
		`/content/workspaces/${workspaceId}/search/content/pages?documentId=${encodeURIComponent(documentId)}&q=${encodeURIComponent(query)}`,
		token
	);

export const logSearch = (token: string, workspaceId: string, query: string) =>
	post<null>(`/content/workspaces/${workspaceId}/search/log`, { query }, token);
