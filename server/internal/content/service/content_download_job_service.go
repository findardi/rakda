package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/content/dto"
	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/watermark"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	DownloadJobStatusPending = "pending"
	DownloadJobStatusReady   = "ready"
	DownloadJobStatusFailed  = "failed"
)

func downloadJobKey(workspaceID, jobID string) string {
	return fmt.Sprintf("downloads/%s/%s.pdf", workspaceID, jobID)
}

type DownloadJobObject struct {
	Key      string
	Size     int64
	FileName string
}

type stampResult struct {
	body *spooledReadCloser
	err  error
}

func (s *ContentService) startDownloadJob(ctx context.Context, workspaceID string, doc contentdb.Document,
	version contentdb.DocumentVersion, pageCount int, renditionKey string, actor Actor,
	mark watermark.Mark) (contentdb.DocumentDownloadJob, error) {
	if existing, ok := s.pendingDownloadJob(ctx, doc.WorkspaceID, version.ID, actor); ok {
		return existing, nil
	}

	select {
	case s.stampAsyncSem <- struct{}{}:
	default:
		return contentdb.DocumentDownloadJob{}, ErrDownloadBusy
	}

	job, err := s.createDownloadJobRow(ctx, doc, version, pageCount, actor)
	if err != nil {
		<-s.stampAsyncSem
		return contentdb.DocumentDownloadJob{}, err
	}

	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), downloadJobTimeout)

	go func() {
		defer cancel()
		defer func() { <-s.stampAsyncSem }()

		body, err := s.rasterWatermarkPDF(jobCtx, workspaceID, uuidString(version.ID), renditionKey, pageCount, mark)
		s.storeDownloadJobResult(jobCtx, workspaceID, job, stampResult{body: body, err: err})
	}()

	return job, nil
}

func (s *ContentService) escalateDownload(ctx context.Context, workspaceID string, doc contentdb.Document,
	version contentdb.DocumentVersion, pageCount int, actor Actor,
	result <-chan stampResult, cancel context.CancelFunc) (contentdb.DocumentDownloadJob, error) {
	job, err := s.createDownloadJobRow(ctx, doc, version, pageCount, actor)
	if err != nil {
		go func() {
			defer cancel()
			defer func() { <-s.stampSem }()

			if res := <-result; res.body != nil {
				res.body.Close()
			}
		}()

		return contentdb.DocumentDownloadJob{}, err
	}

	detached := context.WithoutCancel(ctx)

	go func() {
		defer cancel()
		defer func() { <-s.stampSem }()

		res := <-result

		storeCtx, storeCancel := context.WithTimeout(detached, downloadJobStoreTimeout)
		defer storeCancel()

		s.storeDownloadJobResult(storeCtx, workspaceID, job, res)
	}()

	return job, nil
}

func (s *ContentService) pendingDownloadJob(ctx context.Context, workspaceID, versionID pgtype.UUID, actor Actor) (contentdb.DocumentDownloadJob, bool) {
	var requester pgtype.UUID
	if err := requester.Scan(actor.UserID); err != nil {
		return contentdb.DocumentDownloadJob{}, false
	}

	job, err := s.repo.GetPendingDownloadJob(ctx, contentdb.GetPendingDownloadJobParams{
		WorkspaceID: workspaceID,
		RequestedBy: requester,
		VersionID:   versionID,
	})
	if err != nil {
		return contentdb.DocumentDownloadJob{}, false
	}

	return job, true
}

func (s *ContentService) createDownloadJobRow(ctx context.Context, doc contentdb.Document,
	version contentdb.DocumentVersion, pageCount int, actor Actor) (contentdb.DocumentDownloadJob, error) {
	var requester pgtype.UUID
	if err := requester.Scan(actor.UserID); err != nil {
		return contentdb.DocumentDownloadJob{}, fmt.Errorf("user id parse: %w", err)
	}

	job, err := s.repo.CreateDownloadJob(ctx, contentdb.CreateDownloadJobParams{
		WorkspaceID:  doc.WorkspaceID,
		DocumentID:   doc.ID,
		VersionID:    version.ID,
		RequestedBy:  requester,
		DocumentName: doc.Name,
		VersionNo:    version.VersionNo,
		PageCount:    int32(pageCount),
		ExpiresAt:    pgtype.Timestamptz{Time: time.Now().Add(downloadJobStaleAge + downloadJobTTL), Valid: true},
	})
	if err != nil {
		return contentdb.DocumentDownloadJob{}, fmt.Errorf("create download job: %w", err)
	}

	return job, nil
}

