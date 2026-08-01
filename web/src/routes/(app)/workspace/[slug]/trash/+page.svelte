<script lang="ts">
	import { applyAction, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Toaster, showToast } from '$lib/components/common';
	import { formatBytes, formatDateTime } from '$lib/format';
	import { t } from '$lib/i18n';
	import type { TrashFolder } from '$lib/types/content';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const folders = $derived(data.trash.folders);
	const documents = $derived(data.trash.documents);
	const isEmpty = $derived(folders.length === 0 && documents.length === 0);

	let restoringId = $state<string | null>(null);

	const hoursLeft = (purgeAfter: string) =>
		Math.max(0, Math.ceil((Date.parse(purgeAfter) - Date.now()) / 3_600_000));

	const submitRestore =
		(item: TrashFolder, toastKey: 'trash.folderRestored' | 'trash.docRestored'): SubmitFunction =>
		() => {
			restoringId = item.id;
			return async ({ result }) => {
				restoringId = null;
				if (result.type === 'success') {
					await invalidateAll();
					showToast(t(toastKey, { name: item.name }), 'success');
				} else if (result.type === 'failure') {
					showToast((result.data?.message as string) ?? t('err.generic'), 'error');
				} else {
					await applyAction(result);
				}
			};
		};
</script>

<svelte:head><title>{t('trash.title')} · {t('brand.name')}</title></svelte:head>

{#snippet rowMeta(item: TrashFolder)}
	<span class="hidden flex-none text-xs text-muted lg:inline">
		{t('trash.deletedBy', { who: item.deleted_by_name })}
	</span>
	<time
		datetime={item.deleted_at}
		title={t('trash.deletedAtTitle', { when: formatDateTime(item.deleted_at) })}
		class="hidden flex-none font-mono text-xs text-muted tabular-nums sm:inline"
	>
		{formatDateTime(item.deleted_at)}
	</time>
	<span
		class="flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 font-mono text-[0.6875rem] text-muted tabular-nums"
		title={t('trash.purgeInTitle', { when: formatDateTime(item.purge_after) })}
	>
		{t('trash.purgeIn', { n: hoursLeft(item.purge_after) })}
	</span>
{/snippet}

{#snippet restoreForm(
	item: TrashFolder,
	action: string,
	field: string,
	toastKey: 'trash.folderRestored' | 'trash.docRestored'
)}
	<form method="POST" {action} use:enhance={submitRestore(item, toastKey)} class="flex-none">
		<input type="hidden" name={field} value={item.id} />
		<button
			type="submit"
			disabled={restoringId !== null}
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

<div class="mx-auto w-full max-w-4xl px-6 py-8">
	<header>
		<h1 class="text-2xl font-semibold tracking-[-0.02em]">{t('trash.title')}</h1>
		<p class="mt-1.5 text-sm text-muted">{t('trash.desc')}</p>
	</header>

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
		</div>
	{:else}
		{#if folders.length > 0}
			<section class="mt-8" aria-labelledby="trash-folders">
				<h2
					id="trash-folders"
					class="mb-2 text-[0.6875rem] font-semibold tracking-wide text-muted uppercase"
				>
					{t('trash.section.folders')}
				</h2>
				<ul
					class="divide-y divide-base-content/6 rounded-box border border-base-content/10 bg-base-100"
				>
					{#each folders as item (item.id)}
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
							<span class="min-w-0 flex-1 truncate text-sm" title={item.name}>{item.name}</span>
							{@render rowMeta(item)}
							{@render restoreForm(item, '?/restoreFolder', 'folderId', 'trash.folderRestored')}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if documents.length > 0}
			<section class="mt-8" aria-labelledby="trash-documents">
				<h2
					id="trash-documents"
					class="mb-2 text-[0.6875rem] font-semibold tracking-wide text-muted uppercase"
				>
					{t('trash.section.documents')}
				</h2>
				<ul
					class="divide-y divide-base-content/6 rounded-box border border-base-content/10 bg-base-100"
				>
					{#each documents as item (item.id)}
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
							<span class="min-w-0 flex-1 truncate text-sm" title={item.name}>{item.name}</span>
							<span
								class="hidden w-20 flex-none text-right font-mono text-xs text-muted tabular-nums md:inline"
							>
								{formatBytes(item.size)}
							</span>
							{@render rowMeta(item)}
							{@render restoreForm(item, '?/restoreDocument', 'documentId', 'trash.docRestored')}
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>

<Toaster />
