import { error, redirect } from '@sveltejs/kit';
import { isManager, isRoomOpenTo } from '$lib/access/roles';
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
export const load: LayoutServerLoad = async ({ locals, params, url }) => {
	if (!locals.user || !locals.session) redirect(303, '/login');
	if (locals.user.status === 'pending') redirect(303, '/verify-email');

	const list = await getWorkspaces(locals.session);
	if (!list.ok) error(502, t('ws.loadError'));

	const rooms = list.data.workspaces;
	const match = rooms.find((w) => w.slug === params.slug);
	if (!match) error(404, t('ws.detail.notFound'));

	const myAccessRes = await getMyAccessWorkspace(locals.session, match.id);

	if (!myAccessRes.ok) {
		if (myAccessRes.status === 401) redirect(303, '/login');
		if (myAccessRes.status === 403) error(403, t('ws.detail.forbidden'));
		if (myAccessRes.status === 404) error(404, t('ws.detail.notFound'));
		error(myAccessRes.status || 500, t('err.generic'));
	}

	const access = myAccessRes.data;
	const roomStatus = access.workspace_status ?? match.status;
	const roomOpen = isRoomOpenTo(roomStatus, access.role);

	// A room still in `prepare` is closed to guests on every module, so the room
	// subtree has nothing it can load. Land on the overview, which explains the
	// state as a page instead of an error.
	const overview = `/workspace/${params.slug}`;
	if (!roomOpen && url.pathname !== overview) redirect(303, overview);

	// Q&A shell data, best-effort: the manager badge needs the waiting count,
	// guest entry points need the group's qa_enabled. Neither may break the room.
	let qaWaiting = 0;
	let qaEnabled = true;
	if (roomOpen) {
		if (isManager(access.role)) {
			const countRes = await countWaitingQuestions(locals.session, match.id);
			if (countRes.ok) qaWaiting = countRes.data.waiting_count;
		} else {
			const qaRes = await listQuestions(locals.session, match.id, { limit: 1 });
			if (qaRes.ok) qaEnabled = qaRes.data.qa_enabled;
		}
	}

	return { workspace: match, rooms, access, roomStatus, roomOpen, qaWaiting, qaEnabled };
};
