<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve as resolvePath } from '$app/paths';
	import { cubicOut } from 'svelte/easing';
	import { prefersReducedMotion } from 'svelte/motion';
	import { slide } from 'svelte/transition';
	import { Button, showToast } from '$lib/components/common';
	import { formatBytes, formatDateTime } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { DocumentData, VersionData } from '$lib/types/content';

	type Props = {
		workspaceId: string;
		slug: string;
		/** Folder in the viewer path, so the reader's back link returns to this list. */
		folderId: string;
		documentId: string;
		documentName: string;
		canDownload: boolean;
		canRestore: boolean;
		canUpload: boolean;
	};

	let {
		workspaceId,
		slug,
		folderId,
		documentId,
		documentName,
		canDownload,
		canRestore,
		canUpload
	}: Props = $props();

	const SKELETON_ROWS = [3, 2, 1];

	let versions = $state<VersionData[] | null>(null);
	let loadError = $state<string | null>(null);

	async function messageOf(res: Response): Promise<string> {
		const body = (await res.json().catch(() => null)) as { message?: string } | null;
		return body?.message || t('err.generic');
	}

	async function load(): Promise<void> {
		loadError = null;
		const q = new URLSearchParams({ workspaceId, documentId });
		try {
			const res = await fetch(`/api/content/versions?${q}`);
			if (!res.ok) {
				versions = null;
				loadError = await messageOf(res);
				return;
			}
			versions = (await res.json()) as VersionData[];
		} catch {
			versions = null;
			loadError = t('err.network');
		}
	}

	$effect(() => {
		void load();
	});

	// --- view / download ---------------------------------------------------

	let downloadingId = $state<string | null>(null);

	async function download(v: VersionData): Promise<void> {
		downloadingId = v.id;
		try {
			const q = new URLSearchParams({ workspaceId, documentId, version: v.id });
			const res = await fetch(`/api/content/download?${q}`);
			if (!res.ok) {
				showToast(await messageOf(res), 'error');
				return;
			}
			const { download_url } = (await res.json()) as { download_url: string };
			window.location.href = download_url;
		} catch {
			showToast(t('err.network'), 'error');
		} finally {
			downloadingId = null;
		}
	}

	// --- restore -----------------------------------------------------------
	// Copy-forward, so this is reversible and does not warrant a modal. The
	// confirm step opens in place, under the row it belongs to.

	let confirmingId = $state<string | null>(null);
	let restoringId = $state<string | null>(null);

	async function restore(v: VersionData): Promise<void> {
		restoringId = v.id;
		try {
			const res = await fetch('/api/content/versions/restore', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ workspaceId, documentId, versionId: v.id })
			});
			if (!res.ok) {
				showToast(await messageOf(res), 'error');
				// 409 means someone else moved the document on while this panel sat
				// open; the fresh list is the correction.
				if (res.status === 409) {
					confirmingId = null;
					await load();
				}
				return;
			}
			confirmingId = null;
			showToast(t('doc.ver.restored', { n: v.version_no }), 'success');
			await Promise.all([load(), invalidateAll()]);
		} catch {
			showToast(t('err.network'), 'error');
		} finally {
			restoringId = null;
		}
	}

	// --- upload a new version ----------------------------------------------
	// One presigned PUT, no multipart upstream: a dropped connection restarts the
	// whole file. The hint under the button says so rather than letting a large
	// upload fail silently on the user's assumption that it resumes.

	let fileInput = $state<HTMLInputElement>();
	let uploading = $state(false);
	let uploadName = $state('');
	let uploadPct = $state(0);
	// Request handle, never rendered — a reactive proxy would break xhr.send().
	let request: XMLHttpRequest | null = null;

	function put(url: string, file: File): Promise<void> {
		return new Promise((done, fail) => {
			const xhr = new XMLHttpRequest();
			request = xhr;
			xhr.open('PUT', url, true);
			xhr.setRequestHeader('Content-Type', file.type || 'application/octet-stream');
			xhr.upload.onprogress = (e) => {
				if (e.lengthComputable) uploadPct = Math.round((e.loaded / e.total) * 100);
			};
			xhr.onload = () =>
				xhr.status >= 200 && xhr.status < 300
					? done()
					: fail(new Error(t('doc.upload.err.storage')));
			xhr.onerror = () => fail(new Error(t('err.network')));
			xhr.onabort = () => fail(new Error(t('doc.upload.status.canceled')));
			xhr.send(file);
		});
	}

	async function upload(file: File): Promise<void> {
		uploading = true;
		uploadName = file.name;
		uploadPct = 0;
		try {
			const urlRes = await fetch('/api/content/versions/upload-url', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ workspaceId, documentId })
			});
			if (!urlRes.ok) throw new Error(await messageOf(urlRes));
			const { upload_url, storage_key } = (await urlRes.json()) as {
				upload_url: string;
				storage_key: string;
			};

			await put(upload_url, file);

			const doneRes = await fetch('/api/content/versions', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ workspaceId, documentId, storageKey: storage_key })
			});
			if (!doneRes.ok) throw new Error(await messageOf(doneRes));

			const doc = (await doneRes.json()) as DocumentData;
			showToast(t('doc.ver.uploaded', { n: doc.version_no }), 'success');
			await Promise.all([load(), invalidateAll()]);
		} catch (e) {
			showToast(e instanceof Error ? e.message : t('doc.ver.err.upload'), 'error');
		} finally {
			uploading = false;
			request = null;
		}
	}

	function onPick(e: Event): void {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		input.value = '';
		if (file) void upload(file);
	}
