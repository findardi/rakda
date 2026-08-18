-- +goose Up
alter table document_versions
    add column rendition_error text,
    add column rendition_failed_at timestamptz;

-- +goose Down
alter table document_versions
    drop column rendition_failed_at,
    drop column rendition_error;
