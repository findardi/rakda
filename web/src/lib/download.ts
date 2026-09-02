import { t } from '$lib/i18n';

const objectUrlGraceMs = 40_000;

export interface DownloadTarget {
	workspaceId: string;
	documentId: string;
	versionId?: string;
	fallbackName: string;
}

export type DownloadOutcome =
	| { ok: true; queued?: false }
	| { ok: true; queued: true; jobId: string }
	| { ok: false; message: string };

export function renditionFilename(disposition: string | null, fallbackName: string): string {
	const m = disposition?.match(/filename="?([^";]+)"?/);
	return m?.[1] ?? fallbackName.replace(/\.[^.]+$/, '') + '.pdf';
}

export function saveBlob(blob: Blob, filename: string): void {
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	document.body.append(a);
	a.click();
	a.remove();
	setTimeout(() => URL.revokeObjectURL(url), objectUrlGraceMs);
}

function downloadErrorMessage(res: Response, body: { message?: string } | null): string {
	if (res.status === 429) return t('doc.docs.err.downloadBusy');
	if (res.status === 413) return t('doc.docs.err.downloadTooLarge');
	if (res.status === 403) return t('doc.docs.err.forbiddenDownload');
	if (res.status === 404) return t('doc.docs.err.notFound');
	return body?.message || t('err.generic');
}

export async function downloadRendition(
	target: DownloadTarget,
	signal?: AbortSignal
): Promise<DownloadOutcome> {
	const q = new URLSearchParams({ workspaceId: target.workspaceId, documentId: target.documentId });
	if (target.versionId) q.set('version', target.versionId);

	let res: Response;
	try {
		res = await fetch(`/api/content/download?${q}`, {
			signal,
			headers: { accept: 'application/pdf, application/json' }
		});
	} catch (e) {
		if (e instanceof DOMException && e.name === 'AbortError') return { ok: true };
		return { ok: false, message: t('err.network') };
	}

	if (!res.ok) {
		const body = (await res.json().catch(() => null)) as { message?: string } | null;
		return { ok: false, message: downloadErrorMessage(res, body) };
	}

	if (res.status === 202) {
		const body = (await res.json().catch(() => null)) as { data?: { job_id?: string } } | null;
		const jobId = body?.data?.job_id;
		if (!jobId) return { ok: false, message: t('err.generic') };

		return { ok: true, queued: true, jobId };
	}

	let blob: Blob;
	try {
		blob = await res.blob();
	} catch (e) {
		if (e instanceof DOMException && e.name === 'AbortError') return { ok: true };
		return { ok: false, message: t('err.network') };
	}

	saveBlob(blob, renditionFilename(res.headers.get('content-disposition'), target.fallbackName));
	return { ok: true };
}
