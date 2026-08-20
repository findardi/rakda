import type { ApiResult } from '$lib/types';
import type { SearchData } from '$lib/types/content';
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

export function logSearch(
	token: string,
	workspaceId: string,
	query: string
): Promise<ApiResult<null>> {
	return post<null>(`/content/workspaces/${workspaceId}/search/log`, { query }, token);
}
