<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import type { SearchContentPage, SearchData } from '$lib/types/content';
	import type { WorkspaceData } from '$lib/types/workspace';

	const workspace = $derived((page.data as { workspace: WorkspaceData }).workspace);
	const slug = $derived(page.params.slug!);

	const urlQuery = $derived(page.url.searchParams.get('q') ?? '');
	let input = $state('');
	// The query actually being searched — follows the live input.
	const activeQuery = $derived(input.trim());
	let results = $state<SearchData | null>(null);
	let loading = $state(false);
	let failed = $state(false);
	let controller: AbortController | null = null;

	// URL changes only on commit (palette click or Enter); typed input must
	// never be overwritten by it. Track the last URL value we applied so the
	// sync fires on external navigation, not on keystrokes.
	let syncedQuery = $state('');
	$effect(() => {
		if (syncedQuery === urlQuery) return;
		syncedQuery = urlQuery;
		input = urlQuery;
	});

	// Expanded content hit (L1 → L2). One at a time, keyed by document id.
	let expandedId = $state<string | null>(null);
	let expandedLoading = $state(false);
	let expandedPages = $state<SearchContentPage[] | null>(null);
	let expandedError = $state(false);

	$effect(() => {
		const q = activeQuery;
		if (q.length < 2) {
			results = null;
			loading = false;
			failed = false;
			expandedId = null;
			expandedPages = null;
			return;
		}

		loading = true;
		failed = false;
		controller?.abort();
		controller = new AbortController();

		const timer = setTimeout(async () => {
			const params = `workspaceId=${encodeURIComponent(workspace.id)}&q=${encodeURIComponent(q)}`;
			try {
				const res = await fetch(`/api/search?${params}`, { signal: controller?.signal });
				if (!res.ok) {
					failed = true;
					return;
				}
				results = (await res.json()) as SearchData;
				expandedId = null;
				expandedPages = null;
			} catch (err) {
				if (err instanceof DOMException && err.name === 'AbortError') return;
				failed = true;
			} finally {
				loading = false;
			}
		}, 250);

		return () => {
			clearTimeout(timer);
			controller?.abort();
		};
	});

	function commitInput() {
		const q = activeQuery;
		if (q.length < 2 || q === urlQuery) return;
		logSearch(q);
		goto(`${resolve('/(app)/workspace/[slug]/search', { slug })}?q=${encodeURIComponent(q)}`);
	}

	function logSearch(q: string) {
		fetch('/api/search/log', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ workspaceId: workspace.id, query: q })
		}).catch(() => {});
	}

	async function toggleExpand(documentId: string, q: string) {
		if (expandedId === documentId) {
			expandedId = null;
			expandedPages = null;
			return;
		}

		expandedId = documentId;
		expandedPages = null;
		expandedError = false;
		expandedLoading = true;

		try {
			const params = `workspaceId=${encodeURIComponent(workspace.id)}&documentId=${encodeURIComponent(documentId)}&q=${encodeURIComponent(q)}`;
			const res = await fetch(`/api/search/content/pages?${params}`);
			if (!res.ok) throw new Error('pages failed');
			const data = (await res.json()) as { pages: SearchContentPage[] };
			expandedPages = data.pages;
		} catch {
			expandedError = true;
			expandedPages = null;
		} finally {
			expandedLoading = false;
		}
	}

	function openPage(documentId: string, folderId: string, pageNo: number) {
		// Opening a hit is a commit: the keyword lands in the audit log.
		if (activeQuery.length >= 2) logSearch(activeQuery);
		const viewer = resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
			slug,
			folderId,
			documentId
		});
		goto(`${viewer}?page=${pageNo}`);
	}
</script>

