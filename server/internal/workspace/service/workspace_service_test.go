package service

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"strings"
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
	WorkspaceRepository // nil: any query the test did not stub panics loudly
	getByIDFn           func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error)
	deleteCalled        bool
	execTxCalled        bool
	// execTxErr makes the transaction fail without running its body — the
	// branding tests use it to prove a stored object is rolled back.
	execTxErr error
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
	return f.execTxErr
}

// fakeStore records what the service writes and removes.
type fakeStore struct {
	puts     []string
	putTypes []string
	deletes  []string
	prefixes []string
	putErr   error
}

func (f *fakeStore) Put(_ context.Context, key string, r io.Reader, size int64, contentType string) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.puts = append(f.puts, key)
	f.putTypes = append(f.putTypes, contentType)
	return nil
}

func (f *fakeStore) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("png")), nil
}

func (f *fakeStore) Delete(_ context.Context, key string) error {
	f.deletes = append(f.deletes, key)
	return nil
}

func (f *fakeStore) DeletePrefix(_ context.Context, prefix string) error {
	f.prefixes = append(f.prefixes, prefix)
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
	return NewWorkspaceService(repo, nil, nil, fakeActivityRecorder{}, &fakeStore{})
}

func newTestServiceWithStore(repo *fakeWorkspaceRepo, store *fakeStore) *WorkspaceService {
	return NewWorkspaceService(repo, nil, nil, fakeActivityRecorder{}, store)
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

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))))
	return buf.Bytes()
}

func workspaceWithLogo(status string, key *string) func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
	return func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
		return workspacedb.Workspace{ID: id, Name: "Ruang Uji", Slug: "ruang-uji-ab12", Status: status, LogoKey: key}, nil
	}
}

func TestSetLogo(t *testing.T) {
	owner := Actor{UserID: "u", Name: "Owner", Role: "owner"}

	t.Run("archived room is refused before anything is stored", func(t *testing.T) {
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusArchive)}

		_, err := newTestServiceWithStore(repo, store).SetLogo(context.Background(), testWorkspaceID, owner, bytes.NewReader(testPNG(t, 64, 64)))

		require.ErrorIs(t, err, ErrWorkspaceArchived)
		assert.Empty(t, store.puts)
	})

	t.Run("svg is refused as unsupported, nothing stored", func(t *testing.T) {
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive)}

		_, err := newTestServiceWithStore(repo, store).SetLogo(context.Background(), testWorkspaceID, owner, strings.NewReader("<svg xmlns='http://www.w3.org/2000/svg'/>"))

		require.ErrorIs(t, err, ErrLogoUnsupported)
		assert.Empty(t, store.puts)
	})

	t.Run("garbage is invalid, nothing stored", func(t *testing.T) {
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive)}

		_, err := newTestServiceWithStore(repo, store).SetLogo(context.Background(), testWorkspaceID, owner, bytes.NewReader(append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 40)...)))

		require.ErrorIs(t, err, ErrLogoInvalid)
		assert.Empty(t, store.puts)
	})

	t.Run("valid png lands under asset/logo/{workspace}/ as image/png and the old one is dropped", func(t *testing.T) {
		old := "asset/logo/" + testWorkspaceID + "/old.png"
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithLogo(StatusActive, &old)}

		_, err := newTestServiceWithStore(repo, store).SetLogo(context.Background(), testWorkspaceID, owner, bytes.NewReader(testPNG(t, 64, 64)))

		require.NoError(t, err)
		require.Len(t, store.puts, 1)
		assert.True(t, strings.HasPrefix(store.puts[0], "asset/logo/"+testWorkspaceID+"/"), store.puts[0])
		assert.True(t, strings.HasSuffix(store.puts[0], ".png"))
		assert.Equal(t, []string{"image/png"}, store.putTypes)
		assert.True(t, repo.execTxCalled)
		assert.Equal(t, []string{old}, store.deletes)
	})

	t.Run("a failed transaction removes the object it just wrote", func(t *testing.T) {
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive), execTxErr: assert.AnError}

		_, err := newTestServiceWithStore(repo, store).SetLogo(context.Background(), testWorkspaceID, owner, bytes.NewReader(testPNG(t, 64, 64)))

		require.Error(t, err)
		require.Len(t, store.puts, 1)
		assert.Equal(t, store.puts, store.deletes, "the orphan must be deleted")
	})
}

func TestRemoveLogo(t *testing.T) {
	owner := Actor{UserID: "u", Name: "Owner", Role: "owner"}

	t.Run("no logo is a no-op: no transaction, no delete", func(t *testing.T) {
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithLogo(StatusActive, nil)}

		_, err := newTestServiceWithStore(repo, store).RemoveLogo(context.Background(), testWorkspaceID, owner)

		require.NoError(t, err)
		assert.False(t, repo.execTxCalled)
		assert.Empty(t, store.deletes)
	})

	t.Run("existing logo is cleared in a transaction, then deleted", func(t *testing.T) {
		old := "asset/logo/" + testWorkspaceID + "/old.png"
		store := &fakeStore{}
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithLogo(StatusActive, &old)}

		_, err := newTestServiceWithStore(repo, store).RemoveLogo(context.Background(), testWorkspaceID, owner)

		require.NoError(t, err)
		assert.True(t, repo.execTxCalled)
		assert.Equal(t, []string{old}, store.deletes)
	})
}

