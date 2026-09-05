import { redirect } from '@sveltejs/kit';
import { getReaderPages } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { ReaderPages } from '$lib/types/activity';
import type { PageServerLoad } from './$types';

type Detail = {
	readerId: string | null;
	detail: ReaderPages | null;
	detailError: string | null;
	readerMissing: boolean;
};

// The selected reader lives in the URL so a link to one person's reading is
// shareable and back/forward walks between readers.
export const load: PageServerLoad = async ({ locals, params, parent, url }): Promise<Detail> => {
	if (!locals.session) redirect(303, '/login');

	const none: Detail = { readerId: null, detail: null, detailError: null, readerMissing: false };
	const readerId = url.searchParams.get('reader');
	if (!readerId) return none;

	const { workspaceId, readers } = await parent();
	// Checked against the list already loaded: a stale or hand-edited id is
	// answered here in words, not by a 400 from upstream.
	if (!readers.some((r) => r.actor_id === readerId)) {
		return { ...none, readerId, readerMissing: true };
	}

	const res = await getReaderPages(locals.session, workspaceId, params.documentId, readerId);
	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		// The list is still worth the page; the failure names itself in place.
		return { ...none, readerId, detailError: t('activity.engagement.err.load') };
	}

	return { ...none, readerId, detail: res.data };
};
