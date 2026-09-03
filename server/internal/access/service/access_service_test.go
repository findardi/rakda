package service

import (
	"context"
	"testing"
	"time"

	"github.com/findardi/rakda/server/internal/access/dto"
	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	authdto "github.com/findardi/rakda/server/internal/auth/dto"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/platform/sender"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	uuidWS      = "11111111-1111-1111-1111-111111111111"
	uuidRole    = "22222222-2222-2222-2222-222222222222"
	uuidMember  = "33333333-3333-3333-3333-333333333333"
	uuidActor   = "44444444-4444-4444-4444-444444444444"
	uuidTarget  = "55555555-5555-5555-5555-555555555555"
	uuidUser    = "66666666-6666-6666-6666-666666666666"
	uuidGroup   = "77777777-7777-7777-7777-777777777777"
	uuidOtherWS = "88888888-8888-8888-8888-888888888888"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	require.NoError(t, u.Scan(s))
	return u
}

func strPtr(s string) *string { return &s }

type fakeToken struct{}

func (fakeToken) Generate() string        { return "rawtoken" }
func (fakeToken) Hash(code string) string { return "hashed:" + code }

type fakeRecorder struct{}

// fakeAuth knows no accounts: every invitee is a future user.
type fakeAuth struct{}

func (fakeAuth) UserExists(context.Context, string) (authdto.UserResponse, error) {
	return authdto.UserResponse{}, nil
}

func (fakeRecorder) Record(context.Context, activityservice.Entry)                 {}
func (fakeRecorder) RecordTx(context.Context, pgx.Tx, activityservice.Entry) error { return nil }

type fakeRepo struct {
	AccessRepository

	getRoleFn       func(context.Context, pgtype.UUID) (accessdb.WorkspaceRole, error)
	getMemberFn     func(context.Context, pgtype.UUID) (accessdb.GetMemberRow, error)
	addMemberFn     func(context.Context, accessdb.AddMemberParams) (accessdb.WorkspaceMember, error)
	updateRoleFn    func(context.Context, accessdb.UpdateRoleParams) (accessdb.WorkspaceMember, error)
	updateExpiryFn  func(context.Context, accessdb.UpdateMemberExpiryParams) (accessdb.WorkspaceMember, error)
	reinviteFn      func(context.Context, accessdb.ReinviteWorkspaceInvitationParams) (accessdb.ReinviteWorkspaceInvitationRow, error)
	insertInvFn     func(context.Context, accessdb.InsertWorkspaceInvitationParams) (accessdb.InsertWorkspaceInvitationRow, error)
	deleteMemberFn  func(context.Context, pgtype.UUID) error
	getInvFn        func(context.Context, string) (accessdb.GetInvitationByCodeHashDetailedRow, error)
	updateGroupQAFn func(context.Context, accessdb.UpdateGroupQAParams) (accessdb.WorkspaceGroup, error)
	getGroupFn      func(context.Context, pgtype.UUID) (accessdb.WorkspaceGroup, error)
	getWsInvFn      func(context.Context, pgtype.UUID) (accessdb.GetWorkspaceInvitationRow, error)
	resendFn        func(context.Context, accessdb.ResendInvitationParams) (accessdb.WorkspaceUserInvitation, error)
}

func (f *fakeRepo) UpdateGroupQA(ctx context.Context, arg accessdb.UpdateGroupQAParams) (accessdb.WorkspaceGroup, error) {
	return f.updateGroupQAFn(ctx, arg)
}

func (f *fakeRepo) GetWorkspaceInvitation(ctx context.Context, id pgtype.UUID) (accessdb.GetWorkspaceInvitationRow, error) {
	return f.getWsInvFn(ctx, id)
}

func (f *fakeRepo) ResendInvitation(ctx context.Context, arg accessdb.ResendInvitationParams) (accessdb.WorkspaceUserInvitation, error) {
	return f.resendFn(ctx, arg)
}

