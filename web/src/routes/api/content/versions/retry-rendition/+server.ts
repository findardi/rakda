import { error, json } from '@sveltejs/kit';
import { retryRendition } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Clears the recorded rendition failure for a version; the next open of the
// document retries the conversion. Owner/admin only upstream — guests get 403.
export const POST: RequestHandler = async ({ locals, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const body = (await request.json().catch(() => null)) as {
		workspaceId?: string;
		documentId?: string;
		versionId?: string;
	} | null;

	if (!body?.workspaceId || !body.documentId || !body.versionId) error(400, t('err.generic'));

	const res = await retryRendition(
		locals.session,
		body.workspaceId,
		body.documentId,
		body.versionId
	);
	if (!res.ok) {
		if (res.status === 403) error(403, t('doc.view.failed.noPerm'));
		if (res.status === 404) error(404, t('doc.view.err.notFound'));
		error(res.status || 500, t('doc.view.err.load'));
	}

	return json(res.data);
};
