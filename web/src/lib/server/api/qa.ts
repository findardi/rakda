import type { ApiResult } from '$lib/types';
import type {
	CreateFaqPayload,
	CreateQuestionPayload,
	QaFaqItem,
	QaFilters,
	QaListData,
	QaQuery,
	QaReplyResult,
	QaThread,
	QaWaitingCount
} from '$lib/types/qa';
import { API_URL, get, post, upstreamHeaders } from './client';

export function listQuestions(
	token: string,
	workspaceId: string,
	query: QaQuery = {}
): Promise<ApiResult<QaListData>> {
	const params = new URLSearchParams();
	if (query.limit) params.set('limit', String(query.limit));
	if (query.cursor) params.set('cursor', query.cursor);
	if (query.status) params.set('status', query.status);
	if (query.group_id) params.set('group_id', query.group_id);

	const qs = params.toString();
	return get<QaListData>(`/qa/workspaces/${workspaceId}/questions${qs ? `?${qs}` : ''}`, token);
}

export const createQuestion = (
	token: string,
	workspaceId: string,
	payload: CreateQuestionPayload
) => post<QaThread>(`/qa/workspaces/${workspaceId}/questions`, payload, token);

export const countWaitingQuestions = (token: string, workspaceId: string) =>
	get<QaWaitingCount>(`/qa/workspaces/${workspaceId}/questions/count`, token);

export const getQuestion = (token: string, workspaceId: string, questionId: string) =>
	get<QaThread>(`/qa/workspaces/${workspaceId}/questions/${questionId}`, token);

export const replyQuestion = (
	token: string,
	workspaceId: string,
	questionId: string,
	body: string
) =>
	post<QaReplyResult>(
		`/qa/workspaces/${workspaceId}/questions/${questionId}/replies`,
		{ body },
		token
	);

export const closeQuestion = (token: string, workspaceId: string, questionId: string) =>
	post<null>(`/qa/workspaces/${workspaceId}/questions/${questionId}/close`, {}, token);

export const reopenQuestion = (token: string, workspaceId: string, questionId: string) =>
	post<null>(`/qa/workspaces/${workspaceId}/questions/${questionId}/reopen`, {}, token);

// Answers with a CSV stream, not the JSON envelope — hands back the raw
// Response for the proxy to pipe through (pattern of the activity exports).
export function fetchQaExport(
	token: string,
	workspaceId: string,
	filters: Partial<QaFilters> = {}
): Promise<Response> {
	const params = new URLSearchParams({ format: 'csv' });
	if (filters.status) params.set('status', filters.status);
	if (filters.group_id) params.set('group_id', filters.group_id);

	return fetch(`${API_URL}/qa/workspaces/${workspaceId}/questions/export?${params}`, {
		headers: upstreamHeaders(token)
	});
}

export const listFaqs = (token: string, workspaceId: string) =>
	get<QaFaqItem[]>(`/qa/workspaces/${workspaceId}/faqs`, token);

export const createFaq = (token: string, workspaceId: string, payload: CreateFaqPayload) =>
	post<QaFaqItem>(`/qa/workspaces/${workspaceId}/faqs`, payload, token);
