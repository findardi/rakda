import type { ApiResult } from '$lib/types';
import type {
	ActivityListData,
	ActivityQuery,
	DocumentEngagement,
	RecordDurationsPayload
} from '$lib/types/activity';
import { get, post } from './client';

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

// Dwell ingest. Open to every member — a guest's reading is the signal worth
// having — so this is the one activity endpoint a guest may write to.
export function recordPageDurations(
	token: string,
	workspaceId: string,
	documentId: string,
	payload: RecordDurationsPayload
): Promise<ApiResult<null>> {
	return post<null>(
		`/activity/workspaces/${workspaceId}/documents/${documentId}/duration`,
		payload,
		token
	);
}

// Owner/admin only upstream; a guest earns a 403 and never sees the control.
export function getDocumentEngagement(
	token: string,
	workspaceId: string,
	documentId: string
): Promise<ApiResult<DocumentEngagement>> {
	return get<DocumentEngagement>(
		`/activity/workspaces/${workspaceId}/documents/${documentId}/engagement`,
		token
	);
}
