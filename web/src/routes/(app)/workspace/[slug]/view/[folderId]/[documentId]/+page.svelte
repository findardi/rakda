<script lang="ts">
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { canManageAccess } from '$lib/access/roles';
	import { createDwellTracker, type DwellTracker } from '$lib/activity/dwell';
	import { DocumentEngagement, ViewerPage } from '$lib/components/app';
	import { formatDate } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { WorkspaceData, MyAccessWorkspace } from '$lib/types/workspace';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const meta = $derived(data.meta);
	const forbidden = $derived(data.forbidden);
	const notViewable = $derived(data.notViewable);
	const failed = $derived(data.failed);

	// Empty for guests (upstream withholds history from them) and for documents
	// that never got a second version.
	const versions = $derived(data.versions ?? []);
	// The served version, not the newest one: restore repoints the document, so
	// v2 can be current while v3 exists. `is_current` is the only authority.
	const current = $derived(versions.find((v) => v.is_current));
	const stale = $derived(!!meta && !!current && meta.version_id !== current.id);

	const workspace = $derived((page.data as { workspace: WorkspaceData }).workspace);
	const access = $derived((page.data as { access?: MyAccessWorkspace }).access);
	// Owner/admin manage the room: they may read the engagement panel, and their
	// own reading is never recorded — here or upstream.
	const managesRoom = $derived(canManageAccess(access?.role ?? ''));

	const slug = $derived(page.params.slug!);
	const folderId = $derived(page.params.folderId!);
	const documentId = $derived(page.params.documentId!);

	// The folder lives in the path, so back returns to exactly the list the
	// document was opened from.
	const backHref = $derived(
		resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId })
	);

	const pageCount = $derived(meta?.page_count ?? 0);
	const pages = $derived(Array.from({ length: pageCount }, (_, i) => i + 1));

	// Always name the version, even when it is the current one: a document that
	// gains a version mid-read must not start serving pages from two of them.
	const pageSrc = (n: number) =>
		`/api/content/pages?workspaceId=${encodeURIComponent(workspace.id)}` +
		`&documentId=${encodeURIComponent(documentId)}&page=${n}` +
		(meta?.version_id ? `&version=${encodeURIComponent(meta.version_id)}` : '');

	// Switching version is a navigation, so the version being read stays in the
	// URL and a link to it is shareable.
	function onVersionChange(e: Event): void {
		const value = (e.currentTarget as HTMLSelectElement).value;
		if (!value || value === meta?.version_id) return;

		const href = `${resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
			slug,
			folderId,
			documentId
		})}?version=${encodeURIComponent(value)}`;

		// The route above is resolved; `resolve()` has no parameter for the query
		// string, which is the only part appended here.
		// eslint-disable-next-line svelte/no-navigation-without-resolve
		void goto(href);
	}

	// --- current page tracking (max on-screen coverage wins) ---
	// Plain Maps, deliberately non-reactive: the UI reads only `currentPage`
	// ($state); these are imperative scratch state, read in handlers, never markup.
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	const coverage = new Map<number, number>();
	let currentPage = $state(1);
	// Nothing on screen — a hidden reader (panel open on a narrow viewport) or a
	// version switch mid-remount. The dwell clock stops rather than crediting a
	// page nobody can see.
	let anyVisible = $state(false);

	function onactive(p: number, cov: number) {
		if (cov <= 0) coverage.delete(p);
		else coverage.set(p, cov);
		anyVisible = coverage.size > 0;
		if (coverage.size === 0) return;
		let best = currentPage;
		let bestCov = -1;
		for (const [pg, c] of coverage) {
			if (c > bestCov) {
				bestCov = c;
				best = pg;
			}
		}
		if (best !== currentPage) currentPage = best;
	}

	// --- element registry for jump + step navigation ---
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	const pageEls = new Map<number, HTMLElement>();
	function onregister(p: number, el: HTMLElement | null) {
		if (el) pageEls.set(p, el);
		else pageEls.delete(p);
	}

	let readerEl = $state<HTMLElement>();

	function scrollToPage(n: number) {
		const target = Math.min(Math.max(1, n), pageCount);
		const el = pageEls.get(target);
		if (!el) return;

		const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		el.scrollIntoView({ behavior: reduce ? 'auto' : 'smooth', block: 'start' });
		if (reduce || !readerEl) return;

		// A smooth scroll is dropped without a word when the browser has smooth
		// scrolling switched off, and a page whose image lands mid-flight can
		// cancel it too. The jump has to arrive either way, so check and snap.
		const from = readerEl.scrollTop;
		setTimeout(() => {
			if (readerEl?.scrollTop === from) el.scrollIntoView({ behavior: 'auto', block: 'start' });
		}, 250);
	}

	// A content-search hit deep-links to its page (?page=N). The page element
	// registers only after its image arrives, so retry until it exists.
	let pageJumped = false;
	function jumpToPage(n: number) {
		const target = Math.min(Math.max(1, n), pageCount);
		let tries = 0;
		const attempt = () => {
			if (pageEls.has(target) || tries >= 10) {
				scrollToPage(target);
				return;
			}
			tries += 1;
			setTimeout(attempt, 200);
		};
		attempt();
	}

	$effect(() => {
		if (pageJumped || !meta?.version_id || pageCount === 0) return;
		const p = Number(page.url.searchParams.get('page'));
		if (!Number.isInteger(p) || p < 1) return;
		pageJumped = true;
		jumpToPage(p);
	});

	// --- read-duration beacon ---
	// Guests feed this and nobody else: their reading is the signal the room owner
	// opened the document for, while the room's own managers are not readers.
	// Never rendered, so it stays a plain binding — a reactive proxy would buy
	// nothing and cost a wrapper.
	let dwell: DwellTracker | null = null;

	$effect(() => {
		const versionId = meta?.version_id;
		if (!versionId || pageCount === 0 || managesRoom) return;

		const tracker = createDwellTracker({ workspaceId: workspace.id, documentId, versionId });
		dwell = tracker;

		// Teardown flushes: switching version lands the read under the version it
		// happened on, before the new one starts collecting.
		return () => {
			dwell = null;
			tracker.destroy();
		};
	});

	$effect(() => {
		dwell?.setPage(anyVisible ? currentPage : null);
	});

	// --- engagement panel (owner/admin only) ---
	let panelOpen = $state(false);

	// Below `lg` the panel takes the reader's place, so a jump has to give the
	// reader back before it can scroll to anything.
	async function jumpFromPanel(n: number) {
		if (!window.matchMedia('(min-width: 64rem)').matches) panelOpen = false;
		await tick();
		scrollToPage(n);
	}

	// --- jump-to-page input (display follows scroll unless the user is typing) ---
	let jumpEl = $state<HTMLInputElement>();
	let editing = $state(false);

	$effect(() => {
		if (!editing && jumpEl) jumpEl.value = String(currentPage);
	});

	function commitJump() {
		const n = Number.parseInt(jumpEl?.value ?? '', 10);
		editing = false;
		if (Number.isFinite(n)) scrollToPage(n);
		if (jumpEl) jumpEl.value = String(currentPage);
	}

	function onWindowKey(e: KeyboardEvent) {
		if (e.key !== 'Escape' || editing) return;
		// Escape unwinds one layer at a time: panel first, then the reader itself.
		if (panelOpen) {
			panelOpen = false;
			return;
		}
		goto(backHref);
	}

	// --- download (view-and-download access) ---
	let downloading = $state(false);
	function download() {
		downloading = true;
		// Download what is on screen, not whatever became current since.
		const q = new URLSearchParams(
			meta?.version_id
				? { workspaceId: workspace.id, documentId, version: meta.version_id }
				: { workspaceId: workspace.id, documentId }
		);
		window.location.href = `/api/content/download?${q}`;
	}

	const canDownload = $derived(!!meta && (meta.can_download || meta.can_download_original));
	const downloadLabel = $derived(
		meta?.can_download_original ? t('doc.view.downloadClean') : t('doc.view.downloadMarked')
	);

	// --- rendition failure (owner-only retry) ---
	// The failing version is the one the URL pins, or the current one when
	// unpinned; history is only ever loaded for owner/admin.
	let retrying = $state(false);
	const retryVersionId = $derived(page.url.searchParams.get('version') ?? current?.id);
	function retryRendition() {
		if (!retryVersionId) return;
		retrying = true;
		fetch('/api/content/versions/retry-rendition', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ workspaceId: workspace.id, documentId, versionId: retryVersionId })
		})
			.then((res) => {
				if (!res.ok) throw new Error(String(res.status));
				window.location.reload();
			})
			.catch(() => {
				retrying = false;
			});
	}
