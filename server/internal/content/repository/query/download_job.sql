-- name: CreateDownloadJob :one
insert into document_download_jobs (
    workspace_id, document_id, version_id, requested_by,
    document_name, version_no, page_count, expires_at
) values ($1, $2, $3, $4, $5, $6, $7, $8)
returning *;

-- name: GetDownloadJob :one
select * from document_download_jobs where id = $1;

-- name: GetPendingDownloadJob :one
select * from document_download_jobs
where workspace_id = $1
  and requested_by = $2
  and version_id = $3
  and status = 'pending'
order by created_at desc
limit 1;

-- name: ListDownloadJobsForUser :many
select * from document_download_jobs
where workspace_id = $1
  and requested_by = $2
  and expires_at > now()
order by created_at desc
limit sqlc.arg(limit_count);

-- name: MarkDownloadJobReady :exec
update document_download_jobs
set status = 'ready',
    object_key = $2,
    size_bytes = $3,
    completed_at = now(),
    expires_at = now() + sqlc.arg(ttl)::interval
where id = $1 and status = 'pending';

-- name: MarkDownloadJobFailed :exec
update document_download_jobs
set status = 'failed',
    error = $2,
    completed_at = now()
where id = $1 and status = 'pending';

-- name: MarkReadyDownloadJobLost :exec
update document_download_jobs
set status = 'failed',
    error = $2
where id = $1 and status = 'ready';

-- name: ListStalePendingDownloadJobs :many
select * from document_download_jobs
where status = 'pending' and created_at < sqlc.arg(cutoff);

-- name: ListExpiredDownloadJobs :many
select * from document_download_jobs where expires_at <= now();

-- name: DeleteDownloadJob :exec
delete from document_download_jobs where id = $1;
