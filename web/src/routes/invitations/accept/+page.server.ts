import { fail, redirect } from '@sveltejs/kit';
import { acceptInvitationSignup, getWorkspaces, previewInvitation } from '$lib/server/api';
import { setSession } from '$lib/server/session';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ url, locals }) => {
	const token = url.searchParams.get('token')?.trim() ?? '';
	if (!token) return { invalid: true, loggedIn: !!locals.user };

	const res = await previewInvitation(token);
	if (!res.ok) return { invalid: true, loggedIn: !!locals.user };

	return { invalid: false, loggedIn: !!locals.user, token, preview: res.data };
};

export const actions: Actions = {
	accept: async ({ request, cookies }) => {
		const empty: Record<string, string> = {};

		const data = await request.formData();
		const token = (data.get('token') ?? '').toString().trim();
		if (!token) return fail(400, { invalid: true, fieldErrors: empty, message: undefined });

		const username = (data.get('username') ?? '').toString().trim();
		const password = (data.get('password') ?? '').toString();
		const workspaceName = (data.get('workspace_name') ?? '').toString();

		const fieldErrors: Record<string, string> = {};
		if (!username) fieldErrors.username = t('err.required');
		else if (username.length < 6) fieldErrors.username = t('err.min', { n: 6 });
		if (!password) fieldErrors.password = t('err.required');
		else if (password.length < 6) fieldErrors.password = t('err.min', { n: 6 });
		if (Object.keys(fieldErrors).length) {
			return fail(400, { invalid: false, fieldErrors, message: undefined });
		}

		const res = await acceptInvitationSignup(token, username, password);
		if (!res.ok) {
			// 404 → token consumed/expired/revoked between preview and submit.
			if (res.status === 404) {
				return fail(404, { invalid: true, fieldErrors: empty, message: undefined });
			}
			// 409 → email already registered, or username taken.
			return fail(res.status || 400, {
				invalid: false,
				fieldErrors: res.fieldErrors,
				message: Object.keys(res.fieldErrors).length ? undefined : res.message
			});
		}

		setSession(cookies, res.data);

		// A freshly-created invitee belongs to exactly the room they just joined.
		// Match by name (from the preview), falling back to the sole membership.
		const list = await getWorkspaces(res.data.token);
		const ws = list.ok
			? (list.data.find((w) => w.name === workspaceName) ?? list.data[0])
			: undefined;
		redirect(303, ws ? `/workspace/${ws.slug}` : '/');
	}
};