func (s *ContentService) storeDownloadJobResult(ctx context.Context, workspaceID string,
	job contentdb.DocumentDownloadJob, res stampResult) {
	if res.err != nil {
		s.markDownloadJobFailed(ctx, job.ID, res.err)
		return
	}

	defer res.body.Close()

	size, err := res.body.size()
	if err != nil {
		s.markDownloadJobFailed(ctx, job.ID, err)
		return
	}

	key := downloadJobKey(workspaceID, uuidString(job.ID))
	if err := s.store.Put(ctx, key, res.body, size, "application/pdf"); err != nil {
		s.markDownloadJobFailed(ctx, job.ID, fmt.Errorf("store artifact: %w", err))
		return
	}

	if err := s.repo.MarkDownloadJobReady(ctx, contentdb.MarkDownloadJobReadyParams{
		ID:        job.ID,
		ObjectKey: key,
		SizeBytes: size,
		Ttl:       pgtype.Interval{Microseconds: downloadJobTTL.Microseconds(), Valid: true},
	}); err != nil {
		log.Printf("download job %s: mark ready: %v", uuidString(job.ID), err)
		_ = s.store.Delete(ctx, key)
	}
}

func (s *ContentService) markDownloadJobFailed(ctx context.Context, jobID pgtype.UUID, cause error) {
	if err := s.repo.MarkDownloadJobFailed(ctx, contentdb.MarkDownloadJobFailedParams{
		ID:    jobID,
		Error: cause.Error(),
	}); err != nil {
		log.Printf("download job %s: mark failed: %v", uuidString(jobID), err)
	}
}

func (s *ContentService) getDownloadJobScoped(ctx context.Context, workspaceID, jobID string, actor Actor) (contentdb.DocumentDownloadJob, error) {
	var id pgtype.UUID
	if err := id.Scan(jobID); err != nil {
		return contentdb.DocumentDownloadJob{}, ErrDownloadJobNotFound
	}

	job, err := s.repo.GetDownloadJob(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return contentdb.DocumentDownloadJob{}, ErrDownloadJobNotFound
	}

	if err != nil {
		return contentdb.DocumentDownloadJob{}, fmt.Errorf("get download job: %w", err)
	}

	if uuidString(job.WorkspaceID) != workspaceID || uuidString(job.RequestedBy) != actor.UserID {
		return contentdb.DocumentDownloadJob{}, ErrDownloadJobNotFound
	}

	if job.ExpiresAt.Valid && !job.ExpiresAt.Time.After(time.Now()) {
		return contentdb.DocumentDownloadJob{}, ErrDownloadJobNotFound
	}

	return job, nil
}

func (s *ContentService) GetDownloadJob(ctx context.Context, workspaceID, jobID string, actor Actor) (dto.DownloadJobResponse, error) {
	job, err := s.getDownloadJobScoped(ctx, workspaceID, jobID, actor)
	if err != nil {
		return dto.DownloadJobResponse{}, err
	}

	return downloadJobResponse(job), nil
}

func (s *ContentService) ListDownloadJobs(ctx context.Context, workspaceID string, actor Actor) ([]dto.DownloadJobResponse, error) {
	var wID, requester pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return nil, fmt.Errorf("workspace id parse: %w", err)
	}

	if err := requester.Scan(actor.UserID); err != nil {
		return nil, fmt.Errorf("user id parse: %w", err)
	}

	rows, err := s.repo.ListDownloadJobsForUser(ctx, contentdb.ListDownloadJobsForUserParams{
		WorkspaceID: wID,
		RequestedBy: requester,
		LimitCount:  downloadJobListLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("list download jobs: %w", err)
	}

	out := make([]dto.DownloadJobResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, downloadJobResponse(row))
	}

	return out, nil
}

