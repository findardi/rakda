import { error, redirect } from '@sveltejs/kit';
import { canManageAccess } from '$lib/access/roles';
import { getDocumentReaders } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { LayoutServerLoad } from './$types';

// The reader list rides the layout: picking a reader changes only the search
// params, which re-runs the page load below and leaves this one alone.
export const load: LayoutServerLoad = async ({ locals, params, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { access, workspace } = await parent();
	// Guests are recorded, never readers of the record. Upstream answers 403
	// too; landing them on the overview beats an error page.
	if (!access || !canManageAccess(access.role)) redirect(303, `/workspace/${params.slug}`);

	const res = await getDocumentReaders(locals.session, workspace.id, params.documentId);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		if (res.status === 403) redirect(303, `/workspace/${params.slug}`);
		if (res.status === 404) error(404, t('doc.view.err.notFound'));
		error(res.status || 500, t('activity.engagement.err.load'));
	}

	return {
		workspaceId: workspace.id,
		document: {
			id: res.data.document_id,
			name: res.data.document_name,
			pageCount: res.data.page_count
		},
		readers: res.data.readers ?? [],
		totalReadMs: res.data.total_read_ms
	};
};
