import { fail, redirect } from '@sveltejs/kit';
import {
	applyFolderTemplate,
	bulkDeleteFolders,
	createFolder,
	deleteFolder,
	moveFolder,
	renameFolder,
	resolveWorkspaceId
} from '$lib/server/api';
import { isUuid, parsePosition } from '$lib/dnd';
import { t } from '$lib/i18n';
import { LOCALE_COOKIE, defaultLocale, isLocale } from '$lib/i18n/shared';
import type { Actions } from './$types';

export const actions: Actions = {
	create: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const name = (form.get('name') ?? '').toString().trim();
		const parentId = (form.get('parentId') ?? '').toString();
		if (!name) return fail(400, { message: t('doc.err.nameRequired') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await createFolder(locals.session, wsId, { name, parent_id: parentId });
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 409) return fail(409, { message: t('doc.err.nameTaken') });
			if (res.status === 404) return fail(404, { message: t('doc.err.parentNotFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { created: true };
	},

	rename: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const folderId = (form.get('folderId') ?? '').toString();
		const name = (form.get('name') ?? '').toString().trim();
		if (!folderId || !name) return fail(400, { message: t('doc.err.nameRequired') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await renameFolder(locals.session, wsId, folderId, { name });
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 409) return fail(409, { message: t('doc.err.nameTaken') });
			if (res.status === 404) return fail(404, { message: t('doc.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { renamed: true };
	},

	move: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const folderId = (form.get('folderId') ?? '').toString();
		const parentId = (form.get('parentId') ?? '').toString();
		if (!folderId) return fail(400, { message: t('err.generic') });
		// A non-UUID parent_id reaches the server as an unparsed uuid and comes
		// back as a 500, so it is rejected here instead. '' is legal: root.
		if (parentId && !isUuid(parentId)) return fail(400, { message: t('doc.err.invalidMove') });

		const position = parsePosition(form.get('position'));

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await moveFolder(locals.session, wsId, folderId, {
			parent_id: parentId,
			...(position === null ? {} : { position })
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 403) return fail(403, { message: t('doc.err.defaultLocked') });
			if (res.status === 409) return fail(409, { message: t('doc.err.nameTaken') });
			if (res.status === 404) return fail(404, { message: t('doc.err.notFound') });
			if (res.status === 400) return fail(400, { message: t('doc.err.invalidMove') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { moved: true };
	},

	bulkDelete: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const folderIds = form
			.getAll('folderId')
			.map((v) => v.toString())
			.filter(Boolean);
		if (folderIds.length === 0) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await bulkDeleteFolders(locals.session, wsId, folderIds);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 403) return fail(403, { message: t('doc.err.defaultLocked') });
			if (res.status === 404) return fail(404, { message: t('doc.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { bulkDeleted: folderIds.length };
	},

	applyTemplate: async ({ cookies, locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const templateKey = (form.get('template') ?? '').toString();
		if (!templateKey) return fail(400, { message: t('err.generic') });

		// Folder names follow the reader's locale; the cookie is the same source
		// hooks.server.ts uses, so the action never trusts a form field for it.
		const rawLocale = cookies.get(LOCALE_COOKIE) ?? '';
		const locale = isLocale(rawLocale) ? rawLocale : defaultLocale;

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await applyFolderTemplate(locals.session, wsId, templateKey, locale);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('tpl.err.apply') });
		}

		return { applied: true, created: res.data.created_count, skipped: res.data.skipped_count };
	},

	delete: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const folderId = (form.get('folderId') ?? '').toString();
		if (!folderId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await deleteFolder(locals.session, wsId, folderId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 403) return fail(403, { message: t('doc.err.defaultLocked') });
			if (res.status === 404) return fail(404, { message: t('doc.err.notFound') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { deleted: true };
	}
};
