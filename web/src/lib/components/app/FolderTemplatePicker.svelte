<script lang="ts">
	import { tick } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import { enhance } from '$app/forms';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button, showToast } from '$lib/components/common';
	import { localeState } from '$lib/i18n/locale.svelte';
	import { t, type TKey } from '$lib/i18n';
	import type { FolderTemplateData, FolderTreeNode, TemplateNodeData } from '$lib/types/content';

	type Props = {
		workspaceId: string;
		folders: FolderTreeNode[];
		actionBase: string;
		onApplied?: () => void;
	};
	let { workspaceId, folders, actionBase, onApplied }: Props = $props();

	const HINT_KEY: Record<string, TKey> = {
		'ma-dd': 'tpl.hint.ma-dd',
		fundraising: 'tpl.hint.fundraising',
		property: 'tpl.hint.property',
		audit: 'tpl.hint.audit',
		legal: 'tpl.hint.legal'
	};

	const nodeName = (n: { name_id: string; name_en: string }) =>
		localeState.current === 'en' ? n.name_en : n.name_id;
	const templateDesc = (tpl: FolderTemplateData) =>
		localeState.current === 'en' ? tpl.desc_en : tpl.desc_id;

	let templates = $state<FolderTemplateData[] | null>(null);
	let loading = $state(false);
	let loadError = $state<string | null>(null);

	// Lazy: the list is fetched the first time this component mounts, never in
	// a layout load — static data, rarely used surface.
	$effect(() => {
		if (templates || loading) return;
		loading = true;
		loadError = null;
		fetch(`/api/content/folder-templates?workspaceId=${encodeURIComponent(workspaceId)}`)
			.then(async (res) => {
				if (!res.ok) throw new Error(String(res.status));
				templates = (await res.json()) as FolderTemplateData[];
			})
			.catch(() => {
				loadError = t('tpl.err.load');
			})
			.finally(() => {
				loading = false;
			});
	});

	// list <-> detail; null is the list. `cameFrom` names the card
	// that opened the detail, so leaving it returns focus there.
	let selected = $state<FolderTemplateData | null>(null);
	let listEl = $state<HTMLElement>();
	let backEl = $state<HTMLButtonElement>();
	let cameFrom: string | null = null;

	async function openDetail(tpl: FolderTemplateData) {
		cameFrom = tpl.key;
		selected = tpl;
		applyError = null;
		await tick();
		backEl?.focus();
	}

	async function back() {
		const returnTo = cameFrom;
		cameFrom = null;
		selected = null;
		await tick();
		listEl?.querySelector<HTMLElement>(`[data-tpl="${returnTo}"]`)?.focus();
	}

	// Display-only marker; the server re-decides under the structure lock. The
	// same normalized compare keeps "will create" honest for reused folders.
	const norm = (s: string) => s.trim().toLowerCase();

	function existsTopLevel(node: TemplateNodeData): boolean {
		return folders.some((f) => norm(f.name) === norm(nodeName(node)));
	}

	function existsChild(parent: TemplateNodeData, child: TemplateNodeData): boolean {
		const match = folders.find((f) => norm(f.name) === norm(nodeName(parent)));
		return !!match && match.children.some((c) => norm(c.name) === norm(nodeName(child)));
	}

	// A second template on a structured room is legal (append/merge-down), but
	// it must not look like a first one: the seeded General alone doesn't count.
	const hasStructure = $derived(folders.some((f) => !f.is_default));

	const newCount = $derived.by(() => {
		if (!selected) return 0;
		let count = 0;
		for (const node of selected.folders) {
			const top = existsTopLevel(node);
			if (!top) count++;
			for (const child of node.children ?? []) {
				if (!top || !existsChild(node, child)) count++;
			}
		}
		return count;
	});

	let applySubmitting = $state(false);
	let applyError = $state<string | null>(null);

	const submitApply: SubmitFunction = () => {
		applySubmitting = true;
		return async ({ result }) => {
			applySubmitting = false;
			if (result.type === 'success') {
				const created = (result.data?.created as number) ?? 0;
				const skipped = (result.data?.skipped as number) ?? 0;
				await invalidateAll();
				showToast(t('tpl.applied', { created, skipped }), 'success');
				onApplied?.();
			} else if (result.type === 'failure') {
				applyError = (result.data?.message as string) ?? t('tpl.err.apply');
			} else {
				applyError = t('tpl.err.apply');
			}
		};
	};
</script>

