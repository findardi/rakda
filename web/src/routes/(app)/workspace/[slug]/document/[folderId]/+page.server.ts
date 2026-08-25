import { error, fail, redirect } from '@sveltejs/kit';
import {
	bulkDeleteDocuments,
	deleteDocument,
	listDocuments,
	moveDocument,
	resolveWorkspaceId
} from '$lib/server/api';
import { isUuid, parsePosition } from '$lib/dnd';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace } = await parent();

	const res = await listDocuments(locals.session, workspace.id, params.folderId);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		if (res.status === 403) return { documents: [], forbidden: true };
		if (res.status === 404) error(404, t('doc.err.notFound'));
		error(res.status || 500, t('doc.docs.err.load'));
	}

	return { documents: res.data ?? [], forbidden: false };
};

export const actions: Actions = {
	moveDocument: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const documentId = (form.get('documentId') ?? '').toString();
		const folderId = (form.get('folderId') ?? '').toString();
		if (!documentId || !folderId) return fail(400, { message: t('err.generic') });
		// A non-UUID document id in the path turns into a 500 upstream; reject here.
		if (!isUuid(documentId) || !isUuid(folderId))
			return fail(400, { message: t('doc.docs.err.invalidMove') });

		const position = parsePosition(form.get('position'));

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await moveDocument(locals.session, wsId, documentId, {
			folder_id: folderId,
			...(position === null ? {} : { position })
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('doc.docs.err.notFound') });
			if (res.status === 400) return fail(400, { message: t('doc.docs.err.invalidMove') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { moved: true };
	},

	bulkDeleteDocuments: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const documentIds = form
			.getAll('documentId')
			.map((v) => v.toString())
			.filter(Boolean);
		if (documentIds.length === 0) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await bulkDeleteDocuments(locals.session, wsId, documentIds);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('doc.docs.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { bulkDeleted: documentIds.length };
	},

	deleteDocument: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const documentId = (form.get('documentId') ?? '').toString();
		if (!documentId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await deleteDocument(locals.session, wsId, documentId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('doc.docs.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { deleted: true };
	}
};
