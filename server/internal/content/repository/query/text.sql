-- name: ListPendingTextExtraction :many
select
    v.*,
    d.name as document_name,
    d.workspace_id as workspace_id,
    d.folder_id as folder_id
from document_versions v
join documents d on d.id = v.document_id 
where d.deleted_at is null
    and d.current_version_id = v.id
    and v.text_extracted_at is null
    and v.text_failed_at is null
    and v.rendition_failed_at is null
order by v.created_at
limit $1;

-- name: DeleteVersionPageText :exec
delete from document_page_texts where version_id = $1;

-- name: InsertPageText :exec
insert into document_page_texts (version_id, page_no, content)
values ($1, $2, $3);

-- name: SetVersionTextExtracted :exec
update document_versions
set text_extracted_at = now(),
    text_error = null,
    text_failed_at = null
where id = $1;

-- name: SetVersionTextFailure :exec
update document_versions
set text_error = sqlc.arg(text_error),
    text_failed_at = now()
where id = sqlc.arg(id);

