import { error, redirect } from '@sveltejs/kit';
import { canManageAccess } from '$lib/access/roles';
import {
	countWaitingQuestions,
	getMyAccessWorkspace,
	getWorkspaces,
	listQuestions
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { LayoutServerLoad } from './$types';

// Loaded at the layout level so the whole room subtree (shell sidebar + every
// module page) shares one authoritative workspace record via `page.data`.
export const load: LayoutServerLoad = async ({ locals, params }) => {
	if (!locals.user || !locals.session) redirect(303, '/login');
	if (locals.user.status === 'pending') redirect(303, '/verify-email');

	const list = await getWorkspaces(locals.session);
	if (!list.ok) error(502, t('ws.loadError'));

	const match = list.data.find((w) => w.slug === params.slug);
	if (!match) error(404, t('ws.detail.notFound'));

	const myAccessRes = await getMyAccessWorkspace(locals.session, match.id);

	if (!myAccessRes.ok) {
		if (myAccessRes.status === 401) redirect(303, '/login');
		if (myAccessRes.status === 403) error(403, t('ws.detail.forbidden'));
		if (myAccessRes.status === 404) error(404, t('ws.detail.notFound'));
		error(myAccessRes.status || 500, t('err.generic'));
	}

	// Q&A shell data, best-effort: the manager badge needs the waiting count,
	// guest entry points need the group's qa_enabled. Neither may break the room.
	let qaWaiting = 0;
	let qaEnabled = true;
	if (canManageAccess(myAccessRes.data.role)) {
		const countRes = await countWaitingQuestions(locals.session, match.id);
		if (countRes.ok) qaWaiting = countRes.data.waiting_count;
	} else {
		const qaRes = await listQuestions(locals.session, match.id, { limit: 1 });
		if (qaRes.ok) qaEnabled = qaRes.data.qa_enabled;
	}

	return { workspace: match, access: myAccessRes.data, qaWaiting, qaEnabled };
};
