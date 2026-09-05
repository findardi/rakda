package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/workspace/dto"
	workspacedb "github.com/findardi/rakda/server/internal/workspace/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusPrepare = permission.RoomPrepare
	StatusActive  = permission.RoomActive
	StatusArchive = permission.RoomArchive
)

const OwnedWorkspaceLimit = 3

var statusTransitions = map[string][]string{
	StatusPrepare: {StatusActive, StatusArchive},
	StatusActive:  {StatusArchive},
	StatusArchive: {StatusActive},
}

func canTransition(from, to string) bool {
	for _, allowed := range statusTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

type Actor struct {
	UserID string
	Name   string
	Role   string
}

var (
	ErrWorkspaceNameTaken    = errors.New("workspace name already taken")
	ErrWorkspaceNameInvalid  = errors.New("workspace name produces an empty slug")
	ErrWorkspaceNotFound     = errors.New("workspace not found")
	ErrWorkspaceExceedLimits = errors.New("workspace exceeds limit")
	ErrInvalidStatus         = errors.New("invalid workspace status")
	ErrStatusTransition      = errors.New("workspace status transition not allowed")
	ErrWorkspaceArchived     = errors.New("workspace is archived")
	ErrWorkspaceForbidden    = errors.New("workspace access denied")
)

var slugInvalidChars = regexp.MustCompile(`[^a-z0-9]+`)

type WorkspaceService struct {
	repo     WorkspaceRepository
	access   Provisioner
	content  Provisioner
	activity ActivityRecorder
	store    AssetStore
}

func NewWorkspaceService(repo WorkspaceRepository, access, content Provisioner, activity ActivityRecorder, store AssetStore) *WorkspaceService {
	return &WorkspaceService{
		repo:     repo,
		access:   access,
		content:  content,
		activity: activity,
		store:    store,
	}
}

// workspaceResponse is the one mapper every read path goes through, so a field
// added here (branding, say) reaches the list, the detail, and the update
// response alike.
func workspaceResponse(w workspacedb.Workspace) dto.WorkspaceResponse {
	res := dto.WorkspaceResponse{
		ID:          uuidString(w.ID),
		OwnerID:     uuidString(w.OwnerID),
		Name:        w.Name,
		Slug:        w.Slug,
		Description: deref(w.Description),
		Status:      w.Status,
		CreatedAt:   w.CreatedAt.Time,
		UpdatedAt:   w.UpdatedAt.Time,
	}
	applyBranding(&res, w.Slug, w.LogoKey, w.HeroPreset)
	return res
}

func (s *WorkspaceService) slugBase(name string) string {
	slug := strings.ToLower(name)
	slug = slugInvalidChars.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

func (s *WorkspaceService) slugify(name string) string {
	randomID := make([]byte, 4)
	_, _ = rand.Read(randomID)
	return s.slugBase(name) + "-" + hex.EncodeToString(randomID)
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func isValidStatus(status string) bool {
	switch status {
	case StatusPrepare, StatusActive, StatusArchive:
		return true
	}
	return false
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}

func (s *WorkspaceService) CreateWorkspace(ctx context.Context, req dto.WorkspaceCreateRequest) (dto.WorkspaceResponse, error) {
	var uid pgtype.UUID
	if err := uid.Scan(req.OwnerID); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("parse owner id: %w", err)
	}

	if s.slugBase(req.Name) == "" {
		return dto.WorkspaceResponse{}, ErrWorkspaceNameInvalid
	}
	slug := s.slugify(req.Name)

	cuurentWorkspace, err := s.repo.GetWorkspacesByOwner(ctx, uid)
	if err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("check current workspace: %w", err)
	}
	if len(cuurentWorkspace) >= OwnedWorkspaceLimit {
		return dto.WorkspaceResponse{}, ErrWorkspaceExceedLimits
	}

	if _, err := s.repo.GetWorkspaceByNameAndOwner(ctx, workspacedb.GetWorkspaceByNameAndOwnerParams{
		OwnerID: uid,
		Name:    req.Name,
	}); err == nil {
		return dto.WorkspaceResponse{}, ErrWorkspaceNameTaken
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	var workspace workspacedb.Workspace
	err = s.repo.ExecTx(ctx, func(q *workspacedb.Queries, tx pgx.Tx) error {
		w, err := q.CreateWorkspace(ctx, workspacedb.CreateWorkspaceParams{
			OwnerID:     uid,
			Name:        req.Name,
			Slug:        slug,
			Description: desc,
			Status:      StatusPrepare,
		})
		if err != nil {
			return fmt.Errorf("create workspace: %w", err)
		}

		if err := s.content.ProvisionWorkspace(ctx, tx, w.ID, uid); err != nil {
			return fmt.Errorf("provision workspace content: %w", err)
		}

		if err := s.access.ProvisionWorkspace(ctx, tx, w.ID, uid); err != nil {
			return fmt.Errorf("provision workspace access: %w", err)
		}

		workspace = w
		return nil
	})
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	return workspaceResponse(workspace), nil
}

func (s *WorkspaceService) GetWorkspaces(ctx context.Context, userID string) (dto.WorkspaceListResponse, error) {
	res := dto.WorkspaceListResponse{
		Workspaces: []dto.WorkspaceResponse{},
		OwnedLimit: OwnedWorkspaceLimit,
	}

	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return res, fmt.Errorf("parse user id: %w", err)
	}

	rows, err := s.repo.GetWorkspaces(ctx, uid)
	if err != nil {
		return res, fmt.Errorf("get workspaces: %w", err)
	}

	for _, w := range rows {
		work := dto.WorkspaceResponse{
			ID:          uuidString(w.ID),
			OwnerID:     uuidString(w.OwnerID),
			Name:        w.Name,
			Slug:        w.Slug,
			Description: deref(w.Description),
			Status:      w.Status,
			CreatedAt:   w.CreatedAt.Time,
			UpdatedAt:   w.UpdatedAt.Time,
			Role:        w.RoleName,
		}
		applyBranding(&work, w.Slug, w.LogoKey, w.HeroPreset)
		if w.LastActivityAt.Valid {
			t := w.LastActivityAt.Time
			work.LastActivityAt = &t
		}
		if w.RoleName == permission.RoleOwner {
			res.OwnedCount++
		}

		res.Workspaces = append(res.Workspaces, work)
	}

	return res, nil
}

func (s *WorkspaceService) GetWorkspacesByOwner(ctx context.Context, userID string) ([]dto.WorkspaceResponse, error) {
	workspaces := []dto.WorkspaceResponse{}

	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return workspaces, fmt.Errorf("parse user id: %w", err)
	}

	workspace, err := s.repo.GetWorkspacesByOwner(ctx, uid)
	if err != nil {
		return workspaces, fmt.Errorf("get workspaces: %w", err)
	}

	for _, w := range workspace {
		work := workspaceResponse(w)

		workspaces = append(workspaces, work)
	}

	return workspaces, nil
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, workspaceID, actorID string) (dto.WorkspaceResponse, error) {
	var uid, aid pgtype.UUID
	if err := uid.Scan(workspaceID); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := aid.Scan(actorID); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("parse actor id: %w", err)
	}

	workspace, err := s.repo.GetWorkspaceForMember(ctx, workspacedb.GetWorkspaceForMemberParams{
		WorkspaceID: uid,
		UserID:      aid,
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return dto.WorkspaceResponse{}, ErrWorkspaceNotFound
	} else if err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("get workspace: %w", err)
	}

	return workspaceResponse(workspace), nil
}

