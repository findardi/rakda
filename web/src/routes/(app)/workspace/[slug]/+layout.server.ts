import { error, redirect } from '@sveltejs/kit';
import { getMyAccessWorkspace, getWorkspace, getWorkspaces } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { LayoutServerLoad } from './$types';

// Loaded at the layout level so the whole room subtree (shell sidebar + every
// module page) shares one authoritative workspace record via `page.data`.
export const load: LayoutServerLoad = async ({ locals, params }) => {
	if (!locals.user || !locals.session) redirect(303, '/login');
	if (locals.user.status === 'pending') redirect(303, '/verify-email');

	// No by-slug endpoint exists: resolve slug -> id via the (owner-scoped) list,
	// then fetch the authoritative record by id (which also runs the owner check).
	const list = await getWorkspaces(locals.session);
	if (!list.ok) error(502, t('ws.loadError'));

	const match = list.data.find((w) => w.slug === params.slug);
	if (!match) error(404, t('ws.detail.notFound'));

	const [workRes, myAccessRes] = await Promise.all([
		getWorkspace(locals.session, match.id),
		getMyAccessWorkspace(locals.session, match.id)
	]);

	if (!workRes.ok) {
		if (workRes.status === 401) redirect(303, '/login');
		if (workRes.status === 403) error(403, t('ws.detail.forbidden'));
		if (workRes.status === 404) error(404, t('ws.detail.notFound'));
		error(workRes.status || 500, t('err.generic'));
	}

	if (!myAccessRes.ok) {
		if (myAccessRes.status === 401) redirect(303, '/login');
		if (myAccessRes.status === 403) error(403, t('ws.detail.forbidden'));
		if (myAccessRes.status === 404) error(404, t('ws.detail.notFound'));
		error(myAccessRes.status || 500, t('err.generic'));
	}

	return { workspace: workRes.data, access: myAccessRes.data };
};
