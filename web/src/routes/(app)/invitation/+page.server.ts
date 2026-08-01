import { fail, redirect } from '@sveltejs/kit';
import { acceptMyInvitation, getMyInvitation, rejectMyInvitation } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals }) => {
	if (!locals.user || !locals.session) redirect(303, '/login');
	if (locals.user.status === 'pending') redirect(303, '/verify-email');

	const res = await getMyInvitation(locals.session);
	if (!res.ok) return { invitations: [], loadError: res.message };
	return { invitations: res.data, loadError: null as string | null };
};

function mapActionError(status: number, raw: string): string {
	const m = raw.toLowerCase();
	if (status === 404 || m.includes('not found')) return t('inv.err.notFound');
	if (m.includes('expired')) return t('inv.err.expired');
	if (m.includes('no longer pending') || m.includes('does not belong'))
		return t('inv.err.notPending');
	return t('err.generic');
}

export const actions: Actions = {
	accept: async ({ locals, request }) => {
		if (!locals.session) redirect(303, '/login');
		const id = (await request.formData()).get('id')?.toString() ?? '';
		if (!id) return fail(400, { message: t('err.generic') });

		const res = await acceptMyInvitation(locals.session, id);
		if (!res.ok)
			return fail(res.status || 400, { message: mapActionError(res.status, res.message) });
		return { ok: true };
	},

	reject: async ({ locals, request }) => {
		if (!locals.session) redirect(303, '/login');
		const id = (await request.formData()).get('id')?.toString() ?? '';
		if (!id) return fail(400, { message: t('err.generic') });

		const res = await rejectMyInvitation(locals.session, id);
		if (!res.ok)
			return fail(res.status || 400, { message: mapActionError(res.status, res.message) });
		return { ok: true };
	}
};
