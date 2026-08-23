import { error, fail, redirect } from '@sveltejs/kit';
import {
	getDownloadLimits,
	getFolderAccess,
	removeFolderAccess,
	resolveWorkspaceId,
	setFolderAccess
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type {
	DirectFolderAccess,
	FolderAccessPanel,
	FolderTreeNode,
	InheritedFolderAccess
} from '$lib/types/content';
import type { Actions, PageServerLoad } from './$types';

function trail(
	nodes: FolderTreeNode[],
	id: string,
	path: FolderTreeNode[] = []
): FolderTreeNode[] | null {
	for (const n of nodes) {
		const next = [...path, n];
		if (n.id === id) return next;
		const hit = trail(n.children ?? [], id, next);
		if (hit) return hit;
	}
	return null;
}

export const load: PageServerLoad = async ({ locals, params, parent }) => {
	const token = locals.session;
	if (!token) redirect(303, '/login');

	const { workspace, access, folders } = await parent();
	if (!access?.permissions?.includes('group:assign')) error(403, t('doc.access.forbidden'));

	const path = trail(folders, params.folderId);
	if (!path) error(404, t('facc.err.notFound'));

	const ancestors = path.slice(0, -1).reverse();

	const [limits, self, ...chain] = await Promise.all([
		getDownloadLimits(token, workspace.id),
		getFolderAccess(token, workspace.id, params.folderId),
		...ancestors.map((a) => getFolderAccess(token, workspace.id, a.id))
	]);

	if (!self.ok) {
		if (self.status === 401) redirect(303, '/login');
		error(self.status || 500, t('facc.err.load'));
	}

	const nearest = new Map<string, InheritedFolderAccess>();
	ancestors.forEach((ancestor, i) => {
		const res = chain[i];
		if (!res?.ok) return;
		for (const row of res.data) {
			if (nearest.has(row.group_id)) continue;
			nearest.set(row.group_id, {
				...row,
				source_folder_id: ancestor.id,
				source_folder_name: ancestor.name
			});
		}
	});

	const direct: DirectFolderAccess[] = self.data.map((row) => ({
		...row,
		shadows: nearest.get(row.group_id) ?? null
	}));

	const claimed = new Set(direct.map((r) => r.group_id));
	const inherited = [...nearest.values()]
		.filter((row) => !claimed.has(row.group_id))
		.sort((a, b) => a.group_name.localeCompare(b.group_name));

	const panel: FolderAccessPanel = { folder_id: params.folderId, direct, inherited };
	const watermarkMaxPages = limits.ok ? limits.data.watermark_download_max_pages : null;
	return { panel, watermarkMaxPages };
};

export const actions: Actions = {
	setAccess: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const groupId = (form.get('groupId') ?? '').toString();
		if (!groupId) return fail(400, { message: t('facc.err.pick') });

		const canView = form.get('canView') === 'true';
		const canDownload = form.get('canDownload') === 'true';
		const canWatermark = form.get('canWatermark') === 'true';
		const canDownloadOriginal = form.get('canDownloadOriginal') === 'true';

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await setFolderAccess(locals.session, wsId, params.folderId, {
			group_id: groupId,
			can_view: canView,
			can_download: canDownload,
			can_watermark: canWatermark,
			can_download_original: canDownloadOriginal
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('facc.err.notFound') });
			if (res.status === 400) return fail(400, { message: t('facc.err.invalid') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { accessSet: true };
	},

	removeAccess: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const groupId = (form.get('groupId') ?? '').toString();
		if (!groupId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await removeFolderAccess(locals.session, wsId, params.folderId, groupId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 404) return fail(404, { message: t('facc.err.notFound') });
			if (res.status === 400) return fail(400, { message: t('facc.err.invalid') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { accessRemoved: true };
	}
};
