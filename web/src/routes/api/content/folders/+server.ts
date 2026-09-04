import { error, json } from '@sveltejs/kit';
import { getFoldersTree } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Read-only tree for surfaces that do not live under the document layout (the
// overview's archive-scope picker). Fetched lazily on open, never in a load.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	const res = await getFoldersTree(locals.session, workspaceId);
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
