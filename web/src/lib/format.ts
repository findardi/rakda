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
