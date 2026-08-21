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


-- name: ListPendingOCRPages :many
select
    d.workspace_id,
    pt.version_id,
    pt.page_no,
    v.rendition_key
from document_page_texts pt
join document_versions v on v.id = pt.version_id
join documents d on d.id = v.document_id
where d.deleted_at is null
  and d.current_version_id = v.id
  and v.text_extracted_at is not null
  and v.rendition_key is not null
  and pt.text_source = 'pdf'
  and pt.ocr_at is null
  and length(trim(pt.content)) < 20
order by d.created_at, pt.version_id, pt.page_no
limit sqlc.arg(limit_count);

-- name: SetPageOCRResult :exec
update document_page_texts
set content = sqlc.arg(content),
    words = sqlc.arg(words),
    text_source = 'ocr',
    ocr_at = now(),
    ocr_error = null
where version_id = sqlc.arg(version_id)
  and page_no = sqlc.arg(page_no);

-- name: SetPageOCRFailure :exec
update document_page_texts
set ocr_error = sqlc.arg(ocr_error),
    ocr_at = now()
where version_id = sqlc.arg(version_id)
  and page_no = sqlc.arg(page_no);

-- name: ListPendingWordBoxes :many
select
    d.workspace_id,
    pt.version_id,
    pt.page_no,
    v.rendition_key
from document_page_texts pt
join document_versions v on v.id = pt.version_id
join documents d on d.id = v.document_id
where d.deleted_at is null
  and d.current_version_id = v.id
  and v.text_extracted_at is not null
  and v.rendition_key is not null
  and pt.text_source = 'pdf'
  and pt.words is null
  and length(trim(pt.content)) >= 20
order by d.created_at, pt.version_id, pt.page_no
limit sqlc.arg(limit_count);

-- name: SetPageWordBoxes :exec
update document_page_texts
set words = sqlc.arg(words)
where version_id = sqlc.arg(version_id)
  and page_no = sqlc.arg(page_no);

-- name: SearchPendingBoxPages :many
with q as (
    select
        websearch_to_tsquery('indonesian', sqlc.arg(query)) as idq,
        websearch_to_tsquery('english', sqlc.arg(query)) as enq
)
select distinct pt.page_no
from document_page_texts pt
join documents d on d.current_version_id = pt.version_id
cross join q
where d.id = sqlc.arg(document_id)
  and pt.words is null
  and (pt.tsv_id @@ q.idq or pt.tsv_en @@ q.enq)
order by pt.page_no;

-- name: SearchWordBoxes :many
with q as (
    select
        websearch_to_tsquery('indonesian', sqlc.arg(query)) as idq,
        websearch_to_tsquery('english', sqlc.arg(query)) as enq
)
select
    pt.page_no,
    (w->>'x')::float8 as x,
    (w->>'y')::float8 as y,
    (w->>'w')::float8 as w,
    (w->>'h')::float8 as h
from document_page_texts pt
join documents d on d.current_version_id = pt.version_id
cross join q
cross join lateral jsonb_array_elements(pt.words) w
where d.id = sqlc.arg(document_id)
  and pt.words is not null
  -- Halaman kena secara penuh (AND, konsisten dengan 9-d)…
  and (pt.tsv_id @@ q.idq or pt.tsv_en @@ q.enq)
  -- …lalu kotak kata yang cocok dengan salah satu token query.
  and (
      to_tsvector('indonesian', w->>'text') @@ any (array(
          select to_tsquery('indonesian', t)
          from unnest(sqlc.arg(tokens)::text[]) t
      ))
      or to_tsvector('english', w->>'text') @@ any (array(
          select to_tsquery('english', t)
          from unnest(sqlc.arg(tokens)::text[]) t
      ))
  )
order by pt.page_no, (w->>'y')::float8, (w->>'x')::float8;
