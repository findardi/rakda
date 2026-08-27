package service

import (
	"context"
	"testing"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/workspace/dto"
	workspacedb "github.com/findardi/rakda/server/internal/workspace/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWorkspaceID = "11111111-1111-1111-1111-111111111111"

type fakeWorkspaceRepo struct {
	getByIDFn    func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error)
	deleteCalled bool
	execTxCalled bool
}

func (f *fakeWorkspaceRepo) CreateWorkspace(ctx context.Context, arg workspacedb.CreateWorkspaceParams) (workspacedb.Workspace, error) {
	return workspacedb.Workspace{}, nil
}

func (f *fakeWorkspaceRepo) DeleteWorkspace(ctx context.Context, id pgtype.UUID) error {
	f.deleteCalled = true
	return nil
}

func (f *fakeWorkspaceRepo) GetWorkspaceByNameAndOwner(ctx context.Context, arg workspacedb.GetWorkspaceByNameAndOwnerParams) (workspacedb.Workspace, error) {
	return workspacedb.Workspace{}, pgx.ErrNoRows
}

func (f *fakeWorkspaceRepo) GetWorkspaceBySlugAndOwner(ctx context.Context, arg workspacedb.GetWorkspaceBySlugAndOwnerParams) (workspacedb.Workspace, error) {
	return workspacedb.Workspace{}, pgx.ErrNoRows
}

func (f *fakeWorkspaceRepo) GetWorkspacesByOwner(ctx context.Context, ownerID pgtype.UUID) ([]workspacedb.Workspace, error) {
	return nil, nil
}

func (f *fakeWorkspaceRepo) GetWorkspaces(ctx context.Context, userID pgtype.UUID) ([]workspacedb.GetWorkspacesRow, error) {
	return nil, nil
}

func (f *fakeWorkspaceRepo) GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
	return f.getByIDFn(ctx, id)
}

func (f *fakeWorkspaceRepo) GetWorkspaceForMember(ctx context.Context, arg workspacedb.GetWorkspaceForMemberParams) (workspacedb.Workspace, error) {
	return workspacedb.Workspace{}, pgx.ErrNoRows
}

func (f *fakeWorkspaceRepo) GetMemberRoleName(ctx context.Context, arg workspacedb.GetMemberRoleNameParams) (string, error) {
	return "", pgx.ErrNoRows
}

func (f *fakeWorkspaceRepo) GetWorkspaceSummary(ctx context.Context, workspaceID pgtype.UUID) (workspacedb.GetWorkspaceSummaryRow, error) {
	return workspacedb.GetWorkspaceSummaryRow{}, nil
}

func (f *fakeWorkspaceRepo) UpdateWorkspace(ctx context.Context, arg workspacedb.UpdateWorkspaceParams) (workspacedb.Workspace, error) {
	return workspacedb.Workspace{}, nil
}

func (f *fakeWorkspaceRepo) UpdateWorkspaceStatus(ctx context.Context, arg workspacedb.UpdateWorkspaceStatusParams) (workspacedb.Workspace, error) {
	return workspacedb.Workspace{Status: arg.Status}, nil
}

func (f *fakeWorkspaceRepo) ExecTx(ctx context.Context, fn func(*workspacedb.Queries, pgx.Tx) error) error {
	f.execTxCalled = true
	return nil
}

type fakeActivityRecorder struct{}

func (fakeActivityRecorder) RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error {
	return nil
}

func workspaceWithStatus(status string) func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
	return func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
		return workspacedb.Workspace{Name: "Ruang Uji", Status: status}, nil
	}
}

func newTestService(repo *fakeWorkspaceRepo) *WorkspaceService {
	return NewWorkspaceService(repo, nil, nil, fakeActivityRecorder{})
}

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusPrepare, StatusActive, true},
		{StatusPrepare, StatusArchive, true},
		{StatusActive, StatusArchive, true},
		{StatusArchive, StatusActive, true},

		{StatusActive, StatusPrepare, false},
		{StatusArchive, StatusPrepare, false},

		{StatusPrepare, StatusPrepare, false},
		{StatusActive, StatusActive, false},
		{StatusArchive, StatusArchive, false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, canTransition(c.from, c.to), "%s -> %s", c.from, c.to)
	}
}

func TestUpdateStatusWorkspace(t *testing.T) {
	t.Run("rejects a status outside the enum", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive)}

		_, err := newTestService(repo).UpdateStatusWorkspace(context.Background(),
			dto.WorkspaceUpdateStatusRequest{ID: testWorkspaceID, Status: "frozen"}, Actor{})

		require.ErrorIs(t, err, ErrInvalidStatus)
		assert.False(t, repo.execTxCalled)
	})

	t.Run("rejects reopening a live room back to prepare", func(t *testing.T) {
		for _, from := range []string{StatusActive, StatusArchive} {
			repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(from)}

			_, err := newTestService(repo).UpdateStatusWorkspace(context.Background(),
				dto.WorkspaceUpdateStatusRequest{ID: testWorkspaceID, Status: StatusPrepare}, Actor{})

			require.ErrorIs(t, err, ErrStatusTransition, from)
			assert.False(t, repo.execTxCalled, from)
		}
	})

	t.Run("rejects a no-op transition", func(t *testing.T) {
		for _, status := range []string{StatusPrepare, StatusActive, StatusArchive} {
			repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(status)}

			_, err := newTestService(repo).UpdateStatusWorkspace(context.Background(),
				dto.WorkspaceUpdateStatusRequest{ID: testWorkspaceID, Status: status}, Actor{})

			require.ErrorIs(t, err, ErrStatusTransition, status)
			assert.False(t, repo.execTxCalled, status)
		}
	})

	t.Run("reports a missing workspace as not found", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{
			getByIDFn: func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
				return workspacedb.Workspace{}, pgx.ErrNoRows
			},
		}

		_, err := newTestService(repo).UpdateStatusWorkspace(context.Background(),
			dto.WorkspaceUpdateStatusRequest{ID: testWorkspaceID, Status: StatusActive}, Actor{})

		require.ErrorIs(t, err, ErrWorkspaceNotFound)
		assert.False(t, repo.execTxCalled)
	})

	t.Run("admits every legal transition", func(t *testing.T) {
		legal := []struct{ from, to string }{
			{StatusPrepare, StatusActive},
			{StatusPrepare, StatusArchive},
			{StatusActive, StatusArchive},
			{StatusArchive, StatusActive},
		}

		for _, c := range legal {
			repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(c.from)}

			_, err := newTestService(repo).UpdateStatusWorkspace(context.Background(),
				dto.WorkspaceUpdateStatusRequest{ID: testWorkspaceID, Status: c.to}, Actor{})

			require.NoError(t, err, "%s -> %s", c.from, c.to)
			assert.True(t, repo.execTxCalled, "%s -> %s", c.from, c.to)
		}
	})
}

func TestDeleteWorkspace(t *testing.T) {
	t.Run("refuses to delete an archived room", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusArchive)}

		err := newTestService(repo).DeleteWorkspace(context.Background(), testWorkspaceID)

		require.ErrorIs(t, err, ErrWorkspaceArchived)
		assert.False(t, repo.deleteCalled)
	})

	t.Run("deletes a room that is not archived", func(t *testing.T) {
		for _, status := range []string{StatusPrepare, StatusActive} {
			repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(status)}

			err := newTestService(repo).DeleteWorkspace(context.Background(), testWorkspaceID)

			require.NoError(t, err, status)
			assert.True(t, repo.deleteCalled, status)
		}
	})
}
