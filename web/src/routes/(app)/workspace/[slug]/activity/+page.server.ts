import { error, redirect } from '@sveltejs/kit';
import { isManager } from '$lib/access/roles';
import { getMembers, listActivity } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { ActivityActor, ActivityFilters } from '$lib/types/activity';
import type { PageServerLoad } from './$types';

const PAGE_SIZE = 50;

export const load: PageServerLoad = async ({ locals, params, parent, url }) => {
	if (!locals.session) redirect(303, '/login');

	const { access, workspace } = await parent();
	if (!access || !isManager(access.role)) {
		redirect(303, `/workspace/${params.slug}`);
	}

	const filters: ActivityFilters = {
		from: url.searchParams.get('from') ?? '',
		to: url.searchParams.get('to') ?? '',
		actor_id: url.searchParams.get('actor_id') ?? '',
		action: url.searchParams.get('action') ?? ''
	};

	const [activityRes, membersRes] = await Promise.all([
		listActivity(locals.session, workspace.id, { limit: PAGE_SIZE, ...filters }),
		getMembers(locals.session, workspace.id)
	]);

	const actors: ActivityActor[] = membersRes.ok
		? membersRes.data
				.filter((m) => m.user_id)
				.map((m) => ({ id: m.user_id, name: m.username || m.email }))
				.sort((a, b) => a.name.localeCompare(b.name))
		: [];

	if (!activityRes.ok) {
		if (activityRes.status === 401) redirect(303, '/login');
		if (activityRes.status === 403) redirect(303, `/workspace/${params.slug}`);
		if (activityRes.status === 400) {
			return {
				workspaceId: workspace.id,
				activity: { items: [], next_cursor: '' },
				actors,
				filters,
				pageSize: PAGE_SIZE,
				filterRejected: true
			};
		}
		error(activityRes.status || 500, t('activity.err.load'));
	}

	return {
		workspaceId: workspace.id,
		activity: activityRes.data,
		actors,
		filters,
		pageSize: PAGE_SIZE,
		filterRejected: false
	};
};
