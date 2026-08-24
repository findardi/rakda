import { error, json } from '@sveltejs/kit';
import { completeVersion, listVersions } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Version history. Upstream gates this on owner/admin regardless of the
// `document:view` permission, so a guest gets 403 here even though they can read
// the document itself — the UI hides the panel rather than relying on that.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	const res = await listVersions(locals.session, workspaceId, documentId);
	if (!res.ok) {
		if (res.status === 403) error(403, t('doc.ver.forbidden'));
		if (res.status === 404) error(404, t('doc.docs.err.notFound'));
		error(res.status || 500, t('doc.ver.err.load'));
	}

	return json(res.data ?? []);
};

// Completes a new version: the bytes are already in object storage under
// `storageKey`. The version is registered as *staged* — upstream serves it only
// once its rendition succeeds, so the current version stays untouched for now.
// `fileName` is the picked file's name, for the server's type gate.
export const POST: RequestHandler = async ({ locals, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const body = (await request.json().catch(() => null)) as {
		workspaceId?: string;
		documentId?: string;
		storageKey?: string;
		fileName?: string;
	} | null;

	if (!body?.workspaceId || !body.documentId || !body.storageKey) error(400, t('err.generic'));

	const res = await completeVersion(locals.session, body.workspaceId, body.documentId, {
		storage_key: body.storageKey,
		file_name: body.fileName ?? ''
	});
	if (!res.ok) {
		if (res.status === 404) error(404, t('doc.docs.err.notFound'));
		if (res.status === 415) error(415, t('doc.ver.err.typeMismatch'));
		if (res.status === 413) error(413, t('upload.err.tooLarge'));
		error(res.status || 500, t('doc.ver.err.upload'));
	}

	return json(res.data);
};