func (f *fakeRepo) GetGroup(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceGroup, error) {
	return f.getGroupFn(ctx, id)
}

func (f *fakeRepo) GetRole(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error) {
	return f.getRoleFn(ctx, id)
}

func (f *fakeRepo) GetMember(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
	return f.getMemberFn(ctx, id)
}

func (f *fakeRepo) AddMember(ctx context.Context, arg accessdb.AddMemberParams) (accessdb.WorkspaceMember, error) {
	return f.addMemberFn(ctx, arg)
}

func (f *fakeRepo) UpdateRole(ctx context.Context, arg accessdb.UpdateRoleParams) (accessdb.WorkspaceMember, error) {
	return f.updateRoleFn(ctx, arg)
}

func (f *fakeRepo) UpdateMemberExpiry(ctx context.Context, arg accessdb.UpdateMemberExpiryParams) (accessdb.WorkspaceMember, error) {
	return f.updateExpiryFn(ctx, arg)
}

func (f *fakeRepo) ReinviteWorkspaceInvitation(ctx context.Context, arg accessdb.ReinviteWorkspaceInvitationParams) (accessdb.ReinviteWorkspaceInvitationRow, error) {
	return f.reinviteFn(ctx, arg)
}

func (f *fakeRepo) InsertWorkspaceInvitation(ctx context.Context, arg accessdb.InsertWorkspaceInvitationParams) (accessdb.InsertWorkspaceInvitationRow, error) {
	return f.insertInvFn(ctx, arg)
}

func (f *fakeRepo) DeleteMember(ctx context.Context, id pgtype.UUID) error {
	return f.deleteMemberFn(ctx, id)
}

func (f *fakeRepo) GetInvitationByCodeHashDetailed(ctx context.Context, codeHash string) (accessdb.GetInvitationByCodeHashDetailedRow, error) {
	return f.getInvFn(ctx, codeHash)
}

func newService(repo AccessRepository) *AccessService {
	return NewAccessService(repo, nil, nil, fakeToken{}, "", fakeRecorder{})
}

func TestGuardRoleAssignment(t *testing.T) {
	cases := []struct {
		name      string
		actorRole string
		target    string
		wantErr   error
	}{
		{"owner role never assignable - by owner", permission.RoleOwner, permission.RoleOwner, ErrCannotAssignOwnerRole},
		{"owner role never assignable - by admin", permission.RoleAdmin, permission.RoleOwner, ErrCannotAssignOwnerRole},
		{"admin only by owner - guest rejected", permission.RoleGuest, permission.RoleAdmin, ErrOnlyOwnerAssignsAdmin},
		{"admin only by owner - admin rejected", permission.RoleAdmin, permission.RoleAdmin, ErrOnlyOwnerAssignsAdmin},
		{"admin by owner ok", permission.RoleOwner, permission.RoleAdmin, nil},
		{"guest by anyone ok", permission.RoleGuest, permission.RoleGuest, nil},
		{"guest by owner ok", permission.RoleOwner, permission.RoleGuest, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.ErrorIs(t, guardRoleAssignment(c.actorRole, c.target), c.wantErr)
		})
	}
}

func TestParseAccessExpiry(t *testing.T) {
	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	past := time.Now().Add(-time.Minute).Format(time.RFC3339)

	cases := []struct {
		name    string
		raw     string
		role    string
		wantSet bool
		wantErr error
	}{
		{"empty means never expires", "", permission.RoleGuest, false, nil},
		{"blank is empty, whatever the role", "   ", permission.RoleAdmin, false, nil},
		{"guest with a future date", future, permission.RoleGuest, true, nil},
		{"admin may not carry one", future, permission.RoleAdmin, false, ErrExpiryGuestOnly},
		{"owner may not carry one", future, permission.RoleOwner, false, ErrExpiryGuestOnly},
		{"past date", past, permission.RoleGuest, false, ErrExpiryInPast},
		{"date without time is not RFC 3339", "2030-10-01", permission.RoleGuest, false, ErrExpiryInvalid},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseAccessExpiry(c.raw, c.role)
			if c.wantErr != nil {
				assert.ErrorIs(t, err, c.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.wantSet, got.Valid)
		})
	}
}

