import { error, fail, redirect } from '@sveltejs/kit';
import { createFaq, listFaqs, resolveWorkspaceId } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params, parent }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace } = await parent();
	const res = await listFaqs(locals.session, workspace.id);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		if (res.status === 403) redirect(303, `/workspace/${params.slug}`);
		error(res.status || 500, t('qa.err.load'));
	}

	return { faqs: res.data };
};

export const actions: Actions = {
	faq: async ({ locals, params, request }) => {
		if (!locals.session) redirect(303, '/login');

		const form = await request.formData();
		const questionText = (form.get('question_text') ?? '').toString().trim();
		const answerText = (form.get('answer_text') ?? '').toString().trim();
		const sourceQuestionId = (form.get('source_question_id') ?? '').toString();
		if (!questionText || !answerText) return fail(400, { message: t('qa.err.faqRequired') });

		const wsId = await resolveWorkspaceId(locals.session, params.slug);
		if (!wsId) return fail(404, { message: t('ws.detail.notFound') });

		const res = await createFaq(locals.session, wsId, {
			question_text: questionText,
			answer_text: answerText,
			...(sourceQuestionId ? { source_question_id: sourceQuestionId } : {})
		});
		if (!res.ok) {
			if (res.status === 401) redirect(303, '/login');
			return fail(res.status || 400, { message: res.message || t('err.generic') });
		}

		return { published: true };
	}
};
