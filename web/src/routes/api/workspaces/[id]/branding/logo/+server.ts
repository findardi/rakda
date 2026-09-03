import { error } from '@sveltejs/kit';
import { fetchWorkspaceLogo } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// Auth-injecting proxy for the room logo. Unlike the page-image proxy this one
// passes the upstream cache headers through: the logo is not per-request, its
// ETag is the upload's version token, and a conditional request that matches
// comes back 304 without moving a byte.
export const GET: RequestHandler = async ({ locals, params, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));
	// The id becomes a path segment upstream; refuse anything that is not one.
	if (!UUID_RE.test(params.id)) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await fetchWorkspaceLogo(
			locals.session,
			params.id,
			request.headers.get('if-none-match') ?? undefined
		);
	} catch {
		error(502, t('err.network'));
	}

	const headers: Record<string, string> = {
		'cache-control': upstream.headers.get('cache-control') ?? 'private, max-age=86400'
	};
	const etag = upstream.headers.get('etag');
	if (etag) headers.etag = etag;

	if (upstream.status === 304) return new Response(null, { status: 304, headers });
	if (!upstream.ok) error(upstream.status, upstream.statusText || t('err.generic'));

	headers['content-type'] = upstream.headers.get('content-type') ?? 'image/png';
	return new Response(upstream.body, { status: 200, headers });
};
