<script lang="ts">
	import { formatDuration } from '$lib/format';
	import { t } from '$lib/i18n';
	import { MediaQuery } from 'svelte/reactivity';
	import type { ReaderPageEngagement } from '$lib/types/activity';

	type Props = {
		pages: ReaderPageEngagement[];
		/** Pages of the served version; 0 while no rendition exists. */
		pageCount: number;
		readerName: string;
		/** Viewer deep link to one page — the table view's way into the document. */
		pageHref: (page: number) => string;
	};

	let { pages, pageCount, readerName, pageHref }: Props = $props();

	type Col = { page: number; opens: number; readMs: number; beyond: boolean };

	// One column per page from 1 to the served count, then anything an older
	// version recorded past it. A page never opened is a column too: the gap is
	// the finding, so it is drawn, never folded away.
	const cols = $derived.by(() => {
		const byPage: Record<number, ReaderPageEngagement> = {};
		let maxNo = 0;
		for (const p of pages) {
			if (p.page_no < 1) continue;
			byPage[p.page_no] = p;
			if (p.page_no > maxNo) maxNo = p.page_no;
		}
		const total = Math.max(pageCount, maxNo);
		const out: Col[] = [];
		for (let n = 1; n <= total; n++) {
			const p = byPage[n];
			out.push({
				page: n,
				opens: p?.opens ?? 0,
				readMs: p?.read_ms ?? 0,
				beyond: pageCount > 0 && n > pageCount
			});
		}
		return out;
	});

	const total = $derived(cols.length);
	const opened = (c: Col) => c.opens > 0 || c.readMs > 0;
	const seen = $derived(cols.filter(opened).length);
	const maxMs = $derived(cols.reduce((m, c) => (c.readMs > m ? c.readMs : m), 0));
	// The first column holding the maximum: the one bar that carries a label.
	const peakIdx = $derived(maxMs > 0 ? cols.findIndex((c) => c.readMs === maxMs) : -1);

	// --- geometry -----------------------------------------------------------
	// Columns share the measured width down to a floor that keeps every bar a
	// real hit target; past that the chart scrolls sideways rather than binning
	// pages. Bars never exceed 24px, so a five-page document is not five blocks.
	const SLOT_MIN = 6;
	// A finger needs a wider column than a pointer does.
	const SLOT_MIN_COARSE = 12;
	const SLOT_MAX = 28;
	const BAR_MAX = 24;
	const GAP = 2;
	const PLOT_H = 160;
	// Room above the tallest bar for the peak label or a three-line tooltip.
	const HEADROOM = 72;
	const STUB_H = 2;
	const MIN_BAR_H = 4;

	let width = $state(0);
	const coarse = new MediaQuery('(pointer: coarse)');
	const slotMin = $derived(coarse.current ? SLOT_MIN_COARSE : SLOT_MIN);
	const slot = $derived(
		total === 0 || width === 0
			? SLOT_MAX
			: Math.min(SLOT_MAX, Math.max(slotMin, Math.floor(width / total)))
	);
	const bar = $derived(Math.min(BAR_MAX, slot - GAP));
	const chartWidth = $derived(total * slot);

	// Any recorded time still registers as a mark, never a hairline; an open
	// without dwell gets the same minimum so "opened" and "not opened" differ.
	const barHeight = (c: Col) => {
		if (c.readMs > 0 && maxMs > 0) return Math.max(MIN_BAR_H, (c.readMs / maxMs) * PLOT_H);
		return c.opens > 0 ? MIN_BAR_H : STUB_H;
	};

	// Ticks thin out with the slot width so labels never collide.
	const STEPS = [1, 2, 4, 5, 10, 20, 25, 50, 100, 200, 250, 500];
	const ticks = $derived.by(() => {
		if (total === 0) return [];
		const maxTicks = Math.max(2, Math.floor(chartWidth / 64));
		const step = STEPS.find((s) => total / s <= maxTicks) ?? STEPS[STEPS.length - 1];
		const out: number[] = [];
		for (let n = 1; n <= total; n += step) out.push(n);
		return out;
	});

	// A floating label near either edge would clip on the scroll container, so
	// it hangs off the column's edge instead of its centre.
	const EDGE = 56;
	const anchor = (i: number): 'start' | 'center' | 'end' => {
		const x = (i + 0.5) * slot;
		if (x < EDGE) return 'start';
		if (chartWidth - x < EDGE) return 'end';
		return 'center';
	};
	const anchorStyle = (i: number) => {
		const a = anchor(i);
		if (a === 'start') return `left: ${i * slot}px;`;
		if (a === 'end') return `right: ${chartWidth - (i + 1) * slot}px;`;
		return `left: ${(i + 0.5) * slot}px; transform: translateX(-50%);`;
	};

	// --- hover / focus --------------------------------------------------------
	// One tab stop for the whole chart, arrows inside it. The tooltip follows
	// whichever of pointer or focus touched a column last, and only the source
	// that showed it can hide it — arrowing away from a column the mouse still
	// rests on must not snap the tooltip back.
	let shownIdx = $state<number | null>(null);
	let shownBy: 'hover' | 'focus' | null = null;
	let activeIdx = $state(0);
	const tabIdx = $derived(Math.min(activeIdx, Math.max(0, total - 1)));
	let groupEl = $state<HTMLElement>();

	function show(i: number, by: 'hover' | 'focus') {
		shownIdx = i;
		shownBy = by;
	}
	function hide(i: number, by: 'hover' | 'focus') {
		if (shownIdx === i && shownBy === by) {
			shownIdx = null;
			shownBy = null;
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if (total === 0) return;
		let next: number;
		switch (e.key) {
			case 'ArrowRight':
				next = Math.min(total - 1, tabIdx + 1);
				break;
			case 'ArrowLeft':
				next = Math.max(0, tabIdx - 1);
				break;
			case 'Home':
				next = 0;
				break;
			case 'End':
				next = total - 1;
				break;
			default:
				return;
		}
		e.preventDefault();
		activeIdx = next;
		groupEl?.querySelector<HTMLElement>(`[data-page="${cols[next].page}"]`)?.focus();
	}

	const opensLabel = (n: number) =>
		t(n === 1 ? 'activity.engagement.opensOne' : 'activity.engagement.opens', { n });

	// The whole reading in one sentence per column: what a screen reader gets
	// instead of the tooltip.
	function describe(c: Col): string {
		const page = t('activity.engagement.chart.page', { n: c.page });
		if (!opened(c)) return `${page}: ${t('activity.engagement.chart.unread')}`;
		const time =
			c.readMs >= 1000 ? formatDuration(c.readMs) : t('activity.engagement.chart.noTime');
		return `${page}: ${time}, ${opensLabel(c.opens)}`;
	}

	// --- sideways scroll ------------------------------------------------------
	// On a narrow card the strip scrolls. A fade on the clipped edge says so,
	// because on a phone the scrollbar itself is invisible until touched.
	let scroller = $state<HTMLElement>();
	let moreLeft = $state(false);
	let moreRight = $state(false);
	function syncEdges() {
		const el = scroller;
		if (!el) return;
		moreLeft = el.scrollLeft > 1;
		moreRight = el.scrollLeft + el.clientWidth < el.scrollWidth - 1;
	}
	// A strip that overflows opens centred on the peak, so the one direct label
	// is never born under the fade; the fades then say where the rest lies.
	$effect(() => {
		const el = scroller;
		void chartWidth;
		void width;
		if (!el) return;
		if (peakIdx >= 0) {
			const target = (peakIdx + 0.5) * slot - el.clientWidth / 2;
			el.scrollLeft = Math.max(0, Math.min(target, el.scrollWidth - el.clientWidth));
		}
		syncEdges();
	});

	// --- table twin -----------------------------------------------------------
	let view = $state<'chart' | 'table'>('chart');
	const openedCols = $derived(cols.filter(opened));
	// "3–7, 12, 19–24": the unopened pages as ranges, so the table stays short
	// on a long document read in a few places.
	const unreadRanges = $derived.by(() => {
		const out: string[] = [];
		let start: number | null = null;
		let prev = 0;
		for (const c of cols) {
			if (opened(c)) {
				if (start !== null) out.push(start === prev ? `${start}` : `${start}–${prev}`);
				start = null;
				continue;
			}
			if (start === null) start = c.page;
			prev = c.page;
		}
		if (start !== null) out.push(start === prev ? `${start}` : `${start}–${prev}`);
		return out.join(', ');
	});
</script>

<div class="rakda-rpc" bind:clientWidth={width}>
	<div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
		<p class="text-sm font-medium">{t('activity.engagement.chart.label', { name: readerName })}</p>
		<div class="flex flex-none rounded-field border border-base-content/10 p-0.5" role="group">
			<button
				type="button"
				aria-pressed={view === 'chart'}
				onclick={() => (view = 'chart')}
				class="rakda-rpc-seg rounded-[4px] px-2.5 py-1 text-xs font-medium transition-colors"
			>
				{t('activity.engagement.view.chart')}
			</button>
			<button
				type="button"
				aria-pressed={view === 'table'}
				onclick={() => (view = 'table')}
				class="rakda-rpc-seg rounded-[4px] px-2.5 py-1 text-xs font-medium transition-colors"
			>
				{t('activity.engagement.view.table')}
			</button>
		</div>
	</div>

	{#if total === 0}
		<p class="mt-6 text-sm text-muted">{t('activity.engagement.chart.captionNone')}</p>
	{:else if view === 'chart'}
		<!-- The scroll container owns both axes of overflow, so the tooltip and
		     the peak label live inside it and ride the same headroom. -->
		<div class="relative mt-4">
			<div bind:this={scroller} onscroll={syncEdges} class="rakda-rpc-scroll overflow-x-auto pb-1">
				<div class="relative" style="width: {chartWidth}px; height: {HEADROOM + PLOT_H}px">
					<div
						bind:this={groupEl}
						role="group"
						aria-label={t('activity.engagement.chart.label', { name: readerName })}
						aria-describedby="rakda-rpc-keys"
						class="absolute inset-x-0 bottom-0 flex items-end"
						style="height: {PLOT_H}px"
					>
						{#each cols as c, i (c.page)}
							{@const h = barHeight(c)}
							{@const isOpen = opened(c)}
							<button
								type="button"
								data-page={c.page}
								tabindex={i === tabIdx ? 0 : -1}
								aria-label={describe(c)}
								onkeydown={onKeydown}
								onpointerenter={() => show(i, 'hover')}
								onpointerleave={() => hide(i, 'hover')}
								onfocus={() => {
									show(i, 'focus');
									activeIdx = i;
								}}
								onblur={() => hide(i, 'focus')}
								class="rakda-rpc-col relative flex h-full flex-none items-end justify-center"
								style="width: {slot}px"
							>
								<span
									class="rakda-rpc-bar block {isOpen ? 'is-read' : 'is-stub'} {shownIdx === i
										? 'is-hot'
										: ''}"
									style="width: {bar}px; height: {h}px"
								></span>
							</button>
						{/each}
					</div>

					<span
						class="pointer-events-none absolute inset-x-0 bottom-0 h-px bg-base-content/15"
						aria-hidden="true"
					></span>

					<!-- The extreme is the one direct label; the tooltip carries every
				     other value, the table view carries them all. -->
					{#if peakIdx >= 0 && shownIdx !== peakIdx}
						<span
							class="pointer-events-none absolute font-mono text-xs whitespace-nowrap tabular-nums"
							style="bottom: {barHeight(cols[peakIdx]) + 6}px; {anchorStyle(peakIdx)}"
							aria-hidden="true"
						>
							{formatDuration(cols[peakIdx].readMs)}
						</span>
					{/if}

					{#if shownIdx !== null && cols[shownIdx]}
						{@const c = cols[shownIdx]}
						<div
							class="rakda-rpc-tip pointer-events-none absolute z-10 rounded-field border border-base-content/10 bg-base-100 px-2.5 py-1.5 whitespace-nowrap shadow-md"
							style="bottom: {barHeight(c) + 8}px; {anchorStyle(shownIdx)}"
							aria-hidden="true"
						>
							<p class="text-[0.6875rem] text-muted">
								{t('activity.engagement.chart.page', { n: c.page })}
							</p>
							{#if !opened(c)}
								<p class="text-xs font-medium">{t('activity.engagement.chart.unread')}</p>
							{:else}
								<p class="font-mono text-sm font-semibold tabular-nums">
									{c.readMs >= 1000
										? formatDuration(c.readMs)
										: t('activity.engagement.chart.noTime')}
								</p>
								<p class="font-mono text-[0.6875rem] text-muted tabular-nums">
									{opensLabel(c.opens)}
								</p>
							{/if}
							{#if c.beyond}
								<p class="mt-1 max-w-56 text-[0.6875rem] whitespace-normal text-muted">
									{t('activity.engagement.chart.beyond', { n: c.page })}
								</p>
							{/if}
						</div>
					{/if}
				</div>

				<div class="relative h-5" style="width: {chartWidth}px" aria-hidden="true">
					{#each ticks as n (n)}
						<span
							class="absolute top-1 -translate-x-1/2 font-mono text-[0.6875rem] text-muted tabular-nums"
							style="left: {(n - 0.5) * slot}px"
						>
							{n}
						</span>
					{/each}
				</div>
			</div>
			{#if moreLeft}
				<span class="rakda-rpc-fade is-left" aria-hidden="true"></span>
			{/if}
			{#if moreRight}
				<span class="rakda-rpc-fade is-right" aria-hidden="true"></span>
			{/if}
		</div>
		<p id="rakda-rpc-keys" class="sr-only">{t('activity.engagement.chart.keys')}</p>
	{:else}
		<div class="mt-4 overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-base-content/10 text-left text-xs text-muted">
						<th scope="col" class="py-2 pr-4 font-medium">{t('activity.engagement.table.page')}</th>
						<th scope="col" class="py-2 pr-4 font-medium">{t('activity.engagement.table.read')}</th>
						<th scope="col" class="py-2 pr-4 font-medium">{t('activity.engagement.table.opens')}</th
						>
						<th scope="col" class="py-2 text-right font-medium">
							<span class="sr-only">{t('activity.engagement.table.open')}</span>
						</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-base-content/6">
					{#each openedCols as c (c.page)}
						<tr>
							<td class="py-2 pr-4 font-mono text-xs tabular-nums">{c.page}</td>
							<td class="py-2 pr-4 font-mono text-xs tabular-nums">
								{c.readMs >= 1000
									? formatDuration(c.readMs)
									: t('activity.engagement.chart.noTime')}
							</td>
							<td class="py-2 pr-4 font-mono text-xs text-muted tabular-nums">
								{opensLabel(c.opens)}
							</td>
							<td class="py-2 text-right">
								<!-- Into the viewer: a view must be a completed click, so no hover preload. -->
								<!-- eslint-disable svelte/no-navigation-without-resolve -- href comes from resolve() in the caller -->
								<a
									href={pageHref(c.page)}
									data-sveltekit-preload-data="off"
									aria-label={t('activity.engagement.table.openOf', { n: c.page })}
									class="text-xs text-muted underline decoration-base-content/30 underline-offset-2 hover:text-base-content hover:decoration-current"
								>
									{t('activity.engagement.table.open')}
								</a>
								<!-- eslint-enable svelte/no-navigation-without-resolve -->
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
			{#if unreadRanges}
				<p class="mt-3 font-mono text-xs text-muted tabular-nums">
					{t('activity.engagement.table.unread', { ranges: unreadRanges })}
				</p>
			{/if}
		</div>
	{/if}

	{#if total > 0}
		<p class="mt-4 text-xs text-muted text-pretty">
			{t('activity.engagement.chart.caption', { seen, total })}
		</p>
	{/if}
</div>

<style>
	.rakda-rpc-seg[aria-pressed='true'] {
		background-color: color-mix(in oklch, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
	}
	.rakda-rpc-seg[aria-pressed='false'] {
		color: var(--color-muted);
	}
	.rakda-rpc-seg[aria-pressed='false']:hover {
		color: var(--color-base-content);
	}

	/* Square at the baseline, rounded at the data end; a stub is a stub. */
	.rakda-rpc-bar.is-read {
		background-color: var(--color-primary);
		border-radius: 4px 4px 0 0;
	}
	.rakda-rpc-bar.is-stub {
		background-color: color-mix(in oklch, var(--color-base-content) 18%, transparent);
	}
	.rakda-rpc-bar.is-read.is-hot {
		background-color: color-mix(in oklch, var(--color-primary) 78%, white);
	}
	.rakda-rpc-bar.is-stub.is-hot {
		background-color: color-mix(in oklch, var(--color-base-content) 40%, transparent);
	}
	.rakda-rpc-col:focus-visible {
		outline-offset: -2px;
		border-radius: 3px;
	}
	.rakda-rpc-tip {
		animation: rakda-rpc-tip-in 120ms ease-out;
	}
	/* The strip's own scrollbar, in the palette rather than the browser's grey. */
	.rakda-rpc-scroll {
		scrollbar-width: thin;
		scrollbar-color: color-mix(in oklch, var(--color-base-content) 30%, transparent) transparent;
	}
	.rakda-rpc-fade {
		position: absolute;
		top: 0;
		bottom: 0.75rem;
		width: 2.5rem;
		pointer-events: none;
	}
	.rakda-rpc-fade.is-right {
		right: 0;
		background: linear-gradient(to left, var(--color-base-100), transparent);
	}
	.rakda-rpc-fade.is-left {
		left: 0;
		background: linear-gradient(to right, var(--color-base-100), transparent);
	}
	@keyframes rakda-rpc-tip-in {
		from {
			opacity: 0;
			translate: 0 2px;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-rpc-tip {
			animation: none;
		}
	}
</style>
