import { env } from '$env/dynamic/private';
import { t } from '$lib/i18n';
import { getClientIP } from '$lib/server/client-ip';
import type { ApiResult, Envelope, FieldError } from '$lib/types';

// Shared HTTP layer for every feature's API module.
export const API_URL = env.AUTH_API_URL?.replace(/\/$/, '');

// Headers untuk panggilan upstream. Dipakai `request()` dan fetch mentah yang
// menstream berkas (PNG halaman, unduhan, ekspor) — keduanya butuh IP klien
// di X-Forwarded-For supaya watermark dan rate limit memakai IP pembaca.
export function upstreamHeaders(token?: string): Record<string, string> {
	const headers: Record<string, string> = {};
	if (token) headers.authorization = `Bearer ${token}`;
	const ip = getClientIP();
	if (ip) headers['x-forwarded-for'] = ip;
	return headers;
}

// `token` attaches `Authorization: Bearer` for JWT-protected endpoints.
async function request<T>(
	method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE',
	path: string,
	body: unknown,
	token?: string
): Promise<ApiResult<T>> {
	const headers: Record<string, string> = {
		'content-type': 'application/json',
		...upstreamHeaders(token)
	};

	let res: Response;
	try {
		res = await fetch(`${API_URL}${path}`, {
			method,
			headers,
			body: body === undefined ? undefined : JSON.stringify(body)
		});
	} catch {
		return { ok: false, status: 0, message: t('err.network'), fieldErrors: {} };
	}

	let env: Envelope<T>;
	try {
		env = (await res.json()) as Envelope<T>;
	} catch {
		return { ok: false, status: res.status, message: t('err.generic'), fieldErrors: {} };
	}

	if (res.ok && env.success) {
		return { ok: true, message: env.message, data: env.data as T };
	}

	return {
		ok: false,
		status: res.status,
		message: translateMessage(res.status, env.message),
		fieldErrors: translateFieldErrors(env.errors)
	};
}

export const post = <T>(path: string, body: unknown, token?: string) =>
	request<T>('POST', path, body, token);

export const get = <T>(path: string, token?: string) => request<T>('GET', path, undefined, token);

export const put = <T>(path: string, body: unknown, token?: string) =>
	request<T>('PUT', path, body, token);

export const patch = <T>(path: string, body: unknown, token?: string) =>
	request<T>('PATCH', path, body, token);

// `del` (not `delete` — reserved word). Backend returns 200 + envelope, not 204.
// `body` is last so existing two-argument callers keep working; multipart abort
// is the one DELETE upstream that expects a JSON body.
export const del = <T>(path: string, token?: string, body?: unknown) =>
	request<T>('DELETE', path, body, token);

function translateFieldErrors(errs?: FieldError[] | null): Record<string, string> {
	const out: Record<string, string> = {};
	if (!errs) return out;
	for (const e of errs) out[e.field] = translateFieldMessage(e.message);
	return out;
}

function translateFieldMessage(m: string): string {
	if (m === 'required') return t('err.required');
	if (m === 'invalid email format') return t('err.email');
	let match = m.match(/^minimal (\d+) characters$/);
	if (match) return t('err.min', { n: match[1] });
	match = m.match(/^maximal (\d+) characters$/);
	if (match) return t('err.max', { n: match[1] });
	if (m.startsWith('must fill if')) return t('err.identifierRequired');
	return m;
}

function translateMessage(status: number, raw: string): string {
	if (status === 401) return t('err.invalidCredentials');
	if (status === 423) return t('err.roomArchived');
	if (status === 409) {
		const m = raw.toLowerCase();
		if (m.includes('email')) return t('err.emailTaken');
		if (m.includes('username')) return t('err.usernameTaken');
		if (m.includes('transition')) return t('err.roomTransition');
		return t('err.conflict');
	}
	if (status === 403) {
		const m = raw.toLowerCase();
		if (m.includes('no access')) return t('err.forbiddenContent');
		if (m.includes('not open')) return t('err.roomNotOpen');
		return t('err.forbidden');
	}
	if (status >= 500 || status === 0) return t('err.generic');
	return raw || t('err.generic');
}
