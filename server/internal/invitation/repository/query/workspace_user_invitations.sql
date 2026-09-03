-- name: GetMyInvitations :many
select 
    wi.*,
    u.username as invited_name,
    r.name as role_name,
    w.name as workspace_name
from 
    workspace_user_invitations wi
left join
    workspaces w on w.id = wi.workspace_id
left join
    users u on u.id = wi.invited_by
left join
    workspace_roles r on r.id = wi.role_id
where wi.user_id = $1 and wi.status = 'pending' and wi.expires_at > now()
order by wi.created_at desc;

-- name: AcceptWorkspaceInvitation :one
update workspace_user_invitations set
    status = 'accepted',
    user_id = $2,
    accepted_at = now(),
    updated_at = now()
where id = $1 and status = 'pending'
returning *;

-- name: RejectWorkspaceInvitation :one
update workspace_user_invitations set
    status = 'rejected',
    updated_at = now()
where id = $1 and status = 'pending'
returning *;

-- name: GetWorkspaceInvitation :one
select * from workspace_user_invitations where id = $1;

-- name: InsertMember :exec
insert into workspace_members
    (workspace_id, user_id, role_id, status, expires_at)
values
    ($1, $2, $3, 'active', $4)
on conflict (workspace_id, user_id) do nothing;

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
-- Assigns the accepting guest to the group chosen at invite time. No-ops when
-- the member is not a guest or the group does not belong to the workspace
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