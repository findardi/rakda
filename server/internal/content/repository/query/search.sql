-- name: SearchVisibleFolders :many
with recursive granted as (
    select
        f.id,
        f.parent_id,
        f.name,
        f.position,
        f.is_default,
        f.deleted_at,
        fa.can_view
    from folders f
    join folder_access fa on fa.folder_id = f.id
    join workspace_group_members gm on gm.group_id = fa.group_id
    join workspace_members m on m.id = gm.member_id
    where f.workspace_id = sqlc.arg(workspace_id)
      and m.workspace_id = sqlc.arg(workspace_id)
      and m.user_id = sqlc.arg(user_id)

    union all

    select
        c.id,
        c.parent_id,
        c.name,
        c.position,
        c.is_default,
        c.deleted_at,
        g.can_view
    from folders c
    join granted g on c.parent_id = g.id
    where not exists (
        select 1
        from folder_access fa2
        join workspace_group_members gm2 on gm2.group_id = fa2.group_id
        join workspace_members m2 on m2.id = gm2.member_id
        where fa2.folder_id = c.id
          and m2.workspace_id = sqlc.arg(workspace_id)
          and m2.user_id = sqlc.arg(user_id)
    )
)
select id, parent_id, name, position, is_default
from granted
where can_view
  and deleted_at is null
  and name ilike '%' || sqlc.arg(query) || '%'
order by position, name
limit sqlc.arg(limit_count);

-- name: SearchVisibleDocuments :many
with recursive granted as (
    select
        f.id,
        f.parent_id,
        f.name,
        f.position,
        f.is_default,
        f.deleted_at,
        fa.can_view
    from folders f
    join folder_access fa on fa.folder_id = f.id
    join workspace_group_members gm on gm.group_id = fa.group_id
    join workspace_members m on m.id = gm.member_id
    where f.workspace_id = sqlc.arg(workspace_id)
      and m.workspace_id = sqlc.arg(workspace_id)
      and m.user_id = sqlc.arg(user_id)

    union all

    select
        c.id,
        c.parent_id,
        c.name,
        c.position,
        c.is_default,
        c.deleted_at,
        g.can_view
    from folders c
    join granted g on c.parent_id = g.id
    where not exists (
        select 1
        from folder_access fa2
        join workspace_group_members gm2 on gm2.group_id = fa2.group_id
        join workspace_members m2 on m2.id = gm2.member_id
        where fa2.folder_id = c.id
          and m2.workspace_id = sqlc.arg(workspace_id)
          and m2.user_id = sqlc.arg(user_id)
    )
)
select
    d.id,
    d.name,
    d.folder_id,
    g.name as folder_name,
    v.mime,
    v.size
from granted g
join documents d on d.folder_id = g.id and d.deleted_at is null
left join document_versions v on v.id = d.current_version_id
where g.can_view
  and d.name ilike '%' || sqlc.arg(query) || '%'
order by d.name
limit sqlc.arg(limit_count);

-- name: SearchAllFolders :many
select id, parent_id, name, position, is_default
from folders
where workspace_id = sqlc.arg(workspace_id)
  and deleted_at is null
  and name ilike '%' || sqlc.arg(query) || '%'
order by position, name
limit sqlc.arg(limit_count);

-- name: SearchAllDocuments :many
select
    d.id,
    d.name,
    d.folder_id,
    f.name as folder_name,
    v.mime,
    v.size
from documents d
join folders f on f.id = d.folder_id
left join document_versions v on v.id = d.current_version_id
where d.workspace_id = sqlc.arg(workspace_id)
  and d.deleted_at is null
  and d.name ilike '%' || sqlc.arg(query) || '%'
order by d.name
limit sqlc.arg(limit_count);

-- name: SearchAllFolderBreadcrumbs :many
with recursive chain as (
    select f.id, f.parent_id, f.name, 1 as depth, f.id as root_id, true as visible
    from folders f
    where f.id = any(sqlc.arg(folder_ids)::uuid[])
      and f.deleted_at is null
    union all
    select f.id, f.parent_id, f.name, c.depth + 1, c.root_id, true
    from folders f
    join chain c on f.id = c.parent_id
    where f.deleted_at is null
)
select root_id, name, visible
from chain
order by root_id, depth desc;

