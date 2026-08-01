export interface MyInvitationData {
	id: string;
	workspace_name: string;
	role_name: string;
	invited_by: string;
	expires_at: string;
	status: string;
}

export interface InvitePreviewData {
	email: string;
	workspace_name: string;
	role_name: string;
}
