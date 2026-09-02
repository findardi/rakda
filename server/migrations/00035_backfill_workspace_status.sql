-- +goose Up
-- +goose StatementBegin
update workspaces w set status = 'active', updated_at = now()
where w.status = 'prepare'
  and exists (
    select 1
    from workspace_members m
    join workspace_roles r on r.id = m.role_id
    where m.workspace_id = w.id and r.name = 'guest'
  );
-- +goose StatementEnd

-- +goose StatementBegin
alter table workspaces alter column status set default 'prepare';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table workspaces alter column status set default 'active';
-- +goose StatementEnd
