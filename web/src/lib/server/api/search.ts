import type { ApiResult } from '$lib/types';
import type { SearchBoxesData, SearchContentPagesData, SearchData } from '$lib/types/content';
import { get, post } from './client';

export function searchContent(
	token: string,
	workspaceId: string,
	query: string
): Promise<ApiResult<SearchData>> {
	return get<SearchData>(
		`/content/workspaces/${workspaceId}/search?q=${encodeURIComponent(query)}`,
		token
	);
}

export function searchWordBoxes(
	token: string,
	workspaceId: string,
	documentId: string,
	query: string
): Promise<ApiResult<SearchBoxesData>> {
	return get<SearchBoxesData>(
		`/content/workspaces/${workspaceId}/documents/${encodeURIComponent(documentId)}/search-boxes?q=${encodeURIComponent(query)}`,
		token
	);
}

export function searchContentPages(
	token: string,
	workspaceId: string,
	documentId: string,
	query: string
): Promise<ApiResult<SearchContentPagesData>> {
	return get<SearchContentPagesData>(
		`/content/workspaces/${workspaceId}/search/content/pages?documentId=${encodeURIComponent(documentId)}&q=${encodeURIComponent(query)}`,
		token
	);
}

export function logSearch(
	token: string,
	workspaceId: string,
	query: string
): Promise<ApiResult<null>> {
	return post<null>(`/content/workspaces/${workspaceId}/search/log`, { query }, token);
}
