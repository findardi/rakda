import { error, redirect } from '@sveltejs/kit';
import { fetchActivityExport } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { RequestHandler } from './$types';

// Auth-injecting proxy for the activity-log CSV. Reached by a plain link, not
// fetch: the response is piped straight through so a room with a long history
// exports without ever being held in memory here.
//
// Because a link navigates, a failure would otherwise dump a raw error body over
// the app. An expired session is sent to login — the right destination for a
// document navigation — and anything else goes through `error()` so the app's own
// error page renders instead.
export const GET: RequestHandler = async ({ locals, url }) => {
	if (!locals.session) redirect(303, '/login');

	const workspaceId = url.searchParams.get('workspaceId');
	if (!workspaceId) error(400, t('err.generic'));

	let upstream: Response;
	try {
		upstream = await fetchActivityExport(locals.session, workspaceId, {
			from: url.searchParams.get('from') ?? '',
			to: url.searchParams.get('to') ?? '',
			actor_id: url.searchParams.get('actor_id') ?? '',
			action: url.searchParams.get('action') ?? ''
		});
	} catch {
		error(502, t('err.network'));
	}

	if (upstream.status === 401) redirect(303, '/login');
	if (!upstream.ok) error(upstream.status, t('activity.export.err'));

	return new Response(upstream.body, {
		status: 200,
		headers: {
			'content-type': upstream.headers.get('content-type') ?? 'text/csv; charset=utf-8',
			// The filename is the server's to choose; it already sanitises it.
			'content-disposition':
				upstream.headers.get('content-disposition') ?? 'attachment; filename="activity-log.csv"',
			'cache-control': 'no-store'
		}
	});
};