func (s *WorkspaceService) GetWorkspaceSummary(ctx context.Context, workspaceID, actorID string) (dto.WorkspaceSummaryResponse, error) {
	var wid, aid pgtype.UUID
	if err := wid.Scan(workspaceID); err != nil {
		return dto.WorkspaceSummaryResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}
	if err := aid.Scan(actorID); err != nil {
		return dto.WorkspaceSummaryResponse{}, fmt.Errorf("parse actor id: %w", err)
	}

	role, err := s.repo.GetMemberRoleName(ctx, workspacedb.GetMemberRoleNameParams{
		WorkspaceID: wid,
		UserID:      aid,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.WorkspaceSummaryResponse{}, ErrWorkspaceNotFound
	} else if err != nil {
		return dto.WorkspaceSummaryResponse{}, fmt.Errorf("get member role: %w", err)
	}

	if role != permission.RoleOwner && role != permission.RoleAdmin {
		return dto.WorkspaceSummaryResponse{}, ErrWorkspaceForbidden
	}

	row, err := s.repo.GetWorkspaceSummary(ctx, wid)
	if err != nil {
		return dto.WorkspaceSummaryResponse{}, fmt.Errorf("get workspace summary: %w", err)
	}

	return dto.WorkspaceSummaryResponse{
		DocumentCount: row.DocumentCount,
		FolderCount:   row.FolderCount,
		GuestCount:    row.GuestCount,
	}, nil
}

func (s *WorkspaceService) UpdateStatusWorkspace(ctx context.Context, req dto.WorkspaceUpdateStatusRequest, actor Actor) (dto.WorkspaceResponse, error) {
	if !isValidStatus(req.Status) {
		return dto.WorkspaceResponse{}, ErrInvalidStatus
	}

	var uid pgtype.UUID
	if err := uid.Scan(req.ID); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	current, err := s.repo.GetWorkspaceByID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return dto.WorkspaceResponse{}, ErrWorkspaceNotFound
	} else if err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("get workspace: %w", err)
	}

	if !canTransition(current.Status, req.Status) {
		return dto.WorkspaceResponse{}, ErrStatusTransition
	}

	var updated workspacedb.Workspace
	err = s.repo.ExecTx(ctx, func(q *workspacedb.Queries, tx pgx.Tx) error {
		w, err := q.UpdateWorkspaceStatus(ctx, workspacedb.UpdateWorkspaceStatusParams{
			ID:         uid,
			Status:     req.Status,
			FromStatus: current.Status,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStatusTransition
		}
		if err != nil {
			return fmt.Errorf("update status: %w", err)
		}

		if err := s.activity.RecordTx(ctx, tx, activityservice.Entry{
			WorkspaceID: req.ID,
			ActorID:     actor.UserID,
			ActorName:   actor.Name,
			ActorRole:   actor.Role,
			Action:      activityservice.ActionWorkspaceStatusChanged,
			TargetType:  activityservice.TargetWorkspace,
			TargetID:    req.ID,
			TargetName:  w.Name,
			Metadata: map[string]any{
				"from": current.Status,
				"to":   req.Status,
			},
		}); err != nil {
			return err
		}

		updated = w
		return nil
	})
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	return workspaceResponse(updated), nil
}

