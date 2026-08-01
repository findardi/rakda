-- +goose Up
-- +goose StatementBegin
alter table folders
    add column deleted_at timestamptz,
    add column deleted_by uuid references users (id) on delete restrict,
    add column deleted_root_folder_id uuid references folders (id) on delete cascade;

alter table documents
    add column deleted_at timestamptz,
    add column deleted_by uuid references users (id) on delete restrict,
    add column deleted_root_folder_id uuid references folders (id) on delete cascade;

create index folders_trash_idx
    on folders (workspace_id, deleted_at) where deleted_at is not null;
create index documents_trash_idx
    on documents (workspace_id, deleted_at) where deleted_at is not null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists documents_trash_idx;
drop index if exists folders_trash_idx;
alter table documents
    drop column deleted_root_folder_id,
    drop column deleted_by,
    drop column deleted_at;
alter table folders
    drop column deleted_root_folder_id,
    drop column deleted_by,
    drop column deleted_at;
-- +goose StatementEnd
