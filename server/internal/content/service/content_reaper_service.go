package service

import (
	"context"
	"log"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/jackc/pgx/v5/pgtype"
)

const multipartSweepAge = 24 * time.Hour

type blobRef struct {
	workspaceID string
	versionID   string
	storageKey  string
}

func (s *ContentService) RunReaper(ctx context.Context, interval time.Duration) {
	s.reapOnce(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reapOnce(ctx)
		}
	}
}

func (s *ContentService) reapOnce(ctx context.Context) {
	cutoff := pgtype.Timestamptz{
		Time:  time.Now().Add(-s.trashRetention),
		Valid: true,
	}
	purgeFolders, purgeDocuments := 0, 0

	folders, err := s.repo.ListExpiredTrashFolders(ctx, cutoff)
	if err != nil {
		log.Printf("reaper: list expired folders: %v", err)
	}

	for _, f := range folders {
		versions, err := s.repo.ListVersionsSweptByFolder(ctx, f.ID)
		if err != nil {
			log.Printf("reaper: list versions for folder %s: %v", f.ID.String(), err)
			continue
		}

		refs := make([]blobRef, 0, len(versions))
		for _, v := range versions {
			refs = append(refs, blobRef{
				workspaceID: v.WorkspaceID.String(),
				versionID:   v.ID.String(),
				storageKey:  v.StorageKey,
			})
		}

		if !s.deleteVersionBlobs(ctx, refs) {
			continue
		}

		if err := s.repo.PurgeFolder(ctx, f.ID); err != nil {
			log.Printf("reaper: purge folder %s: %v", f.ID.String(), err)
			continue
		}

		s.activity.Record(ctx, activityservice.Entry{
			WorkspaceID: f.WorkspaceID.String(),
			Action:      activityservice.ActionFolderPurged,
			TargetType:  activityservice.TargetFolder,
			TargetID:    f.ID.String(),
			TargetName:  f.Name,
		})

		purgeFolders++
	}

	documents, err := s.repo.ListExpiredTrashDocuments(ctx, cutoff)
	if err != nil {
		log.Printf("reaper: list expired documents: %v", err)
	}

	for _, d := range documents {
		versions, err := s.repo.ListVersionByDocument(ctx, d.ID)
		if err != nil {
			log.Printf("reaper: list versions for document %s: %v", d.ID.String(), err)
			continue
		}

		refs := make([]blobRef, 0, len(versions))
		for _, v := range versions {
			refs = append(refs, blobRef{
				workspaceID: d.WorkspaceID.String(),
				versionID:   v.ID.String(),
				storageKey:  v.StorageKey,
			})
		}

		if !s.deleteVersionBlobs(ctx, refs) {
			continue
		}

		if err := s.repo.PurgeDocument(ctx, d.ID); err != nil {
			log.Printf("reaper: purge document %s: %v", d.ID.String(), err)
			continue
		}

		s.activity.Record(ctx, activityservice.Entry{
			WorkspaceID: d.WorkspaceID.String(),
			Action:      activityservice.ActionDocumentPurged,
			TargetType:  activityservice.TargetDocument,
			TargetID:    d.ID.String(),
			TargetName:  d.Name,
		})

		purgeDocuments++
	}

	aborted, err := s.store.AbortIncompleteUploads(ctx, multipartSweepAge)
	if err != nil {
		log.Printf("reaper: abort incomplete upload: %v", err)
	}

	if purgeFolders > 0 || purgeDocuments > 0 || aborted > 0 {
		log.Printf("reaper: purged %d folders, %d documents, aborted %d multipart uploads",
			purgeFolders, purgeDocuments, aborted)
	}

	if evicted, freed := s.renditions.Sweep(0); evicted > 0 {
		log.Printf("reaper: evicted %d cached renditions (%d bytes)", evicted, freed)
	}
}

func (s *ContentService) deleteVersionBlobs(ctx context.Context, refs []blobRef) bool {
	for _, ref := range refs {
		if err := s.store.Delete(ctx, ref.storageKey); err != nil {
			log.Printf("reaper: delete blob %s: %v", ref.storageKey, err)
			return false
		}

		if err := s.store.DeletePrefix(ctx, renditionPrefix(ref.workspaceID, ref.versionID)); err != nil {
			log.Printf("reaper: delete renditions for version %s: %v", ref.versionID, err)
			return false
		}

		if err := s.store.DeletePrefix(ctx, pageCachePrefix(ref.workspaceID, ref.versionID)); err != nil {
			log.Printf("reaper: delete page cache for version %s: %v", ref.versionID, err)
			return false
		}

		s.dropCachedVersion(ref)
	}

	return true
}

func (s *ContentService) dropCachedVersion(ref blobRef) {
	if err := s.renditions.Remove(ref.storageKey); err != nil {
		log.Printf("reaper: drop cached blob %s: %v", ref.storageKey, err)
	}

	if _, err := s.renditions.RemovePrefix(renditionPrefix(ref.workspaceID, ref.versionID)); err != nil {
		log.Printf("reaper: drop cached renditions for version %s: %v", ref.versionID, err)
	}

	if _, err := s.pages.RemovePrefix(pageCachePrefix(ref.workspaceID, ref.versionID)); err != nil {
		log.Printf("reaper: drop cached pages for version %s: %v", ref.versionID, err)
	}
}

// RunPageCacheSweeper menghapus PNG halaman yang lebih tua dari ttl. Ia hanya
// melihat prefix page-cache/ — rendition.pdf di prefix renditions/ tidak
// pernah ikut tersapu (keputusan 9.5-c).
func (s *ContentService) RunPageCacheSweeper(ctx context.Context, interval, ttl time.Duration) {
	s.sweepPageCacheOnce(ctx, ttl)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepPageCacheOnce(ctx, ttl)
		}
	}
}

func (s *ContentService) sweepPageCacheOnce(ctx context.Context, ttl time.Duration) {
	if evicted, freed := s.pages.Sweep(ttl); evicted > 0 {
		log.Printf("page cache sweep: evicted %d local pages (%d bytes)", evicted, freed)
	}

	deleted, err := s.store.DeleteOlderThan(ctx, "page-cache/", ttl)
	if err != nil {
		log.Printf("page cache sweep: %v", err)
		return
	}

	if deleted > 0 {
		log.Printf("page cache sweep: deleted %d page cache objects", deleted)
	}
}
