-- name: InsertGroupMember :one
insert into workspace_group_members
    (group_id, member_id)
values
    ($1, $2)
on conflict (member_id) do update
    set group_id = excluded.group_id
returning *;

-- name: DeleteGroupMember :exec
delete from workspace_group_members where
    group_id = $1 and member_id = $2;

-- name: GrantDefaultFolderAccess :exec
insert into folder_access (folder_id, group_id, can_view, can_download, can_watermark)
select f.id, sqlc.arg(group_id), true, false, false
from folders f
where f.workspace_id = sqlc.arg(workspace_id) and f.is_default
on conflict (folder_id, group_id) do nothing;

-- name: AssignDefaultGroupIfGuest :exec
insert into workspace_group_members (group_id, member_id)
select g.id, m.id
from workspace_members m
join workspace_roles r
    on r.id = m.role_id and r.name = 'guest'
join workspace_groups g
    on g.workspace_id = m.workspace_id and g.is_default
where m.workspace_id = sqlc.arg(workspace_id) and m.user_id = sqlc.arg(user_id)
on conflict (member_id) do nothing;

-- name: AssignToGroup :exec
-- Assigns an accepting guest to the group chosen at invite time. No-ops when
-- the member is not a guest or the group belongs to another workspace
-- (invitations are validated on creation; this is defense in depth).
insert into workspace_group_members (group_id, member_id)
select g.id, m.id
from workspace_members m
join workspace_roles r
    on r.id = m.role_id and r.name = 'guest'
join workspace_groups g
    on g.id = sqlc.arg(group_id) and g.workspace_id = m.workspace_id
where m.workspace_id = sqlc.arg(workspace_id) and m.user_id = sqlc.arg(user_id)
on conflict (member_id) do nothing;

-- name: MoveGroupMembersToDefaultGroup :execrows
update workspace_group_members gm
set group_id = dg.id
from workspace_members m, workspace_groups dg
where gm.member_id = m.id
  and dg.workspace_id = m.workspace_id
  and dg.is_default
  and gm.group_id = sqlc.arg(group_id);

-- name: MoveMemberToDefaultGroup :execrows
insert into workspace_group_members (group_id, member_id)
select g.id, m.id
from workspace_members m
join workspace_groups g
    on g.workspace_id = m.workspace_id and g.is_default
where m.id = sqlc.arg(member_id)
on conflict (member_id) do update
    set group_id = excluded.group_id;

-- name: GetGroupMembers :many
select
    gm.*,
    u.username,
    u.email,
    r.name as role_name,
    g.name as group_name
from
    workspace_group_members gm
left join
    workspace_groups g on g.id = gm.group_id
left join 
    workspace_members m on m.id = gm.member_id
left join
    users u on u.id = m.user_id
left join 
    workspace_roles r on r.id = m.role_id
where
    gm.group_id = $1
order by gm.created_at;