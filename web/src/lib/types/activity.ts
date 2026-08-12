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
