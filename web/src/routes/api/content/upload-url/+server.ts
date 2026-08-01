import { error, json } from '@sveltejs/kit';
import { requestUploadUrl } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

export const POST: RequestHandler = async ({ locals, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const body = (await request.json().catch(() => null)) as {
		workspaceId?: string;
		folderId?: string;
		storageKey?: string;
	} | null;

	if (!body?.workspaceId || !body.folderId) error(400, t('err.generic'));

	const res = await requestUploadUrl(
		locals.session,
		body.workspaceId,
		body.folderId,
		body.storageKey
	);
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
