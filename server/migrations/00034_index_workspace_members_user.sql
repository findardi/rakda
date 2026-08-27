-- +goose Up
-- +goose StatementBegin
create index if not exists workspace_members_user_idx on workspace_members (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists workspace_members_user_idx;
-- +goose StatementEnd
