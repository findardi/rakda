-- +goose Up
-- +goose StatementBegin
alter table workspace_groups
    add column qa_enabled boolean not null default true,
    add column qa_question_limit integer;

create table questions (
    id            uuid primary key default gen_random_uuid(),
    workspace_id  uuid not null references workspaces (id) on delete cascade,
    group_id      uuid references workspace_groups (id) on delete set null,
    group_name    text not null,
    number        integer not null,
    author_id     uuid not null,
    author_name   text not null,
    subject       text not null,
    body          text not null,
    status        text not null default 'waiting',
    document_id   uuid references documents (id) on delete set null,
    document_name text not null default '',
    folder_id     uuid references folders (id) on delete set null,
    folder_name   text not null default '',
    created_at    timestamptz not null default now(),

    constraint questions_status_check
        check (status in ('waiting', 'answered', 'closed'))
);

create index if not exists questions_workspace_group_idx
    on questions (workspace_id, group_id, created_at desc);
create index if not exists questions_workspace_status_idx
    on questions (workspace_id, status, created_at);
create unique index if not exists questions_group_number_key
    on questions (group_id, number) where group_id is not null;

create table question_replies (
    id          uuid primary key default gen_random_uuid(),
    question_id uuid not null references questions (id) on delete cascade,
    author_id   uuid not null,
    author_name text not null,
    author_role text not null,
    body        text not null,
    created_at  timestamptz not null default now()
);

create index if not exists question_replies_question_idx
    on question_replies (question_id, created_at);

create table faqs (
    id                 uuid primary key default gen_random_uuid(),
    workspace_id       uuid not null references workspaces (id) on delete cascade,
    question_text      text not null,
    answer_text        text not null,
    source_question_id uuid references questions (id) on delete set null,
    created_by         uuid not null,
    creator_name       text not null,
    created_at         timestamptz not null default now()
);

create index if not exists faqs_workspace_idx
    on faqs (workspace_id, created_at desc);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists faqs;
drop table if exists question_replies;
drop table if exists questions;

alter table workspace_groups
    drop column if exists qa_question_limit,
    drop column if exists qa_enabled;
-- +goose StatementEnd
