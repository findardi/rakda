import { error } from '@sveltejs/kit';
import { fetchViewPage } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Auth-injecting proxy for secure page images. The backend locks /pages/{n}
// behind JWT and streams a per-request watermarked PNG (no-store), so an <img>
// tag cannot reach it directly. This route attaches the session token, forwards
// the bytes, and preserves no-store so the watermark is never cached.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	const page = url.searchParams.get('page');
	// Absent means the current version; a non-current one is owner/admin only.
	const version = url.searchParams.get('version') ?? undefined;
	if (!workspaceId || !documentId || !page) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await fetchViewPage(locals.session, workspaceId, documentId, page, version);
	} catch {
		error(502, t('err.network'));
	}

	// Only a 200 carries image bytes; every other status is a JSON error envelope.
	// Surface the status code so the <img> onerror handler fires — the body is
	// never shown to the user, so there is nothing to translate here.
	if (!upstream.ok) error(upstream.status, upstream.statusText || t('err.generic'));

	return new Response(upstream.body, {
		status: 200,
		headers: {
			'content-type': 'image/png',
			'cache-control': 'no-store'
		}
	});
};
