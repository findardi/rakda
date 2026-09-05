import { browser } from '$app/environment';
import { invalidateAll } from '$app/navigation';
import { t } from '$lib/i18n';
import type { InitMultipartData, MultipartPartsData, UploadedPart } from '$lib/types/content';

export type UploadStatus =
	| 'pending'
	| 'uploading'
	| 'done'
	| 'error'
	| 'canceled'
	// Handle survived a reload but the File did not — the browser cannot re-read
	// a file it was not just handed, so the user has to pick it again.
	| 'stalled';

// The handle is the entire resume state. The server keeps no upload-session row,
// so if this pair is lost the upload can neither be finished nor aborted.
export interface ResumeHandle {
	uploadId: string;
	storageKey: string;
	partSize: number;
	partCount: number;
}

export interface UploadItem {
	id: string;
	name: string;
	size: number;
	progress: number;
	status: UploadStatus;
	message: string | null;
	// A rejection that no retry can ever fix (client-side gate, or a 415/413
	// from the server): the queue refuses to re-run it and the panel hides the
	// retry button for it.
	terminal: boolean;
	workspaceId: string;
	folderId: string;
	folderName: string;
	resume: ResumeHandle | null;
	storageKey: string | null;
}

// Files at or above this go through multipart. The threshold is the client's
// call — the server does not enforce it — but part size is dictated by `init`.
const MULTIPART_THRESHOLD = 16 * 1024 * 1024;

// Cermin dari `convertible` di server/internal/platform/convert/convert.go.
// Server tetap otoritas; ini hanya supaya penolakan terjadi sebelum byte bergerak.
const UPLOADABLE_EXT = new Set([
	'pdf',
	'doc',
	'docx',
	'odt',
	'rtf',
	'txt',
	'xls',
	'xlsx',
	'xlsm',
	'ods',
	'csv',
	'ppt',
	'pptx',
	'odp',
	'jpg',
	'jpeg',
	'png',
	'gif',
	'bmp',
	'tif',
	'tiff',
	'svg'
]);
export const MAX_UPLOAD_BYTES = 500 * 1024 * 1024;

function rejectReason(file: File): string | null {
	const dot = file.name.lastIndexOf('.');
	const ext = dot > 0 ? file.name.slice(dot + 1).toLowerCase() : '';
	if (!UPLOADABLE_EXT.has(ext)) return t('upload.err.type', { ext: ext || '—' });
	if (file.size > MAX_UPLOAD_BYTES) return t('upload.err.tooLarge');
	return null;
}
const MAX_CONCURRENT = 3;
// Parts in flight within a single file. Three keeps a fat pipe busy without
// making one upload starve the other two files.
const PART_CONCURRENCY = 3;
// Upstream rejects a batch larger than this, and presigned URLs live 15 minutes,
// so they are requested in waves rather than all at once.
const URL_BATCH = 100;
const STORE_KEY = 'rakda:uploads:v1';

let items = $state<UploadItem[]>([]);
let panelOpen = $state(true);
// Set when the browser refuses to persist — the panel surfaces it so a lost
// upload has a visible reason rather than looking like a bug.
let storageBlocked = $state(false);

// Plain Maps, not SvelteMap: their values are File and XMLHttpRequest handles.
// A reactive proxy around either breaks `xhr.send(blob)`, and nothing renders
// from them — the reactive view of an upload lives in `items`.
/* eslint-disable svelte/prefer-svelte-reactivity */
const pendingFiles = new Map<string, File>();
const requests = new Map<string, Set<XMLHttpRequest>>();
/* eslint-enable svelte/prefer-svelte-reactivity */
let running = 0;

function find(id: string): UploadItem | undefined {
	return items.find((i) => i.id === id);
}

// --- persistence -------------------------------------------------------
// Only items with a handle are worth storing; a small single-PUT upload has
// nothing to resume from.

