import type { ApiResult } from '$lib/types';
import type {
	CreateFaqPayload,
	CreateQuestionPayload,
	QaFaqItem,
	QaListData,
	QaQuery,
	QaReplyResult,
	QaThread,
	QaWaitingCount
} from '$lib/types/qa';
import { get, post } from './client';

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

export function createQuestion(
	token: string,
	workspaceId: string,
	payload: CreateQuestionPayload
): Promise<ApiResult<QaThread>> {
	return post<QaThread>(`/qa/workspaces/${workspaceId}/questions`, payload, token);
}

export function countWaitingQuestions(
	token: string,
	workspaceId: string
): Promise<ApiResult<QaWaitingCount>> {
	return get<QaWaitingCount>(`/qa/workspaces/${workspaceId}/questions/count`, token);
}

export function getQuestion(
	token: string,
	workspaceId: string,
	questionId: string
): Promise<ApiResult<QaThread>> {
	return get<QaThread>(`/qa/workspaces/${workspaceId}/questions/${questionId}`, token);
}

export function replyQuestion(
	token: string,
	workspaceId: string,
	questionId: string,
	body: string
): Promise<ApiResult<QaReplyResult>> {
	return post<QaReplyResult>(
		`/qa/workspaces/${workspaceId}/questions/${questionId}/replies`,
		{ body },
		token
	);
}

export function closeQuestion(
	token: string,
	workspaceId: string,
	questionId: string
): Promise<ApiResult<null>> {
	return post<null>(`/qa/workspaces/${workspaceId}/questions/${questionId}/close`, {}, token);
}

export function reopenQuestion(
	token: string,
	workspaceId: string,
	questionId: string
): Promise<ApiResult<null>> {
	return post<null>(`/qa/workspaces/${workspaceId}/questions/${questionId}/reopen`, {}, token);
}

export function listFaqs(token: string, workspaceId: string): Promise<ApiResult<QaFaqItem[]>> {
	return get<QaFaqItem[]>(`/qa/workspaces/${workspaceId}/faqs`, token);
}

export function createFaq(
	token: string,
	workspaceId: string,
	payload: CreateFaqPayload
): Promise<ApiResult<QaFaqItem>> {
	return post<QaFaqItem>(`/qa/workspaces/${workspaceId}/faqs`, payload, token);
}
