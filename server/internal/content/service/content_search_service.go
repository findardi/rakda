package service

import (
	"context"
	"fmt"
	"strings"

	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"
	"github.com/findardi/Riksa-App/server/internal/content/dto"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	searchQueryMaxLength = 100
	searchResultLimit    = 20
)

func (s *ContentService) SearchContent(ctx context.Context, workspaceID, query string, actor Actor) (dto.SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return dto.SearchResponse{Folders: []dto.SearchFolderItem{}, Documents: []dto.SearchDocumentItem{}}, nil
	}
	if len([]rune(query)) > searchQueryMaxLength {
		query = string([]rune(query)[:searchQueryMaxLength])
	}

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return dto.SearchResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	limit := int32(searchResultLimit)

	if actor.bypassesContentAccess() {
		allFolders, err := s.repo.SearchAllFolders(ctx, contentdb.SearchAllFoldersParams{
			WorkspaceID: wID,
			Query:       &query,
			LimitCount:  limit,
		})
		if err != nil {
			return dto.SearchResponse{}, fmt.Errorf("search all folders: %w", err)
		}

		allDocuments, err := s.repo.SearchAllDocuments(ctx, contentdb.SearchAllDocumentsParams{
			WorkspaceID: wID,
			Query:       &query,
			LimitCount:  limit,
		})
		if err != nil {
			return dto.SearchResponse{}, fmt.Errorf("search all documents: %w", err)
		}

		folders := toFolderItems(allFolders)
		documents := toDocumentItems(allDocuments)
		if err := s.attachBreadcrumbs(ctx, wID, folders, documents, actor); err != nil {
			return dto.SearchResponse{}, err
		}

		return dto.SearchResponse{Folders: folders, Documents: documents}, nil
	}

	var uID pgtype.UUID
	if err := uID.Scan(actor.UserID); err != nil {
		return dto.SearchResponse{}, ErrContentForbidden
	}

	visibleFolders, err := s.repo.SearchVisibleFolders(ctx, contentdb.SearchVisibleFoldersParams{
		WorkspaceID: wID,
		UserID:      uID,
		Query:       &query,
		LimitCount:  limit,
	})
	if err != nil {
		return dto.SearchResponse{}, fmt.Errorf("search visible folders: %w", err)
	}

	visibleDocuments, err := s.repo.SearchVisibleDocuments(ctx, contentdb.SearchVisibleDocumentsParams{
		WorkspaceID: wID,
		UserID:      uID,
		Query:       &query,
		LimitCount:  limit,
	})
	if err != nil {
		return dto.SearchResponse{}, fmt.Errorf("search visible documents: %w", err)
	}

	folders := toFolderItemsVisible(visibleFolders)
	documents := toDocumentItemsVisible(visibleDocuments)
	if err := s.attachBreadcrumbs(ctx, wID, folders, documents, actor); err != nil {
		return dto.SearchResponse{}, err
	}

	return dto.SearchResponse{Folders: folders, Documents: documents}, nil
}

// LogSearch mencatat kata kunci ke activity_logs ("search", keyword).
// GET /search wajib bebas efek samping, jadi commit-nya lewat endpoint
// terpisah ini; pencarian yang nihil hasil tetap dicatat.
func (s *ContentService) LogSearch(ctx context.Context, workspaceID, query string, actor Actor) {
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > searchQueryMaxLength {
		return
	}

	s.activity.Record(ctx, s.activityEntry(workspaceID, actor,
		activityservice.ActionSearchPerformed, activityservice.TargetSearch,
		"", query, nil))
}

type breadcrumbNode struct {
	name    string
	visible bool
}

