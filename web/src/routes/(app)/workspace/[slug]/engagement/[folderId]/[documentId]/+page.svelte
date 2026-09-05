<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navigating, page } from '$app/state';
	import { ReaderPageChart } from '$lib/components/app';
	import { Alert, Button } from '$lib/components/common';
	import { formatDate, formatDuration } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { ReaderEngagement } from '$lib/types/activity';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const slug = $derived(page.params.slug!);
	const folderId = $derived(page.params.folderId!);
	const documentId = $derived(page.params.documentId!);

	const basePath = $derived(
		resolve('/(app)/workspace/[slug]/engagement/[folderId]/[documentId]', {
			slug,
			folderId,
			documentId
		})
	);
	// The folder is in the path, so back returns to the list the row came from.
	const folderHref = $derived(
		resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId })
	);
	const viewerHref = $derived(
		resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', { slug, folderId, documentId })
	);
	// resolve() has no slot for a query string; the page number and the reader ride on it.
	const pageHref = (n: number) => `${viewerHref}?page=${n}`;
	const readerHref = (id: string) => `${basePath}?reader=${encodeURIComponent(id)}`;
	const exportHref = $derived(
		`/api/activity/engagement/export?workspaceId=${encodeURIComponent(data.workspaceId)}` +
			`&documentId=${encodeURIComponent(documentId)}&format=csv`
	);

	const readers = $derived(data.readers);
	// The account can be gone; the email snapshot on the event row outlives it.
	const readerLabel = (r: ReaderEngagement) => r.actor_name || r.actor_email;

	// A group is the reader's *current* one, so one bidder's team reads as one
	// block. Readers who left the room have none and close the list.
	type Group = { id: string; name: string; readers: ReaderEngagement[]; readMs: number };
	const groups = $derived.by(() => {
		const byGroup: Record<string, Group> = {};
		for (const r of readers) {
			const key = r.group_id || '';
			const g = (byGroup[key] ??= { id: key, name: r.group_name, readers: [], readMs: 0 });
			g.readers.push(r);
			g.readMs += r.read_ms;
		}
		return Object.values(byGroup).sort((a, b) => {
			if ((a.id === '') !== (b.id === '')) return a.id === '' ? 1 : -1;
			return b.readMs - a.readMs || a.name.localeCompare(b.name);
		});
	});

	const selected = $derived(
		data.readerId ? (readers.find((r) => r.actor_id === data.readerId) ?? null) : null
	);
	const detail = $derived(data.detail);
	const detailPages = $derived(detail?.pages ?? []);
	const pageCount = $derived(detail?.page_count ?? data.document.pageCount);

	// The longest page, from the same rows the chart draws.
	const peak = $derived.by(() => {
		let best: { page: number; ms: number } | null = null;
		for (const p of detailPages) {
			if (p.read_ms > 0 && (!best || p.read_ms > best.ms))
				best = { page: p.page_no, ms: p.read_ms };
		}
		return best;
	});

	// Picking a reader is a navigation. The chart on screen stays, dimmed,
	// until the new numbers land — no skeleton flash, no layout jump.
	const switching = $derived(
		!!navigating.to &&
			navigating.to.url.pathname === page.url.pathname &&
			navigating.to.url.searchParams.get('reader') !== data.readerId
	);

	let refreshing = $state(false);
	async function refresh(): Promise<void> {
		if (refreshing) return;
		refreshing = true;
		try {
			await invalidateAll();
		} finally {
			refreshing = false;
		}
	}

	// No plural machinery in t(); the dictionary carries both forms.
	const opensLabel = (n: number) =>
		t(n === 1 ? 'activity.engagement.opensOne' : 'activity.engagement.opens', { n });
	const summary = $derived(
		t(readers.length === 1 ? 'activity.engagement.summaryOne' : 'activity.engagement.summary', {
			n: readers.length,
			read: formatDuration(data.totalReadMs)
		})
	);
	const pageCountLabel = $derived(
		t(
			data.document.pageCount === 1
				? 'activity.engagement.pageCountOne'
				: 'activity.engagement.pageCount',
			{ n: data.document.pageCount }
		)
	);
	const groupSummary = (g: Group) =>
		t(
			g.readers.length === 1
				? 'activity.engagement.group.summaryOne'
				: 'activity.engagement.group.summary',
			{ n: g.readers.length, read: formatDuration(g.readMs) }
		);
	const pagesOf = (r: ReaderEngagement) =>
		t('activity.engagement.pagesOf', { n: r.pages_seen, total: data.document.pageCount });
</script>

<svelte:head>
	<title>{t('activity.engagement.title')} · {data.document.name} · {t('brand.name')}</title>
</svelte:head>

