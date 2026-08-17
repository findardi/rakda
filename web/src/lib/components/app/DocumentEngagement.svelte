<script lang="ts">
	import { tick } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { formatDate, formatDuration } from '$lib/format';
	import { t } from '$lib/i18n';
	import type {
		DocumentReaders,
		ReaderEngagement,
		ReaderPageEngagement,
		ReaderPages
	} from '$lib/types/activity';

	type Props = {
		workspaceId: string;
		documentId: string;
		/** Page count of the version on screen — fills the gaps the server omits. */
		pageCount: number;
		/** Page owning the reader's screen; marked here as state, not decoration. */
		currentPage: number;
		onjump: (page: number) => void;
		onclose: () => void;
	};

	let { workspaceId, documentId, pageCount, currentPage, onjump, onclose }: Props = $props();

	const LIST_SKELETON = [
		[52, 68, 44],
		[38, 74, 50],
		[60, 62, 40],
		[44, 70, 46]
	];
	const PAGE_SKELETON = [72, 34, 56, 18, 46, 27];

	// Below this a fold costs more than it saves: two collapsed rows in place of
	// two page rows is churn, not compression.
	const GAP_MIN = 3;

	let data = $state<DocumentReaders | null>(null);
	let loadError = $state<string | null>(null);

	// The reader being drilled into. Null is the list; this is the only thing
	// separating the two levels.
	let selected = $state<ReaderEngagement | null>(null);
	let detail = $state<ReaderPages | null>(null);
	let detailError = $state<string | null>(null);

	// A refresh keeps the numbers on screen while it runs — a panel that empties
	// itself to reload says "gone", not "checking".
	let busy = $state(false);

	// Runs of unopened pages are folded; opening one is the reader's choice and
	// survives until they leave this person's breakdown.
	const expandedGaps = new SvelteSet<number>();

	let listEl = $state<HTMLElement>();
	let backEl = $state<HTMLButtonElement>();
	// Which row sent us into the detail, so leaving it puts focus back there.
	let cameFrom: string | null = null;

	// Level changes and load results are silent to a screen reader otherwise:
	// the region swaps content without moving focus or announcing anything.
	let status = $state('');

	// Imperative cache: the markup reads `detail`, never this map, so going back
	// and forth between readers costs nothing after the first visit.
	// eslint-disable-next-line svelte/prefer-svelte-reactivity
	const pagesByReader = new Map<string, ReaderPages>();

	const summaryOf = (n: number, ms: number) =>
		t(n === 1 ? 'activity.engagement.summaryOne' : 'activity.engagement.summary', {
			n,
			read: formatDuration(ms)
		});

	// A failed reload leaves `data` alone: stale numbers plus a visible error beat
	// an empty panel that hides what was already known.
	async function load(): Promise<void> {
		try {
			const q = new URLSearchParams({ workspaceId, documentId });
			const res = await fetch(`/api/activity/engagement?${q}`);
			if (!res.ok) {
				const body = (await res.json().catch(() => null)) as { message?: string } | null;
				loadError = body?.message || t('activity.engagement.err.load');
				return;
			}

			const payload = (await res.json()) as DocumentReaders;
			data = payload;
			loadError = null;
			status = summaryOf(payload.readers?.length ?? 0, payload.total_read_ms);
		} catch {
			loadError = t('err.network');
		}
	}

	async function loadReader(actorId: string, force = false): Promise<void> {
		const cached = pagesByReader.get(actorId);
		if (cached && !force) {
			detail = cached;
			detailError = null;
			return;
		}

		try {
			const q = new URLSearchParams({ workspaceId, documentId, actorId });
			const res = await fetch(`/api/activity/engagement?${q}`);

			// A second click while the first request is in flight must not have the
			// slower answer land on the reader now on screen.
			if (selected?.actor_id !== actorId) return;

			if (!res.ok) {
				const body = (await res.json().catch(() => null)) as { message?: string } | null;
				detailError = body?.message || t('activity.engagement.err.load');
				return;
			}

			const pages = (await res.json()) as ReaderPages;
			pagesByReader.set(actorId, pages);
			if (selected?.actor_id !== actorId) return;

			detail = pages;
			detailError = null;
			status = t('activity.engagement.status.reader', { name: readerLabel(selected) });
		} catch {
			if (selected?.actor_id !== actorId) return;
			detailError = t('err.network');
		}
	}

	// The panel is mounted only while open, so mounting is the fetch. No polling:
	// the aggregate is a snapshot, and the refresh control is the way to a newer one.
	$effect(() => {
		void load();
	});

	async function openReader(reader: ReaderEngagement): Promise<void> {
		cameFrom = reader.actor_id;
		selected = reader;
		detail = null;
		detailError = null;
		expandedGaps.clear();
		void loadReader(reader.actor_id);

		// The row that was clicked no longer exists; without this the focus ring
		// falls to <body> and a keyboard reader restarts from the top of the page.
		await tick();
		backEl?.focus();
	}

	async function back(): Promise<void> {
		const returnTo = cameFrom;
		cameFrom = null;
		selected = null;
		detail = null;
		detailError = null;
		expandedGaps.clear();
		status = t('activity.engagement.status.list');

		await tick();
		listEl?.querySelector<HTMLElement>(`[data-actor="${returnTo}"] button`)?.focus();
	}

	// The detail header carries list-level numbers, so refreshing one without the
	// other would leave a fresh breakdown under a stale total.
	async function refresh(): Promise<void> {
		if (busy) return;
		busy = true;
		try {
			if (selected) {
				const actorId = selected.actor_id;
				pagesByReader.delete(actorId);
				await Promise.all([load(), loadReader(actorId, true)]);
				const fresh = data?.readers?.find((r) => r.actor_id === actorId);
				if (fresh && selected?.actor_id === actorId) selected = fresh;
				return;
			}

			pagesByReader.clear();
			await load();
		} finally {
			busy = false;
		}
	}

	const readers = $derived(data?.readers ?? []);
	const isEmpty = $derived(!!data && readers.length === 0);

	// A full 1..pageCount run — a page this reader never opened is a finding, not
	// a gap to hide. Events recorded against a version with more pages than the
	// one on screen still belong to this document, so they follow at the end
	// rather than being silently dropped.
	const detailRows = $derived.by(() => {
		if (!detail) return [];

		const byPage: Record<number, ReaderPageEngagement> = {};
		for (const p of detail.pages ?? []) byPage[p.page_no] = p;

		const out: ReaderPageEngagement[] = [];
		for (let n = 1; n <= pageCount; n++) {
			out.push(byPage[n] ?? { page_no: n, opens: 0, read_ms: 0 });
		}

		const extra = Object.values(byPage)
			.filter((p) => p.page_no > pageCount || p.page_no < 1)
			.sort((a, b) => a.page_no - b.page_no);

		return [...out, ...extra];
	});

	type Row =
		| { kind: 'page'; page: ReaderPageEngagement }
		| { kind: 'gap'; from: number; to: number };

	const isBlank = (p: ReaderPageEngagement) => p.opens === 0 && p.read_ms === 0;

	// Every skipped page still exists here — a long run is folded into one row, not
	// dropped. On a 100-page document read in five, the alternative is 95 identical
	// rows and no way to see the five.
	const rows = $derived.by(() => {
		const src = detailRows;
		const out: Row[] = [];
		let i = 0;

		while (i < src.length) {
			if (isBlank(src[i])) {
				let j = i + 1;
				// Contiguous only: a jump in page_no would make "10–47" a lie.
				while (j < src.length && isBlank(src[j]) && src[j].page_no === src[j - 1].page_no + 1) j++;

				if (j - i >= GAP_MIN) {
					const from = src[i].page_no;
					out.push({ kind: 'gap', from, to: src[j - 1].page_no });
					if (expandedGaps.has(from)) {
						for (let k = i; k < j; k++) out.push({ kind: 'page', page: src[k] });
					}
					i = j;
					continue;
				}
			}

			out.push({ kind: 'page', page: src[i] });
			i++;
		}

		return out;
	});

	const maxPageMs = $derived(detailRows.reduce((max, r) => (r.read_ms > max ? r.read_ms : max), 0));

	// A page with any recorded time must still register as a mark, not a hairline.
	const barWidth = (value: number, max: number) =>
		value <= 0 || max <= 0 ? 0 : Math.max(4, (value / max) * 100);

	function toggleGap(from: number): void {
		if (expandedGaps.has(from)) expandedGaps.delete(from);
		else expandedGaps.add(from);
	}

	// No plural machinery in `t()`; the dictionary carries both forms, as the
	// activity timeline's own counts already do.
	const opensLabel = (n: number) =>
		t(n === 1 ? 'activity.engagement.opensOne' : 'activity.engagement.opens', { n });
	const pagesLabel = (n: number) =>
		t(n === 1 ? 'activity.engagement.pagesOne' : 'activity.engagement.pages', { n });

	// The account can be gone; the email snapshot in the event row outlives it.
	const readerLabel = (r: ReaderEngagement) => r.actor_name || r.actor_email;

	const exportHref = $derived(
		`/api/activity/engagement/export?workspaceId=${encodeURIComponent(workspaceId)}` +
			`&documentId=${encodeURIComponent(documentId)}&format=csv`
	);

	// Follow the reader down the document — only in the page breakdown, where the
	// rows are pages. Instant, not smooth: this is the rail keeping up, not a
	// journey worth animating.
	$effect(() => {
		if (!selected || rows.length === 0) return;
		const el = listEl?.querySelector(`[data-page="${currentPage}"]`);
		el?.scrollIntoView({ block: 'nearest' });
	});
