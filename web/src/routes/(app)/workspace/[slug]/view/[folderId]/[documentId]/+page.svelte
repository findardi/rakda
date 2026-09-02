<script lang="ts">
	import { tick } from 'svelte';
	import { prefersReducedMotion } from 'svelte/motion';
	import { MediaQuery } from 'svelte/reactivity';
	import { fade } from 'svelte/transition';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { canManageAccess } from '$lib/access/roles';
	import { createDwellTracker, type DwellTracker } from '$lib/activity/dwell';
	import { DocumentEngagement, ViewerPage } from '$lib/components/app';
	import { Toaster, showToast } from '$lib/components/common';
	import { downloadRendition } from '$lib/download';
	import { downloadJobs } from '$lib/download/jobs.svelte';
	import { formatDate } from '$lib/format';
	import { t } from '$lib/i18n';
	import { recordDocumentVisit } from '$lib/recents';
	import type { WorkspaceData, MyAccessWorkspace } from '$lib/types/workspace';
	import type { SearchBox, SearchBoxesData } from '$lib/types/content';
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
	$effect(() => downloadJobs.bind(workspace.id));
	const access = $derived((page.data as { access?: MyAccessWorkspace }).access);
	// Owner/admin manage the room: they may read the engagement panel, and their
	// own reading is never recorded — here or upstream.
	const managesRoom = $derived(canManageAccess(access?.role ?? ''));

	const slug = $derived(page.params.slug!);
	const folderId = $derived(page.params.folderId!);
	const documentId = $derived(page.params.documentId!);

	$effect(() => {
		if (meta) recordDocumentVisit(workspace.id, { id: documentId, name: meta.name, folderId });
	});

	// The folder lives in the path, so back returns to exactly the list the
	// document was opened from.
	const backHref = $derived(
		resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId })
	);

	// Below tablet size the viewer is not served at all: raster pages, the
	// protection layers, and honest dwell figures all assume a reading-sized
	// viewport. SSR assumes it fits so desktop never flashes the gate; page
	// images only ever load client-side, so a gated phone fetches nothing.
	const fitsViewer = new MediaQuery('(min-width: 768px) and (min-height: 480px)', true);
	const tooSmall = $derived(!fitsViewer.current);

	// Entry point 11-d: lands on the Q&A page with the ask dialog prefilled.
	// The name in the query is display-only — the server re-resolves the id.
	const qaEnabled = $derived((page.data as { qaEnabled?: boolean }).qaEnabled ?? true);
	const askHref = $derived(
		`${resolve('/(app)/workspace/[slug]/qa', { slug })}?ask-doc=${encodeURIComponent(documentId)}&ask-name=${encodeURIComponent(meta?.name ?? '')}`
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
		if (pageJumped || !meta?.version_id || pageCount === 0 || tooSmall) return;
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
		if (!versionId || pageCount === 0 || managesRoom || tooSmall) return;

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
		dwell?.setPage(curtained || !anyVisible ? null : currentPage);
	});

	const CURTAIN_DELAY_MS = 500;
	let curtained = $state(false);
	let curtainTimer: ReturnType<typeof setTimeout> | undefined;

	function raiseCurtainLater() {
		if (curtainTimer !== undefined) return;
		curtainTimer = setTimeout(() => {
			curtainTimer = undefined;
			if (!document.hasFocus() || document.visibilityState !== 'visible') curtained = true;
		}, CURTAIN_DELAY_MS);
	}

	function dropCurtain() {
		clearTimeout(curtainTimer);
		curtainTimer = undefined;
		curtained = false;
	}

	function onVisibilityChange() {
		if (document.visibilityState === 'hidden') raiseCurtainLater();
		else dropCurtain();
	}

	$effect(() => () => clearTimeout(curtainTimer));

	$effect(() => {
		document.documentElement.classList.add('rakda-print-gate');
		return () => document.documentElement.classList.remove('rakda-print-gate');
	});

	const PRIVACY_KEY = 'rakda:privacy-mode:v1';
	let privacyOn = $state(false);
	let bandWrapEl = $state<HTMLElement>();

	$effect(() => {
		try {
			privacyOn = localStorage.getItem(PRIVACY_KEY) === '1';
		} catch {
			privacyOn = false;
		}
	});

	function togglePrivacy() {
		privacyOn = !privacyOn;
		try {
			if (privacyOn) localStorage.setItem(PRIVACY_KEY, '1');
			else localStorage.removeItem(PRIVACY_KEY);
		} catch {
			return;
		}
	}

	$effect(() => {
		const wrap = bandWrapEl;
		if (!wrap || !privacyOn) return;

		let top = wrap.getBoundingClientRect().top;
		let pendingY = 0;
		let frame = 0;

		const measure = () => {
			top = wrap.getBoundingClientRect().top;
		};
		const onMove = (e: PointerEvent) => {
			if (e.pointerType !== 'mouse' && e.pointerType !== 'pen') return;
			pendingY = e.clientY - top;
			if (frame) return;
			frame = requestAnimationFrame(() => {
				frame = 0;
				wrap.style.setProperty('--band-y', `${pendingY}px`);
			});
		};

		const ro = new ResizeObserver(measure);
		ro.observe(wrap);
		window.addEventListener('resize', measure);
		wrap.addEventListener('pointermove', onMove, { passive: true });
		return () => {
			cancelAnimationFrame(frame);
			ro.disconnect();
			window.removeEventListener('resize', measure);
			wrap.removeEventListener('pointermove', onMove);
		};
	});

	// --- engagement panel (owner/admin only) ---
	let panelOpen = $state(page.url.searchParams.has('readers'));

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
		// Ctrl+F / ⌘F belongs to the document, not the browser: the viewer
		// serves pixels, so the native find has nothing to search.
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'f') {
			if (tooSmall) return;
			e.preventDefault();
			openFind();
			return;
		}

		if (e.key !== 'Escape' || editing) return;
		// Escape unwinds one layer at a time: find first, then the panel,
		// then the reader itself.
		if (findOpen) {
			closeFind();
			return;
		}
		if (panelOpen) {
			panelOpen = false;
			return;
		}
		goto(backHref);
	}

	// --- in-document find (9-f): the server returns boxes, never text ---
	let findOpen = $state(false);
	let findEl = $state<HTMLInputElement>();
	let findQuery = $state('');
	let findLoading = $state(false);
	let findFailed = $state(false);
	let findResults = $state<SearchBoxesData | null>(null);
	let findIndex = $state(0);
	let findController: AbortController | null = null;

	// Flat list of every box across pages, in reading order (page, then y/x).
	const findBoxes = $derived.by(() => {
		const list: { page: number; box: SearchBox }[] = [];
		for (const m of findResults?.matches ?? []) {
			for (const b of m.boxes) list.push({ page: m.page_no, box: b });
		}
		return list;
	});

	// Boxes for the page currently in view, for the overlay.
	const boxesByPage = $derived.by(() => {
		const map = new Map<number, SearchBox[]>();
		for (const b of findBoxes) {
			const arr = map.get(b.page) ?? [];
			arr.push(b.box);
			map.set(b.page, arr);
		}
		return map;
	});

	// Index of the active (navigated) box within its page, for a stronger
	// highlight; absent on pages without the current hit.
	const activeIndexByPage = $derived.by(() => {
		const hit = findBoxes[findIndex];
		if (!hit) return undefined;
		const arr = boxesByPage.get(hit.page);
		if (!arr) return undefined;
		const idx = arr.indexOf(hit.box);
		return idx >= 0 ? { page: hit.page, index: idx } : undefined;
	});

	const pagePending = $derived(!!findResults && findResults.pending.includes(currentPage));

	function openFind() {
		findOpen = true;
		requestAnimationFrame(() => {
			findEl?.focus();
			findEl?.select();
		});
	}

	function closeFind() {
		findOpen = false;
		findQuery = '';
		findResults = null;
		findController?.abort();
	}

	// Committing a find is audited like any search, with the document as target.
	function logFind(q: string) {
		if (q.length < 2) return;
		fetch('/api/search/log', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ workspaceId: workspace.id, documentId, query: q })
		}).catch(() => {});
	}

	$effect(() => {
		const q = findQuery.trim();
		if (!findOpen) return;
		if (q.length < 2) {
			findResults = null;
			findLoading = false;
			findFailed = false;
			return;
		}

		findLoading = true;
		findFailed = false;
		findController?.abort();
		findController = new AbortController();

		const timer = setTimeout(async () => {
			const params = `workspaceId=${encodeURIComponent(workspace.id)}&documentId=${encodeURIComponent(documentId)}&q=${encodeURIComponent(q)}`;
			try {
				const res = await fetch(`/api/content/search-boxes?${params}`, {
					signal: findController?.signal
				});
				if (!res.ok) {
					findFailed = true;
					return;
				}
				findResults = (await res.json()) as SearchBoxesData;
				findIndex = 0;
			} catch (err) {
				if (err instanceof DOMException && err.name === 'AbortError') return;
				findFailed = true;
			} finally {
				findLoading = false;
			}
		}, 200);

		return () => {
			clearTimeout(timer);
			findController?.abort();
		};
	});

	function findNext() {
		const n = findBoxes.length;
		if (n === 0) return;
		findIndex = (findIndex + 1) % n;
		goToFind(findBoxes[findIndex]);
	}

	function findPrev() {
		const n = findBoxes.length;
		if (n === 0) return;
		findIndex = (findIndex - 1 + n) % n;
		goToFind(findBoxes[findIndex]);
	}

	function goToFind(hit: { page: number; box: SearchBox }, retried = false) {
		const reader = readerEl;
		const pageEl = pageEls.get(hit.page);
		if (!reader || !pageEl) {
			if (!retried) setTimeout(() => goToFind(hit, true), 300);
			return;
		}

		const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		const boxCenter = (pr: DOMRect) => pr.top + (hit.box.y + hit.box.h / 2) * pr.height;
		const pr = pageEl.getBoundingClientRect();
		const rr = reader.getBoundingClientRect();
		const from = reader.scrollTop;
		const target = Math.min(
			Math.max(0, from + boxCenter(pr) - (rr.top + rr.height / 2)),
			reader.scrollHeight - reader.clientHeight
		);
		const settleBand = () => {
			bandWrapEl?.style.setProperty(
				'--band-y',
				`${boxCenter(pageEl.getBoundingClientRect()) - reader.getBoundingClientRect().top}px`
			);
		};

		bandWrapEl?.style.setProperty('--band-y', `${boxCenter(pr) - rr.top - (target - from)}px`);
		reader.scrollTo({ top: target, behavior: reduce ? 'auto' : 'smooth' });
		if (reduce) {
			settleBand();
			return;
		}
		setTimeout(() => {
			if (reader.scrollTop === from && target !== from)
				reader.scrollTo({ top: target, behavior: 'auto' });
			settleBand();
		}, 250);
	}

	// Deep link ?page=N&q=… (from 9-d results): run the find once meta is here.
	// Gated viewports wait: the audited find fires only once results can show.
	$effect(() => {
		if (!meta?.version_id || tooSmall) return;
		const q = page.url.searchParams.get('q') ?? '';
		if (!q) return;
		findOpen = true;
		findQuery = q;
		logFind(q);
	});

	// --- download (view-and-download access) ---
	let downloading = $state(false);

	const canDownload = $derived(!!meta && (meta.can_download || meta.can_download_original));
	const downloadBlocked = $derived(
		!!meta &&
			!meta.can_download_original &&
			meta.page_count > (meta.watermark_download_max_pages ?? Infinity)
	);
	const downloadLabel = $derived(
		meta?.can_download_original ? t('doc.view.downloadClean') : t('doc.view.downloadMarked')
	);
	const downloadHint = $derived(
		downloadBlocked
			? t('doc.view.downloadTooLargeHint', {
					pages: meta?.page_count ?? 0,
					max: meta?.watermark_download_max_pages ?? 0
				})
			: meta?.can_download_original
				? downloadLabel
				: t('doc.view.downloadMarkedHint', { max: meta?.watermark_download_max_pages ?? 0 })
	);
	const downloadA11yLabel = $derived(
		downloading
			? t('doc.view.downloadPreparing')
			: downloadBlocked
				? `${downloadLabel} — ${downloadHint}`
				: downloadLabel
	);

	const downloadAbort = new AbortController();
	$effect(() => () => downloadAbort.abort());

	async function download() {
		if (downloading) return;
		// The blocked state's explanation lives in `title`, which touch never
		// shows — so the tap says it instead of silently doing nothing.
		if (downloadBlocked) {
			showToast(downloadHint, 'error');
			return;
		}
		downloading = true;
		// Download what is on screen, not whatever became current since.
		const outcome = await downloadRendition(
			{
				workspaceId: workspace.id,
				documentId,
				versionId: meta?.version_id,
				fallbackName: meta?.name ?? 'document'
			},
			downloadAbort.signal
		);
		downloading = false;
		if (!outcome.ok) {
			showToast(outcome.message, 'error');
			return;
		}

		if (outcome.queued) {
			downloadJobs.track(outcome.jobId);
			showToast(t('doc.dl.queued'));
		}
	}

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
				// The retry started the conversion server-side; reloading joins it
				// and lands on the pages once they exist.
				window.location.reload();
			})
			.catch(() => {
				retrying = false;
				showToast(t('doc.view.failed.retryErr'), 'error');
			});
	}
