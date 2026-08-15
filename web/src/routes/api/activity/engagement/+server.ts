import { error, json } from '@sveltejs/kit';
import { getDocumentEngagement } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Read-side of the engagement pair. Fetched when the reader opens the panel,
// not on page load — the panel is opened rarely and the aggregate is not free.
// Upstream answers 403 for a guest; that status is forwarded untouched.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	const res = await getDocumentEngagement(locals.session, workspaceId, documentId);
	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
