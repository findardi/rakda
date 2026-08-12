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
select * from activity_logs
where
    workspace_id = @workspace_id
    and (sqlc.narg('cursor_created_at')::timestamptz is null
        or (created_at, id) < (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
    and (sqlc.narg('from_time')::timestamptz is null or created_at >= sqlc.narg('from_time'))
    and (sqlc.narg('to_time')::timestamptz is null or created_at <= sqlc.narg('to_time'))
    and (sqlc.narg('actor_id')::uuid is null or actor_id = sqlc.narg('actor_id'))
    and (sqlc.narg('action')::text is null or action = sqlc.narg('action'))
order by created_at desc, id desc
limit @page_size;

