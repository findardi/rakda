-- +goose Up
-- +goose StatementBegin
create extension if not exists pg_trgm;

create index if not exists folders_name_trgm_idx
    on folders using gin (name gin_trgm_ops)
    where deleted_at is null;

create index if not exists documents_name_trgm_idx
    on documents using gin (name gin_trgm_ops)
    where deleted_at is null;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop index if exists documents_name_trgm_idx;
drop index if exists folders_name_trgm_idx;
-- +goose StatementEnd