</script>

{#snippet iconEye()}
	<svg
		class="h-4 w-4"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<path d="M2 12s3.5-6.5 10-6.5S22 12 22 12s-3.5 6.5-10 6.5S2 12 2 12z" />
		<circle cx="12" cy="12" r="2.5" />
	</svg>
{/snippet}

{#snippet iconDownload()}
	<svg
		class="h-4 w-4"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<path d="M12 4v11M7.5 10.5 12 15l4.5-4.5" />
		<path d="M5 19h14" />
	</svg>
{/snippet}

<!-- A check, not a rewind arrow: nothing is being wound back. The document is
     pointed at this version, so the gesture is "pick this one". -->
{#snippet iconUse()}
	<svg
		class="h-4 w-4"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		<circle cx="12" cy="12" r="9" />
		<path d="m8.5 12.5 2.5 2.5 4.5-5" />
	</svg>
{/snippet}

<div
	id="doc-versions-{documentId}"
	transition:slide={{ duration: prefersReducedMotion.current ? 0 : 180, easing: cubicOut }}
	class="bg-base-200 px-4 py-3"
>
	<div class="flex flex-wrap items-center justify-between gap-2">
		<h3 class="text-xs font-medium">{t('doc.ver.title')}</h3>

		{#if canUpload}
			<input
				bind:this={fileInput}
				onchange={onPick}
				type="file"
				class="sr-only"
				aria-label={t('doc.ver.uploadOf', { name: documentName })}
			/>
			<Button size="sm" variant="ghost" disabled={uploading} onclick={() => fileInput?.click()}>
				<svg
					class="h-4 w-4"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M12 19V5M5 12l7-7 7 7" />
				</svg>
				{t('doc.ver.upload')}
			</Button>
		{/if}
	</div>

	{#if uploading}
		<div
			class="mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-field border border-base-content/10 bg-base-100 px-2.5 py-2"
		>
			<span class="min-w-0 flex-1 truncate text-xs">
				{t('doc.ver.uploading', { name: uploadName })}
			</span>
			<div class="flex flex-none items-center gap-2">
				<span
					class="riksa-verbar h-1 w-20 overflow-hidden rounded-full bg-base-content/10"
					role="progressbar"
					aria-valuenow={uploadPct}
					aria-valuemin="0"
					aria-valuemax="100"
					aria-label={t('doc.ver.uploading', { name: uploadName })}
				>
					<span class="block h-full bg-primary" style="width: {uploadPct}%"></span>
				</span>
				<span class="w-9 text-right font-mono text-xs text-muted tabular-nums">{uploadPct}%</span>
				<button
					type="button"
					onclick={() => request?.abort()}
					title={t('doc.ver.cancelUpload')}
					aria-label={t('doc.ver.cancelUpload')}
					class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
				>
					<svg
						class="h-4 w-4"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="1.8"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="M6 6l12 12M18 6 6 18" />
					</svg>
				</button>
			</div>
		</div>
	{/if}

	{#if loadError}
		<div class="mt-2.5 flex flex-wrap items-center gap-2">
			<p class="min-w-0 flex-1 text-xs text-error">{loadError}</p>
			<Button size="sm" variant="ghost" onclick={() => void load()}>{t('doc.ver.retry')}</Button>
		</div>
	{:else if versions === null}
		<ul class="mt-1" aria-busy="true" aria-label={t('doc.ver.loading')}>
			{#each SKELETON_ROWS as row (row)}
				<li class="flex items-center gap-2.5 py-1.5">
					<span class="riksa-verskel h-3.5 w-8 flex-none rounded-selector"></span>
					<span class="riksa-verskel h-3.5 w-28 rounded-selector"></span>
					<span class="flex-1"></span>
					<span class="riksa-verskel hidden h-3.5 w-16 flex-none rounded-selector md:block"></span>
					<span class="riksa-verskel hidden h-3.5 w-32 flex-none rounded-selector sm:block"></span>
				</li>
			{/each}
		</ul>
	{:else}
		<ul class="mt-1 divide-y divide-base-content/6">
			{#each versions as v (v.id)}
				<!-- Not `max(version_no)`: restore repoints the document, so an older
				     number can be the one being served. -->
				{@const isCurrent = v.is_current}
				<li class="py-1.5">
					<div class="flex items-center gap-2.5">
						<span class="w-7 flex-none font-mono text-xs tabular-nums">v{v.version_no}</span>

						{#if isCurrent}
							<span
								class="flex-none rounded-selector bg-primary/10 px-1.5 py-0.5 text-[0.6875rem] font-medium text-primary"
							>
								{t('doc.ver.current')}
							</span>
						{/if}

						<span class="min-w-0 flex-1 truncate text-xs text-muted">
							{t('doc.ver.by', { name: v.uploaded_by_name })}
						</span>

						<span
							class="hidden w-16 flex-none text-right font-mono text-xs text-muted tabular-nums md:inline"
						>
							{formatBytes(v.size)}
						</span>

						<time
							datetime={v.created_at}
							class="hidden w-[8.75rem] flex-none text-right font-mono text-xs text-muted tabular-nums sm:inline"
						>
							{formatDateTime(v.created_at)}
						</time>

						<div class="flex flex-none items-center gap-0.5">
							<a
								href="{resolvePath('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
									slug,
									folderId,
									documentId
								})}?version={encodeURIComponent(v.id)}"
								title={t('doc.ver.viewOf', { n: v.version_no })}
								aria-label={t('doc.ver.viewOf', { n: v.version_no })}
								class="grid h-8 w-8 place-items-center rounded-field text-muted no-underline transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
							>
								{@render iconEye()}
							</a>

							{#if canDownload}
								<button
									type="button"
									onclick={() => void download(v)}
									disabled={downloadingId === v.id}
									title={t('doc.ver.downloadOf', { n: v.version_no })}
									aria-label={t('doc.ver.downloadOf', { n: v.version_no })}
									class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-50 pointer-coarse:h-11 pointer-coarse:w-11"
								>
									{#if downloadingId === v.id}
										<span class="loading loading-spinner loading-xs"></span>
									{:else}
										{@render iconDownload()}
									{/if}
								</button>
							{/if}

							{#if canRestore && !isCurrent}
								<button
									type="button"
									onclick={() => (confirmingId = confirmingId === v.id ? null : v.id)}
									aria-expanded={confirmingId === v.id}
									title={t('doc.ver.useOf', { n: v.version_no })}
									aria-label={t('doc.ver.useOf', { n: v.version_no })}
									class="grid h-8 w-8 place-items-center rounded-field transition-colors hover:bg-primary/8 pointer-coarse:h-11 pointer-coarse:w-11
										{confirmingId === v.id ? 'bg-primary/10 text-primary' : 'text-muted hover:text-primary'}"
								>
									{@render iconUse()}
								</button>
							{/if}
						</div>
					</div>

					{#if confirmingId === v.id}
						<div
							transition:slide={{
								duration: prefersReducedMotion.current ? 0 : 150,
								easing: cubicOut
							}}
						>
							<div
								class="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-field border border-base-content/10 bg-base-100 px-2.5 py-2"
							>
								<p class="min-w-0 flex-1 text-xs text-pretty">
									<span class="font-medium">{t('doc.ver.restore.ask', { n: v.version_no })}</span>
									<span class="text-muted">{t('doc.ver.restore.hint')}</span>
								</p>
								<div class="flex flex-none gap-1.5">
									<Button size="sm" variant="ghost" onclick={() => (confirmingId = null)}>
										{t('doc.ver.restore.cancel')}
									</Button>
									<Button size="sm" loading={restoringId === v.id} onclick={() => void restore(v)}>
										{t('doc.ver.restore.confirm')}
									</Button>
								</div>
							</div>
						</div>
					{/if}
				</li>
			{/each}
		</ul>

		{#if versions.length <= 1}
			<p class="mt-2 text-xs text-muted text-pretty">{t('doc.ver.empty')}</p>
		{:else if canUpload}
			<p class="mt-2 text-xs text-muted text-pretty">{t('doc.ver.uploadHint')}</p>
		{/if}
	{/if}
</div>

<style>
	.riksa-verskel {
		background-color: color-mix(in oklch, var(--color-base-content) 8%, transparent);
		animation: riksa-verpulse 1400ms ease-in-out infinite;
	}
	@keyframes riksa-verpulse {
		50% {
			opacity: 0.45;
		}
	}
	/* Width is the only thing moving here, and it is the progress itself. */
	.riksa-verbar > :global(span) {
		transition: width 180ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	@media (prefers-reduced-motion: reduce) {
		.riksa-verskel {
			animation: none;
		}
		.riksa-verbar > :global(span) {
			transition: none;
		}
	}
</style>