</script>

{#snippet skeletonList()}
	<ul aria-busy="true" aria-label={t('activity.engagement.loading')}>
		{#each LIST_SKELETON as widths, i (i)}
			<li class="grid grid-cols-[1fr_auto] items-center gap-x-3 gap-y-1.5 px-3 py-2.5">
				<span class="riksa-eng-skel h-3 rounded-selector" style="width: {widths[0]}%"></span>
				<span class="riksa-eng-skel h-3 w-10 rounded-selector"></span>
				<span class="riksa-eng-skel col-span-2 h-2.5 rounded-selector" style="width: {widths[1]}%"
				></span>
				<span class="riksa-eng-skel col-span-2 h-2.5 rounded-selector" style="width: {widths[2]}%"
				></span>
			</li>
		{/each}
	</ul>
{/snippet}

{#snippet skeletonPages()}
	<ul aria-busy="true" aria-label={t('activity.engagement.loading')}>
		{#each PAGE_SKELETON as width (width)}
			<li class="grid grid-cols-[1.75rem_1fr] items-center gap-x-2.5 gap-y-1.5 px-3 py-2.5">
				<span class="riksa-eng-skel row-span-2 h-3 rounded-selector"></span>
				<span class="riksa-eng-skel h-1.5 rounded-full" style="width: {width}%"></span>
				<span class="riksa-eng-skel h-3 w-28 rounded-selector"></span>
			</li>
		{/each}
	</ul>
{/snippet}

{#snippet errorBox(message: string, retry: () => void)}
	<div class="flex flex-col items-start gap-2 px-3 py-4">
		<p class="text-sm text-error text-pretty">{message}</p>
		<button
			type="button"
			onclick={retry}
			class="rounded-field px-2 py-1 text-sm font-medium text-primary transition-colors hover:bg-primary/8"
		>
			{t('activity.engagement.retry')}
		</button>
	</div>
{/snippet}

<!-- Reload failed while numbers were already on screen. The old ones stay, the
     failure is stated rather than mistaken for a fresh result. -->
{#snippet errorStrip(message: string, retry: () => void)}
	<div
		class="flex flex-none items-center gap-2 border-b border-error/30 bg-error/10 px-3 py-1.5"
		role="alert"
	>
		<p class="min-w-0 flex-1 text-xs text-error">{message}</p>
		<button
			type="button"
			onclick={retry}
			class="flex-none rounded-field px-1.5 py-0.5 text-xs font-medium text-error transition-colors hover:bg-error/15"
		>
			{t('activity.engagement.retry')}
		</button>
	</div>
{/snippet}

<!-- No entrance transition. The panel is a layout region: opening it reflows the
     reader instantly, so an animated root buys nothing — and a transition on a
     component's own root element leaves the node behind when the parent removes
     it, which strands a full-width ghost over the reader. -->
<aside
	aria-label={t('activity.engagement.title')}
	class="flex w-full min-w-0 flex-none flex-col border-base-content/10 bg-base-100 lg:w-[22rem] lg:border-l"
>
	<!-- Two rows: actions never compete with identity for width. A name and a
	     22rem panel cannot share a line with four icon buttons. -->
	<header class="flex-none border-b border-base-content/10 px-3 py-2.5">
		<div class="flex items-center gap-1">
			{#if selected}
				<button
					type="button"
					bind:this={backEl}
					onclick={() => void back()}
					title={t('activity.engagement.back')}
					aria-label={t('activity.engagement.back')}
					class="grid h-8 w-8 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
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
						<path d="M15 5 8 12l7 7" />
					</svg>
				</button>
			{/if}

			<h2 class="min-w-0 flex-1 truncate text-sm font-medium">
				{t('activity.engagement.title')}
			</h2>

			<!-- eslint-disable svelte/no-navigation-without-resolve -- endpoint, not a page: resolve() has no entry for /api routes -->
			<a
				href={exportHref}
				title={t('activity.engagement.export')}
				aria-label={t('activity.engagement.export')}
				class="grid h-8 w-8 flex-none place-items-center rounded-field text-muted no-underline transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
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
					<path d="M12 4v11M7.5 10.5 12 15l4.5-4.5" />
					<path d="M5 19h14" />
				</svg>
			</a>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->

			<button
				type="button"
				onclick={() => void refresh()}
				disabled={busy}
				aria-busy={busy}
				title={t('activity.engagement.refresh')}
				aria-label={t('activity.engagement.refresh')}
				class="grid h-8 w-8 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-50 pointer-coarse:h-11 pointer-coarse:w-11"
			>
				<svg
					class="h-4 w-4 {busy ? 'animate-spin motion-reduce:animate-none' : ''}"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.8"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M20 11a8 8 0 0 0-13.7-5.3L4 8" />
					<path d="M4 4v4h4" />
					<path d="M4 13a8 8 0 0 0 13.7 5.3L20 16" />
					<path d="M20 20v-4h-4" />
				</svg>
			</button>

			<button
				type="button"
				onclick={onclose}
				title={t('activity.engagement.close')}
				aria-label={t('activity.engagement.close')}
				class="grid h-8 w-8 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
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

		<div class="mt-1.5 min-w-0">
			{#if selected}
				<p class="truncate text-sm font-medium">{readerLabel(selected)}</p>
				{#if selected.actor_name}
					<p class="mt-0.5 truncate font-mono text-xs text-muted">{selected.actor_email}</p>
				{/if}
				<p class="mt-0.5 font-mono text-xs text-muted tabular-nums">
					{opensLabel(selected.opens)} · {formatDuration(selected.read_ms)}
				</p>
			{:else if data}
				<p class="font-mono text-xs text-muted tabular-nums">
					{summaryOf(readers.length, data.total_read_ms)}
				</p>
			{:else}
				<!-- Before the first answer arrives there is no count to state; "0 readers"
				     would be a claim, not a placeholder. Boxed to the line height the real
				     summary occupies so the header does not jump when it lands. -->
				<span class="flex h-4 items-center">
					<span class="riksa-eng-skel block h-3 w-32 rounded-selector"></span>
				</span>
			{/if}
		</div>
	</header>

	<p class="sr-only" aria-live="polite">{status}</p>

	{#if selected}
		{@const sel = selected}
		{#if detailError && detail}
			{@render errorStrip(detailError, () => void loadReader(sel.actor_id, true))}
		{/if}
	{:else if loadError && data}
		{@render errorStrip(loadError, () => void load())}
	{/if}

	<div bind:this={listEl} class="min-h-0 flex-1 overflow-y-auto">
		{#if selected}
			{@const sel = selected}
			{#if detailError && !detail}
				{@render errorBox(detailError, () => void loadReader(sel.actor_id, true))}
			{:else if !detail}
				{@render skeletonPages()}
			{:else}
				<ul class="divide-y divide-base-content/6">
					{#each rows as row (row.kind === 'gap' ? `g${row.from}` : `p${row.page.page_no}`)}
						{#if row.kind === 'gap'}
							{@const open = expandedGaps.has(row.from)}
							{@const holdsCurrent = currentPage >= row.from && currentPage <= row.to}
							<li data-page={!open && holdsCurrent ? currentPage : undefined}>
								<button
									type="button"
									onclick={() => toggleGap(row.from)}
									aria-expanded={open}
									aria-label={t('activity.engagement.gap', { from: row.from, to: row.to })}
									class="grid w-full cursor-pointer grid-cols-[1.75rem_1fr] items-center gap-x-2.5 bg-base-200 px-3 py-1.5 text-left transition-colors hover:bg-base-content/5
										{!open && holdsCurrent ? 'bg-primary/6' : ''}"
								>
									<svg
										class="h-3.5 w-3.5 justify-self-center text-muted transition-transform duration-200 motion-reduce:transition-none {open
											? 'rotate-90'
											: ''}"
										viewBox="0 0 24 24"
										fill="none"
										stroke="currentColor"
										stroke-width="2"
										stroke-linecap="round"
										stroke-linejoin="round"
										aria-hidden="true"
									>
										<path d="m9 6 6 6-6 6" />
									</svg>

									<span class="flex min-w-0 items-baseline gap-2" aria-hidden="true">
										<span class="flex-none font-mono text-xs text-muted tabular-nums">
											{row.from}–{row.to}
										</span>
										<span class="truncate text-xs text-muted">
											{t('activity.engagement.unread')}
										</span>
									</span>
								</button>
							</li>
						{:else}
							{@const page = row.page}
							{@const isCurrent = page.page_no === currentPage}
							{@const blank = isBlank(page)}
							<li data-page={page.page_no}>
								<button
									type="button"
									onclick={() => onjump(page.page_no)}
									class="grid w-full cursor-pointer grid-cols-[1.75rem_1fr] items-center gap-x-2.5 gap-y-1 px-3 py-2 text-left transition-colors hover:bg-base-content/4
										{isCurrent ? 'bg-primary/6' : ''}"
								>
									<span
										class="row-span-2 text-right font-mono text-xs tabular-nums
											{isCurrent ? 'font-medium text-primary' : 'text-muted'}"
									>
										{page.page_no}
									</span>

									<span class="flex min-w-0 items-center gap-2">
										<span
											class="riksa-eng-track h-1.5 min-w-0 flex-1 overflow-hidden rounded-full {blank
												? 'bg-base-content/6'
												: 'bg-base-content/10'}"
											aria-hidden="true"
										>
											<span
												class="riksa-eng-fill block h-full rounded-full bg-primary"
												style="width: {barWidth(page.read_ms, maxPageMs)}%"
											></span>
										</span>
										<span class="flex-none font-mono text-xs whitespace-nowrap tabular-nums">
											{formatDuration(page.read_ms)}
										</span>
									</span>

									{#if blank}
										<span class="text-xs text-muted">{t('activity.engagement.unread')}</span>
									{:else}
										<span class="font-mono text-xs text-muted tabular-nums">
											{opensLabel(page.opens)}
										</span>
									{/if}

									<span class="sr-only">{t('activity.engagement.jump', { n: page.page_no })}</span>
								</button>
							</li>
						{/if}
					{/each}
				</ul>
			{/if}
		{:else if loadError && !data}
			{@render errorBox(loadError, () => void load())}
		{:else if !data}
			{@render skeletonList()}
		{:else if isEmpty}
			<div class="flex flex-col items-center gap-3 px-5 py-12 text-center">
				<svg
					class="h-8 w-8 text-muted/70"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="1.4"
					stroke-linecap="round"
					stroke-linejoin="round"
					aria-hidden="true"
				>
					<path d="M4 19h16" />
					<path d="M7 19v-4M12 19v-7M17 19v-2" />
				</svg>
				<div>
					<p class="text-sm font-medium">{t('activity.engagement.empty.title')}</p>
					<p class="mt-1 text-xs text-muted text-pretty">{t('activity.engagement.empty.body')}</p>
				</div>
			</div>
		{:else}
			<ul class="divide-y divide-base-content/6">
				{#each readers as reader (reader.actor_id)}
					<li data-actor={reader.actor_id}>
						<button
							type="button"
							onclick={() => void openReader(reader)}
							aria-label={t('activity.engagement.openReader', { name: readerLabel(reader) })}
							class="grid w-full cursor-pointer grid-cols-[1fr_auto] items-baseline gap-x-3 gap-y-1 px-3 py-2.5 text-left transition-colors hover:bg-base-content/4"
						>
							<span class="min-w-0 truncate text-sm font-medium">{readerLabel(reader)}</span>
							<span class="flex-none font-mono text-xs whitespace-nowrap tabular-nums">
								{formatDuration(reader.read_ms)}
							</span>

							{#if reader.actor_name}
								<span class="col-span-2 min-w-0 truncate font-mono text-xs text-muted">
									{reader.actor_email}
								</span>
							{/if}

							<span class="col-span-2 font-mono text-xs text-muted tabular-nums">
								{pagesLabel(reader.pages_seen)} · {opensLabel(reader.opens)} · {t(
									'activity.engagement.lastRead',
									{ when: formatDate(reader.last_read_at) }
								)}
							</span>
						</button>
					</li>
				{/each}
			</ul>
		{/if}
	</div>

	<p class="flex-none border-t border-base-content/10 px-3 py-2 text-xs text-muted text-pretty">
		{t('activity.engagement.hint')}
	</p>
</aside>

<style>
	.riksa-eng-skel {
		background-color: color-mix(in oklch, var(--color-base-content) 8%, transparent);
		animation: riksa-eng-pulse 1400ms ease-in-out infinite;
	}
	@keyframes riksa-eng-pulse {
		50% {
			opacity: 0.45;
		}
	}
	/* Width is the datum itself; it only moves when a refresh brings new numbers. */
	.riksa-eng-track > :global(.riksa-eng-fill) {
		transition: width 200ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	@media (prefers-reduced-motion: reduce) {
		.riksa-eng-skel {
			animation: none;
		}
		.riksa-eng-track > :global(.riksa-eng-fill) {
			transition: none;
		}
	}
</style>
