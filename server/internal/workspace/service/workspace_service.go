package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/findardi/rakda/server/internal/workspace/dto"
	workspacedb "github.com/findardi/rakda/server/internal/workspace/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusPrepare = "prepare"
	StatusActive  = "active"
	StatusArchive = "archive"
)

var (
	ErrWorkspaceNameTaken    = errors.New("workspace name already taken")
	ErrWorkspaceNameInvalid  = errors.New("workspace name produces an empty slug")
	ErrWorkspaceNotFound     = errors.New("workspace not found")
	ErrWorkspaceExceedLimits = errors.New("workspace exceeds limit")
	ErrInvalidStatus         = errors.New("invalid workspace status")
)

var slugInvalidChars = regexp.MustCompile(`[^a-z0-9]+`)

type WorkspaceService struct {
	repo    WorkspaceRepository
	access  AccessService
	content ContentProvisioner
}

func NewWorkspaceService(repo WorkspaceRepository, access AccessService, content ContentProvisioner) *WorkspaceService {
	return &WorkspaceService{
		repo:    repo,
		access:  access,
		content: content,
	}
}

func (s *WorkspaceService) slugify(name string) string {
	slug := strings.ToLower(name)
	slug = slugInvalidChars.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	return slug
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

	slug := s.slugify(req.Name)
	if slug == "" {
		return dto.WorkspaceResponse{}, ErrWorkspaceNameInvalid
	}

	cuurentWorkspace, err := s.repo.GetWorkspacesByOwner(ctx, uid)
	if err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("check current workspace: %w", err)
	}
	if len(cuurentWorkspace) >= 3 {
		return dto.WorkspaceResponse{}, ErrWorkspaceExceedLimits
	}

	if _, err := s.repo.GetWorkspaceBySlugAndOwner(ctx, workspacedb.GetWorkspaceBySlugAndOwnerParams{
		OwnerID: uid,
		Slug:    slug,
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

	return dto.WorkspaceResponse{
		ID:          uuidString(workspace.ID),
		OwnerID:     uuidString(workspace.OwnerID),
		Name:        workspace.Name,
		Slug:        workspace.Slug,
		Description: deref(workspace.Description),
		Status:      workspace.Status,
		CreatedAt:   workspace.CreatedAt.Time,
		UpdatedAt:   workspace.UpdatedAt.Time,
	}, nil
}

func (s *WorkspaceService) GetWorkspaces(ctx context.Context, userID string) ([]dto.WorkspaceResponse, error) {
	workspaces := []dto.WorkspaceResponse{}

	var uid pgtype.UUID
	if err := uid.Scan(userID); err != nil {
		return workspaces, fmt.Errorf("parse user id: %w", err)
	}

	workspace, err := s.repo.GetWorkspaces(ctx, uid)
	if err != nil {
		return workspaces, fmt.Errorf("get workspaces: %w", err)
	}

	for _, w := range workspace {
		work := dto.WorkspaceResponse{
			ID:          uuidString(w.ID),
			OwnerID:     uuidString(w.OwnerID),
			Name:        w.Name,
			Slug:        w.Slug,
			Description: deref(w.Description),
			Status:      w.Status,
			CreatedAt:   w.CreatedAt.Time,
			UpdatedAt:   w.UpdatedAt.Time,
		}

		workspaces = append(workspaces, work)
	}

	return workspaces, nil
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
		work := dto.WorkspaceResponse{
			ID:          uuidString(w.ID),
			OwnerID:     uuidString(w.OwnerID),
			Name:        w.Name,
			Slug:        w.Slug,
			Description: deref(w.Description),
			Status:      w.Status,
			CreatedAt:   w.CreatedAt.Time,
			UpdatedAt:   w.UpdatedAt.Time,
		}

		workspaces = append(workspaces, work)
	}

	return workspaces, nil
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, workspaceID string) (dto.WorkspaceResponse, error) {
	var uid pgtype.UUID
	if err := uid.Scan(workspaceID); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	workspace, err := s.repo.GetWorkspaceByID(ctx, uid)

	if errors.Is(err, pgx.ErrNoRows) {
		return dto.WorkspaceResponse{}, ErrWorkspaceNotFound
	} else if err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("get workspace: %w", err)
	}

	return dto.WorkspaceResponse{
		ID:          uuidString(workspace.ID),
		OwnerID:     uuidString(workspace.OwnerID),
		Name:        workspace.Name,
		Slug:        workspace.Slug,
		Description: deref(workspace.Description),
		Status:      workspace.Status,
		CreatedAt:   workspace.CreatedAt.Time,
		UpdatedAt:   workspace.UpdatedAt.Time,
	}, nil
}

func (s *WorkspaceService) UpdateStatusWorkspace(ctx context.Context, req dto.WorkspaceUpdateStatusRequest) error {
	if !isValidStatus(req.Status) {
		return ErrInvalidStatus
	}

	var uid pgtype.UUID
	if err := uid.Scan(req.ID); err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}

	if _, err := s.repo.GetWorkspaceByID(ctx, uid); errors.Is(err, pgx.ErrNoRows) {
		return ErrWorkspaceNotFound
	} else if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}

	if err := s.repo.UpdateWorkspaceStatus(ctx, workspacedb.UpdateWorkspaceStatusParams{
		ID:     uid,
		Status: req.Status,
	}); err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	return nil
}

func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, req dto.WorkspaceUpdateRequest) (dto.WorkspaceResponse, error) {
	var uid pgtype.UUID
	if err := uid.Scan(req.ID); err != nil {
		return dto.WorkspaceResponse{}, fmt.Errorf("parse workspace id: %w", err)
	}

	slug := s.slugify(req.Name)
	if slug == "" {
		return dto.WorkspaceResponse{}, ErrWorkspaceNameInvalid
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}

	w, err := s.repo.UpdateWorkspace(ctx, workspacedb.UpdateWorkspaceParams{
		ID:          uid,
		Name:        req.Name,
		Description: desc,
		Slug:        slug,
	})

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return dto.WorkspaceResponse{}, ErrWorkspaceNotFound
	case isUniqueViolation(err, "workspaces_owner_slug_key"):
		return dto.WorkspaceResponse{}, ErrWorkspaceNameTaken
	case err != nil:
		return dto.WorkspaceResponse{}, fmt.Errorf("update workspace: %w", err)
	}

	return dto.WorkspaceResponse{
		ID:          uuidString(w.ID),
		OwnerID:     uuidString(w.OwnerID),
		Name:        w.Name,
		Slug:        w.Slug,
		Description: deref(w.Description),
		Status:      w.Status,
		CreatedAt:   w.CreatedAt.Time,
		UpdatedAt:   w.UpdatedAt.Time,
	}, nil
}

func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, workspaceID string) error {
	var uid pgtype.UUID
	if err := uid.Scan(workspaceID); err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}

	if _, err := s.repo.GetWorkspaceByID(ctx, uid); errors.Is(err, pgx.ErrNoRows) {
		return ErrWorkspaceNotFound
	} else if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}

	if err := s.repo.DeleteWorkspace(ctx, uid); err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}

	return nil
}
