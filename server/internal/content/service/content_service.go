package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/findardi/Riksa-App/server/internal/content/dto"
	contentdb "github.com/findardi/Riksa-App/server/internal/content/repository/sqlc"
	"github.com/findardi/Riksa-App/server/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxFolderDepth     = 32
	uploadURLTTL       = 15 * time.Minute
	downloadURLTTL     = 5 * time.Minute
	maxBulkFolderNodes = 500
	multipartPartSize  = 8 << 20
	maxMultipartParts  = 1000
	maxPartURLsPerCall = 100
)

var (
	ErrParentCrossWorkspace = errors.New("parent cross workspace")
	ErrParentNotFound       = errors.New("parent not found")
	ErrFolderNameTaken      = errors.New("folder already exists")
	ErrFolderNotFound       = errors.New("folder not found")
	ErrCycle                = errors.New("cannot move folder into its own subtree")
	ErrFolderTreeTooDeep    = errors.New("folder nesting is too deep")
	ErrDocumentNotFound     = errors.New("document not found")
	ErrUploadNotFound       = errors.New("uploaded object not found")
	ErrDeleteDefault        = errors.New("folder is default by system, cant deleted")
	ErrMoveDefault          = errors.New("folder is default by system, cant moved")
	ErrAccessTargetInvalid  = errors.New("group or access level not found in this workspace")
	ErrContentForbidden     = errors.New("no access to this content")
	ErrNotViewable          = errors.New("file type cannot be viewed, download only")
	ErrPageOutOfRange       = errors.New("page out of range")
	ErrBulkTooManyFolders   = errors.New("too many folders in one request")
	ErrBulkTooDeep          = errors.New("folder tree in request is too deep")
	ErrFolderNameInvalid    = errors.New("folder name is invalid")
	ErrInvalidStorageKey    = errors.New("storage key does not belong to this folder")
	ErrUploadTooLarge       = errors.New("file is too large")
	ErrInvalidPartNumber    = errors.New("invalid part number")
	ErrTooManyParts         = errors.New("too many parts requested at once")
	ErrDocumentNameTaken    = errors.New("a document with this name already exists in the folder")
	ErrVersionNotFound      = errors.New("version not found")
	ErrAlreadyCurrent       = errors.New("version is already current")
	ErrNotInTrash           = errors.New("item not found in trash")
)

type ContentService struct {
	repo           ContentRepository
	store          storage.Storage
	viewer         Viewer
	trashRetention time.Duration
}

func NewContentService(repo ContentRepository, store storage.Storage, viewer Viewer, trashRetention time.Duration) *ContentService {
	return &ContentService{
		repo:           repo,
		store:          store,
		viewer:         viewer,
		trashRetention: trashRetention,
	}
}

