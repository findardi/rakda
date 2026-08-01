-- +goose Up
-- +goose StatementBegin
drop index if exists documents_folders_idx;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
create index documents_folders_idx on documents (folder_id, created_at);
-- +goose StatementEnd
