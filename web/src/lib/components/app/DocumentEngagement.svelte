<script lang="ts">
	import { formatDuration } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { DocumentEngagement, PageEngagement } from '$lib/types/activity';

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

	const SKELETON_ROWS = [72, 34, 56, 18, 46, 27];

	let data = $state<DocumentEngagement | null>(null);
	let loadError = $state<string | null>(null);
	let loading = $state(false);
	let listEl = $state<HTMLElement>();

	async function load(): Promise<void> {
		loading = true;
		loadError = null;
		try {
			const q = new URLSearchParams({ workspaceId, documentId });
			const res = await fetch(`/api/activity/engagement?${q}`);
			if (!res.ok) {
				const body = (await res.json().catch(() => null)) as { message?: string } | null;
				data = null;
				loadError = body?.message || t('activity.engagement.err.load');
				return;
			}
			data = (await res.json()) as DocumentEngagement;
		} catch {
			data = null;
			loadError = t('err.network');
		} finally {
			loading = false;
		}
	}

	// The panel is mounted only while open, so mounting is the fetch. No polling:
	// the aggregate is a snapshot, and the refresh control is the way to a newer one.
	$effect(() => {
		void load();
	});

	// A full 1..pageCount run — a page nobody opened is a finding, not a gap to
	// hide. Events recorded against a version with more pages than the one on
	// screen still belong to this document, so they follow at the end rather than
	// being silently dropped.
	const rows = $derived.by(() => {
		const byPage: Record<number, PageEngagement> = {};
		for (const p of data?.pages ?? []) byPage[p.page_no] = p;

		const out: PageEngagement[] = [];
		for (let n = 1; n <= pageCount; n++) {
			out.push(byPage[n] ?? { page_no: n, opens: 0, raw_hits: 0, unique_viewers: 0, read_ms: 0 });
		}

		const extra = Object.values(byPage)
			.filter((p) => p.page_no > pageCount || p.page_no < 1)
			.sort((a, b) => a.page_no - b.page_no);

		return [...out, ...extra];
	});

	const maxOpens = $derived(rows.reduce((max, r) => (r.opens > max ? r.opens : max), 0));
	const isEmpty = $derived(!!data && maxOpens === 0);

	// A single open must still register as a mark, not a hairline.
	const barWidth = (opens: number) => (opens <= 0 ? 0 : Math.max(4, (opens / maxOpens) * 100));

	// No plural machinery in `t()`; the dictionary carries both forms, as the
	// activity timeline's own counts already do.
	const opensLabel = (n: number) =>
		t(n === 1 ? 'activity.engagement.opensOne' : 'activity.engagement.opens', { n });
	const viewersLabel = (n: number) =>
		t(n === 1 ? 'activity.engagement.viewersOne' : 'activity.engagement.viewers', { n });

	// Follow the reader down the document. Instant, not smooth: this is the rail
	// keeping up, not a journey worth animating.
	$effect(() => {
		// Reading the row count is what makes this rerun once the list exists: the
		// first load renders rows after this effect's first pass, and a long
		// document would otherwise open with its current page below the fold.
		if (rows.length === 0) return;
		const el = listEl?.querySelector(`[data-page="${currentPage}"]`);
		el?.scrollIntoView({ block: 'nearest' });
	});
</script>

<!-- No entrance transition. The panel is a layout region: opening it reflows the
     reader instantly, so an animated root buys nothing — and a transition on a
     component's own root element leaves the node behind when the parent removes
     it, which strands a full-width ghost over the reader. -->
<aside
	aria-label={t('activity.engagement.title')}
	class="flex w-full min-w-0 flex-none flex-col border-base-content/10 bg-base-100 lg:w-[22rem] lg:border-l"
>
	<header class="flex flex-none items-start gap-1 border-b border-base-content/10 px-3 py-2.5">
		<div class="min-w-0 flex-1">
			<h2 class="text-sm font-medium">{t('activity.engagement.title')}</h2>
			<p class="mt-0.5 font-mono text-xs text-muted tabular-nums">
				{t(
					(data?.total_opens ?? 0) === 1
						? 'activity.engagement.summaryOne'
						: 'activity.engagement.summary',
					{ opens: data?.total_opens ?? 0, read: formatDuration(data?.total_read_ms ?? 0) }
				)}
			</p>
		</div>

		<button
			type="button"
			onclick={() => void load()}
			disabled={loading}
			title={t('activity.engagement.refresh')}
			aria-label={t('activity.engagement.refresh')}
			class="grid h-8 w-8 flex-none place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-40 pointer-coarse:h-11 pointer-coarse:w-11"
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
	</header>

	<div bind:this={listEl} class="min-h-0 flex-1 overflow-y-auto">
		{#if loadError}
			<div class="flex flex-col items-start gap-2 px-3 py-4">
				<p class="text-sm text-error text-pretty">{loadError}</p>
				<button
					type="button"
					onclick={() => void load()}
					class="rounded-field px-2 py-1 text-sm font-medium text-primary transition-colors hover:bg-primary/8"
				>
					{t('activity.engagement.retry')}
				</button>
			</div>
		{:else if !data}
			<ul aria-busy="true" aria-label={t('activity.engagement.loading')}>
				{#each SKELETON_ROWS as width (width)}
					<li class="grid grid-cols-[1.75rem_1fr] items-center gap-x-2.5 gap-y-1.5 px-3 py-2.5">
						<span class="riksa-eng-skel row-span-2 h-3 rounded-selector"></span>
						<span class="riksa-eng-skel h-1.5 rounded-full" style="width: {width}%"></span>
						<span class="riksa-eng-skel h-3 w-28 rounded-selector"></span>
					</li>
				{/each}
			</ul>
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
				{#each rows as row (row.page_no)}
					{@const isCurrent = row.page_no === currentPage}
					<li data-page={row.page_no}>
						<button
							type="button"
							onclick={() => onjump(row.page_no)}
							title={t('activity.engagement.rawHits', { n: row.raw_hits })}
							class="grid w-full cursor-pointer grid-cols-[1.75rem_1fr] items-center gap-x-2.5 gap-y-1 px-3 py-2 text-left transition-colors hover:bg-base-content/4
								{isCurrent ? 'bg-primary/6' : ''}"
						>
							<span
								class="row-span-2 text-right font-mono text-xs tabular-nums
									{isCurrent ? 'font-medium text-primary' : 'text-muted'}"
							>
								{row.page_no}
							</span>

							<span class="flex min-w-0 items-center gap-2">
								<span
									class="riksa-eng-track h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-base-content/10"
									aria-hidden="true"
								>
									<span
										class="riksa-eng-fill block h-full rounded-full bg-primary"
										style="width: {barWidth(row.opens)}%"
									></span>
								</span>
								<span class="flex-none font-mono text-xs whitespace-nowrap tabular-nums">
									{opensLabel(row.opens)}
								</span>
							</span>

							{#if row.opens === 0}
								<span class="text-xs text-muted">{t('activity.engagement.unread')}</span>
							{:else}
								<span class="font-mono text-xs text-muted tabular-nums">
									{viewersLabel(row.unique_viewers)} · {formatDuration(row.read_ms)}
								</span>
							{/if}

							<span class="sr-only">{t('activity.engagement.jump', { n: row.page_no })}</span>
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
