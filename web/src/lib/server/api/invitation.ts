import type { MyInvitationData } from '$lib/types/invitation';
import { get, post } from './client';

export const getMyInvitation = (token: string) => get<MyInvitationData[]>(`/invitations`, token);

export const acceptMyInvitation = (token: string, invitationId: string) =>
	post<null>(`/invitations/${invitationId}/accept`, null, token);

export const rejectMyInvitation = (token: string, invitationId: string) =>
	post<null>(`/invitations/${invitationId}/reject`, null, token);