func TestMemberStatus(t *testing.T) {
	cases := []struct {
		name    string
		stored  string
		expires pgtype.Timestamptz
		want    string
	}{
		{"no expiry keeps the stored status", MemberStatusActive, pgtype.Timestamptz{}, MemberStatusActive},
		{"future expiry keeps the stored status", MemberStatusActive,
			pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}, MemberStatusActive},
		{"lapsed expiry reads expired", MemberStatusActive,
			pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}, MemberStatusExpired},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, memberStatus(c.stored, c.expires))
		})
	}
}

func TestAddMemberRejectsOwnerRole(t *testing.T) {
	repo := &fakeRepo{
		getRoleFn: func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error) {
			return accessdb.WorkspaceRole{ID: id, Name: permission.RoleOwner}, nil
		},
	}

	_, err := newService(repo).AddMember(context.Background(), dto.CreateWorkspaceMemberRequest{
		WorkspaceId: uuidWS,
		UserId:      uuidUser,
		RoleId:      uuidRole,
		ActorRole:   permission.RoleOwner,
	})

	assert.ErrorIs(t, err, ErrCannotAssignOwnerRole)
}

func TestAddMembersGroupValidation(t *testing.T) {
	roleNamed := func(name string) func(context.Context, pgtype.UUID) (accessdb.WorkspaceRole, error) {
		return func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error) {
			return accessdb.WorkspaceRole{ID: id, Name: name}, nil
		}
	}
	groupIn := func(ws pgtype.UUID) func(context.Context, pgtype.UUID) (accessdb.WorkspaceGroup, error) {
		return func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceGroup, error) {
			return accessdb.WorkspaceGroup{ID: id, WorkspaceID: ws, Name: "Bidder A"}, nil
		}
	}
	groupMissing := func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceGroup, error) {
		return accessdb.WorkspaceGroup{}, pgx.ErrNoRows
	}
	thisWS := mustUUID(t, uuidWS)
	otherWS := mustUUID(t, uuidOtherWS)

	cases := []struct {
		name      string
		role      string
		actorRole string
		getGroup  func(context.Context, pgtype.UUID) (accessdb.WorkspaceGroup, error)
		wantErr   error
	}{
		{"guest with a group of this workspace passes", permission.RoleGuest, permission.RoleOwner, groupIn(thisWS), nil},
		{"group on an admin invite is rejected", permission.RoleAdmin, permission.RoleOwner, groupIn(thisWS), ErrGroupGuestOnly},
		// nil getGroup: the authority guard must fire before any group lookup.
		{"authority is checked before the group", permission.RoleAdmin, permission.RoleAdmin, nil, ErrOnlyOwnerAssignsAdmin},
		{"missing group", permission.RoleGuest, permission.RoleOwner, groupMissing, ErrGroupNotFound},
		{"group of another workspace", permission.RoleGuest, permission.RoleOwner, groupIn(otherWS), ErrGroupNotFound},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{getRoleFn: roleNamed(c.role), getGroupFn: c.getGroup}

			// No emails: every case is decided in validation, before the
			// per-email loop that would need the auth and mail ports.
			_, err := newService(repo).AddMembers(context.Background(), dto.AddMembersRequest{
				WorkspaceId: uuidWS,
				RoleId:      uuidRole,
				GroupId:     uuidGroup,
			}, Actor{UserID: uuidActor, Role: c.actorRole})

			if c.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, c.wantErr)
		})
	}

	t.Run("malformed group id is an error, not a not-found", func(t *testing.T) {
		repo := &fakeRepo{getRoleFn: roleNamed(permission.RoleGuest), getGroupFn: groupIn(thisWS)}

		_, err := newService(repo).AddMembers(context.Background(), dto.AddMembersRequest{
			WorkspaceId: uuidWS,
			RoleId:      uuidRole,
			GroupId:     "not-a-uuid",
		}, Actor{UserID: uuidActor, Role: permission.RoleOwner})

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrGroupNotFound)
	})
}

