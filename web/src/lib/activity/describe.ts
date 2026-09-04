import { roleDisplayName } from '$lib/access/permissions';
import { formatDateLocal, formatNameList } from '$lib/format';
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
			'folder_purged',
			'template_applied'
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
			'role_changed',
			'member_expiry_changed'
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
	{ key: 'activity.group.access', actions: ['folder_access_changed', 'folder_access_removed'] },
	{
		key: 'activity.group.qa',
		actions: [
			'question_submitted',
			'question_replied',
			'question_answered',
			'question_closed',
			'question_reopened',
			'faq_published',
			'qa_settings_changed'
		]
	},
	{
		key: 'activity.group.room',
		actions: [
			'workspace_updated',
			'workspace_status_changed',
			'workspace_branding_changed',
			'archive_exported'
		]
	}
];

const STATUS_KEY: Record<string, TKey> = {
	prepare: 'ws.status.prepare',
	active: 'ws.status.active',
	archive: 'ws.status.archive'
};

function statusDisplayName(status: string): string {
	const key = STATUS_KEY[status];
	return key ? t(key) : status;
}

const PHRASE_KEY: Record<string, TKey> = {
	folder_created: 'activity.action.folder_created',
	folder_renamed: 'activity.action.folder_renamed',
	folder_moved: 'activity.action.folder_moved',
	folder_deleted: 'activity.action.folder_deleted',
	folder_restored: 'activity.action.folder_restored',
	folder_purged: 'activity.action.folder_purged',
	template_applied: 'activity.action.template_applied',
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
	member_expiry_changed: 'activity.action.member_expiry_changed',
	group_created: 'activity.action.group_created',
	group_updated: 'activity.action.group_updated',
	group_deleted: 'activity.action.group_deleted',
	group_assigned: 'activity.action.group_assigned',
	group_unassigned: 'activity.action.group_unassigned',
	folder_access_changed: 'activity.action.folder_access_changed',
	folder_access_removed: 'activity.action.folder_access_removed',
	question_submitted: 'activity.action.question_submitted',
	question_replied: 'activity.action.question_replied',
	question_answered: 'activity.action.question_answered',
	question_closed: 'activity.action.question_closed',
	question_reopened: 'activity.action.question_reopened',
	faq_published: 'activity.action.faq_published',
	qa_settings_changed: 'activity.action.qa_settings_changed',
	archive_exported: 'activity.action.archive_exported'
};

const LABEL_KEY: Record<string, TKey> = {
	folder_created: 'activity.label.folder_created',
	folder_renamed: 'activity.label.folder_renamed',
	folder_moved: 'activity.label.folder_moved',
	folder_deleted: 'activity.label.folder_deleted',
	folder_restored: 'activity.label.folder_restored',
	folder_purged: 'activity.label.folder_purged',
	template_applied: 'activity.label.template_applied',
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
	member_expiry_changed: 'activity.label.member_expiry_changed',
	group_created: 'activity.label.group_created',
	group_updated: 'activity.label.group_updated',
	group_deleted: 'activity.label.group_deleted',
	group_assigned: 'activity.label.group_assigned',
	group_unassigned: 'activity.label.group_unassigned',
	folder_access_changed: 'activity.label.folder_access_changed',
	folder_access_removed: 'activity.label.folder_access_removed',
	question_submitted: 'activity.label.question_submitted',
	question_replied: 'activity.label.question_replied',
	question_answered: 'activity.label.question_answered',
	question_closed: 'activity.label.question_closed',
	question_reopened: 'activity.label.question_reopened',
	faq_published: 'activity.label.faq_published',
	qa_settings_changed: 'activity.label.qa_settings_changed',
	workspace_updated: 'activity.label.workspace_updated',
	workspace_status_changed: 'activity.label.workspace_status_changed',
	workspace_branding_changed: 'activity.label.workspace_branding_changed',
	archive_exported: 'activity.label.archive_exported'
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

		// `to` is null when the limit was removed — a different sentence, not an
		// empty date.
		case 'member_expiry_changed':
			return text(meta.to)
				? {
						key: 'activity.action.member_expiry_changed',
						vars: { ...vars, to: formatDateLocal(text(meta.to)) }
					}
				: { key: 'activity.action.member_expiry_cleared', vars };

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

		case 'question_submitted':
			return {
				key: 'activity.action.question_submitted',
				vars: { ...vars, number: number(meta.number), group: text(meta.group_name) }
			};

		case 'template_applied':
			return {
				key: 'activity.action.template_applied',
				vars: { ...vars, created: number(meta.created) }
			};

		case 'workspace_status_changed':
			return {
				key: 'activity.action.workspace_status_changed',
				vars: {
					from: statusDisplayName(text(meta.from)),
					to: statusDisplayName(text(meta.to))
				}
			};

		// A rename carries from/to; a description-only edit names the room once.
		case 'workspace_updated':
			return text(meta.from) !== text(meta.to)
				? {
						key: 'activity.action.workspace_renamed',
						vars: { from: text(meta.from), to: text(meta.to) }
					}
				: { key: 'activity.action.workspace_described', vars };

		// One action, three sentences: which asset moved and whether it was set or
		// removed both come from the metadata.
		case 'workspace_branding_changed':
			if (meta.kind === 'logo') {
				return {
					key:
						meta.action === 'removed'
							? 'activity.action.workspace_logo_removed'
							: 'activity.action.workspace_logo_set',
					vars
				};
			}
			return { key: 'activity.action.workspace_hero_changed', vars };

		// A whole-room package keeps the plain sentence; a scoped one names its
		// folders from the creation-time snapshot in the metadata.
		case 'archive_exported': {
			const names = Array.isArray(meta.folder_names)
				? meta.folder_names.filter((n): n is string => typeof n === 'string')
				: [];
			return meta.scope === 'folders' && names.length > 0
				? {
						key: 'activity.action.archive_exported_folders',
						vars: { ...vars, folders: formatNameList(names) }
					}
				: { key: 'activity.action.archive_exported', vars };
		}

		default:
			return { key: PHRASE_KEY[item.action] ?? null, vars };
	}
}
