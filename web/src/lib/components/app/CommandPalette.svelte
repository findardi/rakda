<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { t } from '$lib/i18n';
	import type { SearchContentHit, SearchData } from '$lib/types/content';

	type Props = {
		open: boolean;
		workspaceId: string;
		slug: string;
		onclose: () => void;
	};

	let { open, workspaceId, slug, onclose }: Props = $props();

	let dialog = $state<HTMLDialogElement>();
	let input = $state<HTMLInputElement>();
	let query = $state('');
	let results = $state<SearchData | null>(null);
	let loading = $state(false);
	let failed = $state(false);
	let selected = $state(0);
	let controller: AbortController | null = null;

	type Item = {
		kind: 'folder' | 'document' | 'hit';
		id: string;
		name: string;
		folderId: string;
		breadcrumb: string;
		pageCount?: number;
		hitCount?: number;
	};

	const flat = $derived.by(() => {
		const list: Item[] = [];
		const order: ('folder' | 'document' | 'hit')[] = [];
		for (const f of results?.folders ?? []) {
			list.push({ kind: 'folder', id: f.id, name: f.name, folderId: '', breadcrumb: f.breadcrumb });
			if (order[order.length - 1] !== 'folder') order.push('folder');
		}
		for (const d of results?.documents ?? []) {
			list.push({
				kind: 'document',
				id: d.id,
				name: d.name,
				folderId: d.folder_id,
				breadcrumb: d.breadcrumb
			});
			if (order[order.length - 1] !== 'document') order.push('document');
		}
		for (const h of results?.content ?? []) {
			list.push({
				kind: 'hit',
				id: h.document_id,
				name: h.document_name,
				folderId: h.folder_id,
				breadcrumb: h.breadcrumb,
				pageCount: h.page_count,
				hitCount: h.hit_count
			});
			if (order[order.length - 1] !== 'hit') order.push('hit');
		}
		return list;
	});

	// Sync the native dialog with the controlled prop.
	$effect(() => {
		if (open && dialog && !dialog.open) {
			dialog.showModal();
			query = '';
			results = null;
			failed = false;
			selected = 0;
			input?.focus();
		}
	});

	// Debounced search: ≥2 characters, stale requests aborted.
	$effect(() => {
		const q = query.trim();
		if (!open) return;
		if (q.length < 2) {
			results = null;
			loading = false;
			failed = false;
			return;
		}

		loading = true;
		failed = false;
		controller?.abort();
		controller = new AbortController();

		const timer = setTimeout(async () => {
			const params = `workspaceId=${encodeURIComponent(workspaceId)}&q=${encodeURIComponent(q)}`;
			try {
				const res = await fetch(`/api/search?${params}`, { signal: controller?.signal });
				if (!res.ok) {
					failed = true;
					return;
				}
				results = (await res.json()) as SearchData;
				selected = 0;
			} catch (err) {
				if (err instanceof DOMException && err.name === 'AbortError') return;
				failed = true;
			} finally {
				loading = false;
			}
		}, 200);

		return () => {
			clearTimeout(timer);
			controller?.abort();
		};
	});

	// Keep selection inside the list as results change.
	$effect(() => {
		if (selected >= flat.length) selected = Math.max(0, flat.length - 1);
	});

	// Committing a search is audited server-side (even a result-less one);
	// the GET stays side-effect free, so this fires before navigation.
	function commitSearch(query: string) {
		const q = query.trim();
		if (q.length < 2) return;
		fetch('/api/search/log', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ workspaceId, query: q })
		}).catch(() => {});
	}

	function openItem(item: Item) {
		commitSearch(query);
		// Close before navigating: `onclose` clears the controlled `open`
		// state, so the palette never survives a result pick.
		dialog?.close();
		if (item.kind === 'folder') {
			goto(resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId: item.id }));
			return;
		}

		// Content hits stay compact here — the results page owns the L2 pages
		// detail (keputusan 9-d: "⌘K ringkas + halaman hasil").
		if (item.kind === 'hit') {
			goto(
				`${resolve('/(app)/workspace/[slug]/search', { slug })}?q=${encodeURIComponent(query.trim())}`
			);
			return;
		}

		goto(
			resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
				slug,
				folderId: item.folderId,
				documentId: item.id
			})
		);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selected = (selected + 1) % Math.max(flat.length, 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			selected = (selected - 1 + Math.max(flat.length, 1)) % Math.max(flat.length, 1);
		} else if (e.key === 'Escape') {
			// The native dialog still closes on Escape; stop the event from
			// reaching page-level handlers (e.g. the viewer's "Escape leaves
			// the reader" shortcut) while the palette is open.
			e.stopPropagation();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (flat[selected]) {
				openItem(flat[selected]);
			} else {
				// Enter with no selection still counts as a committed search.
				commitSearch(query);
			}
		}
	}
