import type { ApiResult } from '$lib/types';
import type { ActivityListData, ActivityQuery } from '$lib/types/activity';
import { get } from './client';

export function listActivity(
	token: string,
	workspaceId: string,
	query: ActivityQuery = {}
): Promise<ApiResult<ActivityListData>> {
	const params = new URLSearchParams();
	if (query.limit) params.set('limit', String(query.limit));
	if (query.cursor) params.set('cursor', query.cursor);
	if (query.from) params.set('from', query.from);
	if (query.to) params.set('to', query.to);
	if (query.actor_id) params.set('actor_id', query.actor_id);
	if (query.action) params.set('action', query.action);

	const qs = params.toString();
	return get<ActivityListData>(`/activity/workspaces/${workspaceId}${qs ? `?${qs}` : ''}`, token);
}
