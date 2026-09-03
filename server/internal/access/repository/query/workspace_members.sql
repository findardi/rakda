-- name: AddMember :one
insert into workspace_members
    (workspace_id, user_id, role_id, status, expires_at)
values
    ($1, $2, $3, $4, $5)
returning *;

-- name: DeleteMember :exec
delete from workspace_members where id = $1;

-- name: GetMemberByWorkspaceUser :one
select * from workspace_members
where workspace_id = $1 and user_id = $2;

-- name: GetMembershipWithPermissions :one
-- The one membership lookup every room route goes through. A lapsed
-- expires_at reads as "not a member" here, so the gate is fail-closed for all
-- modules at once without touching their resolvers.
select m.status, r.name as role_name, r.permissions, w.status as workspace_status
from workspace_members m
join workspace_roles r on r.id = m.role_id
join workspaces w on w.id = m.workspace_id
where m.workspace_id = $1 and m.user_id = $2
  and (m.expires_at is null or m.expires_at > now());

-- name: GetMembers :many
select 
    m.*,
    r.name as role_name,
    u.username,
    u.email,
    coalesce(
        array_agg(g.name) filter (where g.name is not null),
        '{}'
    )::text[] as group_names
from 
    workspace_members m 
left join
    workspace_roles r 
        on r.id = m.role_id
left join
    users u
        on u.id = m.user_id
left join
    workspace_group_members gm
        on gm.member_id = m.id
left join
    workspace_groups g 
        on g.id = gm.group_id
where
    m.workspace_id = $1
group by m.id, r.name, u.username, u.email
order by m.created_at;

-- name: GetMember :one
select 
    m.*,
    r.name as role_name,
    u.username,
    u.email,
    coalesce(
        array_agg(g.name) filter (where g.name is not null),
        '{}'
    )::text[] as group_names
from 
    workspace_members m 
left join
    workspace_roles r 
        on r.id = m.role_id
left join
    users u
        on u.id = m.user_id
left join
    workspace_group_members gm
        on gm.member_id = m.id
left join
    workspace_groups g 
        on g.id = gm.group_id
where
    m.id = $1
group by m.id, r.name, u.username, u.email;

-- name: UpdateRole :one
update workspace_members set
    role_id = $2,
    updated_at = now()
where id = $1
returning *;

-- name: UpdateMemberExpiry :one
-- NULL clears the expiry; a future date extends or shortens it. Nothing else
-- about the membership moves, so a lapsed member is revived by this alone.
update workspace_members set
    expires_at = $2,
    updated_at = now()
where id = $1
returning *;