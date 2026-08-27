import { fail, redirect } from '@sveltejs/kit';
import { canEditWorkspace } from '$lib/access/roles';
import {
	deleteWorkspace,
	getMembers,
	getWorkspaces,
	updateWorkspace,
	updateWorkspaceStatus
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { WorkspaceStatus } from '$lib/types/workspace';
import type { Actions, PageServerLoad } from './$types';

// The archive confirmation carries the number of guests that lose downloads, so
// it is only worth a request when archiving is actually reachable from here.
export const load: PageServerLoad = async ({ locals, parent }) => {
	const { access, workspace, roomStatus } = await parent();
	if (!locals.session || !canEditWorkspace(access.role) || roomStatus === 'archive') {
		return { guestCount: 0 };
	}

	const res = await getMembers(locals.session, workspace.id);
	return { guestCount: res.ok ? res.data.filter((m) => m.role_name === 'guest').length : 0 };
};

const STATUSES: WorkspaceStatus[] = ['prepare', 'active', 'archive'];

// Routes are by id, but we navigate by slug — resolve via the owner-scoped list.
async function resolveId(session: string, slug: string): Promise<string | null> {
	const list = await getWorkspaces(session);
	if (!list.ok) return null;
	return list.data.find((w) => w.slug === slug)?.id ?? null;
}

export const actions: Actions = {
	updateStatus: async ({ locals, request, params }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const status = (form.get('status') ?? '').toString();
		if (!STATUSES.includes(status as WorkspaceStatus)) {
			return fail(400, { message: t('ws.err.invalidStatus') });
		}

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await updateWorkspaceStatus(locals.session, id, status as WorkspaceStatus);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { status: status as WorkspaceStatus };
	},

	update: async ({ locals, request, params }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const name = (form.get('name') ?? '').toString().trim();
		const description = (form.get('description') ?? '').toString().trim();

		// Mirror the backend `name required` rule to save a round trip.
		if (!name) {
			return fail(400, { message: null, fieldErrors: { name: t('err.required') } });
		}

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound'), fieldErrors: {} });

		const res = await updateWorkspace(locals.session, id, { name, description });
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, {
				message: mapUpdateMessage(res.fieldErrors, res.message),
				fieldErrors: res.fieldErrors
			});
		}

		// Renaming reslugs the room: if the slug moved, the current URL is stale, so
		// land on the authoritative one. If it held, return success so the page can
		// confirm inline (toast) without a navigation.
		if (res.data.slug !== params.slug) {
			redirect(303, `/workspace/${res.data.slug}`);
		}
		return { updated: res.data };
	},

	delete: async ({ locals, params }) => {
		if (!locals.session) redirect(303, '/login');

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await deleteWorkspace(locals.session, id);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		redirect(303, '/workspace');
	}
};

// Map backend form-level update errors to localized copy.
function mapUpdateMessage(fieldErrors: Record<string, string>, raw: string): string | null {
	if (Object.keys(fieldErrors).length) return null; // field-level handles it
	const m = raw.toLowerCase();
	if (m.includes('already taken')) return t('ws.err.nameTaken');
	if (m.includes('empty slug')) return t('ws.err.nameInvalid');
	return raw || t('err.generic');
}
