// Drag payload types. A drag carrying `Files` is an upload from the OS; these
// two mark drags that started inside the app, and `dataTransfer.types` is the
// only part of a drag readable during `dragover` — so they are the switch.
export const FOLDER_MIME = 'application/x-rakda-folder';
export const DOCUMENT_MIME = 'application/x-rakda-document';

// `items` carries directory entries too; only real files survive. Falls back to
// `files` for browsers that leave `items` empty on drop.
export function filesFrom(dt: DataTransfer | null): File[] {
	if (!dt) return [];
	if (dt.items?.length) {
		const out: File[] = [];
		for (const item of Array.from(dt.items)) {
			if (item.kind !== 'file') continue;
			if (item.webkitGetAsEntry?.()?.isFile === false) continue;
			const f = item.getAsFile();
			if (f) out.push(f);
		}
		return out;
	}
	return Array.from(dt.files);
}

// --- dropped folders ---------------------------------------------------

export interface DroppedFile {
	file: File;
	// Folder segments relative to the drop target. Empty = sits at the target.
	path: string[];
}

export interface DroppedTree {
	files: DroppedFile[];
	// Every folder in the drop, as `a/b/c` paths — including empty ones, so the
	// structure the user dropped is the structure they get.
	folders: string[];
}

// Must be called synchronously inside the drop handler: `dataTransfer.items` is
// emptied once the event finishes, so the entries have to be grabbed before any
// await. Walking them afterwards is fine — the entries stay valid.
export function entriesFrom(dt: DataTransfer | null): FileSystemEntry[] {
	if (!dt?.items?.length) return [];
	const out: FileSystemEntry[] = [];
	for (const item of Array.from(dt.items)) {
		if (item.kind !== 'file') continue;
		const entry = item.webkitGetAsEntry?.();
		if (entry) out.push(entry);
	}
	return out;
}

export const hasDirectory = (entries: FileSystemEntry[]) => entries.some((e) => e.isDirectory);

// `readEntries` hands back a bounded batch (~100 in Chrome) and signals the end
// with an empty array. Calling it once truncates large folders silently, which
// is the classic way this feature ships broken.
function readAll(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
	return new Promise((resolve, reject) => {
		const out: FileSystemEntry[] = [];
		const step = () =>
			reader.readEntries((batch) => {
				if (!batch.length) {
					resolve(out);
					return;
				}
				out.push(...batch);
				step();
			}, reject);
		step();
	});
}

const fileOf = (entry: FileSystemFileEntry): Promise<File> =>
	new Promise((resolve, reject) => entry.file(resolve, reject));

async function walk(entry: FileSystemEntry, trail: string[], out: DroppedTree): Promise<void> {
	if (entry.isFile) {
		out.files.push({ file: await fileOf(entry as FileSystemFileEntry), path: trail });
		return;
	}
	if (!entry.isDirectory) return;

	const dir = entry as FileSystemDirectoryEntry;
	const next = [...trail, dir.name];
	out.folders.push(next.join('/'));

	for (const child of await readAll(dir.createReader())) {
		await walk(child, next, out);
	}
}

export async function readTree(entries: FileSystemEntry[]): Promise<DroppedTree> {
	const out: DroppedTree = { files: [], folders: [] };
	for (const entry of entries) await walk(entry, [], out);
	return out;
}

// The non-drag path: `<input webkitdirectory>` reports the same structure as a
// flat list with `webkitRelativePath` on each file. Empty folders are invisible
// to this API — a limitation of the input, not of the traversal above.
export function treeFromInput(files: File[]): DroppedTree {
	const out: DroppedTree = { files: [], folders: [] };
	const seen = new Set<string>();

	for (const file of files) {
		const rel = file.webkitRelativePath;
		const path = rel ? rel.split('/').slice(0, -1) : [];
		out.files.push({ file, path });

		for (let i = 1; i <= path.length; i++) {
			const key = path.slice(0, i).join('/');
			if (seen.has(key)) continue;
			seen.add(key);
			out.folders.push(key);
		}
	}
	return out;
}

// Nested payload for POST /folders/bulk. Sorting puts every parent ahead of its
// children, so each node finds its parent already built.
export function toBulkNodes(paths: string[]): Array<{ name: string; children: unknown[] }> {
	type Node = { name: string; children: Node[] };
	const roots: Node[] = [];
	const index = new Map<string, Node>();

	for (const p of [...paths].sort()) {
		const segs = p.split('/');
		const node: Node = { name: segs[segs.length - 1], children: [] };
		index.set(p, node);
		if (segs.length === 1) roots.push(node);
		else index.get(segs.slice(0, -1).join('/'))?.children.push(node);
	}
	return roots;
}

export const maxDepthOf = (paths: string[]) =>
	paths.reduce((max, p) => Math.max(max, p.split('/').length), 0);

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const isUuid = (v: string) => UUID_RE.test(v);

// A reorder sends `position` through a form field, so it arrives as a string.
// `null` means "no position given" — the server then appends. Anything that is
// not a non-negative integer is treated as absent rather than sent as garbage.
export function parsePosition(raw: FormDataEntryValue | null): number | null {
	if (raw === null) return null;
	const s = raw.toString().trim();
	if (!s) return null;
	const n = Number(s);
	return Number.isInteger(n) && n >= 0 ? n : null;
}
