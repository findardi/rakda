import { dev } from '$app/environment';
import type { Cookies } from '@sveltejs/kit';
import type { LoginData } from '$lib/types';

const ACCESS = 'rakda_session';
const REFRESH = 'rakda_refresh';

const base = {
	path: '/',
	httpOnly: true,
	sameSite: 'lax',
	secure: !dev
} as const;

/** Store tokens in httpOnly cookies — never readable from client JS (VDR posture). */
export function setSession(cookies: Cookies, data: LoginData): void {
	cookies.set(ACCESS, data.token, { ...base, maxAge: 60 * 15 });
	cookies.set(REFRESH, data.refresh_token, { ...base, maxAge: 60 * 60 * 24 * 30 });
}

export function clearSession(cookies: Cookies): void {
	cookies.delete(ACCESS, { path: '/' });
	cookies.delete(REFRESH, { path: '/' });
}

export function getAccessToken(cookies: Cookies): string | null {
	return cookies.get(ACCESS) ?? null;
}

export function getRefreshToken(cookies: Cookies): string | null {
	return cookies.get(REFRESH) ?? null;
}
