import { error, json } from '@sveltejs/kit';
import { requestVersionUpload } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// One presigned PUT per new version. Unlike a first upload there is no multipart
// path upstream, so the browser sends the whole file in a single request.
export const POST: RequestHandler = async ({ locals, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const body = (await request.json().catch(() => null)) as {
		workspaceId?: string;
		documentId?: string;
	} | null;

	if (!body?.workspaceId || !body.documentId) error(400, t('err.generic'));

	const res = await requestVersionUpload(locals.session, body.workspaceId, body.documentId);
	if (!res.ok) {
		if (res.status === 404) error(404, t('doc.docs.err.notFound'));
		error(res.status || 500, t('doc.ver.err.upload'));
	}

	return json(res.data);
};
