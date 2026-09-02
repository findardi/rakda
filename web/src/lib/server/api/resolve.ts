import { getWorkspaces } from './workspace';

// Routes are by id, but we navigate by slug. Resolving through the owner-scoped
// list also ties the action to a workspace the caller actually owns.
export async function resolveWorkspaceId(token: string, slug: string): Promise<string | null> {
	const list = await getWorkspaces(token);
	if (!list.ok) return null;
	return list.data.workspaces.find((w) => w.slug === slug)?.id ?? null;
}
