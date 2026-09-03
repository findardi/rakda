import { fail, redirect } from '@sveltejs/kit';
import {
	deleteMember,
	resolveWorkspaceId,
	updateMemberExpiry,
	updateMemberRole
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions } from './$types';

export const actions: Actions = {
	updateRole: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const memberId = (form.get('memberId') ?? '').toString();
		const roleId = (form.get('roleId') ?? '').toString();
		if (!memberId || !roleId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await updateMemberRole(locals.session, wsId, memberId, { role_id: roleId });
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('member.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { roleUpdated: true };
	},

	updateExpiry: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const memberId = (form.get('memberId') ?? '').toString();
		const expiresAt = (form.get('expiresAt') ?? '').toString();
		if (!memberId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		// Empty = clear: the backend reads null as "never expires".
		const res = await updateMemberExpiry(locals.session, wsId, memberId, {
			expires_at: expiresAt || null
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('member.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { expiryUpdated: true };
	},

	delete: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const memberId = (form.get('memberId') ?? '').toString();
		if (!memberId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await deleteMember(locals.session, wsId, memberId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { deleted: true };
	}
};
