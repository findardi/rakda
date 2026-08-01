import { error, redirect } from '@sveltejs/kit';
import { normalizeRole } from '$lib/access/roles';
import { getViewMeta, listVersions } from '$lib/server/api';
import { t } from '$lib/i18n';
import type { VersionData } from '$lib/types/content';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, params, parent, url }) => {
	if (!locals.session) redirect(303, '/login');

	const { workspace, access } = await parent();

	// The version being read lives in the URL so a link to it is shareable and a
	// switch is an ordinary navigation. Absent means the current version, which
	// is all a guest is ever served.
	const version = url.searchParams.get('version') ?? undefined;

	// History (and therefore the switcher) is owner/admin upstream. Asking as a
	// guest would only earn a 403, so don't ask.
	const role = normalizeRole(access?.role ?? '');
	const mayListVersions = role === 'owner' || role === 'admin';

	const [res, verRes] = await Promise.all([
		getViewMeta(locals.session, workspace.id, params.documentId, version),
		mayListVersions ? listVersions(locals.session, workspace.id, params.documentId) : null
	]);

	if (!res.ok) {
		if (res.status === 401) redirect(303, '/login');
		// Guest whose group has no view access on this folder — or a guest who
		// hand-edited `?version=` to a version that is not the current one.
		if (res.status === 403)
			return { meta: null, versions: [], forbidden: true, notViewable: false };
		// Download-only formats (spreadsheets, video, archives) have no rendition.
		if (res.status === 422)
			return { meta: null, versions: [], forbidden: false, notViewable: true };
		if (res.status === 404) error(404, t('doc.view.err.notFound'));
		error(res.status || 500, t('doc.view.err.load'));
	}

	// A failed history read is not worth failing the reader over: the document
	// still opens, just without the switcher.
	const versions: VersionData[] = verRes?.ok ? (verRes.data ?? []) : [];

	return { meta: res.data, versions, forbidden: false, notViewable: false };
};
