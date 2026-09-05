import type { ApiResult } from '$lib/types';
import type {
	AddMemberResult,
	AddMembersPayload,
	InvitationData,
	UpdateMemberExpiryPayload,
	UpdateMemberRolePayload,
	WorkspaceMemberData
} from '$lib/types/workspace';
import { del, get, patch, post, put } from './client';

// Bulk invite. The backend resolves account existence per email internally and
// returns a per-email outcome — it never tells the caller who was registered.
export const addMembers = (token: string, workspaceId: string, p: AddMembersPayload) =>
	post<AddMemberResult[]>(`/access/workspaces/${workspaceId}/invitations`, p, token);

// Workspace invitations. `status` filters by an exact status; omit for all.
export async function getInvitations(
	token: string,
	workspaceId: string,
	status?: string
): Promise<ApiResult<InvitationData[]>> {
	const q = status ? `?status=${encodeURIComponent(status)}` : '';
	return get<InvitationData[]>(`/access/workspaces/${workspaceId}/invitations${q}`, token);
}

// Re-issue an invitation's token and resend its email (no body).
export const resendInvitation = (token: string, workspaceId: string, invitationId: string) =>
	post<null>(
		`/access/workspaces/${workspaceId}/invitations/${invitationId}/resend`,
		undefined,
		token
	);

// Revoke a pending invitation, invalidating its link (no body).
export const revokeInvitation = (token: string, workspaceId: string, invitationId: string) =>
	post<null>(
		`/access/workspaces/${workspaceId}/invitations/${invitationId}/revoke`,
		undefined,
		token
	);

export const getMembers = (token: string, workspaceId: string) =>
	get<WorkspaceMemberData[]>(`/access/workspaces/${workspaceId}/members`, token);

export const updateMemberRole = (
	token: string,
	workspaceId: string,
	memberId: string,
	p: UpdateMemberRolePayload
) => put<WorkspaceMemberData>(`/access/workspaces/${workspaceId}/members/${memberId}`, p, token);

// Set or clear a guest's access expiry. Guest-only on the backend (400 otherwise).
export const updateMemberExpiry = (
	token: string,
	workspaceId: string,
	memberId: string,
	p: UpdateMemberExpiryPayload
) =>
	patch<WorkspaceMemberData>(
		`/access/workspaces/${workspaceId}/members/${memberId}/expiry`,
		p,
		token
	);

export const deleteMember = (token: string, workspaceId: string, memberId: string) =>
	del<null>(`/access/workspaces/${workspaceId}/members/${memberId}`, token);
