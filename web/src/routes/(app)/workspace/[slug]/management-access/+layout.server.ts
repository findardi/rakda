import { error, redirect } from '@sveltejs/kit';
import { isManager } from '$lib/access/roles';
import { t } from '$lib/i18n';
import type { LayoutServerLoad } from './$types';

// Authoritative gate for the whole access-management subtree. The room layout
// already loaded the viewer's standing; here we refuse non-managers (guests)
// outright rather than relying on the sidebar hiding the link.
export const load: LayoutServerLoad = async ({ parent, params }) => {
	const { access } = await parent();
	if (!access || !isManager(access.role)) {
		redirect(303, `/workspace/${params.slug}`);
	}
};
