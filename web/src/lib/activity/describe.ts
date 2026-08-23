import { roleDisplayName } from '$lib/access/permissions';
import { t, type TKey } from '$lib/i18n';
import type { ActivityItem } from '$lib/types/activity';

export type ActivityTone = 'neutral' | 'positive' | 'negative';

export interface ActivityPhrase {
	key: TKey | null;
	vars: Record<string, string | number>;
}

export const ACTIVITY_GROUPS: { key: TKey; actions: string[] }[] = [
	{
		key: 'activity.group.folder',
		actions: [
			'folder_created',
			'folder_renamed',
			'folder_moved',
			'folder_deleted',
			'folder_restored',
			'folder_purged'
		]
	},
	{
		key: 'activity.group.document',
		actions: [
			'document_uploaded',
			'document_moved',
			'document_deleted',
			'document_restored',
			'document_purged',
			'document_downloaded',
			'document_viewed',
			'search_performed'
		]
	},
	{
		key: 'activity.group.version',
		actions: ['version_uploaded', 'version_restored', 'rendition_retried']
	},
	{
		key: 'activity.group.member',
		actions: [
			'invite_sent',
			'invite_resent',
			'invite_revoked',
			'invite_accepted',
			'invite_rejected',
			'member_removed',
			'role_changed'
		]
	},
	{
		key: 'activity.group.group',
		actions: [
			'group_created',
			'group_updated',
			'group_deleted',
			'group_assigned',
			'group_unassigned'
		]
	},
	{ key: 'activity.group.access', actions: ['folder_access_changed', 'folder_access_removed'] }
];

const PHRASE_KEY: Record<string, TKey> = {
	folder_created: 'activity.action.folder_created',
	folder_renamed: 'activity.action.folder_renamed',
	folder_moved: 'activity.action.folder_moved',
	folder_deleted: 'activity.action.folder_deleted',
	folder_restored: 'activity.action.folder_restored',
	folder_purged: 'activity.action.folder_purged',
	document_uploaded: 'activity.action.document_uploaded',
	document_moved: 'activity.action.document_moved',
	document_deleted: 'activity.action.document_deleted',
	document_restored: 'activity.action.document_restored',
	document_purged: 'activity.action.document_purged',
	document_downloaded: 'activity.action.document_downloaded',
	document_viewed: 'activity.action.document_viewed',
	version_uploaded: 'activity.action.version_uploaded',
	version_restored: 'activity.action.version_restored',
	rendition_retried: 'activity.action.rendition_retried',
	search_performed: 'activity.action.search_performed',
	invite_sent: 'activity.action.invite_sent',
	invite_resent: 'activity.action.invite_resent',
	invite_revoked: 'activity.action.invite_revoked',
	invite_accepted: 'activity.action.invite_accepted',
	invite_rejected: 'activity.action.invite_rejected',
	member_removed: 'activity.action.member_removed',
	role_changed: 'activity.action.role_changed',
	group_created: 'activity.action.group_created',
	group_updated: 'activity.action.group_updated',
	group_deleted: 'activity.action.group_deleted',
	group_assigned: 'activity.action.group_assigned',
	group_unassigned: 'activity.action.group_unassigned',
	folder_access_changed: 'activity.action.folder_access_changed',
	folder_access_removed: 'activity.action.folder_access_removed'
};

const LABEL_KEY: Record<string, TKey> = {
	folder_created: 'activity.label.folder_created',
	folder_renamed: 'activity.label.folder_renamed',
	folder_moved: 'activity.label.folder_moved',
	folder_deleted: 'activity.label.folder_deleted',
	folder_restored: 'activity.label.folder_restored',
	folder_purged: 'activity.label.folder_purged',
	document_uploaded: 'activity.label.document_uploaded',
	document_moved: 'activity.label.document_moved',
	document_deleted: 'activity.label.document_deleted',
	document_restored: 'activity.label.document_restored',
	document_purged: 'activity.label.document_purged',
	document_downloaded: 'activity.label.document_downloaded',
	document_viewed: 'activity.label.document_viewed',
	version_uploaded: 'activity.label.version_uploaded',
	version_restored: 'activity.label.version_restored',
	rendition_retried: 'activity.label.rendition_retried',
	search_performed: 'activity.label.search_performed',
	invite_sent: 'activity.label.invite_sent',
	invite_resent: 'activity.label.invite_resent',
	invite_revoked: 'activity.label.invite_revoked',
	invite_accepted: 'activity.label.invite_accepted',
	invite_rejected: 'activity.label.invite_rejected',
	member_removed: 'activity.label.member_removed',
	role_changed: 'activity.label.role_changed',
	group_created: 'activity.label.group_created',
	group_updated: 'activity.label.group_updated',
	group_deleted: 'activity.label.group_deleted',
	group_assigned: 'activity.label.group_assigned',
	group_unassigned: 'activity.label.group_unassigned',
	folder_access_changed: 'activity.label.folder_access_changed',
	folder_access_removed: 'activity.label.folder_access_removed'
};