func (s *ContentService) attachBreadcrumbs(ctx context.Context, wID pgtype.UUID, folders []dto.SearchFolderItem, documents []dto.SearchDocumentItem, actor Actor) error {
	folderIDs := make([]pgtype.UUID, 0, len(folders)+len(documents))
	for _, f := range folders {
		var id pgtype.UUID
		if err := id.Scan(f.ID); err != nil {
			return fmt.Errorf("parse folder id: %w", err)
		}
		folderIDs = append(folderIDs, id)
	}
	for _, d := range documents {
		var id pgtype.UUID
		if err := id.Scan(d.FolderID); err != nil {
			return fmt.Errorf("parse document folder id: %w", err)
		}
		folderIDs = append(folderIDs, id)
	}

	if len(folderIDs) == 0 {
		return nil
	}

	var rows []breadcrumbRow
	if actor.bypassesContentAccess() {
		all, err := s.repo.SearchAllFolderBreadcrumbs(ctx, folderIDs)
		if err != nil {
			return fmt.Errorf("breadcrumbs all: %w", err)
		}
		rows = make([]breadcrumbRow, len(all))
		for i, r := range all {
			rows[i] = breadcrumbRow{rootID: uuidString(r.RootID), name: r.Name, visible: r.Visible}
		}
	} else {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return ErrContentForbidden
		}

		visible, err := s.repo.SearchVisibleFolderBreadcrumbs(ctx, contentdb.SearchVisibleFolderBreadcrumbsParams{
			WorkspaceID: wID,
			UserID:      uID,
			FolderIds:   folderIDs,
		})
		if err != nil {
			return fmt.Errorf("breadcrumbs visible: %w", err)
		}
		rows = make([]breadcrumbRow, len(visible))
		for i, r := range visible {
			rows[i] = breadcrumbRow{rootID: uuidString(r.RootID), name: r.Name, visible: r.Visible}
		}
	}

	byRoot := make(map[string][]breadcrumbNode)
	for _, r := range rows {
		byRoot[r.rootID] = append(byRoot[r.rootID], breadcrumbNode{name: r.name, visible: r.visible})
	}

	for i := range folders {
		folders[i].Breadcrumb = joinBreadcrumb(byRoot[folders[i].ID])
	}
	for i := range documents {
		documents[i].Breadcrumb = joinBreadcrumb(byRoot[documents[i].FolderID])
	}

	return nil
}

// joinBreadcrumb merangkai path root→folder dan berhenti sebelum leluhur
// pertama yang tidak terlihat pemanggil, tanpa penanda "…". Leluhur yang
// tidak terlihat bukan sekadar estetika: pohon menyembunyikannya, dan
// menampilkannya di sini membocorkan nama yang sengaja ditutup.
func joinBreadcrumb(nodes []breadcrumbNode) string {
	if len(nodes) == 0 {
		return ""
	}

	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if !n.visible {
			break
		}
		parts = append(parts, n.name)
	}

	return strings.Join(parts, " / ")
}

type breadcrumbRow struct {
	rootID  string
	name    string
	visible bool
}

func toFolderItems(rows []contentdb.SearchAllFoldersRow) []dto.SearchFolderItem {
	items := make([]dto.SearchFolderItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchFolderItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			ParentID: uuidString(r.ParentID),
		})
	}
	return items
}

func toFolderItemsVisible(rows []contentdb.SearchVisibleFoldersRow) []dto.SearchFolderItem {
	items := make([]dto.SearchFolderItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchFolderItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			ParentID: uuidString(r.ParentID),
		})
	}
	return items
}

func toDocumentItems(rows []contentdb.SearchAllDocumentsRow) []dto.SearchDocumentItem {
	items := make([]dto.SearchDocumentItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchDocumentItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			FolderID: uuidString(r.FolderID),
			Mime:     deref(r.Mime),
		})
	}
	return items
}

func toDocumentItemsVisible(rows []contentdb.SearchVisibleDocumentsRow) []dto.SearchDocumentItem {
	items := make([]dto.SearchDocumentItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, dto.SearchDocumentItem{
			ID:       uuidString(r.ID),
			Name:     r.Name,
			FolderID: uuidString(r.FolderID),
			Mime:     deref(r.Mime),
		})
	}
	return items
}
