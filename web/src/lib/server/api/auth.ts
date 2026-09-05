import { t } from '$lib/i18n';
import type {
	ApiResult,
	LoginData,
	LoginPayload,
	MeData,
	RegisterData,
	RegisterPayload
} from '$lib/types';
import type { InvitePreviewData } from '$lib/types/invitation';
import { API_URL, get, post } from './client';
import {
	stubCheckEmail,
	stubForgotPassword,
	stubGetMe,
	stubLogin,
	stubRefresh,
	stubRegister,
	stubResendOtp,
	stubResetPassword,
	stubValidateOtp,
	stubVerifyEmail
} from './auth.stub';

export async function registerUser(p: RegisterPayload): Promise<ApiResult<RegisterData>> {
	if (!API_URL) return stubRegister(p);
	return post<RegisterData>('/auth/register', p);
}

export async function loginUser(p: LoginPayload): Promise<ApiResult<LoginData>> {
	if (!API_URL) return stubLogin(p);
	// Backend accepts email OR username (required_without).
	const body = p.identifier.includes('@')
		? { email: p.identifier, password: p.password }
		: { username: p.identifier, password: p.password };
	return post<LoginData>('/auth/login', body);
}

// Early-warning email availability check (step 1 of register).
// 200 → available; 400 (ErrEmailUnique) → taken; 400 validation → bad format.
export async function checkEmailAvailable(
	email: string
): Promise<{ available: boolean; emailError?: string }> {
	if (!API_URL) return stubCheckEmail(email);
	const res = await post<null>('/auth/check-email', { email });
	if (res.ok) return { available: true };
	if (res.fieldErrors.email) return { available: false, emailError: res.fieldErrors.email };
	if (res.status === 400) return { available: false, emailError: t('err.emailTaken') };
	return { available: false, emailError: res.message };
}

// Request a password-reset OTP. Anti-enumeration: backend always 200 regardless
// of whether the email exists, so we surface only network/format problems.
export async function forgotPassword(email: string): Promise<{ sent: boolean; error?: string }> {
	if (!API_URL) return stubForgotPassword();
	const res = await post<null>('/auth/forgot-password', { email });
	if (res.ok) return { sent: true };
	if (res.status === 0) return { sent: false, error: res.message }; // network only
	return { sent: true }; // hide any other backend signal (anti-enum)
}

// Exchange a refresh token for a fresh access+refresh pair (PUBLIC; identity from
// the refresh token itself). Backend rotates: old refresh deleted, new one issued.
export async function refreshSession(refreshToken: string): Promise<LoginData | null> {
	if (!API_URL) return stubRefresh(refreshToken);
	const res = await post<LoginData>('/auth/refresh', { refresh_token: refreshToken });
	return res.ok ? res.data : null;
}

// Current authenticated user (JWT-protected). Returns null on any failure so
// callers can treat "no valid session" uniformly.
export async function getMe(token: string): Promise<MeData | null> {
	if (!API_URL) return stubGetMe(token);
	const res = await get<MeData>('/auth/me', token);
	return res.ok ? res.data : null;
}

// Verify the account email with the 6-digit OTP (JWT-protected; identity from claims).
export async function verifyEmail(
	token: string,
	code: string
): Promise<{ ok: true } | { ok: false; invalidCode: boolean; message: string }> {
	if (!API_URL) return stubVerifyEmail(token, code);
	const res = await post<null>('/auth/verify-email', { code }, token);
	if (res.ok) return { ok: true };
	// Code is the only client input, so a 400/401 means the code is bad/expired.
	return {
		ok: false,
		invalidCode: res.status === 400 || res.status === 401,
		message: t('err.invalidOtp')
	};
}

// Revoke the current device's refresh token server-side (JWT-protected, idempotent).
// Best-effort: the cookie is cleared by the caller regardless of the outcome.
export async function logoutUser(token: string, refreshToken: string): Promise<void> {
	if (!API_URL) return;
	await post<null>('/auth/logout', { refresh_token: refreshToken }, token);
}

// Re-send the email-verification OTP (JWT-protected).
export async function resendOtp(token: string): Promise<{ sent: boolean; error?: string }> {
	if (!API_URL) return stubResendOtp();
	const res = await post<null>('/auth/resend-otp', {}, token);
	if (res.ok) return { sent: true };
	if (res.status === 0) return { sent: false, error: res.message }; // network only
	return { sent: true }; // anti-enumeration: hide other backend signals
}

// Magic-link signup (unregistered invitee). Both endpoints are PUBLIC — identity
// comes from the opaque token in the URL, not a session.

// Preview an invitation by raw token. 200 → {email, workspace_name, role_name};
// 404 (ErrInvitationInvalid) → revoked/expired/already-used.
export const previewInvitation = (token: string) =>
	get<InvitePreviewData>(`/auth/invitations/${token}`, undefined);

// Create the account and auto-join the workspace in one atomic step, then
// auto-login. 200 → {token, refresh_token}; 404 invalid token; 409 email already
// registered / username taken.
export const acceptInvitationSignup = (token: string, username: string, password: string) =>
	post<LoginData>(`/auth/invitations/${token}/accept`, { username, password });

// Step 2 — validate the reset OTP (read-only check).
export async function validateOtp(email: string, code: string): Promise<{ valid: boolean }> {
	if (!API_URL) return stubValidateOtp(code);
	const res = await post<null>('/auth/validation-otp', { email, code });
	return { valid: res.ok };
}

// Step 3 — set the new password using the verified OTP.
export async function resetPassword(
	email: string,
	code: string,
	newPassword: string
): Promise<{ ok: true } | { ok: false; invalidCode: boolean; message: string }> {
	if (!API_URL) return stubResetPassword(email, code, newPassword);
	const res = await post<null>('/auth/reset-password', { email, code, new_password: newPassword });
	if (res.ok) return { ok: true };
	// Password format is validated client-side, so a 400 here means the code is bad/expired.
	return { ok: false, invalidCode: res.status === 400, message: t('err.invalidOtp') };
}
