-- +goose Up
-- +goose StatementBegin
alter table document_versions
    add column text_extracted_at timestamptz,
    add column text_error text,
    add column text_failed_at timestamptz;

create table if not exists document_page_texts (
    version_id uuid not null references document_versions (id) on delete cascade,
    page_no integer not null,
    content text not null,
    tsv_id tsvector generated always as (to_tsvector('indonesian', content)) stored,
    tsv_en tsvector generated always as (to_tsvector('english', content)) stored,
    primary key (version_id, page_no)
);

create index if not exists document_page_texts_tsv_id_idx on document_page_texts using gin (tsv_id);
create index if not exists document_page_texts_tsv_en_idx on document_page_texts using gin (tsv_en);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
drop table if exists document_page_texts;
alter table document_versions
    drop column if exists text_failed_at,
    drop column if exists text_error,
    drop column if exists text_extracted_at;
-- +goose StatementEnd
