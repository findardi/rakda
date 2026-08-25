<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navigating, page } from '$app/state';
	import { Alert, showToast } from '$lib/components/common';
	import { formatDate } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { QaFilters, QaListData, QuestionListItem, QuestionStatus } from '$lib/types/qa';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	let appended = $state<QaListData | null>(null);
	let loadingMore = $state(false);
	let lastPayload: QaListData | null = null;

	$effect(() => {
		if (lastPayload === data.qa) return;
		lastPayload = data.qa;
		appended = null;
	});

	const cursor = $derived(appended ? appended.next_cursor : data.qa.next_cursor);

	const items = $derived.by(() => {
		const seen: Record<string, boolean> = {};
		const merged: QuestionListItem[] = [];
		for (const item of [...data.qa.items, ...(appended?.items ?? [])]) {
			if (seen[item.id]) continue;
			seen[item.id] = true;
			merged.push(item);
		}
		return merged;
	});

	const hasFilter = $derived(Boolean(data.filters.status || data.filters.group_id));
	const reloading = $derived(navigating.to?.url.pathname === page.url.pathname);

	const slug = $derived(page.params.slug ?? '');
	const basePath = $derived(resolve('/(app)/workspace/[slug]/qa', { slug }));

	const threadHref = (questionId: string) =>
		resolve('/(app)/workspace/[slug]/qa/[questionId]', { slug, questionId });

	const statusMeta = (s: QuestionStatus) => {
		if (s === 'waiting') return { label: t('qa.status.waiting'), dot: 'bg-warning' };
		if (s === 'answered') return { label: t('qa.status.answered'), dot: 'bg-success' };
		return { label: t('qa.status.closed'), dot: 'bg-base-content/40' };
	};

	const queryString = (entries: [string, string][]) =>
		entries
			.filter(([, value]) => value)
			.map(([key, value]) => `${key}=${encodeURIComponent(value)}`)
			.join('&');

	function applyFilter(patch: Partial<QaFilters>) {
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
			const res = await fetch(`/api/qa?${qs}`);
			if (!res.ok) throw new Error(String(res.status));
			const payload = (await res.json()) as QaListData;
			appended = {
				...payload,
				items: [...(appended?.items ?? []), ...payload.items]
			};
		} catch {
			showToast(t('qa.err.loadMore'), 'error');
		} finally {
			loadingMore = false;
		}
	}

	const exportHref = $derived(
		`/api/qa/export?${queryString([
			['workspaceId', data.workspaceId],
			...(Object.entries(data.filters) as [string, string][])
		])}`
	);

	const SKELETON_WIDTHS = ['62%', '48%', '71%', '40%', '58%', '66%'];
</script>

<svelte:head>
	<title>{t('qa.title')} · {t('brand.name')}</title>
</svelte:head>

<div class="mt-6 flex flex-wrap items-end gap-x-4 gap-y-2">
	<label class="flex flex-col gap-1">
		<span class="text-xs text-muted">{t('qa.filter.status')}</span>
		<select
			class="select select-sm w-36"
			value={data.filters.status}
			onchange={(e) => applyFilter({ status: e.currentTarget.value })}
		>
			<option value="">{t('qa.filter.allStatus')}</option>
			<option value="waiting">{t('qa.status.waiting')}</option>
			<option value="answered">{t('qa.status.answered')}</option>
			<option value="closed">{t('qa.status.closed')}</option>
		</select>
	</label>

	{#if data.isManager && data.groups.length > 0}
		<label class="flex flex-col gap-1">
			<span class="text-xs text-muted">{t('qa.filter.group')}</span>
			<select
				class="select select-sm w-44"
				value={data.filters.group_id}
				onchange={(e) => applyFilter({ group_id: e.currentTarget.value })}
			>
				<option value="">{t('qa.filter.allGroups')}</option>
				{#each data.groups as group (group.id)}
					<option value={group.id}>{group.name}</option>
				{/each}
			</select>
		</label>
	{/if}

	<div class="ml-auto flex items-end gap-1">
		{#if hasFilter}
			<button type="button" class="btn btn-ghost btn-sm" onclick={resetFilters}>
				{t('qa.filter.reset')}
			</button>
		{/if}
		{#if !data.filterRejected}
			<!-- eslint-disable svelte/no-navigation-without-resolve -- endpoint, not a page: resolve() has no entry for /api routes -->
			<a
				href={exportHref}
				title={hasFilter ? t('qa.export.filtered') : t('qa.export.all')}
				class="btn btn-ghost btn-sm gap-1.5"
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
				{t('qa.export.csv')}
			</a>
		{/if}
	</div>
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
		<Alert align="start">{t('qa.err.filter')}</Alert>
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
			<path d="M21 12a8 8 0 0 1-8 8H5a2 2 0 0 1-2-2v-6a8 8 0 1 1 18 0z" />
			<path
				d="M9.5 10.5c0-1.4 1.1-2.5 2.5-2.5s2.5 1.1 2.5 2.5c0 1.2-.9 1.8-1.7 2.3-.5.3-.8.7-.8 1.2"
			/>
			<path d="M12 16.5h.01" />
		</svg>
		<div>
			<p class="text-[0.9375rem] font-medium">
				{hasFilter ? t('qa.noMatch.title') : t('qa.empty.title')}
			</p>
			<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
				{hasFilter ? t('qa.noMatch.body') : t('qa.empty.body')}
			</p>
		</div>
	</div>
{:else}
	<ul class="mt-6 divide-y divide-base-content/8 border-t border-base-content/8">
		{#each items as item (item.id)}
			{@const status = statusMeta(item.status)}
			<li>
				<a
					href={threadHref(item.id)}
					class="flex items-center gap-3 px-1 py-3 transition-colors hover:bg-base-content/[0.045]"
				>
					<div class="min-w-0 flex-1">
						<p class="truncate text-[0.9375rem] font-medium">{item.subject}</p>
						<p class="mt-0.5 font-mono text-xs text-muted tabular-nums">
							#{item.number}{data.isManager ? ` · ${item.group_name}` : ''} · {formatDate(
								item.created_at
							)} · {t('qa.replyCount', {
								n: item.reply_count
							})}
						</p>
					</div>
					<span class="inline-flex flex-none items-center gap-1.5" title={status.label}>
						<span class="h-1.5 w-1.5 rounded-full {status.dot}" aria-hidden="true"></span>
						<span class="text-xs font-medium">{status.label}</span>
					</span>
				</a>
			</li>
		{/each}
	</ul>

	{#if hasFilter}
		<p class="mt-4 text-xs text-muted" aria-live="polite">
			{#if cursor}
				{t('qa.countMore', { n: items.length })}
			{:else if items.length === 1}
				{t('qa.countOne', { n: items.length })}
			{:else}
				{t('qa.count', { n: items.length })}
			{/if}
		</p>
	{/if}

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
					{t('qa.loadingMore')}
				{:else}
					{t('qa.loadMore')}
				{/if}
			</button>
		</div>
	{/if}
{/if}
