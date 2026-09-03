import { error, fail, redirect } from '@sveltejs/kit';
import {
	addMembers,
	getInvitations,
	resendInvitation,
	resolveWorkspaceId,
	revokeInvitation
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

const INVITE_STATUSES = ['pending', 'accepted', 'expired', 'revoked', 'rejected'];

export const load: PageServerLoad = async ({ locals, parent, url }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace } = await parent();
	// Default to the actionable view (Menunggu) so the tab badge and the rows
	// agree; `all` is the explicit opt-in to the full history.
	const raw = url.searchParams.get('status') ?? 'pending';
	const filter = raw === 'all' || INVITE_STATUSES.includes(raw) ? raw : 'pending';
	// `all` → no backend status (returns every status); otherwise exact.
	const query = filter === 'all' ? undefined : filter;

	const res = await getInvitations(locals.session, workspace.id, query);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		error(res.status || 502, t('pending.err.loadError'));
	}

	return { invitations: res.data, status: filter };
};

export const actions: Actions = {
	// Bulk invite. Emails arrive as repeated `email` fields; the backend decides
	// per email (invite / skip) and never reports who was already registered.
	invite: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const roleId = (form.get('roleId') ?? '').toString();
		const groupId = (form.get('groupId') ?? '').toString();
		const accessExpiresAt = (form.get('accessExpiresAt') ?? '').toString();
		const emails = [
			...new Set(
				form
					.getAll('email')
					.map((e) => e.toString().trim().toLowerCase())
					.filter(Boolean)
			)
		];

		if (!emails.length) return fail(400, { message: t('member.invite.empty') });
		if (!roleId) return fail(400, { fieldErrors: { role: t('err.required') } });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await addMembers(locals.session, wsId, {
			email: emails,
			role_id: roleId,
			group_id: groupId || undefined,
			access_expires_at: accessExpiresAt || undefined
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, {
				fieldErrors: res.fieldErrors?.email ? { email: res.fieldErrors.email } : {},
				message: Object.keys(res.fieldErrors ?? {}).length ? null : res.message || t('err.generic')
			});
		}

		return { invited: true, results: res.data };
	},

	resend: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const invitationId = (form.get('invitationId') ?? '').toString();
		if (!invitationId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await resendInvitation(locals.session, wsId, invitationId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { resent: true };
	},

	revoke: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const invitationId = (form.get('invitationId') ?? '').toString();
		if (!invitationId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await revokeInvitation(locals.session, wsId, invitationId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { revoked: true };
	}
};
