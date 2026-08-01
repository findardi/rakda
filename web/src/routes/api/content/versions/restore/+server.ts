import { error, json } from '@sveltejs/kit';
import { restoreVersion } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Restore copies the chosen version forward as a new current one — nothing is
// overwritten, so this is reversible by restoring again. 409 means the version
// picked is already current, which the UI prevents but a stale panel can hit.
export const POST: RequestHandler = async ({ locals, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const body = (await request.json().catch(() => null)) as {
		workspaceId?: string;
		documentId?: string;
		versionId?: string;
	} | null;

	if (!body?.workspaceId || !body.documentId || !body.versionId) error(400, t('err.generic'));

	const res = await restoreVersion(
		locals.session,
		body.workspaceId,
		body.documentId,
		body.versionId
	);
	if (!res.ok) {
		if (res.status === 409) error(409, t('doc.ver.err.alreadyCurrent'));
		if (res.status === 404) error(404, t('doc.ver.err.notFound'));
		error(res.status || 500, t('doc.ver.err.restore'));
	}

	return json(res.data);
};
