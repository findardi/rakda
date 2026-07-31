-- name: CreateFolder :one
insert into folders
    (workspace_id, parent_id, name, position, created_by)
values
    ($1, $2, $3, $4, $5)
returning *;

-- name: GetFolderByID :one
select * from folders where id = $1 and deleted_at is null;

-- name: GetFoldersByWorkspace :many
select * from folders where workspace_id = $1 and deleted_at is null
order by parent_id nulls first, position, created_at;

-- name: GetMaxPositionInParent :one
select coalesce(max(position), -1)::int as max_position
from folders
where workspace_id = $1 and parent_id is not distinct from $2
    and deleted_at is null;

-- name: RenameFolder :one
update folders set name = $2, updated_at = now() where id = $1 returning *;

-- name: MoveFolder :exec
update folders set
    parent_id = $2,
    position = $3,
    updated_at = now()
where id = $1;

-- name: CreateDefaultFolder :one
insert into folders
    (workspace_id, parent_id, name, position, created_by, is_default)
values
    ($1, null, $2, 0, $3, true)
returning *;

-- name: LockWorkspaceStructure :exec
select pg_advisory_xact_lock(hashtext(sqlc.arg(workspace_id)::uuid::text));

-- name: ReindexFolderSiblings :exec
with ordered as (
    select f.id as folder_id,
        (row_number() over (
            order by position,
                    case when f.id = sqlc.arg(moved_id) then 0 else 1 end,
                    f.name
        ))::int - 1 as rn
    from folders f
    where f.workspace_id = sqlc.arg(workspace_id)
    and f.parent_id is not distinct from sqlc.arg(parent_id)
    and f.deleted_at is null
)
update folders t 
set position = o.rn 
from ordered o 
where t.id = o.folder_id and t.position <> o.rn;

-- name: GetFolderByNameInParent :one
select * from folders
where workspace_id = $1
    and parent_id is not distinct from $2
    and name = $3
    and deleted_at is null;

-- name: SoftDeleteFolderSubtree :exec
with recursive subtree as (
    select id from folders
    where id = sqlc.arg(folder_id) and deleted_at is null
    union all
    select f.id from folders f
    join subtree s on f.parent_id = s.id
    where f.deleted_at is null
)
update folders f
set deleted_at = now(),
    deleted_by = sqlc.arg(deleted_by),
    deleted_root_folder_id = case 
        when f.id = sqlc.arg(folder_id) then null 
        else sqlc.arg(folder_id)
    end
from subtree s 
where f.id = s.id;

-- name: SoftDeleteDocumentsForFolderRoot :exec
update documents d
set deleted_at = now(),
    deleted_by = sqlc.arg(deleted_by),
    deleted_root_folder_id = sqlc.arg(folder_id)
from folders f
where d.folder_id = f.id
    and d.deleted_at is null
    and (f.id = sqlc.arg(folder_id) or f.deleted_root_folder_id = sqlc.arg(folder_id));

-- name: GetTrashedFolderByID :one
select * from folders where id = $1 and deleted_at is not null;

-- name: RestoreFolderRoot :exec
update folders set
    deleted_at = null,
    deleted_by = null,
    parent_id = sqlc.arg(parent_id),
    name = sqlc.arg(name),
    position = sqlc.arg(position),
    updated_at = now()
where id = sqlc.arg(id);

-- name: RestoreFoldersSweptBy :exec
update folders set  
    deleted_at = null,
    deleted_by = null,
    deleted_root_folder_id = null,
    updated_at = now()
where deleted_root_folder_id = $1;

-- name: ListTrashFolders :many
select f.id, f.name, f.deleted_at,
    coalesce(u.username, u.email)::text as deleted_by_name
from folders f
join users u on u.id = f.deleted_by
where f.workspace_id = $1
    and f.deleted_at is not null
    and f.deleted_root_folder_id is null
order by f.deleted_at desc;

-- name: GetDefaultFolder :one
select * from folders
where workspace_id = $1 and is_default = true and deleted_at is null;