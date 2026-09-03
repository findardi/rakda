// Workspace feature contract — mirrors internal/workspace/dto on the Go backend.

// Lifecycle state — server-validated enum; new rooms default to `prepare`.
export type WorkspaceStatus = 'prepare' | 'active' | 'archive';

// owner_id is NOT sent: the backend resolves it from the JWT claims.
export interface CreateWorkspacePayload {
	name: string;
	description: string;
}

// Backend PUT /workspaces/:id — name required, description optional.
export interface UpdateWorkspacePayload {
	name: string;
	description: string;
}

export interface WorkspaceData {
	id: string;
	owner_id: string;
	name: string;
	slug: string;
	description: string;
	status: WorkspaceStatus;
	// ISO-8601 strings over the wire (Go time.Time marshals to RFC3339).
	created_at: string;
	updated_at: string;
	// List-only fields: the caller's role in that room, and its latest
	// activity stamp (drives the default ordering). Absent on by-id reads.
	role?: string;
	last_activity_at?: string;
}

// GET /workspaces — quota rides along so the client never guesses the limit.
export interface WorkspaceListData {
	workspaces: WorkspaceData[];
	owned_count: number;
	owned_limit: number;
}

// GET /workspaces/:id/summary — owner/admin only (403 for guests).
export interface WorkspaceSummaryData {
	document_count: number;
	folder_count: number;
	guest_count: number;
}

// Workspace Role — fixed system roles (owner/admin/guest), read-only.
export interface WorkspaceRoleData {
	id: string;
	workspace_id: string;
	name: string;
	permissions: string[];
	is_system: boolean;
	created_at: string;
	updated_at: string;
}

// Workspace Member
// 'expired' is derived by the server from expires_at; it is never stored.
export type MemberStatus = 'invited' | 'active' | 'suspended' | 'expired';

// Mirrors the Go `GetMemberResponse` (joined view: role name, user, groups).
export interface WorkspaceMemberData {
	id: string;
	workspace_id: string;
	user_id: string;
	role_id: string;
	status: MemberStatus;
	created_at: string;
	updated_at: string;
	role_name: string;
	username: string;
	email: string;
	// Go marshals a nil slice to null, so guard for null on the client.
	group_names: string[] | null;
	// Guest access expiry (ISO 8601); null = never. Once it has passed the
	// server reports status 'expired' and refuses the member at the door.
	expires_at: string | null;
}

export interface UpdateMemberRolePayload {
	role_id: string;
}

// null clears the expiry; a value must be a future ISO 8601 timestamp. The
// backend refuses non-guest targets (400).
export interface UpdateMemberExpiryPayload {
	expires_at: string | null;
}

// Bulk invite — backend field is `email` (an array, max 50), one role per batch.
// `group_id` is optional and only meaningful for guest invites — when omitted
// the guest lands in the room's default group on acceptance.
// `access_expires_at` (ISO 8601, future) is optional and guest-only: the member
// created on acceptance carries it as their expires_at.
export interface AddMembersPayload {
	email: string[];
	role_id: string;
	group_id?: string;
	access_expires_at?: string;
}

// Per-email result. The backend never reveals registration status: existing and
// new users both come back as `invited`. `skipped` carries a reason.
export type InviteOutcome = 'invited' | 'skipped';
export type InviteReason = 'already_member' | 'already_invited';

export interface AddMemberResult {
	email: string;
	outcome: InviteOutcome;
	reason?: InviteReason;
}

// Workspace Invitation — pending invites live apart from active members.
// Mirrors the Go `InvitationResponse`.
export interface InvitationData {
	id: string;
	workspace_id: string;
	email: string;
	role_id: string;
	role_name: string;
	// Guest invites may carry the chosen group; empty means the default group
	// is applied when the invitation is accepted.
	group_id: string;
	group_name: string;
	user_id: string;
	invited_by: string;
	invited_by_username: string;
	status: string;
	expires_at: string;
	// The access window the member gets on acceptance; null = unlimited.
	access_expires_at: string | null;
	created_at: string;
}

// Groups
export interface UpsertGroupWorkspacePayload {
	name: string;
	description: string;
}

export interface GroupWorkspaceData {
	id: string;
	workspace_id: string;
	name: string;
	description: string;
	is_default: boolean;
	qa_enabled: boolean;
	qa_question_limit: number | null;
	created_at: string;
	updated_at: string;
}

export interface GroupQAPayload {
	qa_enabled: boolean;
	question_limit: number | null;
}

// A workspace member assigned to a group — joined view from the Go
// `GroupMemberResponse`. `member_id` is the WorkspaceMemberData id.
export interface GroupMemberData {
	group_id: string;
	member_id: string;
	created_at: string;
	username: string;
	email: string;
	role_name: string;
	group_name: string;
}

// Assign — backend field is `member_id` (an array of workspace member ids).
export interface AssignMembersPayload {
	member_id: string[];
}

export interface MyAccessWorkspace {
	role: string;
	permissions: string[];
	status: string;
	workspace_status: WorkspaceStatus;
	// The caller's own access expiry; null/absent = never.
	expires_at?: string | null;
}
