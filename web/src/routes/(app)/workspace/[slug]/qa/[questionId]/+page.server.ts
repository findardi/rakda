import { error, fail, redirect } from '@sveltejs/kit';
import {
	closeQuestion,
	createFaq,
	getQuestion,
	reopenQuestion,
	replyQuestion,
	resolveWorkspaceId
} from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace } = await parent();
	const res = await getQuestion(locals.session, workspace.id, params.questionId);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		if (res.status === 404) error(404, t('qa.thread.notFound'));
		if (res.status === 403) redirect(303, `/workspace/${params.slug}`);
		error(res.status || 500, t('qa.err.load'));
	}

	return { thread: res.data };
};

export const actions: Actions = {
	reply: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const body = (form.get('body') ?? '').toString().trim();
		if (!body) return fail(400, { message: t('qa.err.replyRequired') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await replyQuestion(locals.session, wsId, params.questionId, body);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			if (res.status === 409) return fail(409, { message: t('qa.err.closed') });
			if (res.status === 403) return fail(403, { message: t('qa.err.disabled') });
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { replied: true };
	},

	close: async ({ locals, params }) => {
		if (!locals.session) redirect(303, '/login');

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await closeQuestion(locals.session, wsId, params.questionId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { closed: true };
	},

	reopen: async ({ locals, params }) => {
		if (!locals.session) redirect(303, '/login');

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await reopenQuestion(locals.session, wsId, params.questionId);
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { reopened: true };
	},

	promote: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const questionText = (form.get('question_text') ?? '').toString().trim();
		const answerText = (form.get('answer_text') ?? '').toString().trim();
		if (!questionText || !answerText) return fail(400, { message: t('qa.err.faqRequired') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await createFaq(locals.session, wsId, {
			question_text: questionText,
			answer_text: answerText,
			source_question_id: params.questionId
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { promoted: true };
	}
};