function persist(): void {
	if (!browser) return;

	const keep = items
		.filter(
			(i) =>
				i.status === 'pending' ||
				i.status === 'uploading' ||
				i.status === 'error' ||
				i.status === 'stalled'
		)
		.map((i) => ({
			id: i.id,
			name: i.name,
			size: i.size,
			workspaceId: i.workspaceId,
			folderId: i.folderId,
			folderName: i.folderName,
			resume: i.resume,
			storageKey: i.storageKey
		}));
	try {
		if (keep.length) localStorage.setItem(STORE_KEY, JSON.stringify(keep));
		else localStorage.removeItem(STORE_KEY);
		storageBlocked = false;
	} catch {
		// Private windows and hardened browser profiles can refuse writes. Losing
		// resumability silently is worse than saying so.
		storageBlocked = true;
	}
}

function restore(): void {
	if (!browser) return;
	let raw: string | null;
	try {
		raw = localStorage.getItem(STORE_KEY);
	} catch {
		return;
	}
	if (!raw) return;

	try {
		const stored = JSON.parse(raw) as Array<Omit<UploadItem, 'progress' | 'status' | 'message'>>;
		items = stored.map((s) => ({
			...s,
			progress: 0,
			status: 'stalled' as const,
			terminal: false,
			message: s.resume || s.storageKey ? t('doc.upload.status.stalled') : null
		}));
	} catch {
		try {
			localStorage.removeItem(STORE_KEY);
		} catch {
			// nothing left to do
		}
	}
}

restore();

// --- transport ---------------------------------------------------------

// The API envelope's `message`, or the generic fallback when the body is not
// JSON. Shared by every browser-side fetch against our /api proxies.
export async function messageOf(res: Response): Promise<string> {
	const body = (await res.json().catch(() => null)) as { message?: string } | null;
	return body?.message || t('err.generic');
}

// A rejection the server will never withdraw: the file type cannot produce a
// PDF (415) or the file exceeds the size cap (413). Retrying it only re-uploads
// the same rejected bytes.
class TerminalUploadError extends Error {}

