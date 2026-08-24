-- +goose Up
alter table documents
    add column staged_version_id uuid;

alter table documents
    add constraint documents_staged_version_id_fkey
    foreign key (staged_version_id) references document_versions (id) on delete set null;

-- +goose Down
alter table documents
    drop constraint documents_staged_version_id_fkey;

alter table documents
    drop column staged_version_id;
