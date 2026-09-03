-- +goose Up
alter table document_versions
    add column rendition_attempts integer not null default 0,
    add column rendition_next_at timestamptz,
    add column rendition_claimed_at timestamptz;

create index document_versions_rendition_pending_idx
    on document_versions (created_at)
    where rendition_key is null and rendition_failed_at is null;

-- +goose Down
drop index if exists document_versions_rendition_pending_idx;

alter table document_versions
    drop column rendition_claimed_at,
    drop column rendition_next_at,
    drop column rendition_attempts;
