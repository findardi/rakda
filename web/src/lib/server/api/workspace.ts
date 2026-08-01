import type { ApiResult } from '$lib/types';
import type {
	CreateWorkspacePayload,
	MyAccessWorkspace,
	UpdateWorkspacePayload,
	WorkspaceData,
	WorkspaceStatus
} from '$lib/types/workspace';
import { del, get, patch, post, put } from './client';

// All endpoints are JWT-protected (RequireAuth + RequireActive) — pass the token.
// By-id operations are additionally owner-only (RequireOwner) on the backend.

export async function getWorkspaces(token: string): Promise<ApiResult<WorkspaceData[]>> {
	return get<WorkspaceData[]>('/workspaces/', token);
}

export async function createWorkspace(
	token: string,
	p: CreateWorkspacePayload
): Promise<ApiResult<WorkspaceData>> {
	return post<WorkspaceData>('/workspaces/', p, token);
}

export async function getWorkspace(token: string, id: string): Promise<ApiResult<WorkspaceData>> {
	return get<WorkspaceData>(`/workspaces/${id}`, token);
}

export async function updateWorkspace(
	token: string,
	id: string,
	p: UpdateWorkspacePayload
): Promise<ApiResult<WorkspaceData>> {
	return put<WorkspaceData>(`/workspaces/${id}`, p, token);
}

// Status PATCH returns an empty data envelope (200, data: null).
export async function updateWorkspaceStatus(
	token: string,
	id: string,
	status: WorkspaceStatus
): Promise<ApiResult<null>> {
	return patch<null>(`/workspaces/${id}/status`, { status }, token);
}

// Delete returns an empty data envelope (200, data: null), not 204.
export async function deleteWorkspace(token: string, id: string): Promise<ApiResult<null>> {
	return del<null>(`/workspaces/${id}`, token);
}

export async function getMyAccessWorkspace(
	token: string,
	id: string
): Promise<ApiResult<MyAccessWorkspace>> {
	return get<MyAccessWorkspace>(`/access/workspaces/${id}/me`, token);
}