{#snippet fact(label: string, value: string, mono: boolean, sub: string | undefined, wide: boolean)}
	<!-- `wide` facts take the whole row on a phone: an identity or a two-part
	     value must not lose its tail to a half-width column. -->
	<div class="min-w-0 {wide ? 'col-span-2 sm:col-span-1' : ''}">
		<p class="text-xs text-muted">{label}</p>
		<!-- A machine value never breaks mid-token: it stays on one line and
		     truncates with its full text on hover; a person's name may wrap. -->
		<p
			class="mt-0.5 text-sm {mono ? 'truncate font-mono tabular-nums' : 'break-words'}"
			title={mono ? value : undefined}
		>
			{value}
		</p>
		{#if sub}
			<p class="truncate font-mono text-xs text-muted" title={sub}>{sub}</p>
		{/if}
	</div>
{/snippet}

{#snippet glyph(paths: string[], cls: string)}
	<svg
		class={cls}
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#each paths as d (d)}<path {d} />{/each}
	</svg>
{/snippet}

<div class="mx-auto w-full max-w-6xl px-6 py-8">
	<a
		href={folderHref}
		class="inline-flex items-center gap-1.5 text-xs font-medium text-muted no-underline transition-colors hover:text-base-content"
	>
		{@render glyph(['m15 6-6 6 6 6'], 'h-3.5 w-3.5 flex-none')}
		{t('activity.engagement.back')}
	</a>

	<header class="mt-2 flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
		<div class="min-w-0 flex-1">
			<h1 class="text-2xl font-semibold tracking-[-0.02em] break-words">
				{data.document.name}
			</h1>
			<p class="mt-1.5 text-sm text-muted text-pretty">{t('activity.engagement.desc')}</p>
			<p class="mt-2 font-mono text-xs text-muted tabular-nums">
				{summary} · {pageCountLabel}
			</p>
		</div>

		<!-- Constrained to the line, so on a phone the buttons wrap instead of
		     pushing the page into a sideways scroll. -->
		<div class="flex max-w-full flex-wrap items-center gap-2">
			<!-- Into the viewer: GET /view writes document_viewed, so a view must be
			     a completed click — never a hover preload. -->
			<a
				href={viewerHref}
				data-sveltekit-preload-data="off"
				class="btn btn-ghost btn-sm gap-1.5 no-underline"
			>
				{@render glyph(
					[
						'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z',
						'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z'
					],
					'h-4 w-4'
				)}
				{t('activity.engagement.openDoc')}
			</a>
			<!-- eslint-disable svelte/no-navigation-without-resolve -- endpoint, not a page: resolve() has no entry for /api routes -->
			<a
				href={exportHref}
				title={t('activity.engagement.exportTitle')}
				class="btn btn-ghost btn-sm gap-1.5 no-underline"
			>
				{@render glyph(['M12 4v11M7.5 10.5 12 15l4.5-4.5', 'M5 19h14'], 'h-4 w-4')}
				{t('activity.engagement.export')}
			</a>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
			<Button variant="ghost" size="sm" onclick={() => void refresh()} loading={refreshing}>
				{@render glyph(
					[
						'M20 11a8 8 0 0 0-13.7-5.3L4 8',
						'M4 4v4h4',
						'M4 13a8 8 0 0 0 13.7 5.3L20 16',
						'M20 20v-4h-4'
					],
					'h-4 w-4'
				)}
				{t('activity.engagement.refresh')}
			</Button>
		</div>
	</header>

	{#if readers.length === 0}
		<div
			class="mt-6 flex flex-col items-center gap-3 rounded-box border border-base-content/10 bg-base-100 px-6 py-16 text-center"
		>
			{@render glyph(['M4 19h16', 'M7 19v-4M12 19v-7M17 19v-2'], 'h-9 w-9 text-muted/70')}
			<div>
				<p class="text-[0.9375rem] font-medium">{t('activity.engagement.empty.title')}</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
					{t('activity.engagement.empty.body')}
				</p>
			</div>
		</div>
	{:else}
		<div class="mt-6 grid items-start gap-6 lg:grid-cols-[19rem_minmax(0,1fr)]">
			<!-- Last on a phone so a pick lands on its result, first on a desktop
			     where both fit side by side. -->
			<nav
				aria-label={t('activity.engagement.readers')}
				class="order-last min-w-0 rounded-box border border-base-content/10 bg-base-100 lg:order-none"
			>
				<h2 class="border-b border-base-content/8 px-4 py-3 text-sm font-semibold">
					{t('activity.engagement.readers')}
				</h2>
				{#each groups as g (g.id)}
					<section aria-labelledby="eng-group-{g.id || 'none'}">
						<!-- Name and total share a line while they fit; a long group name
						     pushes the total to its own line rather than truncating either. -->
						<div
							class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 border-b border-base-content/6 bg-base-200/70 px-4 py-1.5"
						>
							<h3
								id="eng-group-{g.id || 'none'}"
								class="min-w-0 text-xs font-medium"
								title={g.id ? undefined : t('activity.engagement.group.noneHint')}
							>
								{g.id ? g.name : t('activity.engagement.group.none')}
							</h3>
							<span class="font-mono text-[0.6875rem] whitespace-nowrap text-muted tabular-nums">
								{groupSummary(g)}
							</span>
						</div>
						<ul class="divide-y divide-base-content/6">
							{#each g.readers as r (r.actor_id)}
								{@const active = r.actor_id === data.readerId}
								<li>
									<!-- eslint-disable svelte/no-navigation-without-resolve -- the path is from resolve(); only ?reader= is appended -->
									<a
										href={readerHref(r.actor_id)}
										data-sveltekit-noscroll
										aria-current={active ? 'true' : undefined}
										aria-label={t('activity.engagement.openReader', { name: readerLabel(r) })}
										class="flex flex-col gap-y-0.5 px-4 py-2.5 no-underline transition-colors
											{active ? 'bg-primary/6' : 'hover:bg-base-content/4'}"
									>
										<!-- Identity outranks the stat: when both cannot share the line the
										     duration drops beneath the name instead of truncating it. -->
										<span class="flex flex-wrap items-baseline gap-x-3">
											<span
												class="min-w-0 flex-auto truncate text-sm font-medium {active
													? 'text-primary'
													: ''}"
												title={readerLabel(r)}
											>
												{readerLabel(r)}
											</span>
											<span class="ml-auto flex-none font-mono text-xs tabular-nums">
												{formatDuration(r.read_ms)}
											</span>
										</span>
										{#if r.actor_name}
											<span class="truncate font-mono text-xs text-muted" title={r.actor_email}>
												{r.actor_email}
											</span>
										{/if}
										<span class="font-mono text-[0.6875rem] text-muted tabular-nums">
											{pagesOf(r)} · {opensLabel(r.opens)} · {t('activity.engagement.lastRead', {
												when: formatDate(r.last_read_at)
											})}
										</span>
									</a>
									<!-- eslint-enable svelte/no-navigation-without-resolve -->
								</li>
							{/each}
						</ul>
					</section>
				{/each}
			</nav>

			<section
				aria-busy={switching}
				class="min-w-0 rounded-box border border-base-content/10 bg-base-100 transition-opacity duration-150 motion-reduce:transition-none
					{switching ? 'opacity-60' : ''}"
			>
				{#if data.readerMissing}
					<div class="px-6 py-14 text-center">
						<p class="text-sm text-muted text-pretty">{t('activity.engagement.readerMissing')}</p>
					</div>
				{:else if !selected}
					<div class="flex flex-col items-center gap-3 px-6 py-14 text-center">
						{@render glyph(['M4 19h16', 'M7 19v-4M12 19v-7M17 19v-2'], 'h-8 w-8 text-muted/70')}
						<div>
							<p class="text-sm font-medium">{t('activity.engagement.pick.title')}</p>
							<p class="mx-auto mt-1 max-w-sm text-xs text-muted text-pretty">
								{t('activity.engagement.pick.body')}
							</p>
						</div>
					</div>
				{:else if data.detailError || !detail}
					<div class="flex flex-col items-start gap-3 p-5">
						<Alert align="start">{data.detailError ?? t('activity.engagement.err.load')}</Alert>
						<Button variant="ghost" size="sm" onclick={() => void refresh()} loading={refreshing}>
							{t('activity.engagement.retry')}
						</Button>
					</div>
				{:else}
					<div class="grid grid-cols-2 gap-x-6 gap-y-4 px-5 py-5 md:grid-cols-3">
						{@render fact(
							t('activity.engagement.card.reader'),
							readerLabel(selected),
							false,
							selected.actor_name ? selected.actor_email : undefined,
							true
						)}
						{@render fact(
							t('activity.engagement.card.group'),
							selected.group_id ? selected.group_name : t('activity.engagement.group.none'),
							false,
							undefined,
							false
						)}
						{@render fact(
							t('activity.engagement.card.total'),
							formatDuration(selected.read_ms),
							true,
							undefined,
							false
						)}
						{@render fact(
							t('activity.engagement.card.longest'),
							peak
								? t('activity.engagement.card.longestAt', {
										n: peak.page,
										read: formatDuration(peak.ms)
									})
								: '—',
							true,
							undefined,
							true
						)}
						{@render fact(
							t('activity.engagement.card.opens'),
							opensLabel(selected.opens),
							true,
							undefined,
							false
						)}
						{@render fact(
							t('activity.engagement.card.last'),
							formatDate(selected.last_read_at),
							true,
							undefined,
							false
						)}
					</div>

					<div class="border-t border-base-content/8 px-5 py-5">
						<ReaderPageChart
							pages={detailPages}
							{pageCount}
							readerName={readerLabel(selected)}
							{pageHref}
						/>
					</div>
				{/if}
			</section>
		</div>
	{/if}

	<p class="mt-4 text-xs text-muted text-pretty">{t('activity.engagement.hint')}</p>
</div>
