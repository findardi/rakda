<script lang="ts">
	import { SvelteSet } from 'svelte/reactivity';
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import type { SubmitFunction } from '@sveltejs/kit';
	import { Alert, Button } from '$lib/components/common';
	import { t } from '$lib/i18n';
	import type { FolderTreeNode } from '$lib/types/content';

	type Props = {
		workspaceId: string;
		/** Form action URL — `?/createArchive` on the overview. */
		action: string;
		/** Controls visibility; the host opens by setting this true. */
		open?: boolean;
		/** Fired after the 202 and the reload; the host shows the toast. */
		onqueued?: () => void;
	};

	let { workspaceId, action, open = $bindable(false), onqueued }: Props = $props();

	let dialog = $state<HTMLDialogElement>();
	let roots = $state<FolderTreeNode[] | null>(null);
	let loading = $state(false);
	let loadError = $state<string | null>(null);
	let formError = $state<string | null>(null);
	let submitting = $state(false);

	// Unchecked ids, not checked ones: "everything" is the zero state, so the set
	// is right the moment the fetch resolves, and "send nothing when all are
	// checked" is simply `excluded.size === 0`.
	const excluded = new SvelteSet<string>();

	const total = $derived(roots?.length ?? 0);
	const checkedCount = $derived(total - excluded.size);
	const all = $derived(excluded.size === 0);
	const none = $derived(total > 0 && checkedCount === 0);

	// `indeterminate` is a property, not an attribute, and we drive it from a
	// derived value — so it is set imperatively.
	let allBox = $state<HTMLInputElement>();
	$effect(() => {
		if (allBox) allBox.indeterminate = !all && !none;
	});

	// Fresh on every open: a folder created in another tab must show up, and the
	// dialog is rare enough that a cached list buys nothing.
	async function load() {
		loading = true;
		loadError = null;
		roots = null;
		try {
			const res = await fetch(
				`/api/content/folders?workspaceId=${encodeURIComponent(workspaceId)}`
			);
			if (!res.ok) throw new Error(String(res.status));
			roots = (await res.json()) as FolderTreeNode[];
		} catch {
			loadError = t('archive.scope.err.load');
		} finally {
			loading = false;
		}
	}

	function reset() {
		excluded.clear();
		formError = null;
		submitting = false;
	}

	// Opening is driven by the bindable `open`; closing flows back through onclose.
	$effect(() => {
		if (open) {
			reset();
			dialog?.showModal();
			void load();
		}
	});

	function toggle(id: string) {
		if (excluded.has(id)) excluded.delete(id);
		else excluded.add(id);
	}

	function toggleAll() {
		if (all) for (const r of roots ?? []) excluded.add(r.id);
		else excluded.clear();
	}

	// Failures (another package still pending, server busy, a folder gone) stay
	// inside the dialog so the reader can adjust and retry in place.
	const submit: SubmitFunction = ({ cancel }) => {
		if (none || !roots) return cancel();
		submitting = true;
		formError = null;
		return async ({ result }) => {
			submitting = false;
			if (result.type === 'success') {
				dialog?.close();
				await invalidateAll();
				onqueued?.();
			} else if (result.type === 'failure') {
				formError = (result.data?.message as string) ?? t('err.generic');
			} else {
				formError = t('err.generic');
			}
		};
	};
</script>

<dialog
	bind:this={dialog}
	class="modal"
	aria-labelledby="archive-scope-title"
	onclose={() => (open = false)}
>
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="archive-scope-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('archive.scope.title')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('archive.scope.desc')}</p>

		<div class="mt-5">
			{#if loading}
				<div class="flex flex-col gap-2" aria-hidden="true">
					{#each [0, 1, 2] as i (i)}
						<div
							class="h-9 animate-pulse rounded-field bg-base-content/5 motion-reduce:animate-none"
						></div>
					{/each}
				</div>
				<p class="sr-only" aria-live="polite">{t('archive.scope.loading')}</p>
			{:else if loadError}
				<Alert align="start">{loadError}</Alert>
			{:else if roots}
				<label class="flex items-center gap-2 border-b border-base-content/10 pb-2 text-sm">
					<input
						bind:this={allBox}
						type="checkbox"
						checked={all}
						onchange={toggleAll}
						class="checkbox checkbox-sm checkbox-primary"
					/>
					<span class="font-medium">{t('archive.scope.all')}</span>
					<span class="ml-auto font-mono text-xs text-muted tabular-nums" aria-live="polite">
						{t('archive.scope.count', { n: checkedCount, total })}
					</span>
				</label>
				<ul class="mt-2 flex max-h-72 flex-col gap-0.5 overflow-y-auto">
					{#each roots as r (r.id)}
						<li>
							<label
								class="flex items-center gap-2 rounded-field px-1 py-1.5 text-sm hover:bg-base-content/[0.04]"
							>
								<input
									type="checkbox"
									checked={!excluded.has(r.id)}
									onchange={() => toggle(r.id)}
									aria-label={t('archive.scope.itemLabel', { name: r.name })}
									class="checkbox checkbox-sm checkbox-primary"
								/>
								<span class="w-6 flex-none font-mono text-xs text-muted tabular-nums"
									>{r.number}</span
								>
								<span class="min-w-0 truncate">{r.name}</span>
								{#if r.is_default}
									<span
										class="flex-none rounded-field border border-base-content/15 px-1.5 text-[0.6875rem] text-muted"
									>
										{t('doc.defaultBadge')}
									</span>
								{/if}
								{#if r.children.length > 0}
									<span
										class="ml-auto flex-none font-mono text-[0.6875rem] text-muted tabular-nums"
									>
										{t(r.children.length === 1 ? 'doc.childCountOne' : 'doc.childCountMany', {
											n: r.children.length
										})}
									</span>
								{/if}
							</label>
						</li>
					{/each}
				</ul>
				{#if !all}
					<p class="mt-3 max-w-prose text-xs text-warning-ink text-pretty">
						{t('archive.scope.noAudit')}
					</p>
				{/if}
			{/if}
		</div>

		{#if formError}
			<div class="mt-4"><Alert align="start">{formError}</Alert></div>
		{/if}

		<form method="POST" {action} use:enhance={submit} class="mt-5 flex flex-wrap justify-end gap-2">
			{#if roots && !all}
				{#each roots.filter((r) => !excluded.has(r.id)) as r (r.id)}
					<input type="hidden" name="folder_ids" value={r.id} />
				{/each}
			{/if}
			<Button type="button" variant="ghost" onclick={() => dialog?.close()}>
				{t('ws.dialog.cancel')}
			</Button>
			<Button type="submit" wrap loading={submitting} disabled={none || loading || !roots}>
				{all
					? t('archive.scope.submitAll')
					: t('archive.scope.submitCount', { n: checkedCount, total })}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('ws.dialog.cancel')}></button>
	</form>
</dialog>
