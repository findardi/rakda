-- +goose Up
-- +goose StatementBegin
create table workspace_archives (
    id                uuid primary key default gen_random_uuid(),
    workspace_id      uuid not null references workspaces (id) on delete cascade,
    requested_by      uuid not null references users (id) on delete restrict,
    requested_by_name text not null,
    status            text not null default 'pending',
    object_key        text not null default '',
    size_bytes        bigint not null default 0,
    checksum_sha256   text not null default '',
    document_count    integer not null default 0,
    missing_count     integer not null default 0,
    error             text not null default '',
    created_at        timestamptz not null default now(),
    completed_at      timestamptz,
    expires_at        timestamptz not null,

    constraint workspace_archives_status_check
        check (status in ('pending', 'ready', 'failed'))
);

create index if not exists workspace_archives_workspace_created_idx
    on workspace_archives (workspace_id, created_at desc);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists workspace_archives;
-- +goose StatementEnd
