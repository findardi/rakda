import type { WorkspaceRoleData } from '$lib/types/workspace';
import { get } from './client';

// Roles are fixed system roles (owner/admin/guest), seeded at workspace creation.
// They are read-only via the API — there is no create/update/delete surface.
export const getRoles = (token: string, workspaceId: string) =>
	get<WorkspaceRoleData[]>(`/access/workspaces/${workspaceId}/roles`, token);

export const getRole = (token: string, workspaceId: string, roleId: string) =>
	get<WorkspaceRoleData>(`/access/workspaces/${workspaceId}/roles/${roleId}`, token);

// Permission catalog — single source of truth lives in the Go `permission.All`.
// Used read-only to render each role's granted permissions grouped by resource.
export const getPermissions = (token: string) => get<string[]>(`/access/permissions`, token);
