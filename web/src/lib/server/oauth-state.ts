import { dev } from '$app/environment';
import type { Cookies } from '@sveltejs/kit';

const STATE = 'rakda_oauth_state';

// sameSite:'lax' is required: the provider redirects back via a top-level GET,
// and 'strict' would drop the cookie on that cross-site navigation.
const base = {
	path: '/',
	httpOnly: true,
	sameSite: 'lax',
	secure: !dev
} as const;

/** Stash the CSRF `state` for an in-flight handshake (10-minute window). */
export function setOAuthState(cookies: Cookies, state: string): void {
	cookies.set(STATE, state, { ...base, maxAge: 60 * 10 });
}

/** Read and immediately clear the state (single-use). */
export function takeOAuthState(cookies: Cookies): string | null {
	const v = cookies.get(STATE) ?? null;
	cookies.delete(STATE, { path: '/' });
	return v;
}
