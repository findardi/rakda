import { recordPageDurations } from '$lib/server/api';
import type { PageDurationEntry } from '$lib/types/activity';
import type { RequestHandler } from './$types';

// Dwell-beacon sink. `navigator.sendBeacon` cannot set headers, so the reader
// posts here and this route attaches the Bearer token from the session cookie.
// Every member writes here, guests included — their reading is the signal.
//
// Nothing reads the response: the beacon is fire-and-forget, so the body stays
// empty and only the status is honest about what happened.
export const POST: RequestHandler = async ({ locals, request, url }) => {
	if (!locals.session) return new Response(null, { status: 401 });

	const workspaceId = url.searchParams.get('workspaceId');
	const documentId = url.searchParams.get('documentId');
	if (!workspaceId || !documentId) return new Response(null, { status: 400 });

	let body: unknown;
	try {
		body = await request.json();
	} catch {
		return new Response(null, { status: 400 });
	}

	// Rebuild the payload field by field rather than forwarding the parsed body:
	// the shape upstream accepts is fixed, and nothing else belongs in an
	// append-only audit table.
	const raw = body as { version_id?: unknown; durations?: unknown };
	if (typeof raw.version_id !== 'string' || !Array.isArray(raw.durations)) {
		return new Response(null, { status: 400 });
	}

	const durations: PageDurationEntry[] = [];
	for (const entry of raw.durations) {
		const e = entry as { page_no?: unknown; duration_ms?: unknown };
		if (typeof e.page_no !== 'number' || typeof e.duration_ms !== 'number') continue;
		durations.push({ page_no: Math.trunc(e.page_no), duration_ms: Math.trunc(e.duration_ms) });
	}
	if (durations.length === 0) return new Response(null, { status: 400 });

	const res = await recordPageDurations(locals.session, workspaceId, documentId, {
		version_id: raw.version_id,
		durations
	});

	return new Response(null, { status: res.ok ? 204 : res.status || 502 });
};
