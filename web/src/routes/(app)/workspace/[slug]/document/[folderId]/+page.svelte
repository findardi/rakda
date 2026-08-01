<script lang="ts">
	import { applyAction, deserialize, enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { navigating, page } from '$app/state';
	import type { ActionResult, SubmitFunction } from '@sveltejs/kit';
	import { normalizeRole } from '$lib/access/roles';
	import { DocumentVersions } from '$lib/components/app';
	import { Alert, Button, showToast } from '$lib/components/common';
	import { DOCUMENT_MIME, filesFrom, treeFromInput } from '$lib/dnd';
	import { formatBytes, formatDate, formatDateTime } from '$lib/format';
	import { t } from '$lib/i18n';
	import { findNode } from '$lib/tree';
	import type { DocumentData, FolderTreeNode } from '$lib/types/content';
	import type { MyAccessWorkspace } from '$lib/types/workspace';
	import { uploadQueue } from '$lib/upload/queue.svelte';
	import { uploadTree } from '$lib/upload/tree';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const folders = $derived(data.folders);
	const workspace = $derived(data.workspace);
	const slug = $derived(page.params.slug!);
	const folderId = $derived(page.params.folderId!);

	// 'manual' is the server's own order (documents.position) and the only mode
	// where dragging a row into place means anything — a manual position under a
	// name sort would be invisible the moment it was set.
	type SortKey = 'manual' | 'name' | 'updated' | 'size';
	let sortBy = $state<SortKey>('manual');

	const SKELETON_ROWS = [46, 32, 58, 38, 51, 29];

	const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
	const documents = $derived.by(() => {
		const list = [...data.documents];
		if (sortBy === 'updated') {
			return list.sort((a, b) => b.updated_at.localeCompare(a.updated_at));
		}
		if (sortBy === 'size') return list.sort((a, b) => b.size - a.size);
		if (sortBy === 'name') return list.sort((a, b) => collator.compare(a.name, b.name));
		return list;
	});

	const forbidden = $derived(data.forbidden ?? false);

	const access = $derived((page.data as { access?: MyAccessWorkspace }).access);
	const perms = $derived(access?.permissions ?? []);
	const role = $derived(normalizeRole(access?.role ?? ''));
	const canUpload = $derived(perms.includes('document:upload') && !forbidden);
	const canDownload = $derived(perms.includes('document:download') && role !== 'guest');
	// Picking a folder creates subfolders, so it needs more than upload rights.
	const canCreate = $derived(perms.includes('folder:create') && !forbidden);

	let downloadingId = $state<string | null>(null);

	async function download(doc: DocumentData) {
		downloadingId = doc.id;
		try {
			const q = new URLSearchParams({ workspaceId: workspace.id, documentId: doc.id });
			const res = await fetch(`/api/content/download?${q}`);
			if (res.status === 403) {
				showToast(t('doc.docs.err.forbiddenDownload'), 'error');
				return;
			}
			if (!res.ok) {
				showToast(t('err.generic'), 'error');
				return;
			}
			const { download_url } = (await res.json()) as { download_url: string };
			window.location.href = download_url;
		} catch {
			showToast(t('err.network'), 'error');
		} finally {
			downloadingId = null;
		}
	}
	const canDelete = $derived(perms.includes('document:delete'));
	const canEditDoc = $derived(perms.includes('document:edit'));
	// Version history is owner/admin upstream regardless of `document:view`, so a
	// guest never sees the disclosure rather than opening it into a 403.
	const canSeeVersions = $derived(role === 'owner' || role === 'admin');

	// One row's history open at a time: two panels of near-identical numbers
	// invite comparing the wrong pair.
	let openVersions = $state<string | null>(null);
	const toggleVersions = (id: string) => (openVersions = openVersions === id ? null : id);

	const ROLE_KEY = {
		owner: 'role.sys.owner',
		admin: 'role.sys.admin',
		guest: 'role.sys.guest'
	} as const;
	const roleLabel = $derived(t(ROLE_KEY[role]));

	const folder = $derived(findNode(folders, folderId));

	// The load blocks on the server, so the outgoing folder's list would sit
	// frozen on screen until the new one lands. Show the shape instead, and name
	// the folder being opened rather than the one being left.
	const targetId = $derived(navigating.to?.params?.folderId ?? folderId);
	const switching = $derived(targetId !== folderId);
	const shownFolder = $derived(switching ? findNode(folders, targetId) : folder);

	let fileInput = $state<HTMLInputElement>();
	let folderInput = $state<HTMLInputElement>();
	let paneTargeted = $state(false);

	function enqueue(files: File[]) {
		if (!canUpload || !files.length || !folder) return;
		uploadQueue.enqueue(workspace.id, folder.id, folder.name, files);
	}

	function paneDragOver(e: DragEvent) {
		if (!canUpload || !e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		e.dataTransfer.dropEffect = 'copy';
		paneTargeted = true;
	}

	function paneDragLeave(e: DragEvent) {
		const next = e.relatedTarget as Node | null;
		if (next && (e.currentTarget as Element).contains(next)) return;
		paneTargeted = false;
	}

	function paneDrop(e: DragEvent) {
		if (!canUpload || !e.dataTransfer?.types.includes('Files')) return;
		e.preventDefault();
		paneTargeted = false;
		enqueue(filesFrom(e.dataTransfer));
	}

	function onPick(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		enqueue(Array.from(input.files ?? []));
		input.value = '';
	}

	async function onPickFolder(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const picked = Array.from(input.files ?? []);
		input.value = '';
		if (!canUpload || !folder || !picked.length) return;

		try {
			const tree = treeFromInput(picked);
			const created = tree.folders.length;
			// Picking from inside a folder: subfolders go under it, loose files in it.
			await uploadTree(
				workspace.id,
				{ parentId: folder.id, loose: { id: folder.id, name: folder.name } },
				tree
			);
			if (created) {
				await invalidateAll();
				showToast(t('doc.drop.created', { n: created }), 'success');
			}
		} catch (err) {
			showToast(err instanceof Error ? err.message : t('doc.drop.err.read'), 'error');
		}
	}

	// --- drag out (the tree in the layout owns the drop targets) ---
	let draggingDocId = $state<string | null>(null);

	function docDragStart(e: DragEvent, doc: DocumentData) {
		if (!canEditDoc) {
			e.preventDefault();
			return;
		}
		draggingDocId = doc.id;
		e.dataTransfer?.setData(DOCUMENT_MIME, doc.id);
		if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
	}

	// --- reorder within this folder ---------------------------------------
	// The list is rendered straight from the server, which orders by
	// `documents.position`, so in manual mode a row's array index IS its
	// position. `insertAt` is the gap the row would land in: 0..length.

	const canReorder = $derived(canEditDoc && sortBy === 'manual' && !switching);

	let insertAt = $state<number | null>(null);
	// The row a reorder is in flight for; the list reloads wholesale afterwards.
	let reorderingId = $state<string | null>(null);

	// Server-side moves take a per-workspace advisory lock, so overlapping
	// requests only queue on the database. Chaining keeps them ordered.
	let moveChain: Promise<unknown> = Promise.resolve();
	const enqueueMove = (run: () => Promise<unknown>) => {
		moveChain = moveChain.then(run, run);
	};

	const insertLabel = $derived.by(() => {
		if (insertAt === null) return null;
		const before = documents[insertAt];
		if (before) return t('doc.docs.reorder.before', { name: before.name });
		const last = documents[documents.length - 1];
		return last ? t('doc.docs.reorder.after', { name: last.name }) : null;
	});

	function endDocDrag() {
		draggingDocId = null;
		insertAt = null;
	}

	function docDragOver(e: DragEvent, i: number) {
		if (!canReorder || !draggingDocId) return;
		if (!e.dataTransfer?.types.includes(DOCUMENT_MIME)) return;
		// The pane below treats a drag as an upload; a row reorder is not that.
		e.preventDefault();
		e.stopPropagation();
		e.dataTransfer.dropEffect = 'move';

		const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const at = e.clientY - rect.top < rect.height / 2 ? i : i + 1;
		const from = documents.findIndex((d) => d.id === draggingDocId);
		// Both gaps touching the dragged row put it back where it started.
		insertAt = at === from || at === from + 1 ? null : at;
	}

	// The gap was already resolved by the dragover that necessarily preceded this.
	function docDrop(e: DragEvent) {
		if (!canReorder || !draggingDocId) return;
		if (!e.dataTransfer?.types.includes(DOCUMENT_MIME)) return;
		e.preventDefault();
		e.stopPropagation();

		const at = insertAt;
		const id = draggingDocId;
		endDocDrag();
		if (at === null) return;
		enqueueMove(() => reorderDoc(id, at));
	}

	function docKeydown(e: KeyboardEvent, i: number) {
		if (!e.altKey || e.ctrlKey || e.metaKey) return;
		if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
		if (!canReorder) return;
		const dir = e.key === 'ArrowUp' ? -1 : 1;
		const j = i + dir;
		if (j < 0 || j >= documents.length) return;
		e.preventDefault();
		const doc = documents[i];
		// Swap with the neighbour: land in its slot going up, just past it going down.
		enqueueMove(() => reorderDoc(doc.id, dir === -1 ? j : j + 1));
	}

	async function reorderDoc(documentId: string, position: number) {
		const body = new FormData();
		body.set('documentId', documentId);
		body.set('folderId', folderId);
		body.set('position', String(position));

		reorderingId = documentId;
		const res = await fetch(`${page.url.pathname}?/moveDocument`, {
			method: 'POST',
			body,
			headers: { 'x-sveltekit-action': 'true' }
		});
		const result: ActionResult = deserialize(await res.text());
		reorderingId = null;

		if (result.type === 'success') {
			await invalidateAll();
			showToast(t('doc.docs.reordered'), 'success');
		} else if (result.type === 'failure') {
			showToast((result.data?.message as string) ?? t('err.generic'), 'error');
		} else {
			await applyAction(result);
		}
	}

	// --- move dialog (keyboard + touch path; drag is the shortcut) ---
	type Option = { id: string; number: string; name: string; depth: number };
	const moveOptions = $derived.by(() => {
		const out: Option[] = [];
		const build = (nodes: FolderTreeNode[], depth: number) => {
			for (const n of nodes) {
				if (n.id !== folderId) out.push({ id: n.id, number: n.number, name: n.name, depth });
				if (n.children?.length) build(n.children, depth + 1);
			}
		};
		build(folders, 0);
		return out;
	});

	let moveDialog = $state<HTMLDialogElement>();
	let movingDoc = $state<DocumentData | null>(null);
	let moveTarget = $state('');
	let moveError = $state<string | null>(null);
	let moveSubmitting = $state(false);
	const moveReady = $derived(moveTarget !== '');

	function openMove(doc: DocumentData) {
		movingDoc = doc;
		moveTarget = '';
		moveError = null;
		moveDialog?.showModal();
	}

	const submitMove: SubmitFunction = ({ cancel }) => {
		if (!moveReady) return cancel();
		moveSubmitting = true;
		return async ({ result }) => {
			moveSubmitting = false;
			if (result.type === 'success') {
				moveDialog?.close();
				await invalidateAll();
				showToast(t('doc.docs.moved'), 'success');
			} else if (result.type === 'failure') {
				moveError = (result.data?.message as string) ?? t('err.generic');
			} else {
				moveError = t('err.generic');
			}
		};
	};

	const kindOf = (mime: string) =>
		mime.startsWith('image/')
			? 'image'
			: mime.includes('spreadsheet') || mime.includes('excel') || mime.includes('csv')
				? 'sheet'
				: 'file';

	// --- delete dialog ---
	let deleteDialog = $state<HTMLDialogElement>();
	let deleting = $state<DocumentData | null>(null);
	let deleteError = $state<string | null>(null);
	let deleteSubmitting = $state(false);

	function openDelete(doc: DocumentData) {
		deleting = doc;
		deleteError = null;
		deleteDialog?.showModal();
	}

	const submitDelete: SubmitFunction = () => {
		deleteSubmitting = true;
		return async ({ result }) => {
			deleteSubmitting = false;
			if (result.type === 'success') {
				deleteDialog?.close();
				await invalidateAll();
				showToast(t('doc.docs.deleted'), 'success');
			} else if (result.type === 'failure') {
				deleteError = (result.data?.message as string) ?? t('err.generic');
			} else {
				deleteError = t('err.generic');
			}
		};
	};
</script>

{#snippet fileIcon(kind: string)}
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
		{#if kind === 'image'}
			<rect x="3" y="4" width="18" height="16" rx="2" />
			<circle cx="8.5" cy="9.5" r="1.5" />
			<path d="m21 16-5-5L6 20" />
		{:else if kind === 'sheet'}
			<rect x="3" y="4" width="18" height="16" rx="2" />
			<path d="M3 10h18M9 10v10M15 10v10" />
		{:else}
			<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
			<path d="M14 3v5h5" />
		{/if}
	</svg>
{/snippet}

<section
	ondragover={paneDragOver}
	ondragleave={paneDragLeave}
	ondrop={paneDrop}
	aria-label={shownFolder?.name ?? t('doc.title')}
	class="min-h-64 rounded-box border transition-colors
		{paneTargeted ? 'border-primary/50 bg-primary/[0.04]' : 'border-base-content/10 bg-base-100'}"
>
	<header
		class="flex flex-wrap items-center justify-between gap-3 border-b border-base-content/8 px-4 py-3"
	>
		<div class="flex min-w-0 items-baseline gap-2">
			{#if shownFolder}
				<span class="font-mono text-xs tabular-nums text-muted">{shownFolder.number}</span>
			{/if}
			<h2 class="min-w-0 truncate text-[0.9375rem] font-semibold tracking-[-0.01em]">
				{shownFolder?.name ?? t('doc.docs.unknownFolder')}
			</h2>
			{#if !switching}
				<span class="flex-none font-mono text-xs text-muted">
					{t(documents.length === 1 ? 'doc.docs.countOne' : 'doc.docs.countMany', {
						n: documents.length
					})}
				</span>
			{/if}
			<span
				class="flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 text-[0.6875rem] text-muted"
				title={t('doc.access.chip', { role: roleLabel })}
			>
				<span class="sr-only">{t('doc.access.label')}: </span>{roleLabel}
			</span>
		</div>

		<div class="flex flex-none items-center gap-2">
			{#if documents.length > 1 && !switching}
				<select
					bind:value={sortBy}
					aria-label={t('doc.docs.sort.label')}
					title={canEditDoc && sortBy !== 'manual' ? t('doc.docs.reorder.locked') : undefined}
					class="select select-sm w-auto"
				>
					<option value="manual">{t('doc.docs.sort.manual')}</option>
					<option value="name">{t('doc.docs.sort.name')}</option>
					<option value="updated">{t('doc.docs.sort.updated')}</option>
					<option value="size">{t('doc.docs.sort.size')}</option>
				</select>
			{/if}

			{#if canUpload}
				<input
					bind:this={fileInput}
					onchange={onPick}
					type="file"
					multiple
					class="sr-only"
					aria-label={t('doc.docs.upload')}
				/>
				<!-- The non-drag route to the same feature: `webkitdirectory` reports a
				     folder as a flat list carrying webkitRelativePath. -->
				{#if canCreate}
					<input
						bind:this={folderInput}
						onchange={onPickFolder}
						type="file"
						multiple
						{...{ webkitdirectory: true }}
						class="sr-only"
						aria-label={t('doc.drop.folders')}
					/>
					<Button variant="ghost" onclick={() => folderInput?.click()}>
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
							<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
						</svg>
						{t('doc.drop.folders')}
					</Button>
				{/if}
				<Button onclick={() => fileInput?.click()}>
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
						<path d="M12 19V5M5 12l7-7 7 7" />
					</svg>
					{t('doc.docs.upload')}
				</Button>
			{/if}
		</div>
	</header>

	{#if switching}
		<ul class="divide-y divide-base-content/6" aria-hidden="true">
			{#each SKELETON_ROWS as width (width)}
				<li class="flex items-center gap-2.5 px-4 py-2.5">
					<span class="riksa-skeleton h-4 w-4 flex-none rounded-selector"></span>
					<span class="riksa-skeleton h-3.5 rounded-selector" style="width: {width}%"></span>
					<span class="flex-1"></span>
					<span class="riksa-skeleton hidden h-3.5 w-20 flex-none rounded-selector md:block"></span>
					<span class="riksa-skeleton hidden h-3.5 w-24 flex-none rounded-selector sm:block"></span>
				</li>
			{/each}
		</ul>
	{:else if forbidden}
		<div class="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center">
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
				<rect x="4" y="10.5" width="16" height="10" rx="2" />
				<path d="M8 10.5V7a4 4 0 0 1 8 0v3.5" />
			</svg>
			<div>
				<p class="text-[0.9375rem] font-medium">{t('doc.docs.noAccess.title')}</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
					{t('doc.docs.noAccess.body')}
				</p>
			</div>
		</div>
	{:else if documents.length === 0}
		<div class="flex flex-col items-center justify-center gap-3 px-6 py-16 text-center">
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
				<path d="M12 17V9M8.5 12.5 12 9l3.5 3.5" />
				<path d="M20 16.5V8a2 2 0 0 0-2-2h-6L10 4H6a2 2 0 0 0-2 2v10.5" />
				<path d="M4 16.5A1.5 1.5 0 0 0 5.5 18h13a1.5 1.5 0 0 0 1.5-1.5" />
			</svg>
			<div>
				<p class="text-[0.9375rem] font-medium">{t('doc.docs.empty.title')}</p>
				<p class="mx-auto mt-1 max-w-sm text-sm text-muted text-pretty">
					{canUpload ? t('doc.docs.empty.body') : t('doc.docs.empty.readonly')}
				</p>
			</div>
			{#if canUpload}
				<Button onclick={() => fileInput?.click()}>{t('doc.docs.empty.cta')}</Button>
			{/if}
		</div>
	{:else}
		<ul class="divide-y divide-base-content/6">
			{#each documents as doc, i (doc.id)}
				<!-- The row keeps the drag handlers: its own box is what the insertion
				     point is measured against, and an expanded history below would
				     skew that midpoint if it shared the element. -->
				<li
					draggable={canEditDoc}
					ondragstart={(e) => docDragStart(e, doc)}
					ondragend={endDocDrag}
					ondragover={(e) => docDragOver(e, i)}
					ondrop={docDrop}
					class="group relative flex items-center gap-2.5 px-4 py-2.5 transition-colors hover:bg-base-content/[0.045]
						{draggingDocId === doc.id ? 'opacity-40' : ''}
						{reorderingId === doc.id ? 'opacity-60' : ''}"
				>
					{#if insertAt === i}
						<span class="riksa-dropline pointer-events-none absolute inset-x-4 -top-px"></span>
					{/if}
					{#if insertAt === documents.length && i === documents.length - 1}
						<span class="riksa-dropline pointer-events-none absolute inset-x-4 -bottom-px"></span>
					{/if}
					{@render fileIcon(kindOf(doc.mime))}

					<!-- Opens the secure viewer. The folder is in the path so the reader's
					     back link returns here. Non-viewable files land on the viewer's
					     download-only state — the server owns viewability. -->
					<a
						href={resolve('/(app)/workspace/[slug]/view/[folderId]/[documentId]', {
							slug,
							folderId,
							documentId: doc.id
						})}
						draggable="false"
						title={doc.name}
						aria-label={t('doc.docs.viewOf', { name: doc.name })}
						aria-keyshortcuts={canReorder ? 'Alt+ArrowUp Alt+ArrowDown' : undefined}
						onkeydown={(e) => docKeydown(e, i)}
						class="min-w-0 flex-1 truncate rounded-field text-sm no-underline transition-colors hover:text-primary"
					>
						{doc.name}
					</a>

					<!-- The version number is the door to the history: same chip, now a
					     disclosure. Guests keep the static badge — upstream answers 403
					     for anyone below admin. -->
					{#if canSeeVersions}
						<button
							type="button"
							onclick={() => toggleVersions(doc.id)}
							aria-expanded={openVersions === doc.id}
							aria-controls="doc-versions-{doc.id}"
							title={t('doc.ver.toggle', { name: doc.name })}
							class="flex flex-none items-center gap-1 rounded-selector px-1.5 py-0.5 font-mono text-[0.6875rem] tabular-nums transition-colors
								{openVersions === doc.id
								? 'bg-primary/10 text-primary'
								: 'bg-base-content/5 text-muted hover:bg-base-content/10 hover:text-base-content'}"
						>
							v{doc.version_no}
							<svg
								class="riksa-verchev h-3 w-3"
								class:is-open={openVersions === doc.id}
								viewBox="0 0 24 24"
								fill="none"
								stroke="currentColor"
								stroke-width="2.2"
								stroke-linecap="round"
								stroke-linejoin="round"
								aria-hidden="true"
							>
								<path d="m6 9 6 6 6-6" />
							</svg>
						</button>
					{:else}
						<span
							class="flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 font-mono text-[0.6875rem] text-muted"
							title={t('doc.docs.versionTitle', { n: doc.version_no })}
						>
							v{doc.version_no}
						</span>
					{/if}

					<span
						class="hidden w-20 flex-none text-right font-mono text-xs text-muted tabular-nums md:inline"
					>
						{formatBytes(doc.size)}
					</span>

					<time
						datetime={doc.updated_at}
						title={t('doc.docs.updatedTitle', { when: formatDateTime(doc.updated_at) })}
						class="hidden w-24 flex-none text-right font-mono text-xs text-muted tabular-nums sm:inline"
					>
						{formatDate(doc.updated_at)}
					</time>

					<div
						class="flex flex-none items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:gap-1 pointer-coarse:opacity-100"
					>
						{#if canDownload}
							<button
								type="button"
								onclick={() => download(doc)}
								disabled={downloadingId === doc.id}
								draggable="false"
								title={t('doc.docs.download')}
								aria-label={t('doc.docs.downloadOf', { name: doc.name })}
								class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content disabled:pointer-events-none disabled:opacity-50 pointer-coarse:h-11 pointer-coarse:w-11"
							>
								{#if downloadingId === doc.id}
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
										<path d="M12 4v11M7.5 10.5 12 15l4.5-4.5" />
										<path d="M5 19h14" />
									</svg>
								{/if}
							</button>
						{/if}
						{#if canEditDoc && moveOptions.length > 0}
							<button
								type="button"
								onclick={() => openMove(doc)}
								title={t('doc.docs.move')}
								aria-label={t('doc.docs.moveOf', { name: doc.name })}
								class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
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
									<path d="M5 9V5h4M19 15v4h-4M5 5l6 6M19 19l-6-6" />
								</svg>
							</button>
						{/if}
						{#if canDelete}
							<button
								type="button"
								onclick={() => openDelete(doc)}
								title={t('doc.docs.delete')}
								aria-label={t('doc.docs.deleteOf', { name: doc.name })}
								class="grid h-8 w-8 place-items-center rounded-field text-muted transition-colors hover:bg-error/10 hover:text-error pointer-coarse:h-11 pointer-coarse:w-11"
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
									<path
										d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m2 0v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"
									/>
									<path d="M10 11v6M14 11v6" />
								</svg>
							</button>
						{/if}
					</div>
				</li>

				<!-- Its own list item rather than a child of the row: the row's box has
				     to stay the row's box for the reorder measurement above. -->
				{#if canSeeVersions && openVersions === doc.id}
					<li>
						<DocumentVersions
							workspaceId={workspace.id}
							{slug}
							{folderId}
							documentId={doc.id}
							documentName={doc.name}
							{canDownload}
							canRestore={canEditDoc}
							{canUpload}
						/>
					</li>
				{/if}
			{/each}
		</ul>

		{#if canUpload}
			<p class="px-4 py-3 text-xs text-muted">{t('doc.docs.dropHint')}</p>
		{/if}
	{/if}
</section>

<dialog bind:this={moveDialog} class="modal" aria-labelledby="doc-move-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="doc-move-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('doc.docs.move.title')}
		</h2>
		{#if movingDoc}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('doc.docs.move.desc', { name: movingDoc.name })}
			</p>
		{/if}

		{#if moveError}
			<div class="mt-4"><Alert align="start">{moveError}</Alert></div>
		{/if}

		<form method="POST" action="?/moveDocument" use:enhance={submitMove} class="mt-5">
			<input type="hidden" name="documentId" value={movingDoc?.id ?? ''} />
			<label for="doc-move-dest" class="mb-1.5 block text-sm font-medium">
				{t('doc.docs.move.dest')}
			</label>
			<select
				id="doc-move-dest"
				name="folderId"
				bind:value={moveTarget}
				required
				class="select select-sm w-full font-mono"
			>
				<option value="" disabled>{t('doc.docs.move.placeholder')}</option>
				{#each moveOptions as opt (opt.id)}
					<option value={opt.id}>{'  '.repeat(opt.depth)}{opt.number} {opt.name}</option>
				{/each}
			</select>

			<div class="mt-6 flex justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => moveDialog?.close()}>
					{t('doc.cancel')}
				</Button>
				<Button type="submit" disabled={!moveReady} loading={moveSubmitting}>
					{moveSubmitting ? t('doc.docs.move.submitting') : t('doc.docs.move.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('doc.cancel')}></button>
	</form>
</dialog>

<dialog bind:this={deleteDialog} class="modal" aria-labelledby="doc-delete-title">
	<div class="modal-box w-full max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="doc-delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('doc.docs.delete.title')}
		</h2>
		{#if deleting}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('doc.docs.delete.warning', { name: deleting.name })}
			</p>
			{#if deleting.version_no > 1}
				<p class="mt-2 text-sm font-medium text-error text-pretty">
					{t('doc.docs.delete.versions', { n: deleting.version_no })}
				</p>
			{/if}
		{/if}

		{#if deleteError}
			<div class="mt-4"><Alert align="start">{deleteError}</Alert></div>
		{/if}

		<form
			method="POST"
			action="?/deleteDocument"
			use:enhance={submitDelete}
			class="mt-6 flex justify-end gap-2"
		>
			<input type="hidden" name="documentId" value={deleting?.id ?? ''} />
			<Button type="button" variant="ghost" onclick={() => deleteDialog?.close()}>
				{t('doc.cancel')}
			</Button>
			<Button type="submit" variant="danger" loading={deleteSubmitting}>
				{deleteSubmitting ? t('doc.docs.delete.submitting') : t('doc.docs.delete.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('doc.cancel')}></button>
	</form>
</dialog>

<div aria-live="polite" class="sr-only">
	{#if insertLabel}{insertLabel}{/if}
</div>

<style>
	/* Matches the tree's insertion caret: a 2px rule with a knob at its left end. */
	.riksa-dropline {
		height: 2px;
		border-radius: 1px;
		background: var(--color-primary);
	}
	.riksa-dropline::before {
		content: '';
		position: absolute;
		left: 0;
		top: 50%;
		height: 6px;
		width: 6px;
		margin-top: -3px;
		border-radius: 9999px;
		background: var(--color-primary);
	}
	.riksa-skeleton {
		background-color: color-mix(in oklch, var(--color-base-content) 8%, transparent);
		animation: riksa-pulse 1400ms ease-in-out infinite;
	}
	@keyframes riksa-pulse {
		50% {
			opacity: 0.45;
		}
	}
	/* The only thing the chevron says is open or shut; it turns, nothing else. */
	.riksa-verchev {
		transition: transform 180ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.riksa-verchev.is-open {
		transform: rotate(180deg);
	}
	@media (prefers-reduced-motion: reduce) {
		.riksa-skeleton {
			animation: none;
		}
		.riksa-verchev {
			transition: none;
		}
	}
</style>