func TestAddMembersExpiryValidation(t *testing.T) {
	roleNamed := func(name string) func(context.Context, pgtype.UUID) (accessdb.WorkspaceRole, error) {
		return func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error) {
			return accessdb.WorkspaceRole{ID: id, Name: name}, nil
		}
	}
	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

	cases := []struct {
		name    string
		role    string
		expiry  string
		wantErr error
	}{
		{"guest with a future date passes", permission.RoleGuest, future, nil},
		{"admin invite with a date is rejected", permission.RoleAdmin, future, ErrExpiryGuestOnly},
		{"past date is rejected", permission.RoleGuest, past, ErrExpiryInPast},
		{"malformed date is rejected", permission.RoleGuest, "tomorrow", ErrExpiryInvalid},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := &fakeRepo{getRoleFn: roleNamed(c.role)}

			// No emails: every case is decided in validation, before the
			// per-email loop that would need the auth and mail ports.
			_, err := newService(repo).AddMembers(context.Background(), dto.AddMembersRequest{
				WorkspaceId:     uuidWS,
				RoleId:          uuidRole,
				AccessExpiresAt: c.expiry,
			}, Actor{UserID: uuidActor, Role: permission.RoleOwner})

			if c.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, c.wantErr)
		})
	}
}

// The chosen window must land on the invitation row and in the email; a
// validated-but-dropped date would silently invite for ever.
func TestAddMembersExpiryReachesInvitation(t *testing.T) {
	future := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
	var inserted accessdb.InsertWorkspaceInvitationParams
	repo := &fakeRepo{
		getRoleFn: func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error) {
			return accessdb.WorkspaceRole{ID: id, Name: permission.RoleGuest}, nil
		},
		reinviteFn: func(ctx context.Context, arg accessdb.ReinviteWorkspaceInvitationParams) (accessdb.ReinviteWorkspaceInvitationRow, error) {
			return accessdb.ReinviteWorkspaceInvitationRow{}, pgx.ErrNoRows
		},
		insertInvFn: func(ctx context.Context, arg accessdb.InsertWorkspaceInvitationParams) (accessdb.InsertWorkspaceInvitationRow, error) {
			inserted = arg
			return accessdb.InsertWorkspaceInvitationRow{
				ID:              mustUUID(t, uuidTarget),
				ExpiresAt:       arg.ExpiresAt,
				AccessExpiresAt: arg.AccessExpiresAt,
				WorkspaceName:   "Acme",
			}, nil
		},
	}
	mail := captureMail{bodies: make(chan string, 1)}
	svc := NewAccessService(repo, mail, fakeAuth{}, fakeToken{}, "https://web.test", fakeRecorder{})

	out, err := svc.AddMembers(context.Background(), dto.AddMembersRequest{
		WorkspaceId:     uuidWS,
		RoleId:          uuidRole,
		Email:           []string{"guest@example.com"},
		AccessExpiresAt: future.Format(time.RFC3339),
	}, Actor{UserID: uuidActor, Role: permission.RoleOwner})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, OutcomeInvited, out[0].Outcome)
	require.True(t, inserted.AccessExpiresAt.Valid)
	assert.True(t, inserted.AccessExpiresAt.Time.Equal(future))

	select {
	case body := <-mail.bodies:
		assert.Contains(t, body, sender.FormatDateID(future))
	case <-time.After(2 * time.Second):
		t.Fatal("invite email was not sent")
	}
}

// captureMail hands each sent body to the test; sendInviteEmail fires in a
// goroutine, so the channel is the synchronisation point.
type captureMail struct{ bodies chan string }

func (m captureMail) Send(_ context.Context, _, _, textBody, htmlBody string) error {
	m.bodies <- textBody + htmlBody
	return nil
}

