-- name: CreateDocument :one
insert into documents 
    (workspace_id, folder_id, name, position, uploaded_by)
values
    ($1, $2, $3, $4, $5)
returning *;

-- name: CreateDocumentVersion :one
insert into document_versions
    (document_id, version_no, mime, size, storage_key, uploaded_by)
values 
    ($1, $2, $3, $4, $5, $6)
returning *;

-- name: SetCurrentVersion :exec
update documents set 
    current_version_id = $2,
    updated_at = now()
where id = $1;

-- name: GetNextVersionNo :one
select coalesce(max(version_no),0)::int + 1 as next_no
from document_versions where document_id = $1;

-- name: GetDocumentByID :one
select * from documents where id = $1 and deleted_at is null;

-- name: ListDocumentsByFolder :many
select
    d.id,
    d.name,
    d.folder_id,
    d.current_version_id,
    d.uploaded_by,
    d.created_at,
    d.updated_at,
    v.version_no,
    v.mime,
    v.size
from documents d
join document_versions v on v.id = d.current_version_id
where d.folder_id = $1 and d.deleted_at is null
order by d.position, d.name, d.created_at;

-- name: ListVersionByDocument :many
select * from document_versions where document_id = $1 order by version_no desc;

-- name: GetVersionByID :one
select * from document_versions where id = $1;

-- name: GetCurrentVersion :one
select v.* from document_versions v 
join documents d on d.current_version_id = v.id
where d.id = $1;

-- name: MoveDocument :exec
update documents set folder_id = $2, position = $3, updated_at = now() where id = $1;

-- name: SetVersionRendition :exec
update document_versions
set rendition_key = sqlc.arg(rendition_key),
    page_count = sqlc.arg(page_count)
where id = sqlc.arg(id);

-- name: GetMaxPosition :one
select coalesce(max(position), -1)::int as max_position
from documents
where folder_id = $1 and deleted_at is null;

-- name: GetDocumentByNameInFolder :one
select * from documents where folder_id = $1 and name = $2 and deleted_at is null;

-- name: ReindexDocumentSiblings :exec
with ordered as (
    select d.id as document_id,
        (row_number() over (
            order by position,
                    case when d.id = sqlc.arg(moved_id) then 0 else 1 end,
                    d.name 
        ))::int - 1 as rn 
    from documents d
    where d.folder_id = sqlc.arg(folder_id)
    and d.deleted_at is null
)
update documents t
set position = o.rn 
from ordered o 
where t.id = o.document_id and t.position <> o.rn;

-- name: ListVersionsWithUploader :many
-- `is_current` is the served version, which restore repoints freely, so it is
-- not necessarily the highest version_no. current_version_id is nullable.
select
    v.*,
    coalesce(u.username, u.email)::text as uploaded_by_name,
    coalesce(d.current_version_id = v.id, false)::bool as is_current
from document_versions v
join users u on u.id = v.uploaded_by
join documents d on d.id = v.document_id
where v.document_id = $1
order by v.version_no desc;

-- name: SoftDeleteDocument :exec
update documents set 
    deleted_at = now(),
    deleted_by = $2,
    updated_at = now()
where id = $1;

-- name: GetTrashedDocumentByID :one
select * from documents where id = $1 and deleted_at is not null;

-- name: RestoreDocument :exec
update documents set
    deleted_at = null,
    deleted_by = null,
    deleted_root_folder_id = null,
    folder_id = sqlc.arg(folder_id),
    name = sqlc.arg(name),
    position = sqlc.arg(position),
    updated_at = now()
where id = sqlc.arg(id);

-- name: ListTrashDocuments :many
select d.id, d.name, d.deleted_at,
    coalesce(u.username, u.email)::text as deleted_by_name,
    v.mime, v.size
from documents d
join users u on u.id = d.deleted_by
left join document_versions v on v.id = d.current_version_id
where d.workspace_id = $1
    and d.deleted_at is not null
    and d.deleted_root_folder_id is null
order by d.deleted_at desc;

-- name: RestoreDocumentsSweptBy :exec
update documents set
    deleted_at = null,
    deleted_by = null,
    deleted_root_folder_id = null,
    updated_at = now()
where deleted_root_folder_id = $1;
