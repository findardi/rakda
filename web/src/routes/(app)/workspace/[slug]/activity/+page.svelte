<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navigating, page } from '$app/state';
	import { roleDisplayName } from '$lib/access/permissions';
	import {
		ACTIVITY_GROUPS,
		activityActionLabel,
		activityTone,
		describeActivity,
		type ActivityTone
	} from '$lib/activity/describe';
	import { Alert, Toaster, showToast } from '$lib/components/common';
	import { formatDate, formatDateTime, formatTimeUtc } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { ActivityFilters, ActivityItem, ActivityListData } from '$lib/types/activity';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let appended = $state<ActivityListData | null>(null);
	let loadingMore = $state(false);
	let lastPayload: ActivityListData | null = null;

	$effect(() => {
		if (lastPayload === data.activity) return;
		lastPayload = data.activity;
		appended = null;
	});

	const cursor = $derived(appended ? appended.next_cursor : data.activity.next_cursor);

	const items = $derived.by(() => {
		const seen: Record<string, boolean> = {};
		const merged: ActivityItem[] = [];
		for (const item of [...data.activity.items, ...(appended?.items ?? [])]) {
			if (seen[item.id]) continue;
			seen[item.id] = true;
			merged.push(item);
		}
		return merged;
	});

	const days = $derived.by(() => {
		const grouped: { day: string; items: ActivityItem[] }[] = [];
		for (const item of items) {
			const day = formatDate(item.created_at);
			const last = grouped.at(-1);
			if (last && last.day === day) last.items.push(item);
			else grouped.push({ day, items: [item] });
		}
		return grouped;
	});

	const hasFilter = $derived(
		Boolean(data.filters.from || data.filters.to || data.filters.actor_id || data.filters.action)
	);
	const reloading = $derived(navigating.to?.url.pathname === page.url.pathname);

	const todayKey = formatDate(new Date().toISOString());
	const yesterdayKey = formatDate(new Date(Date.now() - 86_400_000).toISOString());

	const TONE_CLASS: Record<ActivityTone, string> = {
		neutral: 'text-muted',
		positive: 'text-success',
		negative: 'text-error'
	};

	const GLYPHS: Record<string, string[]> = {
		folder: ['M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z'],
		document: ['M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z', 'M14 3v5h5'],
		version: ['m12 3 8 4.5-8 4.5-8-4.5z', 'm4 12.5 8 4.5 8-4.5'],
		member: ['M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8', 'M5 20a7 7 0 0 1 14 0'],
		group: [
			'M9 12a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7',
			'M3 20a6 6 0 0 1 12 0',
			'M16 5.5a3 3 0 0 1 0 5.5',
			'M18 13.5a6 6 0 0 1 3 5.5'
		],
		invitation: ['M3 6h18v12H3z', 'm3 7.5 9 5.5 9-5.5'],
		folder_access: ['M5 11h14v9H5z', 'M9 11V7.5a3 3 0 0 1 6 0V11']
	};

	const SKELETON_WIDTHS = ['62%', '48%', '71%', '40%', '58%', '66%'];

	const glyphOf = (targetType: string) => GLYPHS[targetType] ?? GLYPHS.document;

	const dayLabel = (day: string) =>
		day === todayKey ? t('activity.today') : day === yesterdayKey ? t('activity.yesterday') : '';

	const basePath = $derived(
		resolve('/(app)/workspace/[slug]/activity', { slug: page.params.slug ?? '' })
	);

	const queryString = (entries: [string, string][]) =>
		entries
			.filter(([, value]) => value)
			.map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
			.join('&');

	function applyFilter(patch: Partial<ActivityFilters>) {
		const next = { ...data.filters, ...patch };
		const qs = queryString(Object.entries(next) as [string, string][]);
		if (!qs) {
			resetFilters();
			return;
		}
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() cannot carry a query string
		goto(`${basePath}?${qs}`, { keepFocus: true, noScroll: true });
	}

	function resetFilters() {
		goto(basePath, { noScroll: true });
	}

	async function loadMore() {
		if (!cursor || loadingMore) return;
		loadingMore = true;

		const qs = queryString([
			['workspaceId', data.workspaceId],
			['limit', String(data.pageSize)],
			['cursor', cursor],
			...(Object.entries(data.filters) as [string, string][])
		]);

		try {
			const res = await fetch(`/api/activity?${qs}`);
			if (!res.ok) throw new Error(String(res.status));
			const payload = (await res.json()) as ActivityListData;
			appended = {
				items: [...(appended?.items ?? []), ...payload.items],
				next_cursor: payload.next_cursor
			};
		} catch {
			showToast(t('activity.err.loadMore'), 'error');
		} finally {
			loadingMore = false;
		}
	}
</script>

<svelte:head><title>{t('activity.title')} · {t('brand.name')}</title></svelte:head>

