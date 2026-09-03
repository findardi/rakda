-- name: InsertWorkspaceInvitation :one
with created as (
    insert into workspace_user_invitations
        (workspace_id, email, role_id, user_id, group_id, invited_by, code_hash, status, expires_at)
    values
        ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    returning *
)
select
    i.*,
    w.name as workspace_name,
    u.username as invited_by_username
from created i
join workspaces w on w.id = i.workspace_id
left join users u on u.id = i.invited_by;

-- name: GetWorkspaceInvitation :one
select
    i.*,
    w.name as workspace_name,
    u.username as invited_by_username
from workspace_user_invitations i
join workspaces w on w.id = i.workspace_id
left join users u on u.id = i.invited_by
where i.id = $1;

-- name: GetWorkspaceInvitationByCodeHash :one
select * from workspace_user_invitations where code_hash = $1;

-- name: ListWorkspaceInvitations :many
-- A pending row whose expires_at has passed reads as 'expired': nothing flips
-- the stored status, so the lapse is derived here and the filter follows it.
select
    i.id,
    i.workspace_id,
    i.email,
    i.role_id,
    i.user_id,
    i.invited_by,
    i.code_hash,
    (case
        when i.status = 'pending' and i.expires_at <= now() then 'expired'
        else i.status
    end)::text as status,
    i.expires_at,
    i.accepted_at,
    i.created_at,
    i.updated_at,
    i.group_id,
    r.name as role_name,
    u.username as invited_by_username,
    g.name as group_name
from
    workspace_user_invitations i
left join
    workspace_roles r
        on r.id = i.role_id
left join
    users u
        on u.id = i.invited_by
left join
    workspace_groups g
        on g.id = i.group_id
where
    i.workspace_id = @workspace_id
    and (
        sqlc.narg('status')::text is null
        or case
            when i.status = 'pending' and i.expires_at <= now() then 'expired'
            else i.status
        end = sqlc.narg('status')
    )
order by i.created_at desc;

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

-- name: RevokeWorkspaceInvitation :one
update workspace_user_invitations set
    status = 'revoked',
    updated_at = now()
where id = $1 and status = 'pending'
returning *;

-- name: ResendInvitation :one
update workspace_user_invitations set
    status = 'pending',
    expires_at = $2,
    code_hash = $3,
    updated_at = now()
where id = $1 and status in ('pending', 'expired')
returning *;

-- name: ReinviteWorkspaceInvitation :one
with updated as (
    update workspace_user_invitations set
        status = 'pending',
        role_id = @role_id,
        user_id = @user_id,
        group_id = @group_id,
        invited_by = @invited_by,
        code_hash = @code_hash,
        expires_at = @expires_at,
        accepted_at = null,
        updated_at = now()
    where workspace_id = @workspace_id
        and lower(email) = lower(@email)
        and (
            status in ('revoked', 'rejected', 'expired')
            or (status = 'pending' and expires_at <= now())
        )
    returning *
)
select
    i.*,
    w.name as workspace_name,
    u.username as invited_by_username
from updated i
join workspaces w on w.id = i.workspace_id
left join users u on u.id = i.invited_by;

-- name: GetInvitationByCodeHashDetailed :one
select 
    i.*,
    w.name as workspace_name,
    r.name as role_name
from workspace_user_invitations i
join workspaces w on w.id = i.workspace_id
join workspace_roles r on r.id = i.role_id
where i.code_hash = $1;