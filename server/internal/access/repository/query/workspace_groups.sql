-- name: CreateGroup :one
insert into workspace_groups
    (workspace_id, name, description)
values
    ($1, $2, $3)
returning *;

-- name: CreateDefaultGroup :one
insert into workspace_groups
    (workspace_id, name, description, is_default)
values
    ($1, $2, $3, true)
returning *;

-- name: GetGroup :one
select * from workspace_groups
where id = $1;

-- name: GetDefaultGroup :one
select * from workspace_groups
where workspace_id = $1 and is_default;

-- name: GetGroups :many
select * from workspace_groups
where workspace_id = $1
order by created_at;

-- name: UpdateGroup :one
update workspace_groups set 
    name = $2,
    description = $3,
    updated_at = now()
where id = $1
returning *;

-- name: UpdateGroupQA :one
update workspace_groups set
    qa_enabled = @qa_enabled,
    qa_question_limit = @qa_question_limit,
    updated_at = now()
where id = @id and workspace_id = @workspace_id
returning *;

-- name: DeleteGroup :exec
delete from workspace_groups where id = $1;