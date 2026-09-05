<script lang="ts">
	import { applyAction, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tick } from 'svelte';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { showToast } from '$lib/components/common';
	import { formatBytes, formatDateTime } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { RestoreData, TrashFolder, TrashItem } from '$lib/types/content';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const folders = $derived(data.trash.folders);
	const documents = $derived(data.trash.documents);
	const isEmpty = $derived(folders.length === 0 && documents.length === 0);
	const slug = $derived(page.params.slug ?? '');

	const retentionText = $derived(
		data.trash.retention_hours >= 48
			? t('trash.retention.days', { n: Math.floor(data.trash.retention_hours / 24) })
			: t('trash.retention.hours', { n: data.trash.retention_hours })
	);

	let now = $state(Date.now());
	$effect(() => {
		const id = setInterval(() => (now = Date.now()), 60_000);
		return () => clearInterval(id);
	});

	type Tone = 'muted' | 'warning' | 'error';
	const HOUR = 3_600_000;

	function purgeState(purgeAfter: string): { label: string; tone: Tone } {
		const left = Date.parse(purgeAfter) - now;
		if (left <= 0) return { label: t('trash.purge.overdue'), tone: 'error' };
		if (left < HOUR) return { label: t('trash.purge.underHour'), tone: 'error' };
		if (left < 48 * HOUR) {
			return { label: t('trash.purge.hours', { n: Math.ceil(left / HOUR) }), tone: 'warning' };
		}
		return { label: t('trash.purge.days', { n: Math.floor(left / (24 * HOUR)) }), tone: 'muted' };
	}

	const TONE_CLASS: Record<Tone, string> = {
		muted: 'text-muted',
		warning: 'text-warning-ink',
		error: 'text-error'
	};

	function folderContents(item: TrashFolder): string {
		const parts: string[] = [];
		if (item.folder_count > 0) parts.push(t('trash.contains.folders', { n: item.folder_count }));
		if (item.document_count > 0) {
			parts.push(t('trash.contains.documents', { n: item.document_count }));
		}
		return parts.length ? `${parts.join(' · ')} ${t('trash.contains.restoredWith')}` : '';
	}

	let restoringId = $state<string | null>(null);

	type Restored = { kind: 'folder' | 'document'; result: RestoreData };
	let lastRestored = $state<Restored | null>(null);
	let noticeEl = $state<HTMLElement>();

	const restoredHref = $derived.by(() => {
		if (!lastRestored) return null;
		const folderId =
			lastRestored.kind === 'folder' ? lastRestored.result.id : lastRestored.result.folder_id;
		return folderId
			? resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId })
			: resolve('/(app)/workspace/[slug]/document', { slug });
	});

	const restoredMessage = $derived(
		lastRestored
			? t(lastRestored.kind === 'folder' ? 'trash.restored.folder' : 'trash.restored.doc', {
					name: lastRestored.result.name,
					dest: lastRestored.result.folder_name || t('trash.restored.root')
				})
			: ''
	);

	const submitRestore =
		(item: TrashItem, kind: Restored['kind']): SubmitFunction =>
		() => {
			restoringId = item.id;
			return async ({ result }) => {
				restoringId = null;
				if (result.type === 'success') {
					const restored = result.data?.restored as RestoreData | undefined;
					await invalidateAll();
					if (restored) {
						lastRestored = { kind, result: restored };
						await tick();
						noticeEl?.focus();
						showToast(restoredMessage, 'success');
					}
				} else if (result.type === 'failure') {
					showToast((result.data?.message as string) ?? t('err.generic'), 'error');
				} else {
					await applyAction(result);
				}
			};
		};
</script>

<svelte:head><title>{t('trash.title')} · {t('brand.name')}</title></svelte:head>

{#snippet rowMeta(item: TrashItem, origin: string)}
	{@const purge = purgeState(item.purge_after)}
	<p class="mt-0.5 flex flex-wrap items-baseline gap-x-1.5 text-xs text-muted">
		{#if origin}
			<span>{origin}</span>
			<span aria-hidden="true">·</span>
		{/if}
		<span>{t('trash.deletedBy', { who: item.deleted_by_name })}</span>
		<time datetime={item.deleted_at} class="font-mono tabular-nums">
			{t('trash.deletedAt', { when: formatDateTime(item.deleted_at) })}
		</time>
		<span aria-hidden="true">·</span>
		<time datetime={item.purge_after} class="font-mono tabular-nums">
			{t('trash.purge.at', { when: formatDateTime(item.purge_after) })}
		</time>
		<span class="font-medium {TONE_CLASS[purge.tone]}">({purge.label})</span>
	</p>
{/snippet}

{#snippet restoreForm(item: TrashItem, action: string, field: string, kind: Restored['kind'])}
	<form method="POST" {action} use:enhance={submitRestore(item, kind)} class="flex-none">
		<input type="hidden" name={field} value={item.id} />
		<button
			type="submit"
			disabled={restoringId === item.id}
			aria-busy={restoringId === item.id}
			aria-label={t('trash.action.restoreOf', { name: item.name })}
			class="btn btn-ghost btn-sm"
		>
			{#if restoringId === item.id}
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
					<path d="M3 12a9 9 0 1 0 3-6.7" />
					<path d="M3 4v5h5" />
				</svg>
			{/if}
			{t('trash.action.restore')}
		</button>
	</form>
{/snippet}

{#snippet sectionHeading(id: string, label: string, count: number)}
	<h2 {id} class="mb-2 flex items-baseline gap-2 text-sm font-semibold">
		{label}
		<span class="font-mono text-xs font-normal text-muted tabular-nums">{count}</span>
	</h2>
{/snippet}

<div class="mx-auto w-full max-w-4xl px-6 py-8">
	<header>
		<h1 class="text-2xl font-semibold tracking-[-0.02em]">{t('trash.title')}</h1>
		<p class="mt-1.5 max-w-prose text-sm text-muted text-pretty">{retentionText}</p>
	</header>

	{#if lastRestored}
		<div
			bind:this={noticeEl}
			tabindex="-1"
			role="status"
			class="mt-6 flex flex-wrap items-baseline gap-x-3 gap-y-1 rounded-box border border-success/30 bg-success/5 px-4 py-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-primary"
		>
			<span>{restoredMessage}</span>
			{#if lastRestored.result.renamed}
				<span class="text-muted">{t('trash.restored.renamed')}</span>
			{/if}
			{#if restoredHref}
				<a href={restoredHref} class="ml-auto font-medium underline underline-offset-2">
					{t('trash.restored.open')}
				</a>
			{/if}
		</div>
	{/if}

	{#if isEmpty}
		<div
			class="mt-8 flex flex-col items-center justify-center gap-3 rounded-box border border-base-content/10 bg-base-100 px-6 py-16 text-center"
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
				<path d="M3 6h18" />
				<path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
				<path d="M6 6v14a2 2 0 0 0 2 2h8a2 2 0 0 0 2-2V6" />
				<path d="M10 11v6M14 11v6" />
			</svg>
			<div>
				<p class="text-[0.9375rem] font-medium">{t('trash.empty.title')}</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
					{t('trash.empty.body')}
				</p>
			</div>
			<a href={resolve('/(app)/workspace/[slug]/document', { slug })} class="btn btn-ghost btn-sm">
				{t('trash.empty.back')}
			</a>
		</div>
	{:else}
		{#if folders.length > 0}
			<section class="mt-8" aria-labelledby="trash-folders">
				{@render sectionHeading('trash-folders', t('trash.section.folders'), folders.length)}
				<ul
					class="divide-y divide-base-content/6 rounded-box border border-base-content/10 bg-base-100"
				>
					{#each folders as item (item.id)}
						{@const origin = item.parent_gone
							? t('trash.origin.goneFolder')
							: item.parent_name
								? t('trash.origin.folder', { name: item.parent_name })
								: t('trash.origin.root')}
						{@const contents = folderContents(item)}
						<li class="flex items-center gap-2.5 px-4 py-2.5">
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
								<path
									d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"
								/>
							</svg>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm" title={item.name}>
									{item.name}
									{#if contents}
										<span class="ml-1 font-mono text-xs text-muted tabular-nums">{contents}</span>
									{/if}
								</p>
								{@render rowMeta(item, origin)}
							</div>
							{@render restoreForm(item, '?/restoreFolder', 'folderId', 'folder')}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if documents.length > 0}
			<section class="mt-8" aria-labelledby="trash-documents">
				{@render sectionHeading('trash-documents', t('trash.section.documents'), documents.length)}
				<ul
					class="divide-y divide-base-content/6 rounded-box border border-base-content/10 bg-base-100"
				>
					{#each documents as item (item.id)}
						{@const origin = item.folder_gone
							? t('trash.origin.goneDoc')
							: t('trash.origin.folder', { name: item.folder_name })}
						<li class="flex items-center gap-2.5 px-4 py-2.5">
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
								<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
								<path d="M14 3v5h5" />
							</svg>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm" title={item.name}>
									{item.name}
									<span class="ml-1 font-mono text-xs text-muted tabular-nums"
										>{formatBytes(item.size)}</span
									>
								</p>
								{@render rowMeta(item, origin)}
							</div>
							{@render restoreForm(item, '?/restoreDocument', 'documentId', 'document')}
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>