</script>

<dialog
	bind:this={dialog}
	{onclose}
	onkeydown={onKeydown}
	class="modal modal-top"
	aria-label={t('app.search.title')}
>
	<div
		class="mt-14 w-[calc(100%-1.5rem)] max-w-xl justify-self-center rounded-box border border-base-content/10 bg-base-100 shadow-lg"
	>
		<!-- Input row -->
		<div class="flex items-center gap-3 border-b border-base-content/10 px-4">
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
				<circle cx="11" cy="11" r="7" />
				<path d="m21 21-4.3-4.3" />
			</svg>
			<input
				bind:this={input}
				bind:value={query}
				type="search"
				placeholder={t('app.search.placeholder')}
				autocomplete="off"
				spellcheck="false"
				role="combobox"
				aria-expanded="true"
				aria-controls="search-results"
				aria-activedescendant={flat[selected] ? `search-item-${selected}` : undefined}
				class="h-12 w-full bg-transparent text-sm outline-none placeholder:text-muted"
			/>
			<kbd
				class="hidden rounded border border-base-content/15 bg-base-200 px-1.5 py-0.5 font-mono text-[0.6875rem] text-muted sm:block"
				>Esc</kbd
			>
		</div>

		<div id="search-results" role="listbox" class="max-h-[50vh] overflow-y-auto p-2">
			{#if loading}
				<div class="px-3 py-4 text-sm text-muted" aria-live="polite">
					{t('app.search.loading')}
				</div>
			{:else if failed}
				<p class="px-3 py-4 text-sm text-error" role="alert">{t('app.search.error')}</p>
			{:else if query.trim().length < 2}
				<p class="px-3 py-4 text-sm text-muted">{t('app.search.hint')}</p>
			{:else if flat.length === 0}
				<div class="px-3 py-5">
					<p class="text-sm text-base-content">{t('app.search.none', { q: query.trim() })}</p>
					<p class="mt-1 text-xs text-muted">{t('app.search.noneHint')}</p>
				</div>
			{:else}
				<ul class="flex flex-col gap-0.5">
					{#each flat as item, i (item.kind + item.id)}
						{#if i === 0 || flat[i - 1].kind !== item.kind}
							<li
								role="presentation"
								class="px-3 pt-3 pb-1 text-xs font-medium text-muted first:pt-1"
							>
								{item.kind === 'folder'
									? t('app.search.folders')
									: item.kind === 'document'
										? t('app.search.documents')
										: t('app.search.content')}
							</li>
						{/if}
						<li role="option" id="search-item-{i}" aria-selected={i === selected}>
							<button
								type="button"
								onclick={() => openItem(item)}
								onmouseenter={() => (selected = i)}
								class="flex w-full items-center gap-3 rounded-field px-3 py-2 text-left {i ===
								selected
									? 'bg-base-content/5'
									: 'hover:bg-base-content/5'}"
							>
								<span
									class="grid h-7 w-7 flex-none place-items-center rounded-field border border-base-content/10 text-muted"
									aria-hidden="true"
								>
									{#if item.kind === 'folder'}
										<svg
											class="h-3.5 w-3.5"
											viewBox="0 0 24 24"
											fill="none"
											stroke="currentColor"
											stroke-width="1.6"
											stroke-linecap="round"
											stroke-linejoin="round"
										>
											<path
												d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"
											/>
										</svg>
									{:else}
										<svg
											class="h-3.5 w-3.5"
											viewBox="0 0 24 24"
											fill="none"
											stroke="currentColor"
											stroke-width="1.6"
											stroke-linecap="round"
											stroke-linejoin="round"
										>
											<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
											<path d="M14 3v5h5" />
										</svg>
									{/if}
								</span>
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm text-base-content">{item.name}</span>
									{#if item.breadcrumb || item.kind === 'hit'}
										<span class="block truncate text-xs text-muted">
											{#if item.breadcrumb}{item.breadcrumb}{/if}
											{#if item.kind === 'hit'}
												{item.breadcrumb ? ' · ' : ''}
												{t('app.search.hits', { n: item.hitCount ?? 0 })}
												{#if (item.pageCount ?? 0) > 750}
													{' · ' + t('app.search.tooLarge')}
												{/if}
											{/if}
										</span>
									{/if}
								</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<div
			class="flex items-center gap-3 border-t border-base-content/10 px-4 py-2 text-[0.6875rem] text-muted"
		>
			<span>↑↓ {t('app.search.navigate')}</span>
			<span aria-hidden="true">·</span>
			<span>↵ {t('app.search.enter')}</span>
			<span class="ml-auto hidden sm:inline">{t('app.search.onlyVisible')}</span>
		</div>
	</div>
</dialog>
