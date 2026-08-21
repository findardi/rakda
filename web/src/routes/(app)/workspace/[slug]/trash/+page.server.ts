import { error, fail, redirect } from '@sveltejs/kit';
import { canManageAccess } from '$lib/access/roles';
import {
	getTrash,
	resolveWorkspaceId,
	restoreTrashDocument,
	restoreTrashFolder
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { access, workspace } = await parent();
	if (!access || !canManageAccess(access.role)) {
		redirect(303, `/workspace/${params.slug}`);
	}

	const res = await getTrash(locals.session, workspace.id);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		if (res.status === 403) redirect(303, `/workspace/${params.slug}`);
		error(res.status || 500, t('trash.err.load'));
	}

	return { trash: res.data };
};

export const actions: Actions = {
	restoreFolder: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const folderId = (form.get('folderId') ?? '').toString();
		if (!folderId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await restoreTrashFolder(locals.session, wsId, folderId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 403) return fail(403, { message: t('trash.err.forbidden') });
			if (res.status === 404) return fail(404, { message: t('trash.err.gone') });
			if (res.status === 409) return fail(409, { message: t('trash.err.nameConflict') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { restored: res.data };
	},

	restoreDocument: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const documentId = (form.get('documentId') ?? '').toString();
		if (!documentId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await restoreTrashDocument(locals.session, wsId, documentId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 403) return fail(403, { message: t('trash.err.forbidden') });
			if (res.status === 404) return fail(404, { message: t('trash.err.gone') });
			if (res.status === 409) return fail(409, { message: t('trash.err.nameConflict') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { restored: res.data };
	}
};