{#snippet entry(item: ActivityItem)}
	{@const phrase = describeActivity(item)}
	<li class="flex items-baseline gap-3 py-2.5">
		<time
			datetime={item.created_at}
			title={formatDateTime(item.created_at)}
			class="w-11 flex-none font-mono text-xs text-muted tabular-nums"
		>
			{formatTimeUtc(item.created_at)}
		</time>

		<svg
			class="h-4 w-4 flex-none translate-y-0.5 {TONE_CLASS[activityTone(item.action)]}"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="1.6"
			stroke-linecap="round"
			stroke-linejoin="round"
			aria-hidden="true"
		>
			{#each glyphOf(item.target_type) as d (d)}
				<path {d} />
			{/each}
		</svg>

		<p class="min-w-0 flex-1 text-sm text-pretty">
			<span class="font-medium">{item.actor_name || t('activity.actor.system')}</span>
			{#if item.actor_role}
				<span
					class="mx-0.5 rounded-selector bg-base-content/6 px-1.5 py-0.5 align-[0.05em] text-[0.6875rem] text-muted"
					>{roleDisplayName(item.actor_role)}</span
				>
			{/if}
			{#if phrase.key}
				{t(phrase.key, phrase.vars)}
			{:else}
				<code class="font-mono text-xs">{item.action}</code>
				{item.target_name}
			{/if}
		</p>
	</li>
{/snippet}

<div class="mx-auto w-full max-w-4xl px-6 py-8">
	<header>
		<h1 class="text-2xl font-semibold tracking-[-0.02em]">{t('activity.title')}</h1>
		<p class="mt-1.5 max-w-prose text-sm text-muted">{t('activity.desc')}</p>
	</header>

	<div class="mt-6 flex flex-wrap items-end gap-x-3 gap-y-2">
		<label class="flex flex-col gap-1">
			<span class="text-xs text-muted">{t('activity.filter.from')}</span>
			<input
				type="date"
				class="input input-sm w-40"
				value={data.filters.from}
				max={data.filters.to || undefined}
				onchange={(e) => applyFilter({ from: e.currentTarget.value })}
			/>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs text-muted">{t('activity.filter.to')}</span>
			<input
				type="date"
				class="input input-sm w-40"
				value={data.filters.to}
				min={data.filters.from || undefined}
				onchange={(e) => applyFilter({ to: e.currentTarget.value })}
			/>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs text-muted">{t('activity.filter.action')}</span>
			<select
				class="select select-sm w-48"
				value={data.filters.action}
				onchange={(e) => applyFilter({ action: e.currentTarget.value })}
			>
				<option value="">{t('activity.filter.allActions')}</option>
				{#each ACTIVITY_GROUPS as group (group.key)}
					<optgroup label={t(group.key)}>
						{#each group.actions as action (action)}
							<option value={action}>{activityActionLabel(action)}</option>
						{/each}
					</optgroup>
				{/each}
			</select>
		</label>

		<label class="flex flex-col gap-1">
			<span class="text-xs text-muted">{t('activity.filter.actor')}</span>
			<select
				class="select select-sm w-40"
				value={data.filters.actor_id}
				onchange={(e) => applyFilter({ actor_id: e.currentTarget.value })}
			>
				<option value="">{t('activity.filter.allActors')}</option>
				{#each data.actors as actor (actor.id)}
					<option value={actor.id}>{actor.name}</option>
				{/each}
			</select>
		</label>

		{#if hasFilter}
			<button type="button" class="btn btn-ghost btn-sm" onclick={resetFilters}>
				{t('activity.filter.reset')}
			</button>
		{/if}
	</div>

	{#if reloading}
		<ul class="mt-6 border-t border-base-content/8" aria-hidden="true">
			{#each SKELETON_WIDTHS as width (width)}
				<li class="flex items-center gap-3 border-b border-base-content/8 py-3">
					<span
						class="h-2.5 w-11 flex-none animate-pulse rounded-selector bg-base-content/10 motion-reduce:animate-none"
					></span>
					<span
						class="h-2.5 animate-pulse rounded-selector bg-base-content/10 motion-reduce:animate-none"
						style="width: {width}"
					></span>
				</li>
			{/each}
		</ul>
	{:else if data.filterRejected}
		<div class="mt-6">
			<Alert align="start">{t('activity.err.filter')}</Alert>
		</div>
	{:else if items.length === 0}
		<div
			class="mt-6 flex flex-col items-center justify-center gap-3 rounded-box border border-base-content/10 bg-base-100 px-6 py-16 text-center"
		>
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
				<path d="M3 12h4l2 6 4-12 2 6h6" />
			</svg>
			<div>
				<p class="text-[0.9375rem] font-medium">
					{hasFilter ? t('activity.noMatch.title') : t('activity.empty.title')}
				</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
					{hasFilter ? t('activity.noMatch.body') : t('activity.empty.body')}
				</p>
			</div>
		</div>
	{:else}
		{#if hasFilter}
			<p class="mt-4 text-xs text-muted">
				{#if cursor}
					{t('activity.countMore', { n: items.length })}
				{:else if items.length === 1}
					{t('activity.countOne', { n: items.length })}
				{:else}
					{t('activity.count', { n: items.length })}
				{/if}
			</p>
		{/if}

		<div class="mt-4">
			{#each days as group (group.day)}
				{@const label = dayLabel(group.day)}
				<section>
					<h2
						class="sticky top-0 z-overlay -mx-1 flex items-baseline gap-2 bg-base-200 px-1 pt-3 pb-1.5"
					>
						<span class="font-mono text-xs text-muted tabular-nums">{group.day}</span>
						{#if label}<span class="text-xs text-muted">· {label}</span>{/if}
					</h2>
					<ul class="divide-y divide-base-content/8 border-t border-base-content/8">
						{#each group.items as item (item.id)}
							{@render entry(item)}
						{/each}
					</ul>
				</section>
			{/each}
		</div>

		{#if cursor}
			<div class="mt-6 flex justify-center">
				<button
					type="button"
					class="btn btn-ghost btn-sm"
					onclick={loadMore}
					disabled={loadingMore}
					aria-busy={loadingMore}
				>
					{#if loadingMore}
						<span class="loading loading-spinner loading-xs"></span>
						{t('activity.loadingMore')}
					{:else}
						{t('activity.loadMore')}
					{/if}
				</button>
			</div>
		{/if}
	{/if}
</div>

<Toaster />