// UpdateWorkspace renames and/or re-describes a room. A request that changes
// nothing writes nothing — no row, no audit line. The archive check duplicates
// the route gate on purpose: the middleware is the contract, this is what a
// caller without a router still gets.
func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, req dto.WorkspaceUpdateRequest, actor Actor) (dto.WorkspaceResponse, error) {
	uid, current, err := s.writableWorkspace(ctx, req.ID)
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	slug := current.Slug
	nameChanged := req.Name != current.Name
	if nameChanged {
		if s.slugBase(req.Name) == "" {
			return dto.WorkspaceResponse{}, ErrWorkspaceNameInvalid
		}

		if _, err := s.repo.GetWorkspaceByNameAndOwner(ctx, workspacedb.GetWorkspaceByNameAndOwnerParams{
			OwnerID: current.OwnerID,
			Name:    req.Name,
		}); err == nil {
			return dto.WorkspaceResponse{}, ErrWorkspaceNameTaken
		}

		slug = s.slugify(req.Name)
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	descChanged := deref(desc) != deref(current.Description)

	if !nameChanged && !descChanged {
		return workspaceResponse(current), nil
	}

	var updated workspacedb.Workspace
	err = s.repo.ExecTx(ctx, func(q *workspacedb.Queries, tx pgx.Tx) error {
		w, err := q.UpdateWorkspace(ctx, workspacedb.UpdateWorkspaceParams{
			ID:          uid,
			Name:        req.Name,
			Description: desc,
			Slug:        slug,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrWorkspaceNotFound
		case isUniqueViolation(err, "workspaces_owner_slug_key"):
			return ErrWorkspaceNameTaken
		case err != nil:
			return fmt.Errorf("update workspace: %w", err)
		}

		if err := s.activity.RecordTx(ctx, tx, activityservice.Entry{
			WorkspaceID: req.ID,
			ActorID:     actor.UserID,
			ActorName:   actor.Name,
			ActorRole:   actor.Role,
			Action:      activityservice.ActionWorkspaceUpdated,
			TargetType:  activityservice.TargetWorkspace,
			TargetID:    req.ID,
			TargetName:  w.Name,
			Metadata: map[string]any{
				"from":                current.Name,
				"to":                  w.Name,
				"description_changed": descChanged,
			},
		}); err != nil {
			return err
		}

		updated = w
		return nil
	})
	if err != nil {
		return dto.WorkspaceResponse{}, err
	}

	return workspaceResponse(updated), nil
}

func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	var uid pgtype.UUID
	if err := uid.Scan(workspaceID); err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}

	current, err := s.repo.GetWorkspaceByID(ctx, uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrWorkspaceNotFound
	} else if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}

	if current.Status == StatusArchive {
		return ErrWorkspaceArchived
	}

	if err := s.repo.DeleteWorkspace(ctx, uid); err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}

	// The row is gone; its logo would otherwise outlive it for ever. Best
	// effort, like every other object cleanup behind a committed row.
	if err := s.store.DeletePrefix(ctx, logoObjectPrefix(workspaceID)); err != nil {
		log.Printf("workspace: delete assets of %s: %v", workspaceID, err)
	}

	return nil
}