-- name: SearchVisibleFolderBreadcrumbs :many
with recursive granted as (
    select
        f.id,
        fa.can_view
    from folders f
    join folder_access fa on fa.folder_id = f.id
    join workspace_group_members gm on gm.group_id = fa.group_id
    join workspace_members m on m.id = gm.member_id
    where f.workspace_id = sqlc.arg(workspace_id)
      and m.workspace_id = sqlc.arg(workspace_id)
      and m.user_id = sqlc.arg(user_id)

    union all

    select
        c.id,
        g.can_view
    from folders c
    join granted g on c.parent_id = g.id
    where not exists (
        select 1
        from folder_access fa2
        join workspace_group_members gm2 on gm2.group_id = fa2.group_id
        join workspace_members m2 on m2.id = gm2.member_id
        where fa2.folder_id = c.id
          and m2.workspace_id = sqlc.arg(workspace_id)
          and m2.user_id = sqlc.arg(user_id)
    )
),
chain as (
    select f.id, f.parent_id, f.name, 1 as depth, f.id as root_id,
        coalesce(g.can_view, false) as visible
    from folders f
    left join granted g on g.id = f.id
    where f.id = any(sqlc.arg(folder_ids)::uuid[])
      and f.deleted_at is null
    union all
    select f.id, f.parent_id, f.name, c.depth + 1, c.root_id,
        coalesce(g.can_view, false)
    from folders f
    join chain c on f.id = c.parent_id
    left join granted g on g.id = f.id
    where f.deleted_at is null
)
select root_id, name, visible
from chain
order by root_id, depth desc;

-- name: SearchVisibleContent :many
with recursive granted as (
    select
        f.id,
        f.parent_id,
        f.name,
        f.position,
        f.is_default,
        f.deleted_at,
        fa.can_view
    from folders f
    join folder_access fa on fa.folder_id = f.id
    join workspace_group_members gm on gm.group_id = fa.group_id
    join workspace_members m on m.id = gm.member_id
    where f.workspace_id = sqlc.arg(workspace_id)
      and m.workspace_id = sqlc.arg(workspace_id)
      and m.user_id = sqlc.arg(user_id)

    union all

    select
        c.id,
        c.parent_id,
        c.name,
        c.position,
        c.is_default,
        c.deleted_at,
        g.can_view
    from folders c
    join granted g on c.parent_id = g.id
    where not exists (
        select 1
        from folder_access fa2
        join workspace_group_members gm2 on gm2.group_id = fa2.group_id
        join workspace_members m2 on m2.id = gm2.member_id
        where fa2.folder_id = c.id
          and m2.workspace_id = sqlc.arg(workspace_id)
          and m2.user_id = sqlc.arg(user_id)
    )
),
q as (
    select
        websearch_to_tsquery('indonesian', sqlc.arg(query)) as idq,
        websearch_to_tsquery('english', sqlc.arg(query)) as enq
)
select
    d.id as document_id,
    d.name as document_name,
    d.folder_id,
    g.name as folder_name,
    v.page_count,
    count(*) as hit_count,
    max(greatest(ts_rank(pt.tsv_id, q.idq), ts_rank(pt.tsv_en, q.enq)))::float4 as rank
from granted g
join documents d on d.folder_id = g.id
    and d.deleted_at is null
    and g.deleted_at is null
join document_versions v on v.id = d.current_version_id
join document_page_texts pt on pt.version_id = v.id
cross join q
where g.can_view
  and (pt.tsv_id @@ q.idq or pt.tsv_en @@ q.enq)
group by d.id, d.name, d.folder_id, g.name, v.page_count
order by rank desc, d.name
limit sqlc.arg(limit_count);

-- name: SearchAllContent :many
with q as (
    select
        websearch_to_tsquery('indonesian', sqlc.arg(query)) as idq,
        websearch_to_tsquery('english', sqlc.arg(query)) as enq
)
select
    d.id as document_id,
    d.name as document_name,
    d.folder_id,
    f.name as folder_name,
    v.page_count,
    count(*) as hit_count,
    max(greatest(ts_rank(pt.tsv_id, q.idq), ts_rank(pt.tsv_en, q.enq)))::float4 as rank
