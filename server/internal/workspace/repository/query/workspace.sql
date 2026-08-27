-- name: CreateWorkspace :one
insert into workspaces 
    (owner_id, name, slug, description, status)
values 
    ($1, $2, $3, $4, $5)
returning *;

-- name: GetWorkspacesByOwner :many
select * from workspaces where owner_id = $1;

-- name: GetWorkspaceBySlugAndOwner :one
select * from workspaces 
where owner_id = $1 and slug = $2;

-- name: GetWorkspaceByNameAndOwner :one
select * from workspaces 
where owner_id = $1 and name = $2;

-- name: GetWorkspaceByID :one
select * from workspaces where id = $1;

-- name: GetWorkspaceForMember :one
select w.* from workspaces w
join workspace_members m on m.workspace_id = w.id
where w.id = sqlc.arg(workspace_id)
  and m.user_id = sqlc.arg(user_id)
  and m.status = 'active';

-- name: UpdateWorkspaceStatus :one
update workspaces set 
    status = sqlc.arg(status),
    updated_at = now()
where id = sqlc.arg(id) and status = sqlc.arg(from_status)
returning *;

-- name: UpdateWorkspace :one
update workspaces set
    name = $2,
    slug = $3,
    description = $4,
    updated_at = now()
where id = $1
returning *;

-- name: DeleteWorkspace :exec
delete from workspaces where id = $1;

-- name: GetWorkspaces :many
select
    w.*,
    r.name as role_name,
    (
        select max(a.created_at)
        from activity_logs a
        where a.workspace_id = w.id
    )::timestamptz as last_activity_at
from workspaces w
join workspace_members wm on wm.workspace_id = w.id
join workspace_roles r on r.id = wm.role_id
where wm.user_id = sqlc.arg(user_id)
  and wm.status = 'active'
order by last_activity_at desc nulls last, w.created_at desc, w.id;