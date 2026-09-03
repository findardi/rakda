package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	authservice "github.com/findardi/rakda/server/internal/auth/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *AccessService) PreviewInvitation(ctx context.Context, token string) (authservice.InvitePreview, error) {
	row, err := s.repo.GetInvitationByCodeHashDetailed(ctx, s.token.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return authservice.InvitePreview{}, authservice.ErrInvitationInvalid
	}

	if err != nil {
		return authservice.InvitePreview{}, fmt.Errorf("get invitation: %w", err)
	}

	if row.Status != InviteStatusPending || (row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now())) {
		return authservice.InvitePreview{}, authservice.ErrInvitationInvalid
	}

	return authservice.InvitePreview{
		Email:         row.Email,
		WorkspaceName: row.WorkspaceName,
		RoleName:      row.RoleName,
	}, nil
}

func (s *AccessService) ConsumeInvitation(ctx context.Context, tx pgx.Tx, token, newUserID string) error {
	q := accessdb.New(tx)

	inv, err := q.GetWorkspaceInvitationByCodeHash(ctx, s.token.Hash(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return authservice.ErrInvitationInvalid
	}

	if err != nil {
		return fmt.Errorf("get invitation: %w", err)
	}

	if inv.Status != InviteStatusPending ||
		(inv.ExpiresAt.Valid && inv.ExpiresAt.Time.Before(time.Now())) {
		return authservice.ErrInvitationInvalid
	}

	var uID pgtype.UUID
	if err := uID.Scan(newUserID); err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	if _, err := q.AcceptWorkspaceInvitation(ctx, accessdb.AcceptWorkspaceInvitationParams{
		ID:     inv.ID,
		UserID: uID,
	}); err != nil {
		return fmt.Errorf("accept invitation: %w", err)
	}

	if _, err := q.AddMember(ctx, accessdb.AddMemberParams{
		WorkspaceID: inv.WorkspaceID,
		UserID:      uID,
		RoleID:      inv.RoleID,
		Status:      MemberStatusActive,
	}); err != nil {
		return fmt.Errorf("add member: %w", err)
	}

	// A group picked at invite time wins over the default fallback. The
	// insert only fires for guests, so a non-guest invite is never dragged
	// into a group.
	if inv.GroupID.Valid {
		if err := q.AssignToGroup(ctx, accessdb.AssignToGroupParams{
			GroupID:     inv.GroupID,
			WorkspaceID: inv.WorkspaceID,
			UserID:      uID,
		}); err != nil {
			return fmt.Errorf("assign invited group: %w", err)
		}
	}

	if err := q.AssignDefaultGroupIfGuest(ctx, accessdb.AssignDefaultGroupIfGuestParams{
		WorkspaceID: inv.WorkspaceID,
		UserID:      uID,
	}); err != nil {
		return fmt.Errorf("assign default group: %w", err)
	}

	return nil
}