func (s *ContentService) ProvisionWorkspace(ctx context.Context, tx pgx.Tx, workspaceID, ownerID pgtype.UUID) error {
	q := contentdb.New(tx)
	if _, err := q.CreateDefaultFolder(ctx, contentdb.CreateDefaultFolderParams{
		WorkspaceID: workspaceID,
		Name:        "General",
		CreatedBy:   ownerID,
	}); err != nil {
		return fmt.Errorf("seed default folder: %w", err)
	}
	return nil
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func deref[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

func storageKey(workspaceID, folderID string) string {
	return fmt.Sprintf("%s/%s/%s", workspaceID, folderID, uuid.NewString())
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}

func clampPosition(pos, max int32) int32 {
	if pos < 0 {
		return 0
	}
	if pos > max {
		return max
	}

	return pos
}

func validateBulkNodes(nodes []dto.BulkFolderNode, depth int) (int, error) {
	if depth > maxFolderDepth {
		return 0, ErrBulkTooDeep
	}

	total := 0
	for _, n := range nodes {
		name := strings.TrimSpace(n.Name)
		if name == "" || strings.ContainsAny(name, `/\`) {
			return 0, ErrFolderNameInvalid
		}

		sub, err := validateBulkNodes(n.Children, depth+1)
		if err != nil {
			return 0, err
		}

		total += 1 + sub
	}

	return total, nil
}

func validateStorageKey(key, workspaceID, folderID string) error {
	prefix := fmt.Sprintf("%s/%s/", workspaceID, folderID)
	if !strings.HasPrefix(key, prefix) {
		return ErrInvalidStorageKey
	}

	rest := strings.TrimPrefix(key, prefix)
	if rest == "" || strings.Contains(rest, "/") {
		return ErrInvalidStorageKey
	}

	return nil
}

func (s *ContentService) CreateFolder(ctx context.Context, req dto.CreateFolderRequest) (dto.FolderResponse, error) {

	var wID, pID, cID pgtype.UUID

	if err := cID.Scan(req.CreatedBy); err != nil {
		return dto.FolderResponse{}, fmt.Errorf("user id parse: %w", err)
	}

	if req.ParentID != "" {
		if err := pID.Scan(req.ParentID); err != nil {
			return dto.FolderResponse{}, fmt.Errorf("parent id parse: %w", err)
		}
		pFolder, err := s.repo.GetFolderByID(ctx, pID)
		if errors.Is(err, pgx.ErrNoRows) {
			return dto.FolderResponse{}, ErrParentNotFound
		}

		if err != nil {
			return dto.FolderResponse{}, fmt.Errorf("check parent: %w", err)
		}

		if uuidString(pFolder.WorkspaceID) != req.WorkspaceID {
			return dto.FolderResponse{}, ErrParentCrossWorkspace
		}

		cursor := pFolder
		for depth := 0; ; depth++ {
			if !cursor.ParentID.Valid {
				break
			}

			if depth >= maxFolderDepth {
				return dto.FolderResponse{}, ErrFolderTreeTooDeep
			}

			cursor, err = s.repo.GetFolderByID(ctx, cursor.ParentID)
			if err != nil {
				return dto.FolderResponse{}, fmt.Errorf("walk ancestors: %w", err)
			}
		}
	}

	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.FolderResponse{}, fmt.Errorf("worspace id parse: %w", err)
	}
	maxPos, err := s.repo.GetMaxPositionInParent(ctx, contentdb.GetMaxPositionInParentParams{
		WorkspaceID: wID,
		ParentID:    pID,
	})

	if err != nil {
		return dto.FolderResponse{}, fmt.Errorf("check max position: %w", err)
	}

	f, err := s.repo.CreateFolder(ctx, contentdb.CreateFolderParams{
		WorkspaceID: wID,
		ParentID:    pID,
		Name:        req.Name,
		Position:    maxPos + 1,
		CreatedBy:   cID,
	})

	if isUniqueViolation(err, "folders_name_root_key") || isUniqueViolation(err, "folders_name_per_parent_key") {
		return dto.FolderResponse{}, ErrFolderNameTaken
	}

	if err != nil {
		return dto.FolderResponse{}, fmt.Errorf("create folder: %w", err)
	}

	return dto.FolderResponse{
		ID:          uuidString(f.ID),
		WorkspaceID: uuidString(f.WorkspaceID),
		ParentID:    uuidString(f.ParentID),
		Name:        f.Name,
		Position:    f.Position,
		IsDefault:   f.IsDefault,
		CreatedBy:   uuidString(f.CreatedBy),
		CreatedAt:   f.CreatedAt.Time,
		UpdatedAt:   f.UpdatedAt.Time,
	}, nil
}

func (s *ContentService) MoveFolder(ctx context.Context, req dto.MoveFolderRequest) error {

	var fID, pID pgtype.UUID

	if err := fID.Scan(req.FolderID); err != nil {
		return fmt.Errorf("folder id parse: %w", err)
	}

	if req.ParentID != "" {
		if err := pID.Scan(req.ParentID); err != nil {
			return fmt.Errorf("parent id parse: %w", err)
		}
	}

	return s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
		folder, err := q.GetFolderByID(ctx, fID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrFolderNotFound
		}

		if err != nil {
			return fmt.Errorf("get folder: %w", err)
		}

		if folder.IsDefault {
			return ErrMoveDefault
		}

		if err := q.LockWorkspaceStructure(ctx, folder.WorkspaceID); err != nil {
			return fmt.Errorf("lock workspace structure: %w", err)
		}

		oldParent := folder.ParentID

		if pID.Valid {
			if pID == fID {
				return ErrCycle
			}

			parent, err := q.GetFolderByID(ctx, pID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrParentNotFound
			}

			if err != nil {
				return fmt.Errorf("check parent: %w", err)
			}

			if parent.WorkspaceID != folder.WorkspaceID {
				return ErrParentCrossWorkspace
			}

			cursor := parent
			for depth := 0; ; depth++ {
				if cursor.ID == fID {
					return ErrCycle
				}

				if !cursor.ParentID.Valid {
					break
				}

				if depth >= maxFolderDepth {
					return ErrFolderTreeTooDeep
				}

				cursor, err = q.GetFolderByID(ctx, cursor.ParentID)
				if err != nil {
					return fmt.Errorf("walk ancestors: %w", err)
				}
			}
		}

		maxPos, err := q.GetMaxPositionInParent(ctx, contentdb.GetMaxPositionInParentParams{
			WorkspaceID: folder.WorkspaceID,
			ParentID:    pID,
		})

		if err != nil {
			return fmt.Errorf("check max position: %w", err)
		}

		pos := maxPos + 1
		if req.Position != nil {
			pos = clampPosition(int32(*req.Position), maxPos+1)
		}

		err = q.MoveFolder(ctx, contentdb.MoveFolderParams{
			ID:       fID,
			ParentID: pID,
			Position: pos,
		})

		if isUniqueViolation(err, "folders_name_root_key") || isUniqueViolation(err, "folders_name_per_parent_key") {
			return ErrFolderNameTaken
		}

		if err != nil {
			return fmt.Errorf("move folder: %w", err)
		}

		if err := q.ReindexFolderSiblings(ctx, contentdb.ReindexFolderSiblingsParams{
			WorkspaceID: folder.WorkspaceID,
			ParentID:    pID,
			MovedID:     fID,
		}); err != nil {
			return fmt.Errorf("reindex target siblings: %w", err)
		}

		if oldParent != pID {
			if err := q.ReindexFolderSiblings(ctx, contentdb.ReindexFolderSiblingsParams{
				WorkspaceID: folder.WorkspaceID,
				ParentID:    oldParent,
				MovedID:     fID,
			}); err != nil {
				return fmt.Errorf("reindex source siblings: %w", err)
			}
		}

		return nil

	})
}

func (s *ContentService) GetFoldersTree(ctx context.Context, workspaceID string, actor Actor) ([]dto.FolderTreeNode, error) {

	var wID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return nil, fmt.Errorf("workspace id parse: %w", err)
	}

	var rows []contentdb.Folder
	if actor.bypassesContentAccess() {
		all, err := s.repo.GetFoldersByWorkspace(ctx, wID)
		if err != nil {
			return nil, fmt.Errorf("get folders: %w", err)
		}
		rows = all
	} else {
		var uID pgtype.UUID
		if err := uID.Scan(actor.UserID); err != nil {
			return nil, ErrContentForbidden
		}

		visible, err := s.repo.ListVisibleFolders(ctx, contentdb.ListVisibleFoldersParams{
			WorkspaceID: wID,
			UserID:      uID,
		})
		if err != nil {
			return nil, fmt.Errorf("list visible folders: %w", err)
		}

		rows = make([]contentdb.Folder, 0, len(visible))
		for _, v := range visible {
			rows = append(rows, contentdb.Folder{
				ID:        v.ID,
				ParentID:  v.ParentID,
				Name:      v.Name,
				Position:  v.Position,
				IsDefault: v.IsDefault,
			})
		}
	}

	visibleIDs := make(map[string]struct{}, len(rows))
	for _, f := range rows {
		visibleIDs[uuidString(f.ID)] = struct{}{}
	}

	childrenOf := make(map[string][]contentdb.Folder)
	for _, f := range rows {
		key := uuidString(f.ParentID)
		if _, ok := visibleIDs[key]; !ok {
			key = ""
		}
		childrenOf[key] = append(childrenOf[key], f)
	}

	return buildFolderTree(childrenOf, "", ""), nil
}

func buildFolderTree(childrenOf map[string][]contentdb.Folder, parentKey, prefix string) []dto.FolderTreeNode {
	items := childrenOf[parentKey]
	nodes := make([]dto.FolderTreeNode, 0, len(items))

	for i, f := range items {
		number := prefix + strconv.Itoa(i+1)
		id := uuidString(f.ID)

		nodes = append(nodes, dto.FolderTreeNode{
			ID:        id,
			Name:      f.Name,
			Number:    number,
			Position:  f.Position,
			IsDefault: f.IsDefault,
			Children:  buildFolderTree(childrenOf, id, number+"."),
		})
	}

	return nodes
}

func (s *ContentService) RenameFolder(ctx context.Context, req dto.RenameFolderRequest) (dto.FolderResponse, error) {
	var fID pgtype.UUID
	if err := fID.Scan(req.FolderID); err != nil {
		return dto.FolderResponse{}, fmt.Errorf("folder id parse: %w", err)
	}

	f, err := s.repo.RenameFolder(ctx, contentdb.RenameFolderParams{
		ID:   fID,
		Name: req.Name,
	})

	if isUniqueViolation(err, "folders_name_root_key") || isUniqueViolation(err, "folders_name_per_parent_key") {
		return dto.FolderResponse{}, ErrFolderNameTaken
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return dto.FolderResponse{}, ErrFolderNotFound
	}

	if err != nil {
		return dto.FolderResponse{}, fmt.Errorf("rename folder: %w", err)
	}

	return dto.FolderResponse{
		ID:          uuidString(f.ID),
		WorkspaceID: uuidString(f.WorkspaceID),
		ParentID:    uuidString(f.ParentID),
		Name:        f.Name,
		Position:    f.Position,
		IsDefault:   f.IsDefault,
		CreatedBy:   uuidString(f.CreatedBy),
		CreatedAt:   f.CreatedAt.Time,
		UpdatedAt:   f.UpdatedAt.Time,
	}, nil
}

func (s *ContentService) DeleteFolder(ctx context.Context, folderID, workspaceID string, actor Actor) error {
	var fID pgtype.UUID
	if err := fID.Scan(folderID); err != nil {
		return fmt.Errorf("folder id parse: %w", err)
	}

	folder, err := s.repo.GetFolderByID(ctx, fID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFolderNotFound
	} else if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}

	if uuidString(folder.WorkspaceID) != workspaceID {
		return ErrFolderNotFound
	}

	if folder.IsDefault {
		return ErrDeleteDefault
	}

	var uID pgtype.UUID
	if err := uID.Scan(actor.UserID); err != nil {
		return fmt.Errorf("user id parse: %w", err)
	}

	return s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
		if err := q.SoftDeleteFolderSubtree(ctx, contentdb.SoftDeleteFolderSubtreeParams{
			DeletedBy: uID,
			FolderID:  fID,
		}); err != nil {
			return fmt.Errorf("soft delete folder tree: %w", err)
		}

		if err := q.SoftDeleteDocumentsForFolderRoot(ctx, contentdb.SoftDeleteDocumentsForFolderRootParams{
			DeletedBy: uID,
			FolderID:  fID,
		}); err != nil {
			return fmt.Errorf("soft delete documents: %w", err)
		}

		return nil
	})
}

func (s *ContentService) ensureFolderTree(ctx context.Context, q *contentdb.Queries, wID, parentID, cID pgtype.UUID, nodes []dto.BulkFolderNode, prefix string, out *[]dto.BulkFolderResult) error {
	for _, n := range nodes {
		name := strings.TrimSpace(n.Name)
		path := name
		if prefix != "" {
			path = prefix + "/" + name
		}

		created := false
		f, err := q.GetFolderByNameInParent(ctx, contentdb.GetFolderByNameInParentParams{
			WorkspaceID: wID,
			ParentID:    parentID,
			Name:        name,
		})

		if errors.Is(err, pgx.ErrNoRows) {
			maxPos, posErr := q.GetMaxPositionInParent(ctx, contentdb.GetMaxPositionInParentParams{
				WorkspaceID: wID,
				ParentID:    parentID,
			})

			if posErr != nil {
				return fmt.Errorf("check max position: %w", posErr)
			}

			f, err = q.CreateFolder(ctx, contentdb.CreateFolderParams{
				WorkspaceID: wID,
				ParentID:    parentID,
				Name:        name,
				Position:    maxPos + 1,
				CreatedBy:   cID,
			})

			if err != nil {
				return fmt.Errorf("create folder %q: %w", path, err)
			}

			created = true
		} else if err != nil {
			return fmt.Errorf("lookup folder %q: %w", path, err)
		}

		*out = append(*out, dto.BulkFolderResult{
			Path:    path,
			ID:      uuidString(f.ID),
			Created: created,
		})

		if len(n.Children) > 0 {
			if err := s.ensureFolderTree(ctx, q, wID, f.ID, cID, n.Children, path, out); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *ContentService) BulkCreateFolders(ctx context.Context, req dto.BulkCreateFolderRequest) (dto.BulkCreateFolderResponse, error) {
	var wID, pID, cID pgtype.UUID

	if err := wID.Scan(req.WorkspaceID); err != nil {
		return dto.BulkCreateFolderResponse{}, fmt.Errorf("workspace id parse: %w", err)
	}

	if err := cID.Scan(req.CreatedBy); err != nil {
		return dto.BulkCreateFolderResponse{}, fmt.Errorf("user id parse: %w", err)
	}

	if req.ParentID != "" {
		if err := pID.Scan(req.ParentID); err != nil {
			return dto.BulkCreateFolderResponse{}, fmt.Errorf("parent id parse: %w", err)
		}
	}

	total, err := validateBulkNodes(req.Folders, 1)
	if err != nil {
		return dto.BulkCreateFolderResponse{}, err
	}

	if total > maxBulkFolderNodes {
		return dto.BulkCreateFolderResponse{}, ErrBulkTooManyFolders
	}

	out := make([]dto.BulkFolderResult, 0, total)

	err = s.repo.ExecTx(ctx, func(q *contentdb.Queries) error {
		if err := q.LockWorkspaceStructure(ctx, wID); err != nil {
			return fmt.Errorf("lock workspace structure: %w", err)
		}

		if pID.Valid {
			parent, err := q.GetFolderByID(ctx, pID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrParentNotFound
			}

			if err != nil {
				return fmt.Errorf("check parent: %w", err)
			}

			if parent.WorkspaceID != wID {
				return ErrParentCrossWorkspace
			}
		}

		return s.ensureFolderTree(ctx, q, wID, pID, cID, req.Folders, "", &out)
	})

	if err != nil {
		return dto.BulkCreateFolderResponse{}, err
	}

	return dto.BulkCreateFolderResponse{Folders: out}, nil
}
