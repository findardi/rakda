-- +goose Up
-- +goose StatementBegin
alter table document_page_texts
    add column ocr_at timestamptz,
    add column ocr_error text,
    add column text_source text not null default 'pdf',
    add column words jsonb;

alter table document_page_texts
    add constraint document_page_texts_text_source_check
    check (text_source in ('pdf', 'ocr'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table document_page_texts drop constraint if exists document_page_texts_text_source_check;
alter table document_page_texts
    drop column if exists words,
    drop column if exists text_source,
    drop column if exists ocr_error,
    drop column if exists ocr_at;
-- +goose StatementEnd