<svelte:head>
	<title>{t('brand.name')} · {t('app.search.title')}</title>
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6">
	<h1 class="text-xl font-semibold tracking-[-0.01em]">{t('app.search.title')}</h1>

	<form
		class="mt-4 flex items-center gap-2 rounded-field border border-base-content/10 bg-base-100 px-3 focus-within:border-primary/40"
		onsubmit={(e) => {
			e.preventDefault();
			commitInput();
		}}
	>
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
			bind:value={input}
			type="search"
			placeholder={t('app.search.placeholder')}
			autocomplete="off"
			spellcheck="false"
			class="h-12 w-full bg-transparent text-sm outline-none placeholder:text-muted"
		/>
	</form>

	{#if activeQuery.length >= 2}
		<p class="mt-6 text-sm text-muted">{t('app.search.resultTitle', { q: activeQuery })}</p>
	{/if}

	<div class="mt-3">
		{#if loading}
			<p class="px-1 py-6 text-sm text-muted" aria-live="polite">{t('app.search.loading')}</p>
		{:else if failed}
			<p class="px-1 py-6 text-sm text-error" role="alert">{t('app.search.error')}</p>
		{:else if activeQuery.length < 2}
			<p class="px-1 py-6 text-sm text-muted">{t('app.search.hint')}</p>
		{:else if results && results.folders.length === 0 && results.documents.length === 0 && results.content.length === 0}
			<div class="px-1 py-8">
				<p class="text-sm text-base-content">{t('app.search.none', { q: activeQuery })}</p>
				<p class="mt-1 text-xs text-muted">{t('app.search.noneHint')}</p>
			</div>
		{:else if results}
			{#if results.folders.length > 0}
				<section aria-label={t('app.search.folders')}>
					<h2 class="px-1 pt-2 pb-1 text-xs font-medium text-muted">
						{t('app.search.folders')}
					</h2>
					<ul class="flex flex-col">
						{#each results.folders as f}
							<li>
								<a
									href={resolve('/(app)/workspace/[slug]/document/[folderId]', {
										slug,
										folderId: f.id
									})}
									class="flex items-center gap-3 rounded-field px-2 py-2 hover:bg-base-content/5"
								>
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm">{f.name}</span>
										{#if f.breadcrumb}
											<span class="block truncate text-xs text-muted">{f.breadcrumb}</span>
										{/if}
									</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if results.documents.length > 0}
				<section aria-label={t('app.search.documents')}>
					<h2 class="px-1 pt-4 pb-1 text-xs font-medium text-muted">
						{t('app.search.documents')}
					</h2>
					<ul class="flex flex-col">
						{#each results.documents as d}
							<li>
								<a
									href={resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
										slug,
										folderId: d.folder_id,
										documentId: d.id
									})}
									class="flex items-center gap-3 rounded-field px-2 py-2 hover:bg-base-content/5"
								>
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm">{d.name}</span>
										{#if d.breadcrumb}
											<span class="block truncate text-xs text-muted">{d.breadcrumb}</span>
										{/if}
									</span>
								</a>
							</li>
						{/each}
					</ul>
				</section>
			{/if}

			{#if results.content.length > 0}
				<section aria-label={t('app.search.content')}>
					<h2 class="px-1 pt-4 pb-1 text-xs font-medium text-muted">
						{t('app.search.content')}
					</h2>
					<ul class="flex flex-col">
						{#each results.content as h}
							<li>
								<button
									type="button"
									onclick={() => toggleExpand(h.document_id, activeQuery)}
									aria-expanded={expandedId === h.document_id}
									class="flex w-full items-center gap-3 rounded-field px-2 py-2 text-left hover:bg-base-content/5"
								>
									<span
										class="grid h-7 w-7 flex-none place-items-center rounded-field border border-base-content/10 text-muted"
										aria-hidden="true"
									>
										<svg
											class="h-3.5 w-3.5 transition-transform duration-150 motion-reduce:transition-none {expandedId ===
											h.document_id
												? 'rotate-90'
												: ''}"
											viewBox="0 0 24 24"
											fill="none"
											stroke="currentColor"
											stroke-width="1.6"
											stroke-linecap="round"
											stroke-linejoin="round"
										>
											<path d="m9 6 6 6-6 6" />
										</svg>
									</span>
									<span class="min-w-0 flex-1">
										<span class="block truncate text-sm">{h.document_name}</span>
										<span class="block truncate text-xs text-muted">
											{h.breadcrumb ? h.breadcrumb + ' · ' : ''}
											{t('app.search.hits', { n: h.hit_count })}
											{#if h.page_count > 750}
												{' · ' + t('app.search.tooLarge')}
											{/if}
										</span>
									</span>
								</button>

								{#if expandedId === h.document_id}
									<ul class="ml-9 mt-1 flex flex-col gap-0.5 border-l border-base-content/10 pl-3">
										{#if expandedLoading}
											<li class="px-2 py-2 text-xs text-muted">{t('app.search.loading')}</li>
										{:else if expandedError}
											<li class="px-2 py-2 text-xs text-error" role="alert">
												{t('app.search.error')}
											</li>
										{:else if expandedPages && expandedPages.length === 0}
											<li class="px-2 py-2 text-xs text-muted">
												{t('app.search.none', { q: activeQuery })}
											</li>
										{:else if expandedPages}
											{#each expandedPages as p}
												<li>
													<button
														type="button"
														onclick={() => openPage(h.document_id, h.folder_id, p.page_no)}
														class="w-full rounded-field px-2 py-2 text-left hover:bg-base-content/5"
													>
														<span class="flex items-baseline gap-2">
															<span class="flex-none text-xs font-medium text-muted">
																{t('app.search.pageOf', { n: p.page_no })}
															</span>
															<span
																class="line-clamp-2 min-w-0 whitespace-pre-line text-xs text-base-content"
																>{p.snippet}</span
															>
														</span>
													</button>
												</li>
											{/each}
										{/if}
									</ul>
								{/if}
							</li>
						{/each}
					</ul>
				</section>
			{/if}
		{/if}
	</div>
</div>
