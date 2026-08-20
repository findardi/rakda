import { error } from '@sveltejs/kit';
import { downloadDocument } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Auth-injecting proxy for the Model B download: the backend streams the PDF
// rendition (watermarked or clean) with Content-Disposition: attachment, so the
// browser saves the file when this route is navigated to directly. Only a 2xx
// carries PDF bytes; every other status is a JSON error envelope.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	// Absent means the current version; a non-current one is owner/admin only.
	const version = url.searchParams.get('version') ?? undefined;
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await downloadDocument(locals.session, workspaceId, documentId, version);
	} catch {
		error(502, t('err.network'));
	}

	if (!upstream.ok) {
		if (upstream.status === 403) error(403, t('doc.docs.err.forbiddenDownload'));
		if (upstream.status === 404) error(404, t('doc.docs.err.notFound'));
		error(upstream.status || 500, upstream.statusText || t('err.generic'));
	}

	return new Response(upstream.body, {
		status: 200,
		headers: {
			'content-type': upstream.headers.get('content-type') ?? 'application/pdf',
			'content-disposition': upstream.headers.get('content-disposition') ?? 'attachment',
			'cache-control': 'no-store'
		}
	});
};
