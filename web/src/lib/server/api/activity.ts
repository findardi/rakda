import type { ApiResult } from '$lib/types';
import type {
	ActivityFilters,
	ActivityListData,
	ActivityQuery,
	DocumentReaders,
	ReaderPages,
	RecordDurationsPayload
} from '$lib/types/activity';
import { API_URL, get, post, upstreamHeaders } from './client';

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
export const recordPageDurations = (
	token: string,
	workspaceId: string,
	documentId: string,
	payload: RecordDurationsPayload
) =>
	post<null>(
		`/activity/workspaces/${workspaceId}/documents/${documentId}/duration`,
		payload,
		token
	);

// --- exports ---
// These two answer with a CSV stream, not the JSON envelope every other endpoint
// uses, so they hand back the raw Response for the proxy to pipe through. An
// export of a full audit trail is unbounded; buffering it would be a memory
// hazard for the exact rooms that need it most.

export function fetchActivityExport(
	token: string,
	workspaceId: string,
	filters: Partial<ActivityFilters> = {}
): Promise<Response> {
	const params = new URLSearchParams({ format: 'csv' });
	if (filters.from) params.set('from', filters.from);
	if (filters.to) params.set('to', filters.to);
	if (filters.actor_id) params.set('actor_id', filters.actor_id);
	if (filters.action) params.set('action', filters.action);

	return fetch(`${API_URL}/activity/workspaces/${workspaceId}/export?${params}`, {
		headers: { authorization: `Bearer ${token}` }
	});
}

export const fetchEngagementExport = (token: string, workspaceId: string, documentId: string) =>
	fetch(
		`${API_URL}/activity/workspaces/${workspaceId}/documents/${documentId}/engagement/export?format=csv`,
		{ headers: { authorization: `Bearer ${token}` } }
	);

// Owner/admin only upstream; a guest earns a 403 and never sees the control.
export const getDocumentReaders = (token: string, workspaceId: string, documentId: string) =>
	get<DocumentReaders>(
		`/activity/workspaces/${workspaceId}/documents/${documentId}/engagement`,
		token
	);

export const getReaderPages = (
	token: string,
	workspaceId: string,
	documentId: string,
	actorId: string
) =>
	get<ReaderPages>(
		`/activity/workspaces/${workspaceId}/documents/${documentId}/engagement/readers/${actorId}`,
		token
	);
