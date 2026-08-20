import { error, json } from '@sveltejs/kit';
import { logSearch, searchContent } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const q = url.searchParams.get('q') ?? '';
	if (!workspaceId) error(400, t('err.generic'));

	const res = await searchContent(locals.session, workspaceId, q);
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};

// Audit endpoint: the GET above must stay side-effect free, so committing a
// search (even a result-less one) is logged through this separate route.
export const POST: RequestHandler = async ({ locals, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const body = (await request.json().catch(() => null)) as {
		workspaceId?: string;
		query?: string;
	} | null;
	if (!body?.query || !body.workspaceId) error(400, t('err.generic'));

	const res = await logSearch(locals.session, body.workspaceId, body.query.trim());
	if (!res.ok) error(res.status || 500, res.message);

	return json({ ok: true });
};
