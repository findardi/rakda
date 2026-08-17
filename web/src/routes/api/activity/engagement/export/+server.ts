import { error, redirect } from '@sveltejs/kit';
import { fetchEngagementExport } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Per-page engagement CSV for one document. Same link-navigation contract as the
// activity export: pipe the stream, send an expired session to login, render the
// app's error page for everything else.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) redirect(303, '/login');

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	if (!workspaceId || !documentId) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await fetchEngagementExport(locals.session, workspaceId, documentId);
	} catch {
		error(502, t('err.network'));
	}

	if (upstream.status === 401) redirect(303, '/login');
	if (!upstream.ok) error(upstream.status, t('activity.export.err'));

	return new Response(upstream.body, {
		status: 200,
		headers: {
			'content-type': upstream.headers.get('content-type') ?? 'text/csv; charset=utf-8',
			// Upstream names the file after the document; it sanitises it there.
			'content-disposition':
				upstream.headers.get('content-disposition') ?? 'attachment; filename="engagement.csv"',
			'cache-control': 'no-store'
		}
	});
};
