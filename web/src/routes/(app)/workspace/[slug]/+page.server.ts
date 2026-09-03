import { fail, redirect } from '@sveltejs/kit';
import { canEditWorkspace, canManageAccess } from '$lib/access/roles';
import {
	createArchive,
	deleteArchive,
	deleteWorkspace,
	getHeroPresets,
	getWorkspaces,
	getWorkspaceSummary,
	listActivity,
	listArchives,
	removeWorkspaceLogo,
	setWorkspaceHero,
	updateWorkspace,
	updateWorkspaceStatus,
	uploadWorkspaceLogo
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { WorkspaceStatus } from '$lib/types/workspace';
import type { Actions, PageServerLoad } from './$types';

// Summary (counts, manager-only 403 for guests), the recent-activity strip, and
// the archive list all belong to owner/admin; guests get none of them and their
// "recently visited" lives in localStorage, never on the server. The archive
// confirmation reuses summary.guest_count for its counted button.
export const load: PageServerLoad = async ({ locals, parent }) => {
	const { access, workspace } = await parent();
	if (!locals.session) {
		return { guestCount: 0, archives: [], summary: null, recentActivity: [], heroPresets: [] };
	}

	const isManager = canManageAccess(access.role);
	// The preset picker is owner-only; everyone else renders the hero from the
	// hue/phase the workspace record already carries.
	const isOwner = canEditWorkspace(access.role);

	const [summary, archives, activity, presets] = await Promise.all([
		isManager ? getWorkspaceSummary(locals.session, workspace.id) : null,
		isManager ? listArchives(locals.session, workspace.id) : null,
		isManager ? listActivity(locals.session, workspace.id, { limit: 5 }) : null,
		isOwner ? getHeroPresets(locals.session) : null
	]);

	return {
		summary: summary?.ok ? summary.data : null,
		guestCount: summary?.ok ? summary.data.guest_count : 0,
		archives: archives?.ok ? archives.data : [],
		recentActivity: activity?.ok ? activity.data.items : [],
		heroPresets: presets?.ok ? presets.data : []
	};
};

const STATUSES: WorkspaceStatus[] = ['prepare', 'active', 'archive'];

// Routes are by id, but we navigate by slug — resolve via the owner-scoped list.
async function resolveId(session: string, slug: string): Promise<string | null> {
	const list = await getWorkspaces(session);
	if (!list.ok) return null;
	return list.data.workspaces.find((w) => w.slug === slug)?.id ?? null;
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

	uploadLogo: async ({ locals, request, params }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const file = form.get('file');
		if (!(file instanceof File) || file.size === 0) {
			return fail(400, { message: t('err.generic') });
		}

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await uploadWorkspaceLogo(locals.session, id, file);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { logoUpdated: true };
	},

	removeLogo: async ({ locals, params }) => {
		if (!locals.session) redirect(303, '/login');

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await removeWorkspaceLogo(locals.session, id);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { logoRemoved: true };
	},

	setHero: async ({ locals, request, params }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		// Empty = automatic; the backend validates the key against its own list.
		const preset = (form.get('preset') ?? '').toString();

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await setWorkspaceHero(locals.session, id, preset);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { heroUpdated: true };
	},

	createArchive: async ({ locals, params }) => {
		if (!locals.session) redirect(303, '/login');

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await createArchive(locals.session, id);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 409) return fail(409, { message: t('archive.err.pending') });
			if (res.status === 429) return fail(429, { message: t('archive.err.busy') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { archive: res.data };
	},

	deleteArchive: async ({ locals, request, params }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const archiveId = (form.get('archive_id') ?? '').toString();
		if (!archiveId) return fail(400, { message: t('err.generic') });

		const id = await resolveId(locals.session, params.slug);
		if (!id) return fail(404, { message: t('ws.detail.notFound') });

		const res = await deleteArchive(locals.session, id, archiveId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { archiveDeleted: archiveId };
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
