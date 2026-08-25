import { error, fail, redirect } from '@sveltejs/kit';
import {
	assignMembers,
	getGroupDetail,
	getGroups,
	getMembers,
	resolveWorkspaceId,
	unassignMember,
	updateGroupQA
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace } = await parent();

	const [groupsRes, detailRes, membersRes] = await Promise.all([
		getGroups(locals.session, workspace.id),
		getGroupDetail(locals.session, workspace.id, params.groupId),
		getMembers(locals.session, workspace.id)
	]);

	if (!groupsRes.ok) {
		if (groupsRes.status === 401) redirect(303, '/login');
		error(groupsRes.status || 502, t('group.err.loadError'));
	}
	const group = groupsRes.data.find((g) => g.id === params.groupId);
	if (!group) error(404, t('group.err.notFound'));

	if (!detailRes.ok) {
		if (detailRes.status === 401) redirect(303, '/login');
		error(detailRes.status || 502, t('group.err.loadError'));
	}
	if (!membersRes.ok) {
		if (membersRes.status === 401) redirect(303, '/login');
		error(membersRes.status || 502, t('member.err.loadError'));
	}

	return {
		group,
		defaultGroup: groupsRes.data.find((g) => g.is_default) ?? null,
		members: detailRes.data ?? [],
		workspaceMembers: membersRes.data ?? []
	};
};

export const actions: Actions = {
	assign: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const memberIds = form.getAll('memberId').map((v) => v.toString());
		if (memberIds.length === 0) return fail(400, { message: t('group.assign.empty') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await assignMembers(locals.session, wsId, params.groupId, {
			member_id: memberIds
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { assigned: memberIds.length };
	},

	unassign: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const memberId = (form.get('memberId') ?? '').toString();
		if (!memberId) return fail(400, { message: t('err.generic') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await unassignMember(locals.session, wsId, params.groupId, memberId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { unassigned: true };
	},

	// Q&A switch + limit live on their own PATCH — deliberately not part of the
	// name/description PUT, which full-replaces and would reset them.
	qa: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const qaEnabled = form.get('qa_enabled') === 'on';
		const rawLimit = (form.get('question_limit') ?? '').toString().trim();
		let questionLimit: number | null = null;
		if (rawLimit !== '') {
			const n = Number(rawLimit);
			if (!Number.isInteger(n) || n < 0) {
				return fail(400, { message: t('qa.err.limitInvalid') });
			}
			questionLimit = n;
		}

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await updateGroupQA(locals.session, wsId, params.groupId, {
			qa_enabled: qaEnabled,
			question_limit: questionLimit
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { qaSaved: true };
	}
};
