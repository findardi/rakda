import { error, json } from '@sveltejs/kit';
import { getDownloadUrl } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	// Absent means the current version; a non-current one is owner/admin only.
	const version = url.searchParams.get('version') ?? undefined;
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	const res = await getDownloadUrl(locals.session, workspaceId, documentId, version);
	if (!res.ok) {
		if (res.status === 403) error(403, t('doc.docs.err.forbiddenDownload'));
		if (res.status === 404) error(404, t('doc.docs.err.notFound'));
		error(res.status || 500, res.message);
	}

	return json(res.data);
};
