import { fail, redirect } from '@sveltejs/kit';
import { createWorkspace, getWorkspaces } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals }) => {
	if (!locals.user || !locals.session) redirect(303, '/login');
	if (locals.user.status === 'pending') redirect(303, '/verify-email');

	const res = await getWorkspaces(locals.session);
	if (!res.ok) {
		return { workspaces: [], ownedCount: 0, ownedLimit: 0, loadError: res.message };
	}
	return {
		workspaces: res.data.workspaces,
		ownedCount: res.data.owned_count,
		ownedLimit: res.data.owned_limit,
		loadError: null as string | null
	};
};

export const actions: Actions = {
	create: async ({ locals, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const name = (form.get('name') ?? '').toString().trim();
		const description = (form.get('description') ?? '').toString().trim();

		// Mirror the backend `name required` rule client-side to save a round trip.
		if (!name) {
			return fail(400, { message: null, fieldErrors: { name: t('err.required') } });
		}

		const res = await createWorkspace(locals.session, { name, description });
		if (!res.ok) {
			return fail(res.status || 400, {
				message: mapCreateMessage(res.fieldErrors, res.message),
				fieldErrors: res.fieldErrors
			});
		}

		return { created: res.data };
	}
};

// Map backend form-level create errors to localized copy.
function mapCreateMessage(fieldErrors: Record<string, string>, raw: string): string | null {
	if (Object.keys(fieldErrors).length) return null; // field-level handles it
	const m = raw.toLowerCase();
	if (m.includes('already taken')) return t('ws.err.nameTaken');
	if (m.includes('empty slug')) return t('ws.err.nameInvalid');
	if (m.includes('exceed') || m.includes('limit')) return t('ws.err.limit');
	return raw || t('err.generic');
}
