import { error, json } from '@sveltejs/kit';
import { listQuestions } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	const rawLimit = Number(url.searchParams.get('limit'));
	const res = await listQuestions(locals.session, workspaceId, {
		limit: Number.isInteger(rawLimit) && rawLimit > 0 ? rawLimit : undefined,
		cursor: url.searchParams.get('cursor') ?? undefined,
		status: url.searchParams.get('status') ?? undefined,
		group_id: url.searchParams.get('group_id') ?? undefined
	});
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