func TestResendInvitationEmailCarriesRenewedExpiry(t *testing.T) {
	stale := time.Now().Add(-48 * time.Hour)
	var renewed time.Time
	repo := &fakeRepo{
		getWsInvFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetWorkspaceInvitationRow, error) {
			return accessdb.GetWorkspaceInvitationRow{
				ID:            id,
				Email:         "guest@example.com",
				Status:        InviteStatusPending,
				ExpiresAt:     pgtype.Timestamptz{Time: stale, Valid: true},
				WorkspaceName: "Acme",
			}, nil
		},
		resendFn: func(ctx context.Context, arg accessdb.ResendInvitationParams) (accessdb.WorkspaceUserInvitation, error) {
			renewed = arg.ExpiresAt.Time
			return accessdb.WorkspaceUserInvitation{ID: arg.ID, ExpiresAt: arg.ExpiresAt}, nil
		},
	}
	mail := captureMail{bodies: make(chan string, 1)}
	svc := NewAccessService(repo, mail, nil, fakeToken{}, "https://web.test", fakeRecorder{})

	err := svc.ResendInvitation(context.Background(), uuidTarget, Actor{UserID: uuidActor, Role: permission.RoleOwner})
	require.NoError(t, err)

	select {
	case body := <-mail.bodies:
		assert.Contains(t, body, sender.FormatDateID(renewed))
		assert.NotContains(t, body, sender.FormatDateID(stale))
	case <-time.After(2 * time.Second):
		t.Fatal("invite email was not sent")
	}
}

func TestUpdateMemberRoleGuards(t *testing.T) {
	t.Run("cannot change own role", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
				return accessdb.GetMemberRow{ID: id, UserID: mustUUID(t, uuidActor), RoleName: strPtr(permission.RoleGuest)}, nil
			},
		}

		_, err := newService(repo).UpdateMemberRole(context.Background(), dto.UpdateMemberRoleRequest{
			MemberID: uuidMember,
			RoleId:   uuidRole,
		}, Actor{UserID: uuidActor, Role: permission.RoleOwner})

		assert.ErrorIs(t, err, ErrCannotChangeSelfRole)
	})

	t.Run("cannot change owner role", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
				return accessdb.GetMemberRow{ID: id, UserID: mustUUID(t, uuidTarget), RoleName: strPtr(permission.RoleOwner)}, nil
			},
		}

		_, err := newService(repo).UpdateMemberRole(context.Background(), dto.UpdateMemberRoleRequest{
			MemberID: uuidMember,
			RoleId:   uuidRole,
		}, Actor{UserID: uuidActor, Role: permission.RoleOwner})

		assert.ErrorIs(t, err, ErrCannotChangeOwnerRole)
	})

	t.Run("guest actor cannot promote to admin", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
				return accessdb.GetMemberRow{ID: id, UserID: mustUUID(t, uuidTarget), RoleName: strPtr(permission.RoleGuest)}, nil
			},
			getRoleFn: func(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error) {
				return accessdb.WorkspaceRole{ID: id, Name: permission.RoleAdmin}, nil
			},
		}

		_, err := newService(repo).UpdateMemberRole(context.Background(), dto.UpdateMemberRoleRequest{
			MemberID: uuidMember,
			RoleId:   uuidRole,
		}, Actor{UserID: uuidActor, Role: permission.RoleGuest})

		assert.ErrorIs(t, err, ErrOnlyOwnerAssignsAdmin)
	})
}