from documents d
join folders f on f.id = d.folder_id
join document_versions v on v.id = d.current_version_id
join document_page_texts pt on pt.version_id = v.id
cross join q
where d.workspace_id = sqlc.arg(workspace_id)
  and d.deleted_at is null
  and (pt.tsv_id @@ q.idq or pt.tsv_en @@ q.enq)
group by d.id, d.name, d.folder_id, f.name, v.page_count
order by rank desc, d.name
limit sqlc.arg(limit_count);

-- name: SearchVisibleContentPages :many
with recursive granted as (
    select
        f.id,
        f.parent_id,
        f.name,
        f.position,
        f.is_default,
        f.deleted_at,
        fa.can_view
    from folders f
    join folder_access fa on fa.folder_id = f.id
    join workspace_group_members gm on gm.group_id = fa.group_id
    join workspace_members m on m.id = gm.member_id
    where f.workspace_id = sqlc.arg(workspace_id)
      and m.workspace_id = sqlc.arg(workspace_id)
      and m.user_id = sqlc.arg(user_id)

    union all

    select
        c.id,
        c.parent_id,
        c.name,
        c.position,
        c.is_default,
        c.deleted_at,
        g.can_view
    from folders c
    join granted g on c.parent_id = g.id
    where not exists (
        select 1
        from folder_access fa2
        join workspace_group_members gm2 on gm2.group_id = fa2.group_id
        join workspace_members m2 on m2.id = gm2.member_id
        where fa2.folder_id = c.id
          and m2.workspace_id = sqlc.arg(workspace_id)
          and m2.user_id = sqlc.arg(user_id)
    )
),
q as (
    select
        websearch_to_tsquery('indonesian', sqlc.arg(query)) as idq,
        websearch_to_tsquery('english', sqlc.arg(query)) as enq
)
select
    f.page_no,
    case
        when f.rank_id >= f.rank_en
            then ts_headline('indonesian', f.content, f.idq,
                'MaxFragments=1, MaxWords=30, MinWords=10')
        else ts_headline('english', f.content, f.enq,
            'MaxFragments=1, MaxWords=30, MinWords=10')
    end::text as snippet
from (
    select
        pt.page_no,
        pt.content,
        ts_rank(pt.tsv_id, q.idq) as rank_id,
        ts_rank(pt.tsv_en, q.enq) as rank_en,
        greatest(ts_rank(pt.tsv_id, q.idq), ts_rank(pt.tsv_en, q.enq))::float4 as rank,
        q.idq,
        q.enq
    from granted g
    join documents d on d.folder_id = g.id
        and d.deleted_at is null
        and g.deleted_at is null
    join document_page_texts pt on pt.version_id = d.current_version_id
    cross join q
    where g.can_view
      and d.id = sqlc.arg(document_id)
      and (pt.tsv_id @@ q.idq or pt.tsv_en @@ q.enq)
    order by rank desc
    limit sqlc.arg(limit_count)
) f
order by f.rank desc;

-- name: SearchAllContentPages :many
with q as (
    select
        websearch_to_tsquery('indonesian', sqlc.arg(query)) as idq,
        websearch_to_tsquery('english', sqlc.arg(query)) as enq
)
select
    f.page_no,
    case
        when f.rank_id >= f.rank_en
            then ts_headline('indonesian', f.content, f.idq,
                'MaxFragments=1, MaxWords=30, MinWords=10')
        else ts_headline('english', f.content, f.enq,
            'MaxFragments=1, MaxWords=30, MinWords=10')
    end::text as snippet
from (
    select
        pt.page_no,
        pt.content,
        ts_rank(pt.tsv_id, q.idq) as rank_id,
        ts_rank(pt.tsv_en, q.enq) as rank_en,
        greatest(ts_rank(pt.tsv_id, q.idq), ts_rank(pt.tsv_en, q.enq))::float4 as rank,
        q.idq,
        q.enq
    from document_page_texts pt
    join documents d on d.current_version_id = pt.version_id
    cross join q
    where d.id = sqlc.arg(document_id)
      and (pt.tsv_id @@ q.idq or pt.tsv_en @@ q.enq)
    order by rank desc
    limit sqlc.arg(limit_count)
) f
order by f.rank desc;