func TestSetHeroPreset(t *testing.T) {
	owner := Actor{UserID: "u", Name: "Owner", Role: "owner"}

	t.Run("unknown preset is refused", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive)}

		_, err := newTestService(repo).SetHeroPreset(context.Background(), dto.WorkspaceHeroRequest{ID: testWorkspaceID, Preset: "neon"}, owner)

		require.ErrorIs(t, err, ErrHeroPresetInvalid)
		assert.False(t, repo.execTxCalled)
	})

	t.Run("archived room is refused", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusArchive)}

		_, err := newTestService(repo).SetHeroPreset(context.Background(), dto.WorkspaceHeroRequest{ID: testWorkspaceID, Preset: "tide"}, owner)

		require.ErrorIs(t, err, ErrWorkspaceArchived)
	})

	t.Run("known preset and empty (automatic) both commit", func(t *testing.T) {
		for _, preset := range []string{"tide", " ember ", ""} {
			repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive)}

			_, err := newTestService(repo).SetHeroPreset(context.Background(), dto.WorkspaceHeroRequest{ID: testWorkspaceID, Preset: preset}, owner)

			require.NoError(t, err, preset)
			assert.True(t, repo.execTxCalled, preset)
		}
	})
}

func TestApplyBranding(t *testing.T) {
	t.Run("automatic identity is stable and stays in the cool range", func(t *testing.T) {
		a := dto.WorkspaceResponse{}
		b := dto.WorkspaceResponse{}
		applyBranding(&a, "project-falcon-1a2b", nil, nil)
		applyBranding(&b, "project-falcon-1a2b", nil, nil)
		assert.Equal(t, a, b)
		assert.Contains(t, []int{190, 202, 214, 226, 238}, a.HeroHue)
		assert.Less(t, a.HeroPhase, 40)
		assert.Empty(t, a.HeroPreset)
		assert.Empty(t, a.Logo)
	})

	t.Run("preset wins over the slug, unknown preset falls back", func(t *testing.T) {
		known, unknown := "plum", "gone"
		res := dto.WorkspaceResponse{}
		applyBranding(&res, "x", nil, &known)
		assert.Equal(t, "plum", res.HeroPreset)
		assert.Equal(t, 320, res.HeroHue)

		res = dto.WorkspaceResponse{}
		applyBranding(&res, "x", nil, &unknown)
		assert.Empty(t, res.HeroPreset)
		hue, _ := autoHero("x")
		assert.Equal(t, hue, res.HeroHue)
	})

	t.Run("logo version is the uuid segment of the key", func(t *testing.T) {
		key := "asset/logo/ws/0f1e2d3c-aaaa-bbbb-cccc-000000000000.png"
		res := dto.WorkspaceResponse{}
		applyBranding(&res, "x", &key, nil)
		assert.Equal(t, "0f1e2d3c-aaaa-bbbb-cccc-000000000000", res.Logo)
	})
}

func TestDeleteWorkspaceDropsAssets(t *testing.T) {
	store := &fakeStore{}
	repo := &fakeWorkspaceRepo{getByIDFn: workspaceWithStatus(StatusActive)}

	require.NoError(t, newTestServiceWithStore(repo, store).DeleteWorkspace(context.Background(), testWorkspaceID))

	assert.Equal(t, []string{"asset/logo/" + testWorkspaceID + "/"}, store.prefixes)
}

func TestUpdateWorkspace(t *testing.T) {
	owner := Actor{UserID: "u", Name: "Owner", Role: "owner"}
	room := func(status string) func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
		desc := "lama"
		return func(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error) {
			return workspacedb.Workspace{ID: id, Name: "Ruang Uji", Slug: "ruang-uji-ab12", Description: &desc, Status: status}, nil
		}
	}

	t.Run("archived room is refused", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: room(StatusArchive)}

		_, err := newTestService(repo).UpdateWorkspace(context.Background(), dto.WorkspaceUpdateRequest{ID: testWorkspaceID, Name: "Baru", Description: "lama"}, owner)

		require.ErrorIs(t, err, ErrWorkspaceArchived)
		assert.False(t, repo.execTxCalled)
	})

	t.Run("nothing changed writes nothing", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: room(StatusActive)}

		res, err := newTestService(repo).UpdateWorkspace(context.Background(), dto.WorkspaceUpdateRequest{ID: testWorkspaceID, Name: "Ruang Uji", Description: "lama"}, owner)

		require.NoError(t, err)
		assert.False(t, repo.execTxCalled, "an unchanged room must not open a transaction")
		assert.Equal(t, "Ruang Uji", res.Name)
	})

	t.Run("a changed name commits in a transaction", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: room(StatusActive)}

		_, err := newTestService(repo).UpdateWorkspace(context.Background(), dto.WorkspaceUpdateRequest{ID: testWorkspaceID, Name: "Nama Baru", Description: "lama"}, owner)

		require.NoError(t, err)
		assert.True(t, repo.execTxCalled)
	})

	t.Run("a changed description alone also commits", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: room(StatusActive)}

		_, err := newTestService(repo).UpdateWorkspace(context.Background(), dto.WorkspaceUpdateRequest{ID: testWorkspaceID, Name: "Ruang Uji", Description: ""}, owner)

		require.NoError(t, err)
		assert.True(t, repo.execTxCalled)
	})

	t.Run("a name that slugs to nothing is refused", func(t *testing.T) {
		repo := &fakeWorkspaceRepo{getByIDFn: room(StatusActive)}

		_, err := newTestService(repo).UpdateWorkspace(context.Background(), dto.WorkspaceUpdateRequest{ID: testWorkspaceID, Name: "!!!"}, owner)

		require.ErrorIs(t, err, ErrWorkspaceNameInvalid)
		assert.False(t, repo.execTxCalled)
	})
}
