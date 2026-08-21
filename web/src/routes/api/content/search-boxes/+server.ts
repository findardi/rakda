import { error, json } from '@sveltejs/kit';
import { searchWordBoxes } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	const q = url.searchParams.get('q') ?? '';
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	const res = await searchWordBoxes(locals.session, workspaceId, documentId, q);
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