</script>

<svelte:head>
	<title>{meta?.name ?? t('doc.view.tab')} · {t('brand.name')}</title>
</svelte:head>

<svelte:window onkeydown={onWindowKey} onblur={raiseCurtainLater} onfocus={dropCurtain} />
<svelte:document onvisibilitychange={onVisibilityChange} />

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
		{#if meta && versions.length > 1 && !tooSmall}
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
								: v.is_staged
									? t('doc.view.ver.optionStaged', { n: v.version_no })
									: t('doc.view.ver.option', { n: v.version_no, when: formatDate(v.created_at) })}
						</option>
					{/each}
				</select>
			</label>
		{/if}

		{#if meta && pageCount > 0 && !tooSmall}
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

			<!-- In-document find (Ctrl+F / ⌘F) -->
			<button
				type="button"
				onclick={() => (findOpen ? closeFind() : openFind())}
				aria-expanded={findOpen}
				title={findOpen ? t('doc.view.findClose') : t('doc.view.findOpen')}
				class="grid h-8 w-8 flex-none place-items-center rounded-field transition-colors
				{findOpen
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
					<circle cx="11" cy="11" r="7" />
					<path d="m21 21-4.3-4.3" />
				</svg>
				<span class="sr-only">{t('doc.view.findOpen')}</span>
			</button>

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

			<button
				type="button"
				onclick={togglePrivacy}
				aria-pressed={privacyOn}
				title="{privacyOn ? t('doc.view.privacy.off') : t('doc.view.privacy.on')} — {t(
					'doc.view.privacy.hint'
				)}"
				class="grid h-8 w-8 flex-none place-items-center rounded-field transition-colors
					{privacyOn
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
					<rect x="3" y="4" width="18" height="16" rx="2" />
					<path d="M3 10h18M3 14h18" />
				</svg>
				<span class="sr-only">
					{privacyOn ? t('doc.view.privacy.off') : t('doc.view.privacy.on')}
				</span>
			</button>

			<!-- Ask about this document — guests only (managers answer, they don't
			     ask), hidden while the group's Q&A is switched off. -->
			{#if !managesRoom && qaEnabled}
				<!-- eslint-disable svelte/no-navigation-without-resolve -- resolve() cannot carry the ask-doc query string -->
				<a
					href={askHref}
					title={t('qa.askAbout')}
					class="grid h-8 w-8 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
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
						<path d="M21 12a8 8 0 0 1-8 8H5a2 2 0 0 1-2-2v-6a8 8 0 1 1 18 0z" />
						<path d="M12 12.5v-.3c0-.6.3-1 .8-1.3.8-.5 1.2-1 1.2-1.9a2 2 0 1 0-4 0" />
						<path d="M12 16h.01" />
					</svg>
					<span class="sr-only">{t('qa.askAbout')}</span>
				</a>
			{/if}

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
					aria-disabled={downloading || downloadBlocked}
					aria-label={downloadA11yLabel}
					title={downloadHint}
					class="btn btn-ghost btn-sm flex-none gap-1.5
						{downloadBlocked ? 'cursor-not-allowed text-base-content/30 hover:bg-transparent' : ''}"
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
					<span class="hidden sm:inline">
						{downloading ? t('doc.view.downloadPreparing') : downloadLabel}
					</span>
				</button>
			{/if}
		{/if}
	</header>

	<!-- In-document find bar (Ctrl+F / ⌘F). -->
	{#if findOpen && !tooSmall}
		<div
			class="flex flex-none items-center gap-2 border-b border-base-content/10 bg-base-100 px-3 py-2 sm:px-4"
		>
			<svg
				class="h-4 w-4 flex-none text-muted"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.6"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<circle cx="11" cy="11" r="7" />
				<path d="m21 21-4.3-4.3" />
			</svg>
			<input
				bind:this={findEl}
				bind:value={findQuery}
				type="search"
				placeholder={t('doc.view.findPlaceholder')}
				autocomplete="off"
				spellcheck="false"
				onkeydown={(e) => {
					if (e.key === 'Enter') {
						e.preventDefault();
						// A committed find is audited like any search (9-c rule).
						logFind(findQuery.trim());
						if (e.shiftKey) findPrev();
						else findNext();
					}
				}}
				aria-label={t('doc.view.findPlaceholder')}
				class="h-9 w-full min-w-0 flex-1 rounded-field border border-base-content/15 bg-base-200 px-3 text-sm outline-none placeholder:text-muted focus:border-primary"
			/>
			{#if findLoading}
				<span class="loading loading-spinner loading-xs flex-none text-muted"></span>
			{:else if findBoxes.length > 0}
				<span class="flex-none font-mono text-xs text-muted tabular-nums">
					{findIndex + 1}/{findBoxes.length}
				</span>
			{/if}
			<div class="flex flex-none items-center gap-0.5">
				<button
					type="button"
					onclick={findPrev}
					disabled={findBoxes.length === 0}
					aria-label={t('doc.view.findPrev')}
					title={t('doc.view.findPrev')}
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
						<path d="m15 18-6-6 6-6" />
					</svg>
				</button>
				<button
					type="button"
					onclick={findNext}
					disabled={findBoxes.length === 0}
					aria-label={t('doc.view.findNext')}
					title={t('doc.view.findNext')}
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
						<path d="m9 18 6-6-6-6" />
					</svg>
				</button>
				<button
					type="button"
					onclick={closeFind}
					aria-label={t('doc.view.findClose')}
					title={t('doc.view.findClose')}
					class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
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
						<path d="M18 6 6 18M6 6l12 12" />
					</svg>
				</button>
			</div>
			{#if findQuery.trim().length >= 2 && !findLoading && findBoxes.length === 0 && !findFailed}
				<span class="flex-none text-xs text-muted">{t('doc.view.findNone')}</span>
			{/if}
			{#if pagePending}
				<span class="flex-none text-xs text-muted">{t('doc.view.findPending')}</span>
			{/if}
		</div>
	{/if}

	<!-- Reading an old version is a legitimate act, not an error — say which one
	     is on screen and keep the way back one click away. -->
	{#if meta && stale && !tooSmall}
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
					{#if !managesRoom}
						<p class="mt-1 text-sm text-muted text-pretty">{t('doc.view.failed.noPerm')}</p>
					{/if}
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
	{:else if meta && pageCount > 0 && tooSmall}
		<!-- Below tablet size the reader never mounts: no page is fetched, so a
		     phone visit records no page views it could not deliver. -->
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
					<rect x="3" y="7" width="14" height="13" rx="2" />
					<path d="M14 4h6v6" />
					<path d="M20 4l-6.5 6.5" />
				</svg>
				<div>
					<p class="text-[0.9375rem] font-medium">{t('doc.view.small.title')}</p>
					<p class="mt-1 text-sm text-muted text-pretty">{t('doc.view.small.body')}</p>
				</div>
				<a href={backHref} class="btn btn-primary btn-sm">{t('doc.view.small.back')}</a>
			</div>
		</div>
	{:else if meta && pageCount > 0}
		<div class="flex min-h-0 flex-1">
			<!-- Hidden rather than unmounted below `lg`: the pages keep their decoded
			     images, and the beacon reads the hiding as "not being read". -->
			<div
				bind:this={bandWrapEl}
				class="relative min-h-0 flex-1 {panelOpen ? 'hidden lg:block' : ''}"
			>
				<div bind:this={readerEl} class="h-full overflow-y-auto" aria-label={meta.name}>
					<div class="mx-auto flex max-w-205 flex-col gap-4 px-3 py-6 sm:px-4">
						<!-- Keyed by version too: switching must remount the pages rather than
						     leave the previous version's images on screen while they reload. -->
						{#each pages as n (`${meta.version_id}-${n}`)}
							<ViewerPage
								pageNumber={n}
								total={pageCount}
								src={pageSrc(n)}
								boxes={boxesByPage.get(n)}
								activeIndex={activeIndexByPage?.page === n ? activeIndexByPage.index : undefined}
								{onactive}
								{onregister}
							/>
						{/each}
					</div>
				</div>

				{#if privacyOn}
					<div
						class="rakda-band pointer-events-none absolute inset-0 z-panel"
						aria-hidden="true"
						transition:fade={{ duration: prefersReducedMotion.current ? 0 : 150 }}
					></div>
				{/if}

				{#if curtained}
					<div
						class="absolute inset-0 z-panel flex flex-col items-center justify-center gap-1 bg-base-200 px-6 text-center"
						out:fade={{ duration: prefersReducedMotion.current ? 0 : 150 }}
					>
						<p class="text-sm font-medium">{t('doc.view.curtain.title')}</p>
						<p class="text-sm text-muted">{t('doc.view.curtain.hint')}</p>
					</div>
				{/if}
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

	<div class="rakda-print-notice hidden p-8 print:block">
		<p class="text-sm font-semibold">{t('brand.name')}</p>
		{#if meta}
			<p class="mt-1 text-sm">{meta.name}</p>
		{/if}
		<h1 class="mt-6 text-lg font-semibold">{t('doc.view.print.title')}</h1>
		<p class="mt-2 text-sm">{t('doc.view.print.reason')}</p>
		{#if canDownload}
			<p class="mt-2 text-sm">{t('doc.view.print.download')}</p>
		{/if}
	</div>
</div>

<Toaster />

<style>
	.rakda-band {
		--band-half: 50px;
		--band-edge: 24px;
		background-color: color-mix(in oklch, var(--color-base-200) 60%, transparent);
		-webkit-backdrop-filter: blur(16px);
		backdrop-filter: blur(16px);
		mask-image: linear-gradient(
			to bottom,
			black 0,
			black calc(var(--band-y, 50%) - var(--band-half) - var(--band-edge)),
			transparent calc(var(--band-y, 50%) - var(--band-half)),
			transparent calc(var(--band-y, 50%) + var(--band-half)),
			black calc(var(--band-y, 50%) + var(--band-half) + var(--band-edge)),
			black 100%
		);
	}
	@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
		.rakda-band {
			background-color: var(--color-base-content);
		}
	}
	@media (prefers-reduced-transparency: reduce) {
		.rakda-band {
			background-color: var(--color-base-content);
			-webkit-backdrop-filter: none;
			backdrop-filter: none;
		}
	}
</style>
