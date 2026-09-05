import { roleDisplayName } from '$lib/access/permissions';
import { formatDateLocal, formatNameList } from '$lib/format';
import { t, type TKey } from '$lib/i18n';
import { id } from '$lib/i18n/id';
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

// Phrase and label keys follow the action name. `in id` keeps the fallback
// for an action the dictionary does not know (raw action / null) instead of
// leaking a bare key into the timeline.
function dictKey(prefix: 'activity.action' | 'activity.label', action: string): TKey | null {
	const key = `${prefix}.${action}`;
	return key in id ? (key as TKey) : null;
}

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
	const key = dictKey('activity.label', action);
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
				? { key: dictKey('activity.action', item.action), vars }
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
			return {
				key: dictKey('activity.action', item.action),
				vars: { ...vars, version: number(meta.version_no) }
			};

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
			return { key: dictKey('activity.action', item.action), vars };
	}
}
