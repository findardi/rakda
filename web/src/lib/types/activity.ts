export interface ActivityItem {
	id: string;
	actor_id: string;
	actor_name: string;
	actor_role: string;
	action: string;
	target_type: string;
	target_id: string;
	target_name: string;
	metadata: Record<string, unknown> | null;
	created_at: string;
	link_document_id: string;
	link_folder_id: string;
	link_question_id: string;
}

export interface ActivityListData {
	items: ActivityItem[];
	next_cursor: string;
}

export interface ActivityQuery {
	limit?: number;
	cursor?: string;
	from?: string;
	to?: string;
	actor_id?: string;
	action?: string;
}

export interface ActivityFilters {
	from: string;
	to: string;
	actor_id: string;
	action: string;
}

export interface ActivityActor {
	id: string;
	name: string;
}

// --- engagement (content_events aggregation) ---
// Owner/admin only — and their own reading is never recorded, so this answers
// "which guest read this, and for how long".

export interface ReaderEngagement {
	actor_id: string;
	/** Empty when the account is gone; the page falls back to the email snapshot. */
	actor_name: string;
	actor_email: string;
	/** The reader's *current* group — both empty once they are no longer a member. */
	group_id: string;
	group_name: string;
	/** Document-level opens, deduped per 5-minute window — not a sum of page opens. */
	opens: number;
	pages_seen: number;
	/** Sum of browser-reported dwell. Indicative, not precise. */
	read_ms: number;
	last_read_at: string;
}

export interface DocumentReaders {
	document_id: string;
	document_name: string;
	/** Pages of the served version; 0 until a rendition exists. */
	page_count: number;
	readers: ReaderEngagement[] | null;
	total_read_ms: number;
}

export interface ReaderPageEngagement {
	page_no: number;
	opens: number;
	read_ms: number;
}

// Pages with no event for this reader are absent; the chart fills the gaps from
// page_count — a page they skipped is a finding, not a hole. Events recorded
// against an older version can name pages beyond page_count.
export interface ReaderPages {
	document_id: string;
	document_name: string;
	page_count: number;
	actor_id: string;
	pages: ReaderPageEngagement[] | null;
	total_read_ms: number;
}

export interface PageDurationEntry {
	page_no: number;
	duration_ms: number;
}

export interface RecordDurationsPayload {
	version_id: string;
	durations: PageDurationEntry[];
}
