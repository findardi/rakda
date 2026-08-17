import { error, json } from '@sveltejs/kit';
import { getDocumentReaders, getReaderPages } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Read-side of the engagement pair, both levels. Without `actorId` it answers the
// reader list; with one, that reader's page breakdown. Fetched when the panel is
// opened, not on page load. Upstream answers 403 for a guest; that status is
// forwarded untouched.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	const actorId = url.searchParams.get('actorId');
	const res = actorId
		? await getReaderPages(locals.session, workspaceId, documentId, actorId)
		: await getDocumentReaders(locals.session, workspaceId, documentId);

	if (!res.ok) error(res.status || 500, res.message);

	return json(res.data);
};
