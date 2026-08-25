import { error, redirect } from '@sveltejs/kit';
import { canManageAccess } from '$lib/access/roles';
import { listQuestions } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { QaFilters, QaListData } from '$lib/types/qa';
import type { LayoutServerLoad } from './$types';

const PAGE_SIZE = 50;

// One load for the whole Q&A section: initial list, both tab counts, the
// caller's quota and qa_enabled — a single source so they never disagree.
export const load: LayoutServerLoad = async ({ locals, params, parent, url }) => {
	if (!locals.session) redirect(303, '/login');

	const { access, workspace } = await parent();
	const isManager = !!access && canManageAccess(access.role);

	const filters: QaFilters = {
		status: url.searchParams.get('status') ?? '',
		group_id: isManager ? (url.searchParams.get('group_id') ?? '') : ''
	};

	const res = await listQuestions(locals.session, workspace.id, {
		limit: PAGE_SIZE,
		...filters
	});

	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		if (res.status === 403) redirect(303, `/workspace/${params.slug}`);
		if (res.status === 400) {
			const empty: QaListData = {
				items: [],
				next_cursor: '',
				question_count: 0,
				faq_count: 0,
				qa_enabled: true
			};
			return {
				workspaceId: workspace.id,
				qa: empty,
				filters,
				pageSize: PAGE_SIZE,
				isManager,
				filterRejected: true
			};
		}
		error(res.status || 500, t('qa.err.load'));
	}

	return {
		workspaceId: workspace.id,
		qa: res.data,
		filters,
		pageSize: PAGE_SIZE,
		isManager,
		filterRejected: false
	};
};