{#if loading}
	<div class="flex flex-col gap-2" aria-hidden="true">
		{#each [0, 1, 2] as i (i)}
			<div
				class="h-16 animate-pulse rounded-box border border-base-content/8 bg-base-content/5 motion-reduce:animate-none"
			></div>
		{/each}
	</div>
	<p class="sr-only" aria-live="polite">{t('tpl.loading')}</p>
{:else if loadError}
	<Alert align="start">{loadError}</Alert>
{:else if selected}
	{@const tpl = selected}
	<div>
		<button
			bind:this={backEl}
			type="button"
			onclick={back}
			class="inline-flex items-center gap-1.5 text-sm text-muted transition-colors hover:text-base-content"
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
				<path d="m15 18-6-6 6-6" />
			</svg>
			{t('tpl.detail.back')}
		</button>

		<div class="mt-3">
			<p class="text-[0.9375rem] font-medium">{nodeName(tpl)}</p>
			<p class="mt-0.5 text-sm text-muted text-pretty">{templateDesc(tpl)}</p>
		</div>

		{#if hasStructure}
			<p class="mt-3 max-w-prose text-xs text-warning-ink text-pretty">
				{t('tpl.detail.hasStructure')}
			</p>
		{/if}

		<p class="mt-4 text-xs font-medium text-muted">{t('tpl.detail.structure')}</p>
		<ul class="mt-2 flex max-h-72 flex-col gap-1 overflow-y-auto text-sm">
			{#each tpl.folders as node (node.name_id)}
				{@const topExists = existsTopLevel(node)}
				<li>
					<span class="flex items-baseline gap-2">
						<svg
							class="h-3.5 w-3.5 flex-none self-center text-muted"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.8"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
						</svg>
						<span class="min-w-0 truncate">{nodeName(node)}</span>
						{#if topExists}
							<span class="flex-none text-[0.6875rem] text-muted">{t('tpl.detail.existing')}</span>
						{/if}
					</span>
					{#if node.children?.length}
						<ul class="mt-1 flex flex-col gap-1 pl-6">
							{#each node.children as child (child.name_id)}
								<li class="flex items-baseline gap-2">
									<svg
										class="h-3 w-3 flex-none self-center text-muted/70"
										viewBox="0 0 24 24"
										fill="none"
										stroke="currentColor"
										stroke-width="1.8"
										stroke-linecap="round"
										stroke-linejoin="round"
										aria-hidden="true"
									>
										<path
											d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"
										/>
									</svg>
									<span class="min-w-0 truncate text-[0.8125rem]">{nodeName(child)}</span>
									{#if existsChild(node, child)}
										<span class="flex-none text-[0.6875rem] text-muted"
											>{t('tpl.detail.existing')}</span
										>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}
				</li>
			{/each}
		</ul>

		{#if HINT_KEY[tpl.key]}
			<p class="mt-4 text-xs text-muted text-pretty">{t(HINT_KEY[tpl.key])}</p>
		{/if}

		{#if applyError}
			<div class="mt-4"><Alert align="start">{applyError}</Alert></div>
		{/if}

		<form method="POST" action="{actionBase}?/applyTemplate" use:enhance={submitApply}>
			<input type="hidden" name="template" value={tpl.key} />
			<div class="mt-4 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={back}>{t('tpl.detail.back')}</Button>
				<Button type="submit" wrap loading={applySubmitting}>
					{applySubmitting ? t('tpl.applying') : t('tpl.applyCount', { n: newCount })}
				</Button>
			</div>
		</form>
	</div>
{:else if templates}
	<ul bind:this={listEl} class="flex flex-col gap-2">
		{#each templates as tpl (tpl.key)}
			<li>
				<button
					type="button"
					data-tpl={tpl.key}
					onclick={() => openDetail(tpl)}
					class="w-full rounded-box border border-base-content/10 bg-base-100 px-3.5 py-3 text-left transition-colors hover:border-base-content/25 hover:bg-base-content/[0.03]"
				>
					<span class="flex items-baseline justify-between gap-2">
						<span class="min-w-0 truncate text-sm font-medium">{nodeName(tpl)}</span>
						<span class="flex-none font-mono text-xs text-muted tabular-nums"
							>{t('tpl.folderCount', { n: tpl.folder_count })}</span
						>
					</span>
					<span class="mt-1 line-clamp-2 block text-xs text-muted text-pretty"
						>{templateDesc(tpl)}</span
					>
				</button>
			</li>
		{/each}
	</ul>
{/if}