async function postJson<T>(url: string, body: unknown, method = 'POST'): Promise<T> {
	const res = await fetch(url, {
		method,
		headers: { 'content-type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!res.ok) {
		const msg = await messageOf(res);
		if (res.status === 415 || res.status === 413 || res.status === 423)
			throw new TerminalUploadError(msg);
		throw new Error(msg);
	}
	return (await res.json()) as T;
}

function track(id: string, xhr: XMLHttpRequest): void {
	let set = requests.get(id);
	if (!set) {
		// Holds XMLHttpRequest handles, not rendered state — same reason as the
		// Maps above, a reactive proxy would break the requests it wraps.
		/* eslint-disable-next-line svelte/prefer-svelte-reactivity */
		set = new Set();
		requests.set(id, set);
	}
	set.add(xhr);
}

function abortRequests(id: string): void {
	const set = requests.get(id);
	if (!set) return;
	for (const xhr of set) xhr.abort();
	set.clear();
}

// XHR rather than fetch: it is the only way to read upload progress.
function put(id: string, url: string, body: Blob, onProgress?: (loaded: number) => void) {
	return new Promise<void>((resolve, reject) => {
		const xhr = new XMLHttpRequest();
		track(id, xhr);

		xhr.open('PUT', url, true);
		if (body instanceof File) {
			xhr.setRequestHeader('Content-Type', body.type || 'application/octet-stream');
		}

		xhr.upload.onprogress = (e) => {
			if (e.lengthComputable) onProgress?.(e.loaded);
		};
		xhr.onload = () => {
			requests.get(id)?.delete(xhr);
			if (xhr.status >= 200 && xhr.status < 300) resolve();
			else reject(new Error(t('doc.upload.err.storage')));
		};
		xhr.onerror = () => {
			requests.get(id)?.delete(xhr);
			reject(new Error(t('err.network')));
		};
		xhr.onabort = () => {
			requests.get(id)?.delete(xhr);
			reject(new Error(t('doc.upload.status.canceled')));
		};

		xhr.send(body);
	});
}

// --- single-PUT path (small files) -------------------------------------

async function runSimple(item: UploadItem, file: File): Promise<void> {
	const body: { workspaceId: string; folderId: string; storageKey?: string } = {
		workspaceId: item.workspaceId,
		folderId: item.folderId
	};
	if (item.storageKey) body.storageKey = item.storageKey;

	const { upload_url, storage_key } = await postJson<{ upload_url: string; storage_key: string }>(
		'/api/content/upload-url',
		body
	);

	if (!item.storageKey) {
		item.storageKey = storage_key;
		persist();
	}

	await put(item.id, upload_url, file, (loaded) => {
		item.progress = Math.round((loaded / file.size) * 100);
	});

	await postJson('/api/content/documents', {
		workspaceId: item.workspaceId,
		folderId: item.folderId,
		name: item.name,
		storageKey: storage_key
	});

	item.storageKey = null;
}

// --- multipart path ----------------------------------------------------

function partsUrl(item: UploadItem, handle: ResumeHandle): string {
	const q = new URLSearchParams({
		workspaceId: item.workspaceId,
		folderId: item.folderId,
		uploadId: handle.uploadId,
		storageKey: handle.storageKey
	});
	return `/api/content/multipart/parts?${q}`;
}

async function listParts(item: UploadItem, handle: ResumeHandle): Promise<UploadedPart[]> {
	const res = await fetch(partsUrl(item, handle));
	if (!res.ok) throw new Error(await messageOf(res));
	return ((await res.json()) as MultipartPartsData).parts ?? [];
}

async function listPartsWithRetry(
	item: UploadItem,
	handle: ResumeHandle,
	attempts = 3
): Promise<UploadedPart[]> {
	let lastErr: unknown;
	for (let i = 0; i < attempts; i++) {
		try {
			return await listParts(item, handle);
		} catch (e) {
			lastErr = e;
			if (i < attempts - 1) await new Promise((r) => setTimeout(r, 800 * (i + 1)));
		}
	}
	throw lastErr;
}

async function runMultipart(item: UploadItem, file: File): Promise<void> {
	let handle = item.resume;

	// A fresh upload starts empty; a resumed one asks storage what it already has.
	let uploaded: UploadedPart[] = [];
	if (handle) {
		try {
			uploaded = await listPartsWithRetry(item, handle);
		} catch {
			// The session is gone — expired, aborted, or reaped by the bucket's
			// lifecycle rule. Starting over beats dead-ending on a stale handle.
			handle = null;
			item.resume = null;
			persist();
		}
	}

	if (!handle) {
		const init = await postJson<InitMultipartData>('/api/content/multipart/init', {
			workspaceId: item.workspaceId,
			folderId: item.folderId,
			name: item.name,
			size: item.size
		});
		handle = {
			uploadId: init.upload_id,
			storageKey: init.storage_key,
			partSize: init.part_size,
			partCount: init.part_count
		};
		// Persist before a single byte moves: an upload whose handle was never
		// written down is unreachable and unabortable.
		item.resume = handle;
		persist();
	}

	const have = new Set(uploaded.map((p) => p.part_number));
	const missing: number[] = [];
	for (let n = 1; n <= handle.partCount; n++) if (!have.has(n)) missing.push(n);

	let doneBytes = uploaded.reduce((sum, p) => sum + p.size, 0);
	/* eslint-disable-next-line svelte/prefer-svelte-reactivity */
	const inflight = new Map<number, number>();

	const paint = () => {
		let loaded = doneBytes;
		for (const n of inflight.values()) loaded += n;
		item.progress = Math.min(99, Math.round((loaded / item.size) * 100));
	};
	paint();

	const sliceOf = (n: number) =>
		file.slice((n - 1) * handle.partSize, Math.min(n * handle.partSize, item.size));

	// Parts only count once fully uploaded — object storage keeps nothing of a
	// part cut short. Running every part at once means none of them commit until
	// the very end, so an interruption loses all of it. Capping lanes at half the
	// part count keeps early parts landing while later ones are still moving.
	const lanes = Math.max(1, Math.min(PART_CONCURRENCY, Math.floor(handle.partCount / 2)));

	// Waves of at most URL_BATCH, because presigned URLs expire and the server
	// refuses a larger batch outright.
	for (let i = 0; i < missing.length; i += URL_BATCH) {
		const batch = missing.slice(i, i + URL_BATCH);
		const { urls } = await postJson<{ urls: Array<{ part_number: number; url: string }> }>(
			'/api/content/multipart/part-urls',
			{
				workspaceId: item.workspaceId,
				folderId: item.folderId,
				uploadId: handle.uploadId,
				storageKey: handle.storageKey,
				partNumbers: batch
			}
		);

		let cursor = 0;
		const worker = async (): Promise<void> => {
			while (cursor < urls.length) {
				const { part_number, url } = urls[cursor++];
				const blob = sliceOf(part_number);
				await put(item.id, url, blob, (loaded) => {
					inflight.set(part_number, loaded);
					paint();
				});
				inflight.delete(part_number);
				doneBytes += blob.size;
				paint();
			}
		};
		await Promise.all(Array.from({ length: lanes }, worker));
	}

	// ETags come from the server, not from the PUT responses: reading a response
	// header cross-origin needs Access-Control-Expose-Headers, and this avoids
	// depending on how the object store is configured. It also double-checks
	// completeness — a failed `complete` makes the server abort the whole upload,
	// so it must not be attempted against a gap.
	const final = await listPartsWithRetry(item, handle);
	if (final.length < handle.partCount) throw new Error(t('doc.upload.err.incomplete'));

	item.progress = 99;
	item.message = t('doc.upload.completing');

	await postJson('/api/content/multipart/complete', {
		workspaceId: item.workspaceId,
		folderId: item.folderId,
		uploadId: handle.uploadId,
		name: item.name,
		storageKey: handle.storageKey,
		contentType: file.type || 'application/octet-stream',
		parts: final.map((p) => ({ part_number: p.part_number, etag: p.etag }))
	});

	// Completed uploads no longer exist as multipart sessions.
	item.resume = null;
	persist();
}

async function abortRemote(item: UploadItem): Promise<void> {
	if (!item.resume) return;
	const handle = item.resume;
	item.resume = null;
	persist();
	try {
		await postJson(
			'/api/content/multipart/abort',
			{
				workspaceId: item.workspaceId,
				folderId: item.folderId,
				uploadId: handle.uploadId,
				storageKey: handle.storageKey
			},
			'DELETE'
		);
	} catch {
		// Best effort. The bucket's AbortIncompleteMultipartUpload lifecycle rule
		// is what actually guarantees orphaned parts get reclaimed.
	}
}

// --- runner ------------------------------------------------------------

async function run(id: string): Promise<void> {
	const item = find(id);
	const file = pendingFiles.get(id);

	if (!item || !file) {
		running--;
		return;
	}

	item.status = 'uploading';
	item.progress = item.resume || item.storageKey ? item.progress : 0;
	item.message = null;

	try {
		if (file.size >= MULTIPART_THRESHOLD) await runMultipart(item, file);
		else await runSimple(item, file);

		item.status = 'done';
		item.progress = 100;
		pendingFiles.delete(id);
	} catch (e) {
		// Re-read: `cancel()` may have flipped the status while we were awaiting.
		const latest = find(id);
		if (latest && latest.status !== 'canceled') {
			latest.status = 'error';
			latest.message = e instanceof Error ? e.message : t('err.generic');
			latest.terminal = e instanceof TerminalUploadError;
		}
	} finally {
		requests.delete(id);
		running--;
		// Status changed — a finished upload must stop being restorable, and a
		// failed one must stay restorable.
		persist();
		pump();
		settle();
	}
}

function pump(): void {
	while (running < MAX_CONCURRENT) {
		const next = items.find((i) => i.status === 'pending');
		if (!next) return;
		running++;
		void run(next.id);
	}
}

function settle(): void {
	const busy = items.some((i) => i.status === 'pending' || i.status === 'uploading');
	if (busy) return;
	if (items.some((i) => i.status === 'done')) void invalidateAll();
}

function clearDone(): void {
	if (items.some((i) => i.status === 'pending' || i.status === 'uploading')) return;
	for (const item of items) {
		if (item.status !== 'pending' && item.status !== 'uploading' && item.status !== 'stalled') {
			pendingFiles.delete(item.id);
		}
	}
	items = items.filter(
		(i) => i.status === 'pending' || i.status === 'uploading' || i.status === 'stalled'
	);
	persist();
}

function drop(id: string): void {
	pendingFiles.delete(id);
	requests.delete(id);
	items = items.filter((i) => i.id !== id);
	persist();
}

export const uploadQueue = {
	get items(): UploadItem[] {
		return items;
	},
	get busy(): number {
		return items.filter((i) => i.status === 'pending' || i.status === 'uploading').length;
	},
	get failed(): number {
		return items.filter((i) => i.status === 'error').length;
	},
	get stalled(): number {
		return items.filter((i) => i.status === 'stalled').length;
	},
	get storageBlocked(): boolean {
		return storageBlocked;
	},
	get open(): boolean {
		return panelOpen;
	},
	set open(v: boolean) {
		panelOpen = v;
	},

	enqueue(workspaceId: string, folderId: string, folderName: string, files: File[]): void {
		if (!files.length) return;

		for (const file of files) {
			const id = crypto.randomUUID();
			const reason = rejectReason(file);
			if (reason) {
				items.push({
					id,
					name: file.name,
					size: file.size,
					progress: 0,
					status: 'error',
					message: reason,
					terminal: true,
					workspaceId,
					folderId,
					folderName,
					resume: null,
					storageKey: null
				});
				continue;
			}
			pendingFiles.set(id, file);
			items.push({
				id,
				name: file.name,
				size: file.size,
				progress: 0,
				status: 'pending',
				message: null,
				terminal: false,
				workspaceId,
				folderId,
				folderName,
				resume: null,
				storageKey: null
			});
		}

		panelOpen = true;
		// Before the first byte moves: a reload seconds later must still find it.
		persist();
		pump();
	},

	// Hands a stalled entry its file back. Name and size must match, otherwise
	// the parts already in storage belong to different bytes and completing
	// would splice two files together.
	attach(id: string, file: File): boolean {
		const item = find(id);
		if (!item || item.status !== 'stalled') return false;

		if (file.name !== item.name || file.size !== item.size) {
			item.message = t('doc.upload.err.mismatch');
			return false;
		}

		pendingFiles.set(id, file);
		item.message = null;
		item.status = 'pending';
		pump();
		return true;
	},

	cancel(id: string): void {
		const item = find(id);
		if (!item) return;

		const wasRunning = item.status === 'uploading';
		item.status = 'canceled';
		item.message = null;
		void abortRemote(item);
		if (wasRunning) abortRequests(id);
		else settle();
	},

	retry(id: string): void {
		const item = find(id);
		if (!item) return;
		if (item.terminal) return;
		if (!pendingFiles.has(id)) {
			if (item.resume || item.storageKey) item.status = 'stalled';
			return;
		}

		item.status = 'pending';
		item.progress = 0;
		item.message = null;
		pump();
	},

	remove(id: string): void {
		const item = find(id);
		if (!item) return;
		if (item.status === 'uploading') abortRequests(id);
		if (item.status !== 'done') void abortRemote(item);
		drop(id);
		settle();
	},

	clearFinished(): void {
		clearDone();
	}
};
