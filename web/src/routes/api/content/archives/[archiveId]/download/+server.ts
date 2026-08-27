import { error } from '@sveltejs/kit';
import { downloadArchive } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Auth-injecting proxy for the archive ZIP. Unlike the document download this
// one is Range-aware in both directions: the request's `range` goes up, and a
// 206 plus `content-range`/`accept-ranges`/`content-length` comes back down.
// Without that passthrough a dropped connection restarts a room-sized transfer
// from byte zero, which is the whole reason the artifact is stored at all.
export const GET: RequestHandler = async ({ locals, params, request, url }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	const range = request.headers.get('range') ?? undefined;

	let upstream: Response;
	try {
		upstream = await downloadArchive(locals.session, workspaceId, params.archiveId, range);
	} catch {
		error(502, t('err.network'));
	}

	if (upstream.status !== 200 && upstream.status !== 206) {
		if (upstream.status === 403) error(403, t('err.forbidden'));
		if (upstream.status === 404) error(404, t('archive.err.notFound'));
		if (upstream.status === 409) error(409, t('archive.err.notReady'));
		error(upstream.status || 500, upstream.statusText || t('err.generic'));
	}

	const headers: Record<string, string> = {
		'content-type': upstream.headers.get('content-type') ?? 'application/zip',
		'content-disposition': upstream.headers.get('content-disposition') ?? 'attachment',
		'accept-ranges': upstream.headers.get('accept-ranges') ?? 'bytes',
		'cache-control': 'no-store'
	};

	const length = upstream.headers.get('content-length');
	if (length) headers['content-length'] = length;

	const contentRange = upstream.headers.get('content-range');
	if (contentRange) headers['content-range'] = contentRange;

	return new Response(upstream.body, { status: upstream.status, headers });
};
