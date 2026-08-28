<script lang="ts">
	import { tick } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import { applyAction, deserialize, enhance } from '$app/forms';
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import type { ActionResult, SubmitFunction } from '@sveltejs/kit';
	import { FolderTemplatePicker, UploadQueue } from '$lib/components/app';
	import { Alert, Button, Field, Toaster, showToast } from '$lib/components/common';
	import {
		DOCUMENT_MIME,
		FOLDER_MIME,
		entriesFrom,
		filesFrom,
		hasDirectory,
		readTree
	} from '$lib/dnd';
	import { roomWritableFrom } from '$lib/access/roles';
	import { t } from '$lib/i18n';
	import { findNode } from '$lib/tree';
	import type { FolderTreeNode } from '$lib/types/content';
	import type { MyAccessWorkspace } from '$lib/types/workspace';
	import { uploadQueue } from '$lib/upload/queue.svelte';
	import { uploadTree, type TreeDestination } from '$lib/upload/tree';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	const folders = $derived(data.folders);
	const workspace = $derived(data.workspace);
	const noAccess = $derived(data.noAccess);

	const ROOT = '';

	const slug = $derived(page.params.slug!);
	const actionBase = $derived(resolve('/(app)/workspace/[slug]/document', { slug }));
	const docHref = (folderId: string) =>
		resolve('/(app)/workspace/[slug]/document/[folderId]', { slug, folderId });
	const accessHref = (folderId: string) =>
		resolve('/(app)/workspace/[slug]/document/[folderId]/access', { slug, folderId });
	const activeId = $derived(page.params.folderId ?? null);

	const onAccess = $derived(page.url.pathname.endsWith('/access'));
	const rowHref = (folderId: string) => (onAccess ? accessHref(folderId) : docHref(folderId));

	const perms = $derived((page.data as { access?: MyAccessWorkspace }).access?.permissions ?? []);
	// An archived room is frozen for every role: the server answers 423, so the
	// controls that would trigger it are not offered.
	const writable = $derived(roomWritableFrom(page.data));
	const canCreate = $derived(perms.includes('folder:create') && writable);
	const canEdit = $derived(perms.includes('folder:edit') && writable);
	const canDelete = $derived(perms.includes('folder:delete') && writable);
	const canUpload = $derived(perms.includes('document:upload') && writable);
	const canEditDoc = $derived(perms.includes('document:edit') && writable);
	const canAssign = $derived(perms.includes('group:assign') && writable);
	const canAct = $derived(canCreate || canEdit || canDelete || canAssign);

	const defaultFolder = $derived(folders.find((f) => f.is_default) ?? null);
	const activeFolder = $derived(activeId ? findNode(folders, activeId) : null);
	const fallbackFolder = $derived(activeFolder ?? defaultFolder);

	// Top-level folders reveal their children; deeper levels start closed, so a
	// 300-folder index opens as a readable table of contents rather than a wall.
	const DEFAULT_OPEN_DEPTH = 1;

	let expanded = $state<Record<string, boolean>>({});
	const isExpanded = (id: string, depth: number) => expanded[id] ?? depth < DEFAULT_OPEN_DEPTH;
	function toggle(id: string, depth: number) {
		expanded[id] = !isExpanded(id, depth);
	}

	let query = $state('');
	const q = $derived(query.trim().toLowerCase());

	// `null` = no search. Otherwise the ids to render: every match plus the
	// ancestors needed to reach it, so a hit never appears without its context.
	const matched = $derived.by(() => {
		if (!q) return null;
		const keep: Record<string, true> = {};
		const visit = (nodes: FolderTreeNode[], trail: string[]): void => {
			for (const n of nodes) {
				const hit = n.name.toLowerCase().includes(q) || n.number.toLowerCase().includes(q);
				if (hit) {
					keep[n.id] = true;
					for (const id of trail) keep[id] = true;
				}
				visit(n.children ?? [], [...trail, n.id]);
			}
		};
		visit(folders, []);
		return keep;
	});

	type Row = { node: FolderTreeNode; depth: number; hasChildren: boolean; open: boolean };
	function walk(nodes: FolderTreeNode[], depth: number, out: Row[]) {
		for (const n of nodes) {
			if (matched && !matched[n.id]) continue;
			const kids = (n.children ?? []).filter((k) => !matched || matched[k.id]);
			// A search result is always drilled open; otherwise honour the toggle.
			const open = matched ? true : isExpanded(n.id, depth);
			out.push({ node: n, depth, hasChildren: kids.length > 0, open });
			if (kids.length && open) walk(kids, depth + 1, out);
		}
	}
	const rows = $derived.by(() => {
		const out: Row[] = [];
		walk(folders, 0, out);
		return out;
	});

	const depthOf = $derived.by(() => {
		const map: Record<string, number> = {};
		const build = (nodes: FolderTreeNode[], depth: number) => {
			for (const n of nodes) {
				map[n.id] = depth;
				if (n.children?.length) build(n.children, depth + 1);
			}
		};
		build(folders, 0);
		return map;
	});

	const branchIds = $derived.by(() => {
		const out: string[] = [];
		const visit = (nodes: FolderTreeNode[]) => {
			for (const n of nodes) {
				if (n.children?.length) out.push(n.id);
				visit(n.children ?? []);
			}
		};
		visit(folders);
		return out;
	});

	const anyCollapsed = $derived(branchIds.some((id) => !isExpanded(id, depthOf[id] ?? 0)));

	function setAll(open: boolean) {
		const next: Record<string, boolean> = {};
		for (const id of branchIds) next[id] = open;
		expanded = next;
	}

	const totalCount = $derived.by(() => {
		let n = 0;
		const stack = [...folders];
		while (stack.length) {
			const f = stack.pop()!;
			n++;
			if (f.children?.length) stack.push(...f.children);
		}
		return n;
	});

	const parentOf = $derived.by(() => {
		const map: Record<string, string> = {};
		const build = (nodes: FolderTreeNode[], parent: string) => {
			for (const n of nodes) {
				map[n.id] = parent;
				if (n.children?.length) build(n.children, n.id);
			}
		};
		build(folders, ROOT);
		return map;
	});

	function descendantCount(node: FolderTreeNode): number {
		let n = 0;
		for (const c of node.children ?? []) n += 1 + descendantCount(c);
		return n;
	}

	function subtreeIds(node: FolderTreeNode, into: string[] = []): string[] {
		into.push(node.id);
		for (const c of node.children ?? []) subtreeIds(c, into);
		return into;
	}

	// Siblings straight from the server payload, so `position` below is the
	// server's own index — not an array index that search or folder-level
	// visibility could have thinned out.
	const siblingsOf = $derived.by(() => {
		const map: Record<string, FolderTreeNode[]> = { [ROOT]: folders };
		const visit = (nodes: FolderTreeNode[]) => {
			for (const n of nodes) {
				const kids = n.children ?? [];
				if (!kids.length) continue;
				map[n.id] = kids;
				visit(kids);
			}
		};
		visit(folders);
		return map;
	});

	const indentRem = (depth: number) => `${depth * 1.375 + 0.25}rem`;
	const indent = (depth: number) => `padding-left: ${indentRem(depth)}`;

	// --- drag & drop -------------------------------------------------------
	// `Files` is an upload from the OS, FOLDER_MIME moves a folder, DOCUMENT_MIME
	// moves a document. `types` is the only payload readable during dragover, so
	// it is the switch.

	type Edge = 'before' | 'into' | 'after';

	let draggingId = $state<string | null>(null);
	let dropTarget = $state<string | null>(null);
	// 'into' reparents, 'before'/'after' reorder among the target's siblings.
	let dropEdge = $state<Edge>('into');
	let dragKind = $state<'files' | 'folder' | 'document' | null>(null);
	let fileDragging = $state(false);
	// The row a move is in flight for — the tree reloads wholesale afterwards,
	// so this is the only feedback between drop and the fresh numbering.
	let pendingId = $state<string | null>(null);
	let dragDepth = 0;

	// Moves are serialised per workspace by an advisory lock on the server, so
	// firing them concurrently just makes them queue on the database. Chaining
	// keeps held-down Alt+Arrow ordered and keeps the lock uncontended.
	let moveChain: Promise<unknown> = Promise.resolve();
	const enqueueMove = (run: () => Promise<unknown>) => {
		moveChain = moveChain.then(run, run);
	};

	const hasFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files');
	const hasFolder = (e: DragEvent) => !!e.dataTransfer?.types.includes(FOLDER_MIME);
	const hasDocument = (e: DragEvent) => !!e.dataTransfer?.types.includes(DOCUMENT_MIME);

	// A document is only ever dragged out of the folder currently on screen, so
	// the folder it came from is `activeId` — no payload lookup needed to reject
	// a drop back onto its own folder.
	const canMoveDocInto = (targetId: string) => canEditDoc && !!activeId && targetId !== activeId;

	const dropLabel = $derived.by(() => {
		if (dropTarget === ROOT) return defaultFolder?.name ?? null;
		if (dropTarget) return findNode(folders, dropTarget)?.name ?? null;
		return fallbackFolder?.name ?? null;
	});

	// What releasing right now would do. A dragover cannot see inside the payload
	// — `types` only ever says "Files" — so at the root, where a folder and a file
	// land in two different places, the hint has to name both outcomes rather than
	// promise the wrong one.
	const dropHint = $derived.by(() => {
		const rootNow = dropTarget === ROOT || (dropTarget === null && !activeFolder);
		if (rootNow) {
			return defaultFolder
				? t('doc.drop.atRoot', { name: defaultFolder.name })
				: t('doc.upload.noDefault');
		}
		const name = dropTarget ? (findNode(folders, dropTarget)?.name ?? null) : dropLabel;
		return name ? t('doc.drop.intoFolder', { name }) : t('doc.upload.noDefault');
	});

	// The announcement has to match the gesture: an upload, a reparent, and an
	// insertion between two rows are three different outcomes on the same target.
	const dropMessage = $derived.by(() => {
		if (dragKind === 'files') return dropHint;
		if (dropTarget === null || !dropLabel) return null;
		if (dropEdge === 'before') return t('doc.reorder.before', { name: dropLabel });
		if (dropEdge === 'after') return t('doc.reorder.after', { name: dropLabel });
		return t('doc.reorder.into', { name: dropLabel });
	});

	function resetDrag() {
		dragDepth = 0;
		fileDragging = false;
		dropTarget = null;
		dropEdge = 'into';
		dragKind = null;
		draggingId = null;
	}

	function uploadTo(folderId: string, folderName: string, files: File[]) {
		if (!canUpload || !files.length) return;
		uploadQueue.enqueue(workspace.id, folderId, folderName, files);
	}

	// A dropped folder needs its structure created before any file has somewhere
	// to go. `entriesFrom` has to run inside the drop handler — dataTransfer is
	// emptied when the event ends — so the entries are captured first and walked
	// afterwards.
	function dropAt(e: DragEvent, dest: TreeDestination) {
		if (!canUpload) return;
		const entries = entriesFrom(e.dataTransfer);

		if (!hasDirectory(entries)) {
			// Plain files never land at the root — they follow the loose target.
			if (!dest.loose) {
				showToast(t('doc.upload.noDefault'), 'error');
				return;
			}
			uploadTo(dest.loose.id, dest.loose.name, filesFrom(e.dataTransfer));
			return;
		}
		if (!canCreate) {
			showToast(t('doc.drop.err.noCreate'), 'error');
			return;
		}
		void ingestTree(entries, dest);
	}

	// Dropping into a folder: that folder is both the parent for new subfolders
	// and the home for loose files.
	const intoFolder = (id: string, name: string): TreeDestination => ({
		parentId: id,
		loose: { id, name }
	});

	// Dropping at the root: folders stay at the root, loose files go to General.
	const atRoot = (): TreeDestination => ({
		parentId: ROOT,
		loose: defaultFolder ? { id: defaultFolder.id, name: defaultFolder.name } : null
	});

	async function ingestTree(entries: FileSystemEntry[], dest: TreeDestination) {
		try {
			const tree = await readTree(entries);
			const created = tree.folders.length;
			await uploadTree(workspace.id, dest, tree);
			if (created) {
				// New folders only exist upstream until the tree is refetched.
				await invalidateAll();
				showToast(t('doc.drop.created', { n: created }), 'success');
			}
		} catch (err) {
			showToast(err instanceof Error ? err.message : t('doc.drop.err.read'), 'error');
		}
	}

	// A loose drop belongs to whatever folder the user is looking at; the index
	// itself (no folder open) behaves like a drop at the root.
	function dropIntoFallback(e: DragEvent) {
		if (activeFolder) dropAt(e, intoFolder(activeFolder.id, activeFolder.name));
		else dropAt(e, atRoot());
	}

	function canMoveInto(targetId: string): boolean {
		if (!draggingId || !canEdit) return false;
		if (targetId === draggingId) return false;
		if (parentOf[draggingId] === targetId) return false;
		const node = findNode(folders, draggingId);
		return !!node && !subtreeIds(node).includes(targetId);
	}

	// Top and bottom quarters insert between rows; the middle half keeps the
	// existing "drop into this folder" behaviour.
	function edgeAt(e: DragEvent, row: Row): Edge {
		// Under search the visible rows are not contiguous siblings, so an
		// insertion point between two of them would be a lie. Only 'into' is honest.
		if (matched) return 'into';
		const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
		if (!rect.height) return 'into';
		const y = (e.clientY - rect.top) / rect.height;
		if (y <= 0.25) return 'before';
		// An open branch is followed by its own first child, so "after it" and
		// "before that child" would point at the same gap with different meanings.
		// The child's own 'before' band owns it.
		if (y >= 0.75 && !(row.open && row.hasChildren)) return 'after';
		return 'into';
	}

	function canReorderAt(node: FolderTreeNode, edge: Edge): boolean {
		if (!draggingId || !canEdit || edge === 'into') return false;
		if (node.id === draggingId) return false;
		// General is pinned to position 0 of the root list; nothing goes above it.
		if (edge === 'before' && node.is_default) return false;

		const parentId = parentOf[node.id] ?? ROOT;
		const dragged = findNode(folders, draggingId);
		if (!dragged || subtreeIds(dragged).includes(parentId)) return false;

		// Landing back where it already sits: both edges around the dragged row's
		// own slot are no-ops, and a no-op still costs a round trip.
		if (parentId === (parentOf[draggingId] ?? ROOT)) {
			const position = edge === 'before' ? node.position : node.position + 1;
			if (position === dragged.position || position === dragged.position + 1) return false;
		}
		return true;
	}

	// `position` means "insert before whoever currently holds index N", and the
	// server's reindex lets the moved row win that tie — so the target's own
	// position is exactly right for 'before', and one past it for 'after'.
	// No direction correction is needed for downward moves.
	const reorderTarget = (node: FolderTreeNode, edge: Edge) => ({
		parentId: parentOf[node.id] ?? ROOT,
		position: edge === 'before' ? node.position : node.position + 1
	});

	async function moveTo(folderId: string, parentId: string, position?: number) {
		// Read before the round trip: `parentOf` is rebuilt by invalidateAll().
		const reorderOnly = (parentOf[folderId] ?? ROOT) === parentId;

		const body = new FormData();
		body.set('folderId', folderId);
		body.set('parentId', parentId);
		if (position !== undefined) body.set('position', String(position));

		pendingId = folderId;
		const res = await fetch(`${actionBase}?/move`, {
			method: 'POST',
			body,
			headers: { 'x-sveltekit-action': 'true' }
		});
		const result: ActionResult = deserialize(await res.text());
		pendingId = null;

		if (result.type === 'success') {
			await invalidateAll();
			showToast(reorderOnly ? t('doc.reordered') : t('doc.moved'), 'success');
		} else if (result.type === 'failure') {
			showToast((result.data?.message as string) ?? t('err.generic'), 'error');
		} else {
			await applyAction(result);
		}
	}

	async function moveDocumentTo(documentId: string, folderId: string) {
		if (!activeId) return;

		const body = new FormData();
		body.set('documentId', documentId);
		body.set('folderId', folderId);

		const res = await fetch(`${docHref(activeId)}?/moveDocument`, {
			method: 'POST',
			body,
			headers: { 'x-sveltekit-action': 'true' }
		});
		const result: ActionResult = deserialize(await res.text());

		if (result.type === 'success') {
			await invalidateAll();
			showToast(t('doc.docs.moved'), 'success');
		} else if (result.type === 'failure') {
			showToast((result.data?.message as string) ?? t('err.generic'), 'error');
		} else {
			await applyAction(result);
		}
	}

	function rowDragStart(e: DragEvent, node: FolderTreeNode) {
		if (!canEdit || node.is_default) {
			e.preventDefault();
			return;
		}
		draggingId = node.id;
		e.dataTransfer?.setData(FOLDER_MIME, node.id);
		if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
	}

	// A row claims every drag of a kind it handles, even when the target is
	// illegal — it just refuses it. Letting an illegal drag fall through would
	// hand it to the rail below, silently turning "drop onto my own folder" into
	// "move to root".
	function claim(e: DragEvent) {
		e.preventDefault();
		e.stopPropagation();
	}

	function rowDragOver(e: DragEvent, row: Row) {
		if (selectMode) return;
		const node = row.node;
		if (hasFiles(e)) {
			if (!canUpload) return;
			claim(e);
			if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
			dragKind = 'files';
			dropEdge = 'into';
			dropTarget = node.id;
		} else if (hasFolder(e) && canEdit && draggingId) {
			claim(e);
			dragKind = 'folder';
			const edge = edgeAt(e, row);
			const ok = edge === 'into' ? canMoveInto(node.id) : canReorderAt(node, edge);
			if (e.dataTransfer) e.dataTransfer.dropEffect = ok ? 'move' : 'none';
			dropEdge = edge;
			dropTarget = ok ? node.id : null;
		} else if (hasDocument(e) && canEditDoc && activeId) {
			claim(e);
			dragKind = 'document';
			// A document dragged onto the tree is always a reparent; ordering it
			// happens in the document list, where its siblings are visible.
			dropEdge = 'into';
			const ok = canMoveDocInto(node.id);
			if (e.dataTransfer) e.dataTransfer.dropEffect = ok ? 'move' : 'none';
			dropTarget = ok ? node.id : null;
		}
	}

	function rowDragLeave(e: DragEvent, node: FolderTreeNode) {
		const next = e.relatedTarget as Node | null;
		if (next && (e.currentTarget as Element).contains(next)) return;
		// Only clear a target this row still owns: dragover on the row being
		// entered can fire before dragleave on the row being left, and clearing
		// unconditionally would drop the new row's indicator on every crossing.
		if (dropTarget === node.id) dropTarget = null;
	}

	function rowDrop(e: DragEvent, row: Row) {
		if (selectMode) return;
		const node = row.node;
		if (hasFiles(e)) {
			if (!canUpload) return;
			claim(e);
			dropAt(e, intoFolder(node.id, node.name));
			resetDrag();
		} else if (hasFolder(e) && canEdit && draggingId) {
			claim(e);
			const id = draggingId;
			const edge = edgeAt(e, row);
			if (edge === 'into') {
				if (canMoveInto(node.id)) enqueueMove(() => moveTo(id, node.id));
			} else if (canReorderAt(node, edge)) {
				const { parentId, position } = reorderTarget(node, edge);
				enqueueMove(() => moveTo(id, parentId, position));
			}
			resetDrag();
		} else if (hasDocument(e) && canEditDoc && activeId) {
			claim(e);
			const documentId = e.dataTransfer!.getData(DOCUMENT_MIME);
			if (documentId && canMoveDocInto(node.id)) void moveDocumentTo(documentId, node.id);
			resetDrag();
		}
	}

	function railDragOver(e: DragEvent) {
		if (selectMode) return;
		dropEdge = 'into';
		if (hasFiles(e)) {
			// `types` cannot tell a folder from a file during dragover, so the rail
			// accepts the drag either way and dropAt sorts out where things land.
			if (!canUpload) return;
			e.preventDefault();
			if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
			dragKind = 'files';
			dropTarget = ROOT;
		} else if (hasFolder(e) && draggingId && canEdit && parentOf[draggingId] !== ROOT) {
			e.preventDefault();
			if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
			dragKind = 'folder';
			dropTarget = ROOT;
		} else if (hasDocument(e) && defaultFolder && canMoveDocInto(defaultFolder.id)) {
			e.preventDefault();
			if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
			dragKind = 'document';
			dropTarget = ROOT;
		}
	}

	function railDragLeave(e: DragEvent) {
		const next = e.relatedTarget as Node | null;
		if (next && (e.currentTarget as Element).contains(next)) return;
		if (dropTarget === ROOT) dropTarget = null;
	}

	function railDrop(e: DragEvent) {
		if (selectMode) return;
		if (hasFiles(e)) {
			// No `defaultFolder` check: a dropped folder belongs at the root and
			// needs no default. Only loose files do, and dropAt reports that.
			if (!canUpload) return;
			e.preventDefault();
			dropAt(e, atRoot());
			resetDrag();
		} else if (hasFolder(e) && draggingId && canEdit && parentOf[draggingId] !== ROOT) {
			e.preventDefault();
			const id = draggingId;
			enqueueMove(() => moveTo(id, ROOT));
			resetDrag();
		} else if (hasDocument(e) && defaultFolder && canMoveDocInto(defaultFolder.id)) {
			e.preventDefault();
			const documentId = e.dataTransfer!.getData(DOCUMENT_MIME);
			if (documentId) void moveDocumentTo(documentId, defaultFolder.id);
			resetDrag();
		}
	}

	// Reordering by keyboard. The Move dialog already covers reparenting; this
	// covers the one thing drag does that no dialog expresses — position among
	// siblings — so the feature is not mouse-only.
	function nudge(node: FolderTreeNode, dir: -1 | 1) {
		if (!canEdit || node.is_default) return;
		const parentId = parentOf[node.id] ?? ROOT;
		const sibs = siblingsOf[parentId] ?? [];
		const i = sibs.findIndex((s) => s.id === node.id);
		const neighbour = sibs[i + dir];
		if (i < 0 || !neighbour) return;
		// Same reason as canReorderAt: nothing is allowed above General.
		if (dir === -1 && neighbour.is_default) return;

		const position = dir === -1 ? neighbour.position : neighbour.position + 1;
		enqueueMove(() => moveTo(node.id, parentId, position));
	}

	// Listens on the window, not on the row: clicking a folder navigates, and
	// SvelteKit moves focus off the link afterwards, so a handler bound to the
	// row would never fire for the ordinary click-then-reorder flow.
	function treeKeydown(e: KeyboardEvent) {
		if (!e.altKey || e.ctrlKey || e.metaKey) return;
		if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
		if (!canEdit || matched) return;

		const el = e.target as HTMLElement | null;
		// Never steal Alt+Arrow from a field or an open dialog.
		if (el?.closest('input, textarea, select, [contenteditable="true"], dialog')) return;

		// A row holding focus wins; otherwise the shortcut acts on the folder
		// currently open, which is what the user just clicked.
		const id = el?.closest<HTMLElement>('[data-folder-id]')?.dataset.folderId ?? activeId;
		if (!id) return;

		const node = findNode(folders, id);
		if (!node || node.is_default) return;

		e.preventDefault();
		nudge(node, e.key === 'ArrowUp' ? -1 : 1);
	}

	function winDragEnter(e: DragEvent) {
		dragDepth++;
		if (hasFiles(e)) fileDragging = true;
	}

	function winDragLeave() {
		dragDepth = Math.max(0, dragDepth - 1);
		if (dragDepth === 0) fileDragging = false;
	}

	function winDragOver(e: DragEvent) {
		if (selectMode || !hasFiles(e) || !canUpload) return;
		e.preventDefault();
		if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
	}

	function winDrop(e: DragEvent) {
		if (selectMode) return;
		const handled = e.defaultPrevented;
		e.preventDefault();

		if (!handled && hasFiles(e) && canUpload) dropIntoFallback(e);
		resetDrag();
	}

	// --- inline create ('' = root, id = under folder, null = idle) ---
	let creatingParent = $state<string | null>(null);
	let createError = $state<string | null>(null);
	let createSubmitting = $state(false);

	async function startCreate(parentId: string) {
		renamingId = null;
		createError = null;
		creatingParent = parentId;
		if (parentId !== ROOT) expanded[parentId] = true;
		await tick();
		document.getElementById('folder-create-input')?.focus();
	}
	function cancelCreate() {
		creatingParent = null;
		createError = null;
	}

	const submitCreate: SubmitFunction = () => {
		createSubmitting = true;
		return async ({ result }) => {
			createSubmitting = false;
			if (result.type === 'success') {
				creatingParent = null;
				createError = null;
				await invalidateAll();
				showToast(t('doc.created'), 'success');
			} else if (result.type === 'failure') {
				createError = (result.data?.message as string) ?? t('err.generic');
			} else {
				createError = t('err.generic');
			}
		};
	};

	// --- inline rename ---
	let renamingId = $state<string | null>(null);
	let renameError = $state<string | null>(null);
	let renameSubmitting = $state(false);

	async function startRename(node: FolderTreeNode) {
		creatingParent = null;
		renameError = null;
		renamingId = node.id;
		await tick();
		const el = document.getElementById('folder-rename-input') as HTMLInputElement | null;
		el?.focus();
		el?.select();
	}
	function cancelRename() {
		renamingId = null;
		renameError = null;
	}

	const submitRename: SubmitFunction = () => {
		renameSubmitting = true;
		return async ({ result }) => {
			renameSubmitting = false;
			if (result.type === 'success') {
				renamingId = null;
				renameError = null;
				await invalidateAll();
				showToast(t('doc.renamed'), 'success');
			} else if (result.type === 'failure') {
				renameError = (result.data?.message as string) ?? t('err.generic');
			} else {
				renameError = t('err.generic');
			}
		};
	};

	// --- move dialog (keyboard + touch path; drag is the shortcut) ---
	let templateDialog = $state<HTMLDialogElement>();
	// The picker mounts on first open only, so its lazy template fetch never
	// fires for readers who never touch the dialog.
	let templatesOpened = $state(false);

	function openTemplates() {
		templatesOpened = true;
		templateDialog?.showModal();
	}

	let moveDialog = $state<HTMLDialogElement>();
	let moving = $state<FolderTreeNode | null>(null);
	let moveTarget = $state(ROOT);
	let moveError = $state<string | null>(null);
	let moveSubmitting = $state(false);

	type Option = { id: string; number: string; name: string; depth: number };
	const moveOptions = $derived.by(() => {
		const out: Option[] = [];
		const skip = moving?.id;
		const build = (nodes: FolderTreeNode[], depth: number) => {
			for (const n of nodes) {
				if (n.id === skip) continue;
				out.push({ id: n.id, number: n.number, name: n.name, depth });
				if (n.children?.length) build(n.children, depth + 1);
			}
		};
		build(folders, 0);
		return out;
	});

	function openMove(node: FolderTreeNode) {
		moving = node;
		moveTarget = ROOT;
		moveError = null;
		moveDialog?.showModal();
	}

	const submitMove: SubmitFunction = () => {
		moveSubmitting = true;
		return async ({ result }) => {
			moveSubmitting = false;
			if (result.type === 'success') {
				moveDialog?.close();
				await invalidateAll();
				showToast(t('doc.moved'), 'success');
			} else if (result.type === 'failure') {
				moveError = (result.data?.message as string) ?? t('err.generic');
			} else {
				moveError = t('err.generic');
			}
		};
	};

	// --- bulk select (folders) ---
	// Selection is the current view only; General never gets a checkbox, and
	// every drag gesture is parked while the mode is on — the two share rows.
	let selectMode = $state(false);
	const selectedFolders = new SvelteSet<string>();
	let bulkDialog = $state<HTMLDialogElement>();
	let bulkError = $state<string | null>(null);
	let bulkSubmitting = $state(false);

	function exitSelect() {
		selectMode = false;
		selectedFolders.clear();
	}

	function toggleSelected(id: string) {
		if (selectedFolders.has(id)) selectedFolders.delete(id);
		else selectedFolders.add(id);
	}

	function openBulkDelete() {
		bulkError = null;
		bulkDialog?.showModal();
	}

	const submitBulkDelete: SubmitFunction = ({ cancel }) => {
		if (selectedFolders.size === 0) return cancel();
		bulkSubmitting = true;

		const stranded =
			!!activeId &&
			[...selectedFolders].some((id) => {
				const node = findNode(folders, id);
				return !!node && subtreeIds(node).includes(activeId);
			});

		return async ({ result }) => {
			bulkSubmitting = false;
			if (result.type === 'success') {
				const n = selectedFolders.size;
				bulkDialog?.close();
				exitSelect();
				if (stranded) await goto(actionBase, { invalidateAll: true });
				else await invalidateAll();
				showToast(t('doc.select.deletedFolders', { n }), 'success');
			} else if (result.type === 'failure') {
				bulkError = (result.data?.message as string) ?? t('err.generic');
			} else {
				bulkError = t('err.generic');
			}
		};
	};

	// --- delete dialog ---
	let deleteDialog = $state<HTMLDialogElement>();
	let deleting = $state<FolderTreeNode | null>(null);
	let deleteError = $state<string | null>(null);
	let deleteSubmitting = $state(false);
	let deleteConfirm = $state('');
	const deletingKids = $derived(deleting ? descendantCount(deleting) : 0);
	const deleteReady = $derived(!!deleting && deleteConfirm.trim() === deleting.name);

	function openDelete(node: FolderTreeNode) {
		deleting = node;
		deleteConfirm = '';
		deleteError = null;
		deleteDialog?.showModal();
	}

	const submitDelete: SubmitFunction = ({ cancel }) => {
		if (!deleteReady) return cancel();
		deleteSubmitting = true;

		// The server cascades to descendants, so viewing any of them strands the
		// page on a folder that no longer exists.
		const removed = deleting ? subtreeIds(deleting) : [];
		const stranded = !!activeId && removed.includes(activeId);

		return async ({ result }) => {
			deleteSubmitting = false;
			if (result.type === 'success') {
				deleteDialog?.close();
				if (stranded) await goto(actionBase, { invalidateAll: true });
				else await invalidateAll();
				showToast(t('doc.deleted'), 'success');
			} else if (result.type === 'failure') {
				deleteError = (result.data?.message as string) ?? t('err.generic');
			} else {
				deleteError = t('err.generic');
			}
		};
	};
