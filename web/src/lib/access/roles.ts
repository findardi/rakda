// Role-based capability gate. The backend (`permission` pkg) is the real
// authority; this mirrors the *intent* of the three system roles so the UI can
// hide controls and the room layout can block routes. Server load still enforces
// the gate — these helpers are convenience, not security.
//
// Strict three-role model: owner / admin / guest. Any other role name (e.g. a
// legacy contributor/viewer still returned by the backend) is treated as guest,
// i.e. least privilege.

import type { WorkspaceStatus } from '$lib/types/workspace';

export type WorkspaceRole = 'owner' | 'admin' | 'guest';

export function normalizeRole(role: string): WorkspaceRole {
	return role === 'owner' || role === 'admin' ? role : 'guest';
}

const MANAGER: WorkspaceRole[] = ['owner', 'admin'];

/** May open the access-management area (members / roles / groups). */
export function canManageAccess(role: string): boolean {
	return MANAGER.includes(normalizeRole(role));
}

/** May invite, change role, or remove members. */
export function canManageMembers(role: string): boolean {
	return MANAGER.includes(normalizeRole(role));
}

/** May create, edit, or delete workspace roles. */
export function canManageRoles(role: string): boolean {
	return MANAGER.includes(normalizeRole(role));
}

/** May create, edit, or delete groups. */
export function canManageGroups(role: string): boolean {
	return MANAGER.includes(normalizeRole(role));
}

// Workspace-level mutations (edit details, change status, delete) are all
// guarded by `RequireOwner` on the backend (workspace/module.go) — owner only,
// NOT admin. Keep these two split in case the backend later relaxes edit/status.

/** Owner-only: edit workspace details and change its status. */
export function canEditWorkspace(role: string): boolean {
	return normalizeRole(role) === 'owner';
}

/** Owner-only: delete the workspace itself. */
export function canDeleteWorkspace(role: string): boolean {
	return normalizeRole(role) === 'owner';
}

// Which roles the viewer may grant (when inviting or changing a member's role).
// Mirrors the backend hardening: owner grants anything but owner; admin may only
// grant guest (can't promote/demote into the privileged tier); guest grants none.
// Generic so it filters both WorkspaceRoleData and any `{ name }` list.
export function assignableRoles<T extends { name: string }>(viewerRole: string, roles: T[]): T[] {
	const r = normalizeRole(viewerRole);
	if (r === 'owner') return roles.filter((x) => x.name !== 'owner');
	if (r === 'admin') return roles.filter((x) => x.name === 'guest');
	return [];
}

// Room lifecycle. Mirrors the server gate (platform/middleware
// RequireRoomOpenForGuests + RequireRoomWritable) so the UI can stop offering
// what the server will refuse. Same disclaimer as the role helpers above: the
// server decides, these only shape what is shown.

export function isRoomReadOnly(status: string): boolean {
	return status === 'archive';
}

export function isRoomOpenTo(status: string, role: string): boolean {
	return !(status === 'prepare' && normalizeRole(role) === 'guest');
}

export function canMutateRoom(status: string, role: string): boolean {
	return !isRoomReadOnly(status) && isRoomOpenTo(status, role);
}

const ROOM_TRANSITIONS: Record<string, WorkspaceStatus[]> = {
	prepare: ['active', 'archive'],
	active: ['archive'],
	archive: ['active']
};

export function canTransitionRoom(from: string, to: WorkspaceStatus): boolean {
	return (ROOM_TRANSITIONS[from] ?? []).includes(to);
}

// Reads the room gate straight off `page.data`, which the room layout load
// fills. Structural param, not `App.PageData`, so this stays importable from
// server load files too.
type RoomContext = { roomStatus?: string; access?: { role?: string } } | undefined;

export function roomWritableFrom(data: RoomContext): boolean {
	return canMutateRoom(data?.roomStatus ?? '', data?.access?.role ?? '');
}
