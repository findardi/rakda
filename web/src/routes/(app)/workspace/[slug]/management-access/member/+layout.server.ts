import { error, redirect } from '@sveltejs/kit';
import { getGroups, getInvitations, getMembers, getRoles } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { LayoutServerLoad } from './$types';

// Loaded once for the member section so both sub-tabs (members + invitations)
// share data and their tab counts always agree. Groups ride along for the
// invite dialog's group picker.
export const load: LayoutServerLoad = async ({ locals, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace } = await parent();
	// Only pending invitations feed the sub-tab badge ("awaiting a response");
	// the invite page loads its own filtered list separately.
	const [membersRes, rolesRes, invitesRes, groupsRes] = await Promise.all([
		getMembers(locals.session, workspace.id),
		getRoles(locals.session, workspace.id),
		getInvitations(locals.session, workspace.id, 'pending'),
		getGroups(locals.session, workspace.id)
	]);

	if (!membersRes.ok) {
		if (membersRes.status === 401) redirect(303, '/login');
		error(membersRes.status || 502, t('member.err.loadError'));
	}
	if (!rolesRes.ok) {
		if (rolesRes.status === 401) redirect(303, '/login');
		error(rolesRes.status || 502, t('member.err.loadError'));
	}
	if (!invitesRes.ok) {
		if (invitesRes.status === 401) redirect(303, '/login');
		error(invitesRes.status || 502, t('pending.err.loadError'));
	}
	if (!groupsRes.ok) {
		if (groupsRes.status === 401) redirect(303, '/login');
		error(groupsRes.status || 502, t('group.err.loadError'));
	}
	return {
		members: membersRes.data,
		roles: rolesRes.data,
		groups: groupsRes.data,
		pendingCount: invitesRes.data.length,
		ownerId: workspace.owner_id
	};
};
