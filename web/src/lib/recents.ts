// Per-device visit history for the overview page, stored in localStorage —
// deliberately NOT a server feature: guests never read activity data (their
// own included), and folder visits are not recorded server-side at all. Same
// contract as "Mode privasi": a reader convenience owners cannot see or force.

export interface RecentFolder {
	id: string;
	name: string;
	at: number;
}

export interface RecentDocument {
	id: string;
	name: string;
	folderId: string;
	at: number;
}

interface RecentsStore {
	folders: RecentFolder[];
	documents: RecentDocument[];
}

const LIMIT = 5;

const keyFor = (workspaceId: string) => `rakda:recents:${workspaceId}`;

function read(workspaceId: string): RecentsStore {
	try {
		const raw = localStorage.getItem(keyFor(workspaceId));
		if (!raw) return { folders: [], documents: [] };
		const parsed = JSON.parse(raw) as Partial<RecentsStore>;
		return {
			folders: Array.isArray(parsed.folders) ? parsed.folders : [],
			documents: Array.isArray(parsed.documents) ? parsed.documents : []
		};
	} catch {
		return { folders: [], documents: [] };
	}
}

function write(workspaceId: string, store: RecentsStore) {
	try {
		localStorage.setItem(keyFor(workspaceId), JSON.stringify(store));
	} catch {
		// Storage full or blocked — the list is a convenience, never required.
	}
}

export function readRecents(workspaceId: string): RecentsStore {
	return read(workspaceId);
}

export function recordFolderVisit(workspaceId: string, folder: { id: string; name: string }) {
	const store = read(workspaceId);
	store.folders = [
		{ id: folder.id, name: folder.name, at: Date.now() },
		...store.folders.filter((f) => f.id !== folder.id)
	].slice(0, LIMIT);
	write(workspaceId, store);
}

export function recordDocumentVisit(
	workspaceId: string,
	doc: { id: string; name: string; folderId: string }
) {
	const store = read(workspaceId);
	store.documents = [
		{ id: doc.id, name: doc.name, folderId: doc.folderId, at: Date.now() },
		...store.documents.filter((d) => d.id !== doc.id)
	].slice(0, LIMIT);
	write(workspaceId, store);
}