func (s *ContentService) GetDownloadJobObject(ctx context.Context, workspaceID, jobID string, actor Actor) (DownloadJobObject, error) {
	job, err := s.getDownloadJobScoped(ctx, workspaceID, jobID, actor)
	if err != nil {
		return DownloadJobObject{}, err
	}

	if job.Status != DownloadJobStatusReady {
		return DownloadJobObject{}, ErrDownloadJobNotReady
	}

	doc, err := s.getDocumentScoped(ctx, workspaceID, uuidString(job.DocumentID))
	if err != nil {
		return DownloadJobObject{}, err
	}

	access, err := s.resolveViewAccess(ctx, workspaceID, uuidString(doc.FolderID), actor)
	if err != nil {
		return DownloadJobObject{}, err
	}

	if !access.canDownload && !access.canDownloadOriginal {
		return DownloadJobObject{}, ErrContentForbidden
	}

	return DownloadJobObject{
		Key:      job.ObjectKey,
		Size:     job.SizeBytes,
		FileName: downloadName(job.DocumentName),
	}, nil
}

func (s *ContentService) RecordDownloadJobDelivery(ctx context.Context, workspaceID, jobID string, actor Actor) {
	job, err := s.getDownloadJobScoped(ctx, workspaceID, jobID, actor)
	if err != nil {
		return
	}

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionDocumentDownloaded, activityservice.TargetDocument,
		uuidString(job.DocumentID), job.DocumentName,
		map[string]any{"version_no": job.VersionNo, "variant": "watermarked", "async": true}))
}

func (s *ContentService) OpenDownloadJobRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	return s.store.GetRange(ctx, key, offset, length)
}

func downloadJobResponse(job contentdb.DocumentDownloadJob) dto.DownloadJobResponse {
	res := dto.DownloadJobResponse{
		ID:           uuidString(job.ID),
		DocumentID:   uuidString(job.DocumentID),
		VersionID:    uuidString(job.VersionID),
		DocumentName: job.DocumentName,
		FileName:     downloadName(job.DocumentName),
		VersionNo:    job.VersionNo,
		PageCount:    job.PageCount,
		Status:       job.Status,
		SizeBytes:    job.SizeBytes,
		Error:        job.Error,
		CreatedAt:    job.CreatedAt.Time,
	}

	if job.CompletedAt.Valid {
		t := job.CompletedAt.Time
		res.CompletedAt = &t
	}

	if job.ExpiresAt.Valid {
		res.ExpiresAt = job.ExpiresAt.Time
	}

	return res
}

func (s *ContentService) RunDownloadJobSweeper(ctx context.Context, interval time.Duration) {
	s.sweepDownloadJobsOnce(ctx, downloadJobTTL)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepDownloadJobsOnce(ctx, downloadJobTTL)
		}
	}
}

func (s *ContentService) sweepDownloadJobsOnce(ctx context.Context, ttl time.Duration) {
	if _, err := s.store.DeleteOlderThan(ctx, "downloads/", ttl); err != nil {
		log.Printf("download job sweep: delete objects: %v", err)
	}

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-downloadJobStaleAge), Valid: true}

	stale, err := s.repo.ListStalePendingDownloadJobs(ctx, cutoff)
	if err != nil {
		log.Printf("download job sweep: list stale pending: %v", err)
	}

	for _, job := range stale {
		s.markDownloadJobFailed(ctx, job.ID, errors.New("perakitan unduhan terhenti sebelum selesai"))
	}

	expired, err := s.repo.ListExpiredDownloadJobs(ctx)
	if err != nil {
		log.Printf("download job sweep: list expired: %v", err)
		return
	}

	for _, job := range expired {
		if job.ObjectKey != "" {
			if err := s.store.Delete(ctx, job.ObjectKey); err != nil {
				log.Printf("download job sweep: delete %s: %v", job.ObjectKey, err)
				continue
			}
		}

		if err := s.repo.DeleteDownloadJob(ctx, job.ID); err != nil {
			log.Printf("download job sweep: delete row %s: %v", uuidString(job.ID), err)
		}
	}
}
