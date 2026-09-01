-- +goose Up
-- +goose StatementBegin
create table document_download_jobs (
    id            uuid primary key default gen_random_uuid(),
    workspace_id  uuid not null references workspaces (id) on delete cascade,
    document_id   uuid not null,
    version_id    uuid not null,
    requested_by  uuid not null references users (id) on delete restrict,
    document_name text not null,
    version_no    integer not null default 1,
    page_count    integer not null default 0,
    status        text not null default 'pending',
    object_key    text not null default '',
    size_bytes    bigint not null default 0,
    error         text not null default '',
    created_at    timestamptz not null default now(),
    completed_at  timestamptz,
    expires_at    timestamptz not null,

    constraint document_download_jobs_status_check
        check (status in ('pending', 'ready', 'failed'))
);

create index if not exists document_download_jobs_requester_idx
    on document_download_jobs (workspace_id, requested_by, created_at desc);

create index if not exists document_download_jobs_expiry_idx
    on document_download_jobs (expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists document_download_jobs;
-- +goose StatementEnd
