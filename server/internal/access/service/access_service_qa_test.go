package service

import (
	"context"
	"testing"

	"github.com/findardi/rakda/server/internal/access/dto"
	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(v bool) *bool  { return &v }
func i32Ptr(v int32) *int32 { return &v }

func TestUpdateGroupQA(t *testing.T) {
	actor := Actor{UserID: uuidActor, Name: "Owner", Role: permission.RoleOwner}

	t.Run("unknown group", func(t *testing.T) {
		repo := &fakeRepo{
			updateGroupQAFn: func(ctx context.Context, arg accessdb.UpdateGroupQAParams) (accessdb.WorkspaceGroup, error) {
				return accessdb.WorkspaceGroup{}, pgx.ErrNoRows
			},
		}
		_, err := newService(repo).UpdateGroupQA(context.Background(), dto.UpdateGroupQARequest{
			WorkspaceID: uuidWS,
			GroupID:     uuidTarget,
			QAEnabled:   boolPtr(false),
		}, actor)
		assert.ErrorIs(t, err, ErrGroupNotFound)
	})

	t.Run("updates and echoes qa fields", func(t *testing.T) {
		var captured accessdb.UpdateGroupQAParams
		repo := &fakeRepo{
			updateGroupQAFn: func(ctx context.Context, arg accessdb.UpdateGroupQAParams) (accessdb.WorkspaceGroup, error) {
				captured = arg
				return accessdb.WorkspaceGroup{
					ID:              arg.ID,
					WorkspaceID:     mustUUID(t, uuidWS),
					Name:            "Grup A",
					QaEnabled:       arg.QaEnabled,
					QaQuestionLimit: arg.QaQuestionLimit,
					CreatedAt:       pgtype.Timestamptz{Valid: true},
					UpdatedAt:       pgtype.Timestamptz{Valid: true},
				}, nil
			},
		}

		res, err := newService(repo).UpdateGroupQA(context.Background(), dto.UpdateGroupQARequest{
			WorkspaceID:   uuidWS,
			GroupID:       uuidTarget,
			QAEnabled:     boolPtr(true),
			QuestionLimit: i32Ptr(25),
		}, actor)
		require.NoError(t, err)

		assert.Equal(t, uuidTarget, captured.ID.String())
		assert.Equal(t, uuidWS, captured.WorkspaceID.String())
		assert.True(t, res.QAEnabled)
		require.NotNil(t, res.QAQuestionLimit)
		assert.EqualValues(t, 25, *res.QAQuestionLimit)
	})
}
