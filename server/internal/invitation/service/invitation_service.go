package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/findardi/rakda/server/internal/platform/actor"
	"github.com/findardi/rakda/server/internal/platform/ptr"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/invitation/dto"
	invitationdb "github.com/findardi/rakda/server/internal/invitation/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvitationNotFound   = errors.New("invitation not found")
	ErrInvitationForbidden  = errors.New("invitation does not belong to this user")
	ErrInvitationNotPending = errors.New("invitation is no longer pending")
	ErrInvitationExpired    = errors.New("invitation has expired")
)

type InvitationService struct {
	repo     InvitationRepo
	activity ActivityRecorder
}

func NewInvitationService(repo InvitationRepo, activity ActivityRecorder) *InvitationService {
	return &InvitationService{
		repo:     repo,
		activity: activity,
	}
}

type Actor = actor.Actor

func inviteEntry(a Actor, workspaceID, action, invitationID, email string) activityservice.Entry {
	return activityservice.NewEntry(workspaceID, a.UserID, a.Name, "", action, activityservice.TargetInvitation, invitationID, email, nil)
}

func (s *InvitationService) GetListInvitations(ctx context.Context, userID string) ([]dto.GetMyInvitationsRow, error) {
	invitations := []dto.GetMyInvitationsRow{}
	var uID pgtype.UUID
	if err := uID.Scan(userID); err != nil {
		return invitations, fmt.Errorf("parse user id: %w", err)
	}

	invts, err := s.repo.GetMyInvitations(ctx, uID)
	if err != nil {
		return invitations, fmt.Errorf("get invitations: %w", err)
	}

	for _, inv := range invts {
		invitations = append(invitations, dto.GetMyInvitationsRow{
			ID:            inv.ID.String(),
			WorkspaceName: ptr.Deref(inv.WorkspaceName),
			RoleName:      ptr.Deref(inv.RoleName),
			InvitedBy:     ptr.Deref(inv.InvitedName),
			ExpiresAt:     inv.ExpiresAt.Time,
			Status:        inv.Status,
		})
	}

	return invitations, nil
}

func (s *InvitationService) AcceptInvitation(ctx context.Context, invitationID string, actor Actor) error {
	var invID, uID pgtype.UUID
	if err := invID.Scan(invitationID); err != nil {
		return fmt.Errorf("parse invitation id: %w", err)
	}
	if err := uID.Scan(actor.UserID); err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	return s.repo.ExecTxTx(ctx, func(q *invitationdb.Queries, tx pgx.Tx) error {
		inv, err := q.GetWorkspaceInvitation(ctx, invID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvitationNotFound
		}
		if err != nil {
			return fmt.Errorf("get invitation: %w", err)
		}

		if inv.UserID.String() != actor.UserID {
			return ErrInvitationForbidden
		}
		if inv.Status != "pending" {
			return ErrInvitationNotPending
		}
		if inv.ExpiresAt.Valid && inv.ExpiresAt.Time.Before(time.Now()) {
			return ErrInvitationExpired
		}
		// The access window chosen at invite time has already closed: accepting
		// would create a member who is expired on arrival.
		if inv.AccessExpiresAt.Valid && !inv.AccessExpiresAt.Time.After(time.Now()) {
			return ErrInvitationExpired
		}

		if _, err := q.AcceptWorkspaceInvitation(ctx, invitationdb.AcceptWorkspaceInvitationParams{
			ID:     invID,
			UserID: uID,
		}); err != nil {
			return fmt.Errorf("accept invitation: %w", err)
		}

		if err := q.InsertMember(ctx, invitationdb.InsertMemberParams{
			WorkspaceID: inv.WorkspaceID,
			UserID:      uID,
			RoleID:      inv.RoleID,
			ExpiresAt:   inv.AccessExpiresAt,
		}); err != nil {
			return fmt.Errorf("add member: %w", err)
		}

		// A group picked at invite time wins over the default fallback. The
		// insert only fires for guests, so a later role change on the
		// invitation can never drag a non-guest into a group.
		if inv.GroupID.Valid {
			if err := q.AssignToGroup(ctx, invitationdb.AssignToGroupParams{
				GroupID:     inv.GroupID,
				WorkspaceID: inv.WorkspaceID,
				UserID:      uID,
			}); err != nil {
				return fmt.Errorf("assign invited group: %w", err)
			}
		}

		if err := q.AssignDefaultGroupIfGuest(ctx, invitationdb.AssignDefaultGroupIfGuestParams{
			WorkspaceID: inv.WorkspaceID,
			UserID:      uID,
		}); err != nil {
			return fmt.Errorf("assign default group: %w", err)
		}

		return s.activity.RecordTx(ctx, tx,
			inviteEntry(actor, inv.WorkspaceID.String(), activityservice.ActionInviteAccepted, invitationID, inv.Email))
	})
}

func (s *InvitationService) RejectInvitation(ctx context.Context, invitationID string, actor Actor) error {
	var invID pgtype.UUID
	if err := invID.Scan(invitationID); err != nil {
		return fmt.Errorf("parse invitation id: %w", err)
	}

	inv, err := s.repo.GetWorkspaceInvitation(ctx, invID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvitationNotFound
	}
	if err != nil {
		return fmt.Errorf("get invitation: %w", err)
	}

	if inv.UserID.String() != actor.UserID {
		return ErrInvitationForbidden
	}

	if _, err := s.repo.RejectWorkspaceInvitation(ctx, invID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvitationNotPending
		}
		return fmt.Errorf("reject invitation: %w", err)
	}

	s.activity.Record(ctx,
		inviteEntry(actor, inv.WorkspaceID.String(), activityservice.ActionInviteRejected, invitationID, inv.Email))

	return nil
}
