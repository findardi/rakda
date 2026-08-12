-- +goose Up
-- +goose StatementBegin
create table if not exists activity_logs(
    id uuid primary key default gen_random_uuid(),
    workspace_id uuid not null references workspaces (id) on delete cascade,
    actor_id uuid, 
    actor_name text not null default '',
    actor_role text not null default '',
    action text not null,
    target_type text not null,
    target_id uuid,
    target_name text not null default '',
    metadata jsonb not null default '{}',
    created_at timestamptz not null default now()
);

create index if not exists activity_logs_workspace_time_idx
    on activity_logs (workspace_id, created_at desc);

create table if not exists content_events(
    id bigint generated always as identity primary key,
    workspace_id uuid not null references workspaces (id) on delete cascade,
    document_id uuid not null,
    document_name text not null,
    version_id uuid,
    page_no integer,
    event_type text not null,
    duration_ms integer,
    actor_id uuid not null,
    actor_email text not null,
    created_at timestamptz not null default now()
);

create index if not exists content_events_document_idx
    on content_events (workspace_id, document_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists content_events;
drop table if exists activity_logs;
-- +goose StatementEnd