func TestUpdateMemberExpiry(t *testing.T) {
	memberNamed := func(role string, expires pgtype.Timestamptz) func(context.Context, pgtype.UUID) (accessdb.GetMemberRow, error) {
		return func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
			return accessdb.GetMemberRow{
				ID:        id,
				UserID:    mustUUID(t, uuidTarget),
				RoleName:  strPtr(role),
				ExpiresAt: expires,
			}, nil
		}
	}
	capture := func(into *accessdb.UpdateMemberExpiryParams) func(context.Context, accessdb.UpdateMemberExpiryParams) (accessdb.WorkspaceMember, error) {
		return func(ctx context.Context, arg accessdb.UpdateMemberExpiryParams) (accessdb.WorkspaceMember, error) {
			*into = arg
			return accessdb.WorkspaceMember{ID: arg.ID, ExpiresAt: arg.ExpiresAt}, nil
		}
	}
	owner := Actor{UserID: uuidActor, Role: permission.RoleOwner}
	future := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	past := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)

	t.Run("admin target is refused, even when clearing", func(t *testing.T) {
		repo := &fakeRepo{getMemberFn: memberNamed(permission.RoleAdmin, pgtype.Timestamptz{})}

		_, err := newService(repo).UpdateMemberExpiry(context.Background(), dto.UpdateMemberExpiryRequest{
			MemberID: uuidMember,
		}, owner)

		assert.ErrorIs(t, err, ErrExpiryGuestOnly)
	})

	t.Run("past date is refused", func(t *testing.T) {
		repo := &fakeRepo{getMemberFn: memberNamed(permission.RoleGuest, pgtype.Timestamptz{})}

		_, err := newService(repo).UpdateMemberExpiry(context.Background(), dto.UpdateMemberExpiryRequest{
			MemberID:  uuidMember,
			ExpiresAt: strPtr(past),
		}, owner)

		assert.ErrorIs(t, err, ErrExpiryInPast)
	})

	t.Run("future date reaches the row", func(t *testing.T) {
		var got accessdb.UpdateMemberExpiryParams
		repo := &fakeRepo{
			getMemberFn:    memberNamed(permission.RoleGuest, pgtype.Timestamptz{}),
			updateExpiryFn: capture(&got),
		}

		_, err := newService(repo).UpdateMemberExpiry(context.Background(), dto.UpdateMemberExpiryRequest{
			MemberID:  uuidMember,
			ExpiresAt: strPtr(future),
		}, owner)

		require.NoError(t, err)
		assert.True(t, got.ExpiresAt.Valid)
		assert.Equal(t, mustUUID(t, uuidMember), got.ID)
	})

	t.Run("null clears a lapsed member", func(t *testing.T) {
		lapsedAt := pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}
		var got accessdb.UpdateMemberExpiryParams
		repo := &fakeRepo{
			getMemberFn:    memberNamed(permission.RoleGuest, lapsedAt),
			updateExpiryFn: capture(&got),
		}

		_, err := newService(repo).UpdateMemberExpiry(context.Background(), dto.UpdateMemberExpiryRequest{
			MemberID: uuidMember,
		}, owner)

		require.NoError(t, err)
		assert.False(t, got.ExpiresAt.Valid, "clearing must write NULL, not keep the lapsed date")
	})
}

func TestDeleteMemberGuards(t *testing.T) {
	t.Run("cannot remove self", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
				return accessdb.GetMemberRow{ID: id, UserID: mustUUID(t, uuidActor), RoleName: strPtr(permission.RoleGuest)}, nil
			},
		}

		err := newService(repo).DeleteMember(context.Background(), uuidMember, Actor{UserID: uuidActor})

		assert.ErrorIs(t, err, ErrCannotRemoveSelf)
	})

	t.Run("cannot remove owner", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
				return accessdb.GetMemberRow{ID: id, UserID: mustUUID(t, uuidTarget), RoleName: strPtr(permission.RoleOwner)}, nil
			},
		}

		err := newService(repo).DeleteMember(context.Background(), uuidMember, Actor{UserID: uuidActor})

		assert.ErrorIs(t, err, ErrCannotRemoveOwner)
	})

	t.Run("member not found bubbles up", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberFn: func(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error) {
				return accessdb.GetMemberRow{}, pgx.ErrNoRows
			},
		}

		err := newService(repo).DeleteMember(context.Background(), uuidMember, Actor{UserID: uuidActor})

		assert.ErrorIs(t, err, ErrMemberNotFound)
	})
}
