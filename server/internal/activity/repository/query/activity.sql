-- name: InsertActivityLog :exec
insert into activity_logs 
    (workspace_id, actor_id, actor_name, actor_role, action, target_type, target_id, target_name, metadata)
values
    ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: InsertContentEvent :exec
insert into content_events
    (workspace_id, document_id, document_name, version_id, page_no, event_type, duration_ms, actor_id, actor_email)
values
    ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: ListActivityLogs :many
select
    a.*,
    coalesce(d.id, dv.id) as link_document_id,
    coalesce(d.folder_id, dv.folder_id, f.id) as link_folder_id,
    qn.id as link_question_id
from activity_logs a
left join documents d
    on a.target_type = 'document' and d.id = a.target_id and d.deleted_at is null
left join document_versions v
    on a.target_type = 'version' and v.id = a.target_id
left join documents dv
    on dv.id = v.document_id and dv.deleted_at is null
left join folders f
    on a.target_type = 'folder' and f.id = a.target_id and f.deleted_at is null
left join questions qn
    on a.target_type = 'question' and qn.id = a.target_id
where
    a.workspace_id = @workspace_id
    and (sqlc.narg('cursor_created_at')::timestamptz is null
        or (a.created_at, a.id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
    and (sqlc.narg('from_time')::timestamptz is null or a.created_at >= sqlc.narg('from_time'))
    and (sqlc.narg('to_time')::timestamptz is null or a.created_at <= sqlc.narg('to_time'))
    and (sqlc.narg('actor_id')::uuid is null or a.actor_id = sqlc.narg('actor_id'))
    and (sqlc.narg('action')::text is null or a.action = sqlc.narg('action'))
order by a.created_at desc, a.id desc
limit @page_size;

-- name: GetDocumentForEvent :one
-- page_count is the served version's; null until a rendition exists. The
-- engagement page draws its page axis from it so it never has to call
-- GET /view, which writes an audit row.
select d.id, d.workspace_id, d.name, dv.page_count
from documents d
left join document_versions dv on dv.id = d.current_version_id
where d.id = $1 and d.deleted_at is null;

-- name: ListDocumentReaders :many
-- The group is the reader's *current* one (content_events snapshots the
-- actor, not the group): a reader who left the room has none. One membership
-- per (workspace, user) and one group per member, so the joins never fan out.
select
    ce.actor_id,
    coalesce(u.username, '')::text as actor_name,
    max(ce.actor_email)::text as actor_email,
    g.id as group_id,
    coalesce(g.name, '')::text as group_name,
    (count(distinct date_bin('5 minutes', ce.created_at, timestamptz 'epoch'))
        filter (where ce.event_type = 'view_page')
    )::bigint as opens,
    (count(distinct ce.page_no) filter (where ce.event_type = 'view_page'))::bigint as pages_seen,
    coalesce(sum(ce.duration_ms) filter (where ce.event_type = 'page_duration'), 0)::bigint as read_ms,
    max(ce.created_at)::timestamptz as last_read_at
from content_events ce
left join users u on u.id = ce.actor_id
left join workspace_members wm on wm.workspace_id = ce.workspace_id and wm.user_id = ce.actor_id
left join workspace_group_members wgm on wgm.member_id = wm.id
left join workspace_groups g on g.id = wgm.group_id
where ce.workspace_id = @workspace_id
    and ce.document_id = @document_id
group by ce.actor_id, u.username, g.id, g.name
order by read_ms desc, last_read_at desc;

-- name: ListReaderPages :many
select
    page_no,
    (count(distinct date_bin('5 minutes', created_at, timestamptz 'epoch'))
        filter (where event_type = 'view_page')
    )::bigint as opens,
    coalesce(sum(duration_ms) filter (where event_type = 'page_duration'), 0)::bigint as read_ms
from content_events
where workspace_id = @workspace_id
    and document_id = @document_id
    and actor_id = @actor_id
    and page_no is not null
group by page_no
order by page_no;

-- name: ListEngagementBreakdown :many
select
    ce.actor_id,
    coalesce(u.username, '')::text as actor_name,
    max(ce.actor_email)::text as actor_email,
    coalesce(g.name, '')::text as group_name,
    ce.page_no,
    (count(distinct date_bin('5 minutes', ce.created_at, timestamptz 'epoch'))
        filter (where ce.event_type = 'view_page')
    )::bigint as opens,
    coalesce(sum(ce.duration_ms) filter (where ce.event_type = 'page_duration'), 0)::bigint as read_ms
from content_events ce
left join users u on u.id = ce.actor_id
left join workspace_members wm on wm.workspace_id = ce.workspace_id and wm.user_id = ce.actor_id
left join workspace_group_members wgm on wgm.member_id = wm.id
left join workspace_groups g on g.id = wgm.group_id
where ce.workspace_id = @workspace_id
    and ce.document_id = @document_id
    and ce.page_no is not null
group by ce.actor_id, u.username, g.name, ce.page_no
order by actor_name, ce.actor_id, ce.page_no;
