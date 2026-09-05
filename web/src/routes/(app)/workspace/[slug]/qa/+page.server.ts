import { fail, redirect } from '@sveltejs/kit';
import { isManager } from '$lib/access/roles';
import { createQuestion, getGroups, resolveWorkspaceId } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { GroupWorkspaceData } from '$lib/types/workspace';
import type { Actions, PageServerLoad } from './$types';

// The list itself comes from the qa layout load; the index only adds the
// group options for the manager's filter select.
export const load: PageServerLoad = async ({ locals, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { access, workspace } = await parent();
	let groups: GroupWorkspaceData[] = [];
	if (access && isManager(access.role)) {
		const res = await getGroups(locals.session, workspace.id);
		if (res.ok) groups = res.data;
	}

	return { groups };
};

export const actions: Actions = {
	ask: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const subject = (form.get('subject') ?? '').toString().trim();
		const body = (form.get('body') ?? '').toString().trim();
		const documentId = (form.get('document_id') ?? '').toString();
		if (!subject) return fail(400, { message: t('qa.err.subjectRequired') });
		if (!body) return fail(400, { message: t('qa.err.bodyRequired') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await createQuestion(locals.session, wsId, {
			subject,
			body,
			...(documentId ? { document_id: documentId } : {})
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 409) return fail(409, { message: t('qa.err.limit') });
			if (res.status === 403) return fail(403, { message: t('qa.err.disabled') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { asked: true };
	}
};