</script>

<svelte:window
	ondragenter={winDragEnter}
	ondragleave={winDragLeave}
	ondragover={winDragOver}
	ondrop={winDrop}
	ondragend={resetDrag}
	onkeydown={treeKeydown}
/>

<svelte:head><title>{t('doc.title')} · {t('brand.name')}</title></svelte:head>

{#snippet folderIcon(open: boolean)}
	<svg
		class="h-4 w-4 flex-none text-muted"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.7"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
	>
		{#if open}
			<path d="M3 8a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v1H7l-2 8" />
			<path d="M5 19h13.5a1.5 1.5 0 0 0 1.46-1.14L21.5 11H8.5a1.5 1.5 0 0 0-1.46 1.14z" />
		{:else}
			<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
		{/if}
	</svg>
{/snippet}

{#snippet createInputRow(depth: number, parentId: string)}
	<li class="flex items-start gap-1.5 py-1.5 pr-2" style={indent(depth)}>
		<span class="mt-1.5 w-6 flex-none pointer-coarse:w-9"></span>
		<span class="mt-1.5">{@render folderIcon(false)}</span>
		<div class="min-w-0 flex-1">
			<form
				method="POST"
				action="{actionBase}?/create"
				use:enhance={submitCreate}
				class="flex flex-wrap items-center gap-1.5"
			>
				<input type="hidden" name="parentId" value={parentId} />
				<input
					id="folder-create-input"
					name="name"
					autocomplete="off"
					maxlength="120"
					placeholder={t('doc.namePlaceholder')}
					aria-label={t('doc.namePlaceholder')}
					class="input input-sm w-full max-w-56"
					onkeydown={(e) => e.key === 'Escape' && cancelCreate()}
				/>
				<button type="submit" class="btn btn-primary btn-sm" disabled={createSubmitting}>
					{createSubmitting ? t('doc.adding') : t('doc.add')}
				</button>
				<button type="button" class="btn btn-ghost btn-sm" onclick={cancelCreate}>
					{t('doc.cancel')}
				</button>
			</form>
			{#if createError}<p class="mt-1 text-xs text-error">{createError}</p>{/if}
		</div>
	</li>
{/snippet}

<div class="mx-auto w-full max-w-7xl px-6 py-8">
	<header class="flex flex-wrap items-end justify-between gap-3">
		<div>
			<h1 class="text-2xl font-semibold tracking-[-0.02em]">{t('doc.title')}</h1>
			<p class="mt-1.5 text-sm text-muted">
				{t('doc.desc')}
				{#if totalCount > 0}
					<span aria-hidden="true"> · </span>
					<span class="font-mono text-xs">
						{t(totalCount === 1 ? 'doc.countOne' : 'doc.countMany', { n: totalCount })}
					</span>
				{/if}
			</p>
		</div>
		{#if canCreate}
			<div class="flex flex-wrap items-center gap-2">
				<Button variant="ghost" onclick={openTemplates}>
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
						<path d="M12 11v5M9.5 13.5h5" />
					</svg>
					{t('tpl.open')}
				</Button>
				<Button onclick={() => startCreate(ROOT)}>
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
						<path d="M12 5v14M5 12h14" />
					</svg>
					{t('doc.newFolder')}
				</Button>
			</div>
		{/if}
	</header>

	<div class="mt-6 grid gap-6 lg:grid-cols-[minmax(17rem,20rem)_1fr] lg:items-start">
		<!-- Index rail -->
		<nav
			aria-label={t('doc.index')}
			ondragover={railDragOver}
			ondragleave={railDragLeave}
			ondrop={railDrop}
			class="flex min-h-64 flex-col rounded-box border bg-base-100 transition-colors lg:sticky lg:top-6 lg:max-h-[calc(100dvh-3rem)]
				{dropTarget === ROOT ? 'border-primary/50 bg-primary/[0.04]' : 'border-base-content/10'}"
		>
			<div class="flex-none border-b border-base-content/8">
				<div class="flex items-center justify-between gap-2 px-3 py-2">
					<h2 class="text-xs font-medium text-muted">{t('doc.index')}</h2>
					<div class="flex items-center gap-1">
						{#if canDelete && folders.length > 0 && !selectMode}
							<button
								type="button"
								onclick={() => (selectMode = true)}
								class="rounded-field px-1.5 py-0.5 text-xs text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
							>
								{t('doc.select.enter')}
							</button>
						{/if}
						{#if branchIds.length > 0}
							<button
								type="button"
								onclick={() => setAll(anyCollapsed)}
								class="rounded-field px-1.5 py-0.5 text-xs text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
							>
								{anyCollapsed ? t('doc.expandAll') : t('doc.collapseAll')}
							</button>
						{/if}
					</div>
				</div>

				{#if selectMode}
					<div
						class="flex items-center justify-between gap-2 border-t border-base-content/8 bg-primary/[0.04] px-3 py-1.5"
					>
						<span class="font-mono text-xs tabular-nums" aria-live="polite">
							{t('doc.select.count', { n: selectedFolders.size })}
						</span>
						<div class="flex items-center gap-1">
							<button
								type="button"
								onclick={openBulkDelete}
								disabled={selectedFolders.size === 0}
								class="rounded-field px-1.5 py-0.5 text-xs font-medium text-error transition-colors hover:bg-error/10 disabled:pointer-events-none disabled:opacity-40"
							>
								{t('doc.select.delete')}
							</button>
							<button
								type="button"
								onclick={exitSelect}
								class="rounded-field px-1.5 py-0.5 text-xs text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
							>
								{t('doc.cancel')}
							</button>
						</div>
					</div>
				{/if}

				{#if folders.length > 0}
					<div class="relative px-2 pb-2">
						<input
							type="search"
							bind:value={query}
							placeholder={t('doc.search.placeholder')}
							aria-label={t('doc.search.label')}
							autocomplete="off"
							class="input input-sm w-full pr-7"
						/>
						{#if query}
							<button
								type="button"
								onclick={() => (query = '')}
								aria-label={t('doc.search.clear')}
								class="absolute inset-y-0 right-3.5 my-auto grid h-6 w-6 place-items-center rounded-field text-muted transition-colors hover:bg-base-content/5 hover:text-base-content"
							>
								<svg
									class="h-3.5 w-3.5"
									viewBox="0 0 24 24"
									fill="none"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
									aria-hidden="true"
								>
									<path d="M18 6 6 18M6 6l12 12" />
								</svg>
							</button>
						{/if}
					</div>
				{/if}
			</div>

			{#if folders.length === 0 && creatingParent === null}
				{#if canCreate && !noAccess}
					<!-- Empty room: the five template cards inline — zero clicks to see
					     the options; manual creation stays as a secondary link. -->
					<div class="flex flex-1 flex-col gap-3 px-4 py-5">
						<div>
							<p class="text-sm font-medium">{t('tpl.title')}</p>
							<p class="mt-1 text-xs text-muted text-pretty">{t('tpl.desc')}</p>
						</div>
						<FolderTemplatePicker workspaceId={workspace.id} {folders} {actionBase} />
						<button
							type="button"
							onclick={() => startCreate(ROOT)}
							class="self-center text-xs text-muted underline decoration-base-content/30 underline-offset-2 transition-colors hover:text-base-content"
						>
							{t('tpl.manualLink')}
						</button>
					</div>
				{:else}
					<div
						class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center"
					>
						<svg
							class="h-8 w-8 text-muted/70"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.4"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
							<path d="M12 11v5M9.5 13.5h5" />
						</svg>
						<div>
							<p class="text-sm font-medium">
								{noAccess ? t('doc.noAccess.title') : t('doc.empty.title')}
							</p>
							<p class="mx-auto mt-1 max-w-xs text-xs text-muted text-pretty">
								{noAccess ? t('doc.noAccess.body') : t('doc.empty.body')}
							</p>
						</div>
						{#if noAccess}
							<span></span>
						{:else}
							<p class="text-xs text-muted">{t('doc.empty.readonly')}</p>
						{/if}
					</div>
				{/if}
			{:else if matched && rows.length === 0}
				<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
					<p class="text-sm text-muted text-pretty">{t('doc.search.none', { q: query.trim() })}</p>
					<button
						type="button"
						onclick={() => (query = '')}
						class="rounded-field px-1.5 py-1 text-xs font-medium text-muted transition-colors hover:text-primary"
					>
						{t('doc.search.clear')}
					</button>
				</div>
			{:else}
				<ul class="flex-1 overflow-y-auto py-1">
					{#each rows as row (row.node.id)}
						{@const node = row.node}
						{@const open = row.open}
						{@const renaming = renamingId === node.id}
						{@const active = activeId === node.id}
						{@const targeted = dropTarget === node.id}
						{@const into = targeted && dropEdge === 'into'}
						<li
							data-folder-id={node.id}
							draggable={canEdit && !node.is_default && !renaming && !selectMode}
							ondragstart={(e) => rowDragStart(e, node)}
							ondragover={(e) => rowDragOver(e, row)}
							ondragleave={(e) => rowDragLeave(e, node)}
							ondrop={(e) => rowDrop(e, row)}
							class="group relative flex items-start gap-1.5 py-1.5 pr-1 transition-colors
								{into
								? 'bg-primary/8 ring-1 ring-primary/40 ring-inset'
								: active
									? 'bg-primary/6'
									: 'hover:bg-base-content/[0.045]'}
								{draggingId === node.id ? 'opacity-40' : ''}
								{pendingId === node.id ? 'opacity-60' : ''}"
							style={indent(row.depth)}
						>
							{#if targeted && dropEdge !== 'into'}
								<span
									class="rakda-dropline pointer-events-none absolute right-1 {dropEdge === 'before'
										? '-top-px'
										: '-bottom-px'}"
									style="left: {indentRem(row.depth)}"
									aria-hidden="true"
								></span>
							{/if}
							{#if selectMode}
								{#if node.is_default}
									<span class="w-5 flex-none"></span>
								{:else}
									<input
										type="checkbox"
										checked={selectedFolders.has(node.id)}
										onchange={() => toggleSelected(node.id)}
										aria-label={t('doc.select.itemLabel', { name: node.name })}
										class="checkbox checkbox-sm checkbox-primary mt-0.5 flex-none"
									/>
								{/if}
							{/if}
							{#if row.hasChildren}
								<button
									type="button"
									onclick={() => toggle(node.id, row.depth)}
									aria-expanded={open}
									aria-label={open ? t('doc.collapse') : t('doc.expand')}
									class="grid h-6 w-6 flex-none place-items-center rounded text-muted transition-colors hover:text-base-content pointer-coarse:h-9 pointer-coarse:w-9"
								>
									<svg
										class="rakda-caret h-3.5 w-3.5 {open ? 'rotate-90' : ''}"
										viewBox="0 0 24 24"
										fill="none"
										stroke="currentColor"
										stroke-width="2"
										stroke-linecap="round"
										stroke-linejoin="round"
										aria-hidden="true"
									>
										<path d="m9 6 6 6-6 6" />
									</svg>
								</button>
							{:else}
								<span class="w-6 flex-none pointer-coarse:w-9"></span>
							{/if}

							<span class="mt-1">{@render folderIcon((open && row.hasChildren) || active)}</span>

							{#if renaming}
								<div class="min-w-0 flex-1">
									<form
										method="POST"
										action="{actionBase}?/rename"
										use:enhance={submitRename}
										class="flex flex-wrap items-center gap-1.5"
									>
										<input type="hidden" name="folderId" value={node.id} />
										<input
											id="folder-rename-input"
											name="name"
											value={node.name}
											autocomplete="off"
											maxlength="120"
											aria-label={t('doc.action.rename')}
											class="input input-sm w-full max-w-56"
											onkeydown={(e) => e.key === 'Escape' && cancelRename()}
										/>
										<button
											type="submit"
											class="btn btn-primary btn-sm"
											disabled={renameSubmitting}
										>
											{renameSubmitting ? t('doc.saving') : t('doc.save')}
										</button>
										<button type="button" class="btn btn-ghost btn-sm" onclick={cancelRename}>
											{t('doc.cancel')}
										</button>
									</form>
									{#if renameError}<p class="mt-1 text-xs text-error">{renameError}</p>{/if}
								</div>
							{:else}
								<a
									href={rowHref(node.id)}
									draggable="false"
									aria-current={active ? 'page' : undefined}
									aria-keyshortcuts={canEdit && !node.is_default
										? 'Alt+ArrowUp Alt+ArrowDown'
										: undefined}
									class="mt-0.5 flex min-w-0 flex-1 items-baseline gap-1.5 rounded-field no-underline"
								>
									<span class="font-mono text-xs tabular-nums text-muted">{node.number}</span>
									<span class="min-w-0 flex-1 truncate text-sm {active ? 'font-medium' : ''}">
										{node.name}
									</span>
								</a>

								{#if node.is_default}
									<span
										class="mt-0.5 flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 text-[0.6875rem] text-muted"
									>
										{t('doc.defaultBadge')}
									</span>
								{/if}

								{#if row.hasChildren && !open}
									<span
										class="mt-0.5 flex-none rounded-selector bg-base-content/5 px-1.5 py-0.5 font-mono text-[0.6875rem] text-muted"
										title={t(
											node.children.length === 1 ? 'doc.childCountOne' : 'doc.childCountMany',
											{ n: node.children.length }
										)}
									>
										{node.children.length}
									</span>
								{/if}

								{#if canAct}
									<div
										class="-mt-1 ml-1 flex flex-none items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100 pointer-coarse:-mt-2.5 pointer-coarse:gap-1 pointer-coarse:opacity-100"
									>
										{#if canCreate}
											<button
												type="button"
												onclick={() => startCreate(node.id)}
												title={t('doc.action.addSub')}
												aria-label={t('doc.action.addSubOf', { name: node.name })}
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
													<path d="M12 8v8M8 12h8" />
												</svg>
											</button>
										{/if}
										{#if canEdit}
											<button
												type="button"
												onclick={() => startRename(node)}
												title={t('doc.action.rename')}
												aria-label={t('doc.action.renameOf', { name: node.name })}
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
													<path d="M12 20h9" />
													<path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4z" />
												</svg>
											</button>
											{#if !node.is_default}
												<button
													type="button"
													onclick={() => openMove(node)}
													title="{t('doc.action.move')} · {t('doc.reorder.up')}/{t(
														'doc.reorder.down'
													)}: Alt+↑ / Alt+↓"
													aria-keyshortcuts="Alt+ArrowUp Alt+ArrowDown"
													aria-label={t('doc.action.moveOf', { name: node.name })}
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
										{/if}
										{#if canAssign}
											<a
												href={accessHref(node.id)}
												draggable="false"
												title={t('doc.action.access')}
												aria-label={t('doc.action.accessOf', { name: node.name })}
												class="grid h-8 w-8 place-items-center rounded-field text-muted no-underline transition-colors hover:bg-base-content/5 hover:text-base-content pointer-coarse:h-11 pointer-coarse:w-11"
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
													<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
													<circle cx="9" cy="7" r="4" />
													<path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
												</svg>
											</a>
										{/if}
										{#if canDelete && !node.is_default}
											<button
												type="button"
												onclick={() => openDelete(node)}
												title={t('doc.action.delete')}
												aria-label={t('doc.action.deleteOf', { name: node.name })}
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
								{/if}
							{/if}
						</li>

						{#if creatingParent === node.id}
							{@render createInputRow(row.depth + 1, node.id)}
						{/if}
					{/each}

					{#if creatingParent === ROOT}
						{@render createInputRow(0, ROOT)}
					{/if}
				</ul>

				{#if canCreate && creatingParent !== ROOT}
					<button
						type="button"
						onclick={() => startCreate(ROOT)}
						class="m-2 mt-1 inline-flex flex-none items-center gap-1.5 self-start rounded-field px-1.5 py-1 text-xs font-medium text-muted transition-colors hover:text-primary"
					>
						<svg
							class="h-3.5 w-3.5"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							stroke-width="1.8"
							stroke-linecap="round"
							stroke-linejoin="round"
							aria-hidden="true"
						>
							<path d="M12 5v14M5 12h14" />
						</svg>
						{t('doc.newRootFolder')}
					</button>
				{/if}
			{/if}
		</nav>

		<!-- Documents pane -->
		<div class="min-w-0">
			{@render children()}
		</div>
	</div>
</div>

{#if fileDragging && canUpload}
	<div
		class="rakda-overlay pointer-events-none fixed inset-x-0 top-4 z-overlay flex justify-center px-4"
		aria-hidden="true"
	>
		<div
			class="max-w-full rounded-box border border-primary/40 bg-base-100/95 px-3.5 py-2 shadow-sm"
		>
			<p class="text-xs text-pretty">{dropHint}</p>
		</div>
	</div>
{/if}

<div aria-live="polite" class="sr-only">
	{#if dropMessage}
		{dropMessage}
	{/if}
</div>

<UploadQueue />

<!-- Move -->
<dialog bind:this={moveDialog} class="modal" aria-labelledby="folder-move-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="folder-move-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('doc.move.title')}
		</h2>
		{#if moving}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('doc.move.desc', { name: moving.name })}
			</p>
		{/if}

		{#if moveError}
			<div class="mt-4"><Alert align="start">{moveError}</Alert></div>
		{/if}

		<form method="POST" action="{actionBase}?/move" use:enhance={submitMove} class="mt-5">
			<input type="hidden" name="folderId" value={moving?.id ?? ''} />
			<label for="move-dest" class="mb-1.5 block text-sm font-medium">{t('doc.move.dest')}</label>
			<select
				id="move-dest"
				name="parentId"
				bind:value={moveTarget}
				class="select select-sm w-full font-mono"
			>
				<option value={ROOT}>{t('doc.move.root')}</option>
				{#each moveOptions as opt (opt.id)}
					<option value={opt.id}>{'  '.repeat(opt.depth)}{opt.number} {opt.name}</option>
				{/each}
			</select>

			<div class="mt-6 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => moveDialog?.close()}>
					{t('doc.cancel')}
				</Button>
				<Button type="submit" loading={moveSubmitting}>
					{moveSubmitting ? t('doc.move.submitting') : t('doc.move.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('doc.cancel')}></button>
	</form>
</dialog>

<!-- Delete -->
<dialog bind:this={deleteDialog} class="modal" aria-labelledby="folder-delete-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="folder-delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('doc.delete.title')}
		</h2>
		{#if deleting}
			<p class="mt-1 text-sm text-muted text-pretty">
				{t('doc.delete.warning', { name: deleting.name })}
			</p>
			<p class="mt-2 text-sm font-medium text-error text-pretty">{t('doc.delete.contents')}</p>
			{#if deletingKids > 0}
				<p class="mt-1 text-sm text-error text-pretty">
					{t(deletingKids === 1 ? 'doc.delete.cascadeOne' : 'doc.delete.cascadeMany', {
						n: deletingKids
					})}
				</p>
			{/if}
		{/if}

		{#if deleteError}
			<div class="mt-4"><Alert align="start">{deleteError}</Alert></div>
		{/if}

		<form
			method="POST"
			action="{actionBase}?/delete"
			use:enhance={submitDelete}
			class="mt-5 flex flex-col gap-4"
		>
			<input type="hidden" name="folderId" value={deleting?.id ?? ''} />
			<Field
				id="folder-delete-confirm"
				name="confirm"
				label={t('doc.delete.confirmLabel', { name: deleting?.name ?? '' })}
				bind:value={deleteConfirm}
				placeholder={deleting?.name ?? ''}
				autocomplete="off"
			/>
			<div class="mt-1 flex flex-wrap justify-end gap-2">
				<Button type="button" variant="ghost" onclick={() => deleteDialog?.close()}>
					{t('doc.cancel')}
				</Button>
				<Button type="submit" variant="danger" disabled={!deleteReady} loading={deleteSubmitting}>
					{deleteSubmitting ? t('doc.delete.submitting') : t('doc.delete.submit')}
				</Button>
			</div>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('doc.cancel')}></button>
	</form>
</dialog>

<dialog bind:this={bulkDialog} class="modal" aria-labelledby="folder-bulk-delete-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="folder-bulk-delete-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('doc.select.foldersTitle')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">
			{t('doc.select.folders', { n: selectedFolders.size })}
		</p>

		{#if bulkError}
			<div class="mt-4"><Alert align="start">{bulkError}</Alert></div>
		{/if}

		<form
			method="POST"
			action="{actionBase}?/bulkDelete"
			use:enhance={submitBulkDelete}
			class="mt-6 flex flex-wrap justify-end gap-2"
		>
			{#each [...selectedFolders] as id (id)}
				<input type="hidden" name="folderId" value={id} />
			{/each}
			<Button type="button" variant="ghost" onclick={() => bulkDialog?.close()}>
				{t('doc.cancel')}
			</Button>
			<Button type="submit" variant="danger" loading={bulkSubmitting}>
				{bulkSubmitting ? t('doc.delete.submitting') : t('doc.delete.submit')}
			</Button>
		</form>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('doc.cancel')}></button>
	</form>
</dialog>

<dialog bind:this={templateDialog} class="modal" aria-labelledby="folder-template-title">
	<div class="modal-box max-w-md rounded-box border border-base-content/10 bg-base-100 p-6">
		<h2 id="folder-template-title" class="text-lg font-semibold tracking-[-0.01em]">
			{t('tpl.title')}
		</h2>
		<p class="mt-1 text-sm text-muted text-pretty">{t('tpl.desc')}</p>

		<div class="mt-5">
			{#if templatesOpened}
				<FolderTemplatePicker
					workspaceId={workspace.id}
					{folders}
					{actionBase}
					onApplied={() => templateDialog?.close()}
				/>
			{/if}
		</div>

		<div class="mt-6 flex justify-end">
			<Button type="button" variant="ghost" onclick={() => templateDialog?.close()}>
				{t('doc.cancel')}
			</Button>
		</div>
	</div>
	<form method="dialog" class="modal-backdrop">
		<button aria-label={t('doc.cancel')}></button>
	</form>
</dialog>

<Toaster />

<style>
	.rakda-caret {
		transition: transform 150ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	.rakda-overlay {
		animation: rakda-fade-in 150ms ease-out;
	}
	/* The insertion point reads as a caret, not a border: a 2px rule with a knob
	   at its left end, starting at the indent of the level being joined. */
	.rakda-dropline {
		height: 2px;
		border-radius: 1px;
		background: var(--color-primary);
	}
	.rakda-dropline::before {
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
	@keyframes rakda-fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.rakda-caret {
			transition: none;
		}
		.rakda-overlay {
			animation: none;
		}
	}
</style>
