import type { ApiResult } from '$lib/types';
import type { MyInvitationData } from '$lib/types/invitation';
import { get, post } from './client';

export async function getMyInvitation(token: string): Promise<ApiResult<MyInvitationData[]>> {
	return get<MyInvitationData[]>(`/invitations`, token);
}

export async function acceptMyInvitation(
	token: string,
	invitationId: string
): Promise<ApiResult<null>> {
	return post<null>(`/invitations/${invitationId}/accept`, null, token);
}

export async function rejectMyInvitation(
	token: string,
	invitationId: string
): Promise<ApiResult<null>> {
	return post<null>(`/invitations/${invitationId}/reject`, null, token);
}
