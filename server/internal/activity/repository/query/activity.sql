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

