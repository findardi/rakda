-- name: CreateArchive :one
insert into workspace_archives
    (workspace_id, requested_by, requested_by_name, expires_at,
     scope_folder_ids, scope_folder_names)
values
    ($1, $2, $3, $4, $5, $6)
returning *;

-- name: SetArchiveObjectKey :exec
update workspace_archives set object_key = sqlc.arg(object_key)
where id = sqlc.arg(id);

-- name: GetArchive :one
select * from workspace_archives
where id = sqlc.arg(id) and workspace_id = sqlc.arg(workspace_id);

-- name: ListArchives :many
select * from workspace_archives
where workspace_id = $1
order by created_at desc;

-- name: CountPendingArchives :one
select count(*)::int from workspace_archives
where workspace_id = $1 and status = 'pending';

-- name: MarkArchiveReady :exec
update workspace_archives set
    status = 'ready',
    size_bytes = sqlc.arg(size_bytes),
    checksum_sha256 = sqlc.arg(checksum_sha256),
    document_count = sqlc.arg(document_count),
    missing_count = sqlc.arg(missing_count),
    completed_at = now()
where id = sqlc.arg(id) and status = 'pending';

-- name: MarkArchiveFailed :exec
update workspace_archives set
    status = 'failed',
    error = sqlc.arg(error),
    completed_at = now()
where id = sqlc.arg(id) and status = 'pending';

-- name: DeleteArchive :exec
delete from workspace_archives
where id = sqlc.arg(id) and workspace_id = sqlc.arg(workspace_id);

-- name: ListExpiredArchives :many
select id, workspace_id, object_key from workspace_archives
where expires_at < now();

-- name: ListStalePendingArchives :many
select id, workspace_id, object_key from workspace_archives
where status = 'pending' and created_at < sqlc.arg(cutoff);

-- name: ListArchiveFolders :many
select id, parent_id, name, position, is_default
from folders
where workspace_id = $1 and deleted_at is null
order by parent_id nulls first, position, created_at;

-- name: ListArchiveDocuments :many
select
    d.id,
    d.folder_id,
    d.name,
    d.position,
    v.id as version_id,
    v.version_no,
    v.mime,
    v.size,
    v.rendition_key,
    v.page_count,
    v.rendition_failed_at,
    coalesce(u.username, '')::text as uploaded_by_name,
    v.created_at as version_created_at
from documents d
join document_versions v on v.id = d.current_version_id
left join users u on u.id = v.uploaded_by
where d.workspace_id = $1 and d.deleted_at is null
order by d.folder_id, d.position, d.name, d.created_at;

-- name: ListArchiveMembers :many
select
    coalesce(u.username, '')::text as username,
    coalesce(u.email, '')::text as email,
    coalesce(r.name, '')::text as role_name,
    m.status,
    m.created_at,
    coalesce(
        array_agg(g.name order by g.name) filter (where g.name is not null),
        '{}'
    )::text[] as group_names
from workspace_members m
left join workspace_roles r on r.id = m.role_id
left join users u on u.id = m.user_id
left join workspace_group_members gm on gm.member_id = m.id
left join workspace_groups g on g.id = gm.group_id
where m.workspace_id = $1
group by m.id, u.username, u.email, r.name
order by r.name, u.username;

-- name: ListFolderAccessMatrix :many
with recursive granted as (
    select
        f.id as folder_id,
        f.parent_id,
        fa.group_id,
        fa.can_view,
        fa.can_download,
        fa.can_watermark,
        fa.can_download_original,
        f.id as source_folder_id
    from folders f
    join folder_access fa on fa.folder_id = f.id
    where f.workspace_id = sqlc.arg(workspace_id) and f.deleted_at is null

    union all

    select
        c.id,
        c.parent_id,
        g.group_id,
        g.can_view,
        g.can_download,
        g.can_watermark,
        g.can_download_original,
        g.source_folder_id
    from folders c
    join granted g on c.parent_id = g.folder_id
    where c.deleted_at is null
      and not exists (
        select 1
        from folder_access fa2
        where fa2.folder_id = c.id and fa2.group_id = g.group_id
      )
)
select
    g.folder_id,
    g.group_id,
    wg.name as group_name,
    g.can_view,
    g.can_download,
    g.can_watermark,
    g.can_download_original,
    g.source_folder_id
from granted g
join workspace_groups wg on wg.id = g.group_id
order by wg.name, g.folder_id;

-- name: GetWorkspaceSlugForArchive :one
select slug from workspaces where id = $1;
