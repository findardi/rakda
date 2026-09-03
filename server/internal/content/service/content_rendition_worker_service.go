package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	contentdb "github.com/findardi/rakda/server/internal/content/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/convert"
	"github.com/findardi/rakda/server/internal/platform/render"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *ContentService) RunRenditionWorkers(ctx context.Context) {
	var wg sync.WaitGroup

	for range s.rendition.Workers {
		wg.Add(1)

		go func() {
			defer wg.Done()
			s.RunRenditionWorker(ctx, s.rendition.Interval)
		}()
	}

	wg.Wait()
}

func (s *ContentService) RunRenditionWorker(ctx context.Context, interval time.Duration) {
	s.drainRenditions(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.drainRenditions(ctx)
		case <-s.rendition.Wake:
			s.drainRenditions(ctx)
		}
	}
}

func (s *ContentService) wakeRenditionWorker() {
	select {
	case s.rendition.Wake <- struct{}{}:
	default:
	}
}

func (s *ContentService) drainRenditions(ctx context.Context) {
	for ctx.Err() == nil {
		version, err := s.repo.ClaimPendingRendition(ctx, pgInterval(renditionClaimStaleAge))
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}

		if err != nil {
			log.Printf("rendition worker: claim: %v", err)
			return
		}

		s.runRenditionJob(ctx, version, renditionJobTimeout, renditionBookkeepingTimeout)
	}
}

type renditionOutcome int

const (
	renditionShutdown renditionOutcome = iota
	renditionTransient
	renditionPermanent
)

func (s *ContentService) runRenditionJob(ctx context.Context, version contentdb.DocumentVersion, jobTimeout, bookTimeout time.Duration) {
	bookCtx, cancelBook := context.WithTimeout(context.WithoutCancel(ctx), bookTimeout)
	defer cancelBook()

	doc, err := s.repo.GetDocumentByID(ctx, version.DocumentID)
	if err != nil {
		s.releaseRendition(bookCtx, version.ID, fmt.Errorf("get document: %w", err))
		return
	}

	workCtx, cancelWork := context.WithTimeout(ctx, jobTimeout)
	key, pages, err := s.buildRendition(workCtx, uuidString(doc.WorkspaceID), doc, version)
	cancelWork()

	if err == nil {
		s.finishRendition(bookCtx, doc, version, key, pages)
		return
	}

	switch classifyRenditionError(ctx, err) {
	case renditionShutdown:
		s.releaseRendition(bookCtx, version.ID, err)
	case renditionPermanent:
		s.failRendition(bookCtx, version.ID, err)
	default:
		s.postponeRendition(bookCtx, version, err)
	}
}

func classifyRenditionError(ctx context.Context, err error) renditionOutcome {
	if ctx.Err() != nil {
		return renditionShutdown
	}

	if status, ok := errors.AsType[*convert.StatusError](err); ok && status.Code >= 400 && status.Code < 500 {
		return renditionPermanent
	}

	if errors.Is(err, convert.ErrUnsupportedFile) || errors.Is(err, render.ErrRenderFailed) {
		return renditionPermanent
	}

	return renditionTransient
}

func (s *ContentService) finishRendition(ctx context.Context, doc contentdb.Document, version contentdb.DocumentVersion, key string, pages int) {
	versionID := uuidString(version.ID)
	pc := int32(pages)

	if err := s.repo.SetVersionRendition(ctx, contentdb.SetVersionRenditionParams{
		RenditionKey: &key,
		PageCount:    &pc,
		ID:           version.ID,
	}); err != nil {
		log.Printf("rendition worker: version %s: record rendition: %v", versionID, err)
		return
	}

	if pages > maxRenditionPages {
		log.Printf("rendition worker: version %s ready with %d pages, above the %d cap, not promoted", versionID, pages, maxRenditionPages)
		return
	}

	s.promoteStaged(ctx, doc, version)
	log.Printf("rendition worker: version %s ready (%d pages)", versionID, pages)
}

func (s *ContentService) failRendition(ctx context.Context, versionID pgtype.UUID, cause error) {
	msg := cause.Error()
	log.Printf("rendition worker: version %s failed permanently: %s", uuidString(versionID), msg)

	if err := s.repo.SetVersionRenditionFailure(ctx, contentdb.SetVersionRenditionFailureParams{
		RenditionError: &msg,
		ID:             versionID,
	}); err != nil {
		log.Printf("rendition worker: version %s: record failure: %v", uuidString(versionID), err)
	}
}

func (s *ContentService) postponeRendition(ctx context.Context, version contentdb.DocumentVersion, cause error) {
	attempt := int(version.RenditionAttempts) + 1
	if attempt >= maxRenditionAttempts {
		s.failRendition(ctx, version.ID, fmt.Errorf("gave up after %d attempts: %w", attempt, cause))
		return
	}

	wait := renditionBackoff[min(attempt-1, len(renditionBackoff)-1)]
	msg := cause.Error()
	log.Printf("rendition worker: version %s attempt %d failed, retry in %s: %s", uuidString(version.ID), attempt, wait, msg)

	if err := s.repo.SetVersionRenditionTransientFailure(ctx, contentdb.SetVersionRenditionTransientFailureParams{
		Backoff:        pgInterval(wait),
		RenditionError: &msg,
		ID:             version.ID,
	}); err != nil {
		log.Printf("rendition worker: version %s: record transient failure: %v", uuidString(version.ID), err)
	}
}

func (s *ContentService) releaseRendition(ctx context.Context, versionID pgtype.UUID, cause error) {
	log.Printf("rendition worker: version %s released: %v", uuidString(versionID), cause)

	if err := s.repo.ReleaseRenditionClaim(ctx, versionID); err != nil {
		log.Printf("rendition worker: version %s: release claim: %v", uuidString(versionID), err)
	}
}
