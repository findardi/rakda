import { error, redirect } from '@sveltejs/kit';
import { fetchQaExport } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Auth-injecting proxy for the Q&A CSV. Reached by a plain link, not fetch:
// the response is piped straight through so a long queue exports without ever
// being held in memory here.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) redirect(303, '/login');

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await fetchQaExport(locals.session, workspaceId, {
			status: url.searchParams.get('status') ?? '',
			group_id: url.searchParams.get('group_id') ?? ''
		});
	} catch {
		error(502, t('err.network'));
	}

	if (upstream.status === 401) redirect(303, '/login');
	if (!upstream.ok) error(upstream.status, t('qa.export.err'));

	return new Response(upstream.body, {
		status: 200,
		headers: {
			'content-type': upstream.headers.get('content-type') ?? 'text/csv; charset=utf-8',
			'content-disposition':
				upstream.headers.get('content-disposition') ?? 'attachment; filename="qa-questions.csv"',
			'cache-control': 'no-store'
		}
	});
};