</script>

<svelte:head>
	<title>{meta?.name ?? t('doc.view.tab')} · {t('brand.name')}</title>
</svelte:head>

<svelte:window onkeydown={onWindowKey} />

<div class="flex h-full min-h-0 flex-col bg-base-200">
	<!-- Reader chrome -->
	<header
		class="flex flex-none items-center gap-2 border-b border-base-content/10 bg-base-100 px-3 py-2 sm:gap-3 sm:px-4"
	>
		<a
			href={backHref}
			class="flex flex-none items-center gap-1.5 rounded-field px-2 py-1.5 text-sm text-muted no-underline transition-colors hover:bg-base-content/5 hover:text-base-content"
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
				<path d="M19 12H5M12 19l-7-7 7-7" />
			</svg>
			<span class="hidden sm:inline">{t('doc.view.back')}</span>
			<span class="sr-only sm:hidden">{t('doc.view.back')}</span>
		</a>

		<span class="h-5 w-px flex-none bg-base-content/10" aria-hidden="true"></span>

		<h1 class="min-w-0 flex-1 truncate text-sm font-medium" title={meta?.name}>
			{meta?.name ?? t('doc.view.tab')}
		</h1>

		<!-- Only owners and admins ever receive a version list, so this is their
		     control alone; everyone else reads the current version, unlabelled. -->
		{#if meta && versions.length > 1}
			<label class="flex-none">
				<span class="sr-only">{t('doc.view.ver.label')}</span>
				<select
					value={meta.version_id}
					onchange={onVersionChange}
					title={t('doc.view.ver.label')}
					class="select select-sm w-auto font-mono text-xs"
				>
					{#each versions as v (v.id)}
						<option value={v.id}>
							{v.is_current
								? t('doc.view.ver.optionCurrent', { n: v.version_no })
								: t('doc.view.ver.option', { n: v.version_no, when: formatDate(v.created_at) })}
						</option>
					{/each}
				</select>
			</label>
		{/if}

		{#if meta && pageCount > 0}
			<!-- Page stepper -->
			<div class="flex flex-none items-center gap-0.5">
				<button
					type="button"
					onclick={() => scrollToPage(currentPage - 1)}
					disabled={currentPage <= 1}
					aria-label={t('doc.view.prev')}
					title={t('doc.view.prev')}
					class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-40"
				>
					<svg
						class="h-4 w-4"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="m18 15-6-6-6 6" />
					</svg>
				</button>

				<div class="flex items-center gap-1 font-mono text-xs text-muted tabular-nums">
					<input
						id="viewer-jump"
						bind:this={jumpEl}
						type="text"
						inputmode="numeric"
						onfocus={() => {
							editing = true;
							jumpEl?.select();
						}}
						onblur={commitJump}
						onkeydown={(e) => {
							if (e.key === 'Enter') {
								e.preventDefault();
								commitJump();
								jumpEl?.blur();
							} else if (e.key === 'Escape') {
								e.stopPropagation();
								editing = false;
								jumpEl?.blur();
							}
						}}
						aria-label={t('doc.view.jumpLabel')}
						class="w-9 rounded-field border border-base-content/15 bg-base-100 px-1 py-0.5 text-center text-xs tabular-nums focus:border-primary focus:outline-none"
					/>
					<span aria-hidden="true">/</span>
					<span aria-label={t('doc.view.pageOf', { n: currentPage, total: pageCount })}>
						{pageCount}
					</span>
				</div>

				<button
					type="button"
					onclick={() => scrollToPage(currentPage + 1)}
					disabled={currentPage >= pageCount}
					aria-label={t('doc.view.next')}
					title={t('doc.view.next')}
					class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-40"
				>
					<svg
						class="h-4 w-4"
						viewBox="0 0 24 24"
						fill="none"
						stroke="currentColor"
						stroke-width="2"
						stroke-linecap="round"
						stroke-linejoin="round"
						aria-hidden="true"
					>
						<path d="m6 9 6 6 6-6" />
					</svg>
				</button>
			</div>

			<span class="hidden h-5 w-px flex-none bg-base-content/10 sm:block" aria-hidden="true"></span>

			<!-- Protection signal — trust is shown, not claimed -->
			<span
				class="hidden flex-none items-center gap-1.5 text-xs text-muted sm:flex"
				title={t('doc.view.protected')}
			>
				<svg
					class="h-4 w-4 text-primary"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.7"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M12 3 4 6v5c0 4.5 3 8 8 10 5-2 8-5.5 8-10V6z" />
					<path d="m9 12 2 2 4-4" />
				</svg>
				<span class="hidden lg:inline">{t('doc.view.watermarked')}</span>
			</span>

			<!-- Who read what is owner/admin knowledge. A guest is recorded, never a
			     reader of the record, so the control does not exist for them. -->
			{#if managesRoom}
				<button
					type="button"
					onclick={() => (panelOpen = !panelOpen)}
					aria-expanded={panelOpen}
					title={panelOpen ? t('activity.engagement.close') : t('activity.engagement.open')}
					class="grid h-8 w-8 flex-none place-items-center rounded-field transition-colors
						{panelOpen
						? 'bg-primary/10 text-primary'
						: 'text-muted hover:bg-base-content/5 hover:text-base-content'}"
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
						<path d="M4 19h16" />
						<path d="M7 19v-4M12 19v-7M17 19v-2" />
					</svg>
					<span class="sr-only">
						{panelOpen ? t('activity.engagement.close') : t('activity.engagement.open')}
					</span>
				</button>
			{/if}

			{#if canDownload}
				<button
					type="button"
					onclick={download}
					disabled={downloading}
					aria-label={downloadLabel}
					class="btn btn-ghost btn-sm flex-none gap-1.5"
				>
					{#if downloading}
						<span class="loading loading-spinner loading-xs"></span>
					{:else}
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
					{/if}
					<span class="hidden sm:inline">{downloadLabel}</span>
				</button>
			{/if}
		{/if}
	</header>

	<!-- Reading an old version is a legitimate act, not an error — say which one
	     is on screen and keep the way back one click away. -->
	{#if meta && stale}
		<div
			class="flex flex-none flex-wrap items-center gap-x-3 gap-y-1 border-b border-warning/35 bg-warning/15 px-3 py-1.5 sm:px-4"
		>
			<p class="flex min-w-0 flex-1 items-center gap-2 text-xs">
				<svg
					class="h-4 w-4 flex-none"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.7"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<circle cx="12" cy="12" r="9" />
					<path d="M12 7v5l3 2" />
				</svg>
				{t('doc.view.ver.stale', { n: meta.version_no, cur: current?.version_no ?? '' })}
			</p>
			{#if current}
				<a
					href="{resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
						slug,
						folderId,
						documentId
					})}?version={encodeURIComponent(current.id)}"
					class="flex-none text-xs font-medium text-primary underline-offset-2 hover:underline"
				>
					{t('doc.view.ver.toCurrent')}
				</a>
			{/if}
		</div>
	{/if}

	{#if forbidden}
		<div class="flex flex-1 items-center justify-center overflow-y-auto px-6 py-16">
			<div class="flex max-w-sm flex-col items-center gap-3 text-center">
				<svg
					class="h-9 w-9 text-muted/70"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.4"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<rect x="4" y="10.5" width="16" height="10" rx="2" />
					<path d="M8 10.5V7a4 4 0 0 1 8 0v3.5" />
				</svg>
				<div>
					<p class="text-[0.9375rem] font-medium">{t('doc.view.forbidden.title')}</p>
					<p class="mt-1 text-sm text-muted text-pretty">{t('doc.view.forbidden.body')}</p>
				</div>
			</div>
		</div>
	{:else if notViewable}
		<div class="flex flex-1 items-center justify-center overflow-y-auto px-6 py-16">
			<div class="flex max-w-sm flex-col items-center gap-4 text-center">
				<svg
					class="h-9 w-9 text-muted/70"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.4"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
					<path d="M14 3v5h5" />
					<path d="M12 10v4" />
					<path d="M12 17.5h.01" />
				</svg>
				<div>
					<p class="text-[0.9375rem] font-medium">{t('doc.view.unsupported.title')}</p>
					<p class="mt-1 text-sm text-muted text-pretty">{t('doc.view.unsupported.body')}</p>
				</div>
			</div>
		</div>
	{:else if failed}
		<div class="flex flex-1 items-center justify-center overflow-y-auto px-6 py-16">
			<div class="flex max-w-sm flex-col items-center gap-4 text-center">
				<svg
					class="h-9 w-9 text-muted/70"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.4"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
					<path d="M14 3v5h5" />
					<path d="M12 10v4" />
					<path d="M12 17.5h.01" />
				</svg>
				<div>
					<p class="text-[0.9375rem] font-medium">{t('doc.view.failed.title')}</p>
					<p class="mt-1 text-sm text-muted text-pretty">{t('doc.view.failed.body')}</p>
				</div>
				{#if managesRoom && retryVersionId}
					<button
						type="button"
						onclick={retryRendition}
						disabled={retrying}
						class="btn btn-primary btn-sm gap-1.5"
					>
						{#if retrying}
							<span class="loading loading-spinner loading-xs"></span>
						{/if}
						{t('doc.view.failed.retry')}
					</button>
				{/if}
			</div>
		</div>
	{:else if meta && pageCount > 0}
		<div class="flex min-h-0 flex-1">
			<!-- Hidden rather than unmounted below `lg`: the pages keep their decoded
			     images, and the beacon reads the hiding as "not being read". -->
			<div
				bind:this={readerEl}
				class="min-h-0 flex-1 overflow-y-auto {panelOpen ? 'hidden lg:block' : ''}"
				aria-label={meta.name}
			>
				<div class="mx-auto flex max-w-205 flex-col gap-4 px-3 py-6 sm:px-4">
					<!-- Keyed by version too: switching must remount the pages rather than
					     leave the previous version's images on screen while they reload. -->
					{#each pages as n (`${meta.version_id}-${n}`)}
						<ViewerPage pageNumber={n} total={pageCount} src={pageSrc(n)} {onactive} {onregister} />
					{/each}
				</div>
			</div>

			{#if panelOpen && managesRoom}
				<DocumentEngagement
					workspaceId={workspace.id}
					{documentId}
					{pageCount}
					{currentPage}
					onjump={(n) => void jumpFromPanel(n)}
					onclose={() => (panelOpen = false)}
				/>
			{/if}
		</div>
	{:else}
		<div class="flex flex-1 items-center justify-center overflow-y-auto px-6 py-16">
			<p class="max-w-sm text-center text-sm text-muted text-pretty">{t('doc.view.emptyPages')}</p>
		</div>
	{/if}
</div>
