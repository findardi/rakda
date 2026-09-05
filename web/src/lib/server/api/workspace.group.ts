import type {
	AssignMembersPayload,
	GroupMemberData,
	GroupQAPayload,
	GroupWorkspaceData,
	UpsertGroupWorkspacePayload
} from '$lib/types/workspace';
import { del, get, patch, post, put } from './client';

export const createGroup = (token: string, workspaceId: string, p: UpsertGroupWorkspacePayload) =>
	post<GroupWorkspaceData>(`/access/workspaces/${workspaceId}/groups`, p, token);

export const updateGroup = (
	token: string,
	workspaceId: string,
	groupId: string,
	p: UpsertGroupWorkspacePayload
) => put<GroupWorkspaceData>(`/access/workspaces/${workspaceId}/groups/${groupId}`, p, token);

// Q&A switch + limit travel on their own PATCH — never on the full-replace PUT
// above, which would silently reset them from stale forms.
export const updateGroupQA = (
	token: string,
	workspaceId: string,
	groupId: string,
	p: GroupQAPayload
) => patch<GroupWorkspaceData>(`/access/workspaces/${workspaceId}/groups/${groupId}/qa`, p, token);

export const getGroups = (token: string, workspaceId: string) =>
	get<GroupWorkspaceData[]>(`/access/workspaces/${workspaceId}/groups`, token);

export const deleteGroup = (token: string, workspaceId: string, groupId: string) =>
	del<null>(`/access/workspaces/${workspaceId}/groups/${groupId}`, token);

// Group detail = the members assigned to it (the backend returns the member
// list, not the group's name/description).
export const getGroupDetail = (token: string, workspaceId: string, groupId: string) =>
	get<GroupMemberData[]>(`/access/workspaces/${workspaceId}/groups/${groupId}`, token);

// Assign workspace members to a group. Idempotent — already-assigned ids are
// skipped server-side. Returns the group's updated member list.
export const assignMembers = (
	token: string,
	workspaceId: string,
	groupId: string,
	p: AssignMembersPayload
) =>
	post<GroupMemberData[]>(`/access/workspaces/${workspaceId}/groups/${groupId}/assign`, p, token);

export const unassignMember = (
	token: string,
	workspaceId: string,
	groupId: string,
	memberId: string
) => del<null>(`/access/workspaces/${workspaceId}/groups/${groupId}/unassign/${memberId}`, token);
