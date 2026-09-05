import { maxDepthOf, toBulkNodes, type DroppedTree } from '$lib/dnd';
import { t } from '$lib/i18n';
import type { BulkCreateFolderData, BulkFolderNode } from '$lib/types/content';
import { messageOf, uploadQueue } from './queue.svelte';

// Mirrors the server's own limits so the drop is refused before anything is
// created, rather than half-built and then rejected.
const MAX_NODES = 500;
const MAX_DEPTH = 32;

export interface DropTarget {
	id: string;
	name: string;
}

// Two different questions, and a drop at the room root answers them differently:
// folders belong at the root, but files cannot live there, so loose files go to
// the default folder instead. Inside a folder both answers are that folder.
export interface TreeDestination {
	// Parent for the dropped folders. '' = the room root.
	parentId: string;
	// Where files sitting at the top of the drop go. Null when the room has no
	// default folder — then loose files have nowhere legal to land.
	loose: DropTarget | null;
}

// Creates the folder structure in one transaction, then hands each file to the
// upload queue against the folder it belongs in. Files sitting at the top of the
// drop go to the target folder itself.
export async function uploadTree(
	workspaceId: string,
	dest: TreeDestination,
	tree: DroppedTree
): Promise<void> {
	if (!tree.files.length && !tree.folders.length) return;

	const hasLoose = tree.files.some((f) => !f.path.length);
	if (hasLoose && !dest.loose) throw new Error(t('doc.upload.noDefault'));

	if (tree.folders.length > MAX_NODES) {
		throw new Error(t('doc.drop.err.tooMany', { n: tree.folders.length, max: MAX_NODES }));
	}
	if (maxDepthOf(tree.folders) > MAX_DEPTH) {
		throw new Error(t('doc.drop.err.tooDeep', { max: MAX_DEPTH }));
	}

	const byPath = new Map<string, string>();

	if (tree.folders.length) {
		const res = await fetch('/api/content/folders/bulk', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({
				workspaceId,
				parentId: dest.parentId,
				folders: toBulkNodes(tree.folders) as BulkFolderNode[]
			})
		});
		if (!res.ok) throw new Error(await messageOf(res));

		const data = (await res.json()) as BulkCreateFolderData;
		for (const f of data.folders) byPath.set(f.path, f.id);
	}

	// One enqueue per destination folder keeps the panel's per-folder labelling
	// intact and lets the queue schedule across the whole drop.
	const groups = new Map<string, { name: string; files: File[] }>();

	for (const { file, path } of tree.files) {
		const key = path.join('/');
		const folderId = key ? byPath.get(key) : dest.loose?.id;
		// A folder the server did not return has no id to upload against; the
		// bulk call is atomic, so this only happens if it silently skipped one.
		if (!folderId) continue;

		const name = key ? path[path.length - 1] : (dest.loose?.name ?? '');
		let group = groups.get(folderId);
		if (!group) {
			group = { name, files: [] };
			groups.set(folderId, group);
		}
		group.files.push(file);
	}

	for (const [folderId, group] of groups) {
		uploadQueue.enqueue(workspaceId, folderId, group.name, group.files);
	}
}