const DESTRUCTIVE = new Set([
	'folder_deleted',
	'folder_purged',
	'document_deleted',
	'document_purged',
	'invite_revoked',
	'member_removed',
	'group_deleted',
	'group_unassigned',
	'folder_access_removed'
]);

const RESTORATIVE = new Set([
	'folder_restored',
	'document_restored',
	'version_restored',
	'invite_accepted'
]);

const CAPABILITIES: [string, TKey][] = [
	['can_view', 'facc.cap.view'],
	['can_download', 'facc.cap.download'],
	['can_watermark', 'facc.cap.watermark'],
	['can_download_original', 'facc.cap.downloadOriginal']
];

const text = (v: unknown): string => (typeof v === 'string' ? v : '');
const number = (v: unknown): number => (typeof v === 'number' ? v : 0);

function capabilitySummary(meta: Record<string, unknown>): string {
	const granted = CAPABILITIES.filter(([flag]) => meta[flag] === true).map(([, key]) =>
		t(key).toLocaleLowerCase()
	);
	return granted.length > 0 ? granted.join(', ') : t('level.none').toLocaleLowerCase();
}

export function activityActionLabel(action: string): string {
	const key = LABEL_KEY[action];
	return key ? t(key) : action;
}

export function activityTone(action: string): ActivityTone {
	if (RESTORATIVE.has(action)) return 'positive';
	if (DESTRUCTIVE.has(action)) return 'negative';
	return 'neutral';
}

export function describeActivity(item: ActivityItem): ActivityPhrase {
	const meta = item.metadata ?? {};
	const vars: Record<string, string | number> = { target: item.target_name };

	switch (item.action) {
		case 'folder_created':
			return meta.bulk === true
				? { key: 'activity.action.folder_created_bulk', vars: { count: number(meta.count) } }
				: { key: 'activity.action.folder_created', vars };

		case 'folder_moved':
			return text(meta.to_parent_id)
				? { key: 'activity.action.folder_moved', vars }
				: { key: 'activity.action.folder_moved_root', vars };

		case 'folder_renamed':
			return {
				key: 'activity.action.folder_renamed',
				vars: { from: text(meta.from), to: text(meta.to) }
			};

		case 'role_changed':
			return {
				key: 'activity.action.role_changed',
				vars: {
					...vars,
					from: roleDisplayName(text(meta.from)),
					to: roleDisplayName(text(meta.to))
				}
			};

		case 'invite_sent':
			return {
				key: 'activity.action.invite_sent',
				vars: { ...vars, role: roleDisplayName(text(meta.role)) }
			};

		case 'folder_access_changed':
			return {
				key: 'activity.action.folder_access_changed',
				vars: { ...vars, caps: capabilitySummary(meta) }
			};

		case 'folder_purged':
		case 'document_purged':
			return item.target_name
				? { key: PHRASE_KEY[item.action], vars }
				: {
						key:
							item.action === 'folder_purged'
								? 'activity.action.folder_purged_unnamed'
								: 'activity.action.document_purged_unnamed',
						vars: { id: item.target_id.slice(0, 8) }
					};

		case 'document_downloaded':
			return {
				key:
					meta.variant === 'watermarked'
						? 'activity.action.document_downloaded_watermarked'
						: meta.variant === 'clean'
							? 'activity.action.document_downloaded_clean'
							: 'activity.action.document_downloaded',
				vars: { ...vars, version: number(meta.version_no) }
			};

		case 'document_viewed':
		case 'version_uploaded':
		case 'version_restored':
		case 'rendition_retried':
			return { key: PHRASE_KEY[item.action], vars: { ...vars, version: number(meta.version_no) } };

		default:
			return { key: PHRASE_KEY[item.action] ?? null, vars };
	}
}
