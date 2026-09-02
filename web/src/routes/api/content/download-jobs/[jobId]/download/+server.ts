import { error } from '@sveltejs/kit';
import { downloadJobArtifact } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Artefak unduhan tertunda disajikan lewat <a href> polos, bukan fetch+blob:
// Content-Length + Accept-Ranges diteruskan apa adanya supaya transfer yang
// putus bisa dilanjutkan, dan berkas tidak pernah singgah di memori tab.
export const GET: RequestHandler = async ({ locals, params, url, request }) => {
	if (!locals.session) error(401, t('err.invalidCredentials'));

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await downloadJobArtifact(
			locals.session,
			workspaceId,
			params.jobId,
			request.headers.get('range')
		);
	} catch {
		error(502, t('err.network'));
	}

	if (!upstream.ok && upstream.status !== 206) {
		if (upstream.status === 403) error(403, t('doc.docs.err.forbiddenDownload'));
		if (upstream.status === 404) error(404, t('doc.dl.err.gone'));
		if (upstream.status === 409) error(409, t('doc.dl.err.notReady'));
		error(upstream.status || 500, upstream.statusText || t('err.generic'));
	}

	const headers = new Headers();
	for (const key of [
		'content-type',
		'content-disposition',
		'content-length',
		'content-range',
		'accept-ranges'
	]) {
		const value = upstream.headers.get(key);
		if (value) headers.set(key, value);
	}
	headers.set('cache-control', 'no-store');

	return new Response(upstream.body, { status: upstream.status, headers });
};
