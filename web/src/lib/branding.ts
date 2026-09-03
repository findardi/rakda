import type { WorkspaceData } from '$lib/types/workspace';

// The logo is fetched through the SvelteKit proxy (session cookie → Bearer),
// never from object storage; the version token changes with every upload, so
// the day-long private cache can never show a stale picture.
export function workspaceLogoUrl(ws: Pick<WorkspaceData, 'id' | 'logo'>): string {
	return ws.logo ? `/api/workspaces/${ws.id}/branding/logo?v=${encodeURIComponent(ws.logo)}` : '';
}

// Lightness and chroma are fixed here on purpose: a room picks a hue, never a
// loud colour, so the hero can never compete with the information on top of it.
export function heroColor(hue: number): string {
	return `oklch(0.45 0.07 ${hue})`;
}
