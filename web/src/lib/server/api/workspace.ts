import type { ApiResult } from '$lib/types';
import type {
	CreateWorkspacePayload,
	HeroPreset,
	MyAccessWorkspace,
	UpdateWorkspacePayload,
	WorkspaceData,
	WorkspaceListData,
	WorkspaceStatus,
	WorkspaceSummaryData
} from '$lib/types/workspace';
import { API_URL, del, get, patch, post, put, putForm, upstreamHeaders } from './client';

// All endpoints are JWT-protected (RequireAuth + RequireActive) — pass the token.
// By-id operations are additionally owner-only (RequireOwner) on the backend.

export const getWorkspaces = (token: string) => get<WorkspaceListData>('/workspaces/', token);

export const createWorkspace = (token: string, p: CreateWorkspacePayload) =>
	post<WorkspaceData>('/workspaces/', p, token);

export const getWorkspaceSummary = (token: string, id: string) =>
	get<WorkspaceSummaryData>(`/workspaces/${id}/summary`, token);

export const updateWorkspace = (token: string, id: string, p: UpdateWorkspacePayload) =>
	put<WorkspaceData>(`/workspaces/${id}`, p, token);

// Status PATCH returns an empty data envelope (200, data: null).
export const updateWorkspaceStatus = (token: string, id: string, status: WorkspaceStatus) =>
	patch<null>(`/workspaces/${id}/status`, { status }, token);

// Delete returns an empty data envelope (200, data: null), not 204.
export const deleteWorkspace = (token: string, id: string) => del<null>(`/workspaces/${id}`, token);

// --- branding (owner-only mutations; the logo read is member-gated) --------

export const getHeroPresets = (token: string) =>
	get<HeroPreset[]>('/workspaces/hero-presets', token);

// The picture goes through the server, which sniffs, resizes, and re-encodes
// it; the backend answers 413 / 415 / 400 for size, format, and content.
export async function uploadWorkspaceLogo(
	token: string,
	id: string,
	file: File
): Promise<ApiResult<WorkspaceData>> {
	const form = new FormData();
	form.set('file', file, file.name || 'logo');
	return putForm<WorkspaceData>(`/workspaces/${id}/branding/logo`, form, token);
}

export const removeWorkspaceLogo = (token: string, id: string) =>
	del<WorkspaceData>(`/workspaces/${id}/branding/logo`, token);

// Empty preset = back to the automatic identity.
export const setWorkspaceHero = (token: string, id: string, preset: string) =>
	put<WorkspaceData>(`/workspaces/${id}/branding/hero`, { preset }, token);

// Raw upstream response for the logo proxy: PNG bytes (or 304), not an
// envelope, so it bypasses the typed client like fetchViewPage does.
export function fetchWorkspaceLogo(
	token: string,
	id: string,
	ifNoneMatch?: string
): Promise<Response> {
	const headers = upstreamHeaders(token);
	if (ifNoneMatch) headers['if-none-match'] = ifNoneMatch;
	return fetch(`${API_URL}/workspaces/${id}/branding/logo`, { headers });
}

export const getMyAccessWorkspace = (token: string, id: string) =>
	get<MyAccessWorkspace>(`/access/workspaces/${id}/me`, token);
