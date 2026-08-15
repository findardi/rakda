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
// Owner/admin only. Pages with no event are absent from `pages`; the viewer
// fills the gaps from the view meta's page_count.

export interface PageEngagement {
	page_no: number;
	/** Meaningful opens — deduped per actor per 5-minute window. The headline number. */
	opens: number;
	/** Raw hits before dedup. Detail only, never the headline. */
	raw_hits: number;
	unique_viewers: number;
	/** Sum of browser-reported dwell. Indicative, not precise. */
	read_ms: number;
}

export interface DocumentEngagement {
	document_id: string;
	document_name: string;
	pages: PageEngagement[] | null;
	total_opens: number;
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
