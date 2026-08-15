// Per-page read-duration tracking for the secure viewer.
//
// Browser-only, deliberately non-reactive: nothing here is rendered, so it stays
// a plain module rather than `.svelte.ts` state. The viewer already computes
// which page owns the screen (largest visible slice); this accumulates how long
// that page held it and beacons the totals out at flush time.
//
// Attribution is exclusive — exactly one page is on the clock at any moment — so
// two half-visible pages can never both bank the same second.
//
// The numbers are indicative, not forensic: they come from the reader's browser,
// they stop when the tab is hidden, and they are dropped on the floor when the
// network refuses them. `content_events` treats them accordingly.

import type { PageDurationEntry, RecordDurationsPayload } from '$lib/types/activity';

/** Below this a page was scrolled past, not read. */
const MIN_ENTRY_MS = 1_000;
/** No pointer, key, wheel, or scroll for this long: the reader walked away. */
const IDLE_MS = 180_000;
/** Upstream accepts at most this many entries per request. */
const MAX_ENTRIES = 200;
/** Input events are continuous; settling the clock more than once a second is waste. */
const INPUT_SETTLE_MS = 1_000;

export interface DwellTarget {
	workspaceId: string;
	documentId: string;
	versionId: string;
}

export interface DwellTracker {
	/** The page currently owning the screen, or `null` when none is visible. */
	setPage(page: number | null): void;
	/**
	 * Final flush and listener teardown. Switching version destroys the tracker
	 * and builds a new one, so this is also what banks a read under the version
	 * it actually happened on.
	 */
	destroy(): void;
}

// Monotonic: a wall-clock jump mid-read must not invent or erase minutes.
const clock = () => performance.now();

export function createDwellTracker(target: DwellTarget): DwellTracker {
	const totals = new Map<number, number>();

	let page: number | null = null;
	// 0 means the clock is stopped (hidden tab, idle reader, or no page on screen).
	let since = 0;
	let lastInput = clock();
	let alive = true;

	const running = () => document.visibilityState === 'visible';

	function start(): void {
		if (page !== null && since === 0 && running()) since = clock();
	}

	// Credits the open segment, clamped at the point the reader went idle. Always
	// stops the clock; every caller that wants it running again calls start().
	function commit(): void {
		if (page === null || since === 0) {
			since = 0;
			return;
		}
		const end = Math.min(clock(), lastInput + IDLE_MS);
		const ms = end - since;
		since = 0;
		if (ms <= 0) return;
		totals.set(page, (totals.get(page) ?? 0) + ms);
	}

	function send(payload: RecordDurationsPayload): void {
		const url =
			`/api/activity/duration?workspaceId=${encodeURIComponent(target.workspaceId)}` +
			`&documentId=${encodeURIComponent(target.documentId)}`;
		const body = JSON.stringify(payload);

		// sendBeacon survives the unload the fetch would be cancelled by. It cannot
		// set headers, which is why the request goes through a same-origin proxy
		// that reads the session cookie.
		try {
			const blob = new Blob([body], { type: 'application/json' });
			if (navigator.sendBeacon(url, blob)) return;
		} catch {
			// Queue full or beacon unavailable — fall through to keepalive fetch.
		}

		void fetch(url, {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body,
			keepalive: true
		}).catch(() => {
			// Indicative telemetry: a lost batch is not worth a retry queue.
		});
	}

	function flush(): void {
		commit();
		if (totals.size === 0) return;

		// Longest first, so a document with more read pages than one request can
		// carry sends its most meaningful rows now and the rest on the next flush.
		const ready: PageDurationEntry[] = [];
		for (const [pageNo, ms] of totals) {
			if (ms >= MIN_ENTRY_MS) ready.push({ page_no: pageNo, duration_ms: Math.round(ms) });
		}
		if (ready.length === 0) return;
		ready.sort((a, b) => b.duration_ms - a.duration_ms);

		const batch = ready.slice(0, MAX_ENTRIES);
		// Sub-threshold crumbs stay in the map so repeated short visits to the same
		// page eventually add up to something worth reporting.
		for (const entry of batch) totals.delete(entry.page_no);

		send({ version_id: target.versionId, durations: batch });
	}

	function onVisibility(): void {
		if (!running()) {
			flush();
			return;
		}
		// A tab that has been in the background for an hour is not an idle reader.
		lastInput = clock();
		start();
	}

	function onInput(): void {
		const now = clock();
		if (now - lastInput < INPUT_SETTLE_MS) {
			lastInput = now;
			return;
		}
		// Commit before moving the marker: the gap that just ended is the one the
		// idle clamp has to judge.
		commit();
		lastInput = now;
		start();
	}

	const inputEvents = ['pointermove', 'keydown', 'wheel', 'touchstart'] as const;
	for (const type of inputEvents) window.addEventListener(type, onInput, { passive: true });
	// Scroll does not bubble, but the capture phase still sees the reader's own
	// scroll container — which is the strongest signal that someone is reading.
	window.addEventListener('scroll', onInput, { passive: true, capture: true });
	document.addEventListener('visibilitychange', onVisibility);
	// pagehide covers unload and bfcache; visibilitychange alone misses Safari.
	window.addEventListener('pagehide', flush);

	return {
		setPage(next: number | null): void {
			if (!alive || next === page) return;
			commit();
			page = next;
			start();
		},

		destroy(): void {
			if (!alive) return;
			alive = false;
			flush();
			for (const type of inputEvents) window.removeEventListener(type, onInput);
			window.removeEventListener('scroll', onInput, { capture: true });
			document.removeEventListener('visibilitychange', onVisibility);
			window.removeEventListener('pagehide', flush);
		}
	};
}
