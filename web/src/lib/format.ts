const UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];

export function formatBytes(bytes: number): string {
	if (bytes <= 0) return '0 B';

	let value = bytes;
	let unit = 0;
	while (value >= 1024 && unit < UNITS.length - 1) {
		value /= 1024;
		unit++;
	}

	const digits = unit === 0 || value >= 100 ? 0 : value >= 10 ? 1 : 2;
	return `${value.toFixed(digits)} ${UNITS[unit]}`;
}

// UTC so server and client render the same string; a data room's timeline is
// an audit fact, not a local convenience.
export function formatDate(iso: string): string {
	const d = new Date(iso);
	return Number.isNaN(d.getTime()) ? '—' : d.toISOString().slice(0, 10);
}

export function formatTimeUtc(iso: string): string {
	const d = new Date(iso);
	return Number.isNaN(d.getTime()) ? '—' : d.toISOString().slice(11, 16);
}

export function formatDateTime(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '—';
	const s = d.toISOString();
	return `${s.slice(0, 10)} ${s.slice(11, 16)} UTC`;
}
