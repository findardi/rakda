import { t } from '$lib/i18n';

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

// Local calendar day, for dates a person chose ("valid until 30 Sep") —
// unlike formatDate, which stays UTC because audit facts must not drift.
const dateLocalFmt = new Intl.DateTimeFormat('id-ID', {
	day: '2-digit',
	month: 'short',
	year: 'numeric'
});
export function formatDateLocal(iso: string): string {
	const d = new Date(iso);
	return Number.isNaN(d.getTime()) ? '—' : dateLocalFmt.format(d);
}

// <input type="date"> speaks local calendar days. These two convert between
// that and an instant: the end of the chosen day in the browser's timezone is
// what "until 30 Sep" means to the person typing it, so the browser decides,
// not the server.
export function localDayString(d: Date): string {
	const pad = (n: number) => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}
export function localDayEnd(ymd: string): Date {
	const [y, m, d] = ymd.split('-').map(Number);
	return new Date(y, m - 1, d, 23, 59, 59);
}
// Tomorrow as a local calendar day — the earliest expiry worth offering, since
// a window that closes today would lapse within hours of being set.
export function localTomorrowString(): string {
	const d = new Date();
	d.setDate(d.getDate() + 1);
	return localDayString(d);
}

export function formatTimeUtc(iso: string): string {
	const d = new Date(iso);
	return Number.isNaN(d.getTime()) ? '—' : d.toISOString().slice(11, 16);
}

// Read durations are browser-reported and indicative; anything under a second
// is noise, and an em dash says "nothing recorded" more honestly than "0s".
export function formatDuration(ms: number): string {
	if (!Number.isFinite(ms) || ms < 1000) return '—';

	const seconds = Math.round(ms / 1000);
	if (seconds < 60) return t('fmt.dur.s', { s: seconds });

	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) {
		const rest = seconds % 60;
		return rest === 0 ? t('fmt.dur.m', { m: minutes }) : t('fmt.dur.ms', { m: minutes, s: rest });
	}

	const hours = Math.floor(minutes / 60);
	const rest = minutes % 60;
	return rest === 0 ? t('fmt.dur.h', { h: hours }) : t('fmt.dur.hm', { h: hours, m: rest });
}

export function formatDateTime(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return '—';
	const s = d.toISOString();
	return `${s.slice(0, 10)} ${s.slice(11, 16)} UTC`;
}

// "A, B, +3" — the first `max` names in full, then a count of the rest. Used
// wherever a scoped archive package names its folders.
export function formatNameList(names: string[], max = 2): string {
	if (names.length <= max) return names.join(', ');
	return `${names.slice(0, max).join(', ')}, ${t('archive.scope.more', { n: names.length - max })}`;
}
