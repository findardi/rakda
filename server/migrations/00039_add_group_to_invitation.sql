-- +goose Up
-- +goose StatementBegin
alter table workspace_user_invitations
    add column group_id uuid references workspace_groups (id) on delete set null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table workspace_user_invitations
    drop column group_id;
-- +goose StatementEnd
