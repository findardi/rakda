import { error, json } from '@sveltejs/kit';
import { getDownloadJob } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, params, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	const res = await getDownloadJob(locals.session, workspaceId, params.jobId);
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
