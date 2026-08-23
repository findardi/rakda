package service

import (
	"context"
	"testing"
	"time"

	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	authservice "github.com/findardi/rakda/server/internal/auth/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewInvitation(t *testing.T) {
	future := pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}
	past := pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}

	t.Run("invalid when not found", func(t *testing.T) {
		repo := &fakeRepo{
			getInvFn: func(ctx context.Context, codeHash string) (accessdb.GetInvitationByCodeHashDetailedRow, error) {
				return accessdb.GetInvitationByCodeHashDetailedRow{}, pgx.ErrNoRows
			},
		}

		_, err := newService(repo).PreviewInvitation(context.Background(), "rawtoken")

		assert.ErrorIs(t, err, authservice.ErrInvitationInvalid)
	})

	t.Run("invalid when status not pending", func(t *testing.T) {
		repo := &fakeRepo{
			getInvFn: func(ctx context.Context, codeHash string) (accessdb.GetInvitationByCodeHashDetailedRow, error) {
				return accessdb.GetInvitationByCodeHashDetailedRow{Status: InviteStatusRevoked, ExpiresAt: future}, nil
			},
		}

		_, err := newService(repo).PreviewInvitation(context.Background(), "rawtoken")

		assert.ErrorIs(t, err, authservice.ErrInvitationInvalid)
	})

	t.Run("invalid when expired", func(t *testing.T) {
		repo := &fakeRepo{
			getInvFn: func(ctx context.Context, codeHash string) (accessdb.GetInvitationByCodeHashDetailedRow, error) {
				return accessdb.GetInvitationByCodeHashDetailedRow{Status: InviteStatusPending, ExpiresAt: past}, nil
			},
		}

		_, err := newService(repo).PreviewInvitation(context.Background(), "rawtoken")

		assert.ErrorIs(t, err, authservice.ErrInvitationInvalid)
	})

	t.Run("looks up by hashed token and returns preview", func(t *testing.T) {
		var capturedHash string
		repo := &fakeRepo{
			getInvFn: func(ctx context.Context, codeHash string) (accessdb.GetInvitationByCodeHashDetailedRow, error) {
				capturedHash = codeHash
				return accessdb.GetInvitationByCodeHashDetailedRow{
					Status:        InviteStatusPending,
					ExpiresAt:     future,
					Email:         "invitee@example.com",
					WorkspaceName: "Acme",
					RoleName:      "guest",
				}, nil
			},
		}

		preview, err := newService(repo).PreviewInvitation(context.Background(), "rawtoken")

		require.NoError(t, err)
		assert.Equal(t, "hashed:rawtoken", capturedHash)
		assert.Equal(t, "invitee@example.com", preview.Email)
		assert.Equal(t, "Acme", preview.WorkspaceName)
		assert.Equal(t, "guest", preview.RoleName)
	})
}
