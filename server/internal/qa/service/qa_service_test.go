package service

import (
	"context"
	"testing"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/permission"
	"github.com/findardi/rakda/server/internal/qa/dto"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	uuidWS       = "11111111-1111-1111-1111-111111111111"
	uuidGroup    = "22222222-2222-2222-2222-222222222222"
	uuidGroupB   = "33333333-3333-3333-3333-333333333333"
	uuidGuest    = "44444444-4444-4444-4444-444444444444"
	uuidQuestion = "55555555-5555-5555-5555-555555555555"
	uuidDoc      = "66666666-6666-6666-6666-666666666666"
	uuidFolder   = "77777777-7777-7777-7777-777777777777"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	require.NoError(t, u.Scan(s))
	return u
}

func i32Ptr(v int32) *int32 { return &v }

type fakeRecorder struct{}

func (fakeRecorder) RecordTx(context.Context, pgx.Tx, activityservice.Entry) error { return nil }

type fakeChecker struct {
	allowFn func(ctx context.Context, workspaceID, folderID, userID string) (bool, error)
}

func (f fakeChecker) CanUserViewFolder(ctx context.Context, workspaceID, folderID, userID string) (bool, error) {
	if f.allowFn == nil {
		return true, nil
	}
	return f.allowFn(ctx, workspaceID, folderID, userID)
}

type fakeRepo struct {
	QARepository

	getMemberGroupQAFn func(context.Context, qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error)
	listQuestionsFn    func(context.Context, qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error)
	countQuestionsFn   func(context.Context, qadb.CountQuestionsParams) (int64, error)
	countWaitingFn     func(context.Context, pgtype.UUID) (int64, error)
	getQuestionFn      func(context.Context, qadb.GetQuestionParams) (qadb.Question, error)
	listRepliesFn      func(context.Context, pgtype.UUID) ([]qadb.QuestionReply, error)
	countFaqsFn        func(context.Context, pgtype.UUID) (int64, error)
	getDocumentRefFn   func(context.Context, pgtype.UUID) (qadb.GetDocumentForRefRow, error)
	getFolderRefFn     func(context.Context, pgtype.UUID) (qadb.GetFolderForRefRow, error)
}

func (f *fakeRepo) GetMemberGroupQA(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
	return f.getMemberGroupQAFn(ctx, arg)
}

func (f *fakeRepo) ListQuestions(ctx context.Context, arg qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error) {
	return f.listQuestionsFn(ctx, arg)
}

func (f *fakeRepo) CountQuestions(ctx context.Context, arg qadb.CountQuestionsParams) (int64, error) {
	return f.countQuestionsFn(ctx, arg)
}

func (f *fakeRepo) CountWaitingQuestions(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
	return f.countWaitingFn(ctx, workspaceID)
}

func (f *fakeRepo) GetQuestion(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
	return f.getQuestionFn(ctx, arg)
}

func (f *fakeRepo) ListQuestionReplies(ctx context.Context, questionID pgtype.UUID) ([]qadb.QuestionReply, error) {
	return f.listRepliesFn(ctx, questionID)
}

func (f *fakeRepo) CountFaqs(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
	return f.countFaqsFn(ctx, workspaceID)
}

func (f *fakeRepo) GetDocumentForRef(ctx context.Context, id pgtype.UUID) (qadb.GetDocumentForRefRow, error) {
	return f.getDocumentRefFn(ctx, id)
}

func (f *fakeRepo) GetFolderForRef(ctx context.Context, id pgtype.UUID) (qadb.GetFolderForRefRow, error) {
	return f.getFolderRefFn(ctx, id)
}

func newService(repo QARepository, checker ContentAccessChecker) *QAService {
	if checker == nil {
		checker = fakeChecker{}
	}
	return NewQAService(repo, checker, fakeRecorder{})
}

func guestActor() Actor {
	return Actor{UserID: uuidGuest, Name: "Guest", Role: permission.RoleGuest}
}

func managerActor() Actor {
	return Actor{UserID: uuidGuest, Name: "Owner", Role: permission.RoleOwner}
}

func memberGroup(t *testing.T, enabled bool, limit *int32) qadb.GetMemberGroupQARow {
	return qadb.GetMemberGroupQARow{
		ID:              mustUUID(t, uuidGroup),
		Name:            "Grup A",
		QaEnabled:       enabled,
		QaQuestionLimit: limit,
	}
}

func sampleQuestion(t *testing.T, groupID string, status string) qadb.Question {
	q := qadb.Question{
		ID:          mustUUID(t, uuidQuestion),
		WorkspaceID: mustUUID(t, uuidWS),
		GroupName:   "Grup A",
		Number:      1,
		AuthorID:    mustUUID(t, uuidGuest),
		AuthorName:  "Guest",
		Subject:     "subject",
		Body:        "body",
		Status:      status,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	if groupID != "" {
		q.GroupID = mustUUID(t, groupID)
	}
	return q
}

func TestNextStatusAfterReply(t *testing.T) {
	cases := []struct {
		name string
		role string
		want string
	}{
		{"guest reply flips back to waiting", permission.RoleGuest, StatusWaiting},
		{"admin reply marks answered", permission.RoleAdmin, StatusAnswered},
		{"owner reply marks answered", permission.RoleOwner, StatusAnswered},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, nextStatusAfterReply(c.role))
		})
	}
}

func TestParseQuestionCursor(t *testing.T) {
	t.Run("roundtrip", func(t *testing.T) {
		at := time.Now().Truncate(time.Microsecond)
		cursor := questionCursor(at, uuidQuestion)

		createdAt, id, err := parseQuestionCursor(cursor)
		require.NoError(t, err)
		assert.Equal(t, at.UnixMicro(), createdAt.Time.UnixMicro())
		assert.Equal(t, uuidQuestion, id.String())
	})

	for _, cursor := range []string{"abc", "12_not-a-uuid", "x_" + uuidQuestion} {
		t.Run("invalid "+cursor, func(t *testing.T) {
			_, _, err := parseQuestionCursor(cursor)
			assert.ErrorIs(t, err, ErrInvalidCursor)
		})
	}
}

func TestSubmitQuestionGuards(t *testing.T) {
	t.Run("manager cannot ask", func(t *testing.T) {
		_, err := newService(&fakeRepo{}, nil).SubmitQuestion(context.Background(),
			dto.SubmitQuestionRequest{WorkspaceID: uuidWS, Subject: "s", Body: "b"}, managerActor())
		assert.ErrorIs(t, err, ErrOnlyGuestCanAsk)
	})

	t.Run("disabled group rejects submit", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, false, nil), nil
			},
		}
		_, err := newService(repo, nil).SubmitQuestion(context.Background(),
			dto.SubmitQuestionRequest{WorkspaceID: uuidWS, Subject: "s", Body: "b"}, guestActor())
		assert.ErrorIs(t, err, ErrQADisabled)
	})
}

func TestSubmitQuestionRefGuards(t *testing.T) {
	baseRepo := func() *fakeRepo {
		return &fakeRepo{
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, true, nil), nil
			},
		}
	}
	req := dto.SubmitQuestionRequest{WorkspaceID: uuidWS, Subject: "s", Body: "b", DocumentID: uuidDoc}

	t.Run("deleted document", func(t *testing.T) {
		repo := baseRepo()
		repo.getDocumentRefFn = func(ctx context.Context, id pgtype.UUID) (qadb.GetDocumentForRefRow, error) {
			return qadb.GetDocumentForRefRow{}, pgx.ErrNoRows
		}
		_, err := newService(repo, nil).SubmitQuestion(context.Background(), req, guestActor())
		assert.ErrorIs(t, err, ErrReferenceNotFound)
	})

	t.Run("cross workspace document", func(t *testing.T) {
		repo := baseRepo()
		repo.getDocumentRefFn = func(ctx context.Context, id pgtype.UUID) (qadb.GetDocumentForRefRow, error) {
			return qadb.GetDocumentForRefRow{ID: id, WorkspaceID: mustUUID(t, uuidGroupB), FolderID: mustUUID(t, uuidFolder), Name: "doc"}, nil
		}
		_, err := newService(repo, nil).SubmitQuestion(context.Background(), req, guestActor())
		assert.ErrorIs(t, err, ErrReferenceNotFound)
	})

	t.Run("view revoked on referenced folder", func(t *testing.T) {
		repo := baseRepo()
		repo.getDocumentRefFn = func(ctx context.Context, id pgtype.UUID) (qadb.GetDocumentForRefRow, error) {
			return qadb.GetDocumentForRefRow{ID: id, WorkspaceID: mustUUID(t, uuidWS), FolderID: mustUUID(t, uuidFolder), Name: "doc"}, nil
		}
		checker := fakeChecker{allowFn: func(ctx context.Context, workspaceID, folderID, userID string) (bool, error) {
			return false, nil
		}}
		_, err := newService(repo, checker).SubmitQuestion(context.Background(), req, guestActor())
		assert.ErrorIs(t, err, ErrReferenceNotFound)
	})
}

func TestListQuestionsGuestSilo(t *testing.T) {
	var captured qadb.ListQuestionsParams
	repo := &fakeRepo{
		getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
			return memberGroup(t, true, i32Ptr(5)), nil
		},
		listQuestionsFn: func(ctx context.Context, arg qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error) {
			captured = arg
			return nil, nil
		},
		countQuestionsFn: func(ctx context.Context, arg qadb.CountQuestionsParams) (int64, error) {
			assert.Equal(t, uuidGroup, arg.GroupID.String())
			return 3, nil
		},
		countFaqsFn: func(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
			return 1, nil
		},
	}

	res, err := newService(repo, nil).ListQuestions(context.Background(),
		dto.ListQuestionsRequest{WorkspaceID: uuidWS, GroupID: uuidGroupB}, guestActor())
	require.NoError(t, err)

	assert.Equal(t, uuidGroup, captured.GroupID.String())
	assert.True(t, res.QAEnabled)
	require.NotNil(t, res.QuestionLimit)
	assert.EqualValues(t, 5, *res.QuestionLimit)
	require.NotNil(t, res.QuotaRemaining)
	assert.EqualValues(t, 2, *res.QuotaRemaining)
	assert.EqualValues(t, 3, res.QuestionCount)
	assert.EqualValues(t, 1, res.FaqCount)
	assert.Nil(t, res.WaitingCount)
}

func TestListQuestionsQuotaExhausted(t *testing.T) {
	repo := &fakeRepo{
		getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
			return memberGroup(t, true, i32Ptr(2)), nil
		},
		listQuestionsFn: func(ctx context.Context, arg qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error) {
			return nil, nil
		},
		countQuestionsFn: func(ctx context.Context, arg qadb.CountQuestionsParams) (int64, error) {
			return 5, nil
		},
		countFaqsFn: func(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
			return 0, nil
		},
	}

	res, err := newService(repo, nil).ListQuestions(context.Background(),
		dto.ListQuestionsRequest{WorkspaceID: uuidWS}, guestActor())
	require.NoError(t, err)
	require.NotNil(t, res.QuotaRemaining)
	assert.EqualValues(t, 0, *res.QuotaRemaining)
}

func TestListQuestionsManager(t *testing.T) {
	t.Run("waiting count and group filter", func(t *testing.T) {
		var captured qadb.ListQuestionsParams
		repo := &fakeRepo{
			listQuestionsFn: func(ctx context.Context, arg qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error) {
				captured = arg
				return nil, nil
			},
			countQuestionsFn: func(ctx context.Context, arg qadb.CountQuestionsParams) (int64, error) {
				assert.False(t, arg.GroupID.Valid)
				return 7, nil
			},
			countWaitingFn: func(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
				return 4, nil
			},
			countFaqsFn: func(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
				return 2, nil
			},
		}

		res, err := newService(repo, nil).ListQuestions(context.Background(),
			dto.ListQuestionsRequest{WorkspaceID: uuidWS, GroupID: uuidGroupB, Status: StatusWaiting}, managerActor())
		require.NoError(t, err)

		assert.Equal(t, uuidGroupB, captured.GroupID.String())
		require.NotNil(t, captured.Status)
		assert.Equal(t, StatusWaiting, *captured.Status)
		require.NotNil(t, res.WaitingCount)
		assert.EqualValues(t, 4, *res.WaitingCount)
		assert.EqualValues(t, 7, res.QuestionCount)
		assert.Nil(t, res.QuotaRemaining)
	})

	t.Run("invalid status filter", func(t *testing.T) {
		_, err := newService(&fakeRepo{}, nil).ListQuestions(context.Background(),
			dto.ListQuestionsRequest{WorkspaceID: uuidWS, Status: "open"}, managerActor())
		assert.ErrorIs(t, err, ErrInvalidFilter)
	})

	t.Run("invalid group filter", func(t *testing.T) {
		_, err := newService(&fakeRepo{}, nil).ListQuestions(context.Background(),
			dto.ListQuestionsRequest{WorkspaceID: uuidWS, GroupID: "not-a-uuid"}, managerActor())
		assert.ErrorIs(t, err, ErrInvalidFilter)
	})
}

func TestGetQuestionVisibility(t *testing.T) {
	repoFor := func(q qadb.Question) *fakeRepo {
		return &fakeRepo{
			getQuestionFn: func(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
				return q, nil
			},
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, true, nil), nil
			},
			listRepliesFn: func(ctx context.Context, questionID pgtype.UUID) ([]qadb.QuestionReply, error) {
				return nil, nil
			},
		}
	}

	t.Run("guest cannot read another group's question", func(t *testing.T) {
		_, err := newService(repoFor(sampleQuestion(t, uuidGroupB, StatusWaiting)), nil).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		assert.ErrorIs(t, err, ErrQuestionNotFound)
	})

	t.Run("guest cannot read orphaned question", func(t *testing.T) {
		_, err := newService(repoFor(sampleQuestion(t, "", StatusWaiting)), nil).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		assert.ErrorIs(t, err, ErrQuestionNotFound)
	})

	t.Run("guest reads own group question", func(t *testing.T) {
		res, err := newService(repoFor(sampleQuestion(t, uuidGroup, StatusWaiting)), nil).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		require.NoError(t, err)
		assert.Equal(t, uuidQuestion, res.ID)
	})

	t.Run("manager reads any question", func(t *testing.T) {
		res, err := newService(repoFor(sampleQuestion(t, uuidGroupB, StatusWaiting)), nil).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, managerActor())
		require.NoError(t, err)
		assert.Equal(t, uuidQuestion, res.ID)
	})
}

func TestGetQuestionRefFiltering(t *testing.T) {
	withDocRef := func() qadb.Question {
		q := sampleQuestion(t, uuidGroup, StatusWaiting)
		q.DocumentID = mustUUID(t, uuidDoc)
		q.DocumentName = "doc snapshot"
		return q
	}
	repoFor := func(q qadb.Question, docErr error) *fakeRepo {
		return &fakeRepo{
			getQuestionFn: func(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
				return q, nil
			},
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, true, nil), nil
			},
			listRepliesFn: func(ctx context.Context, questionID pgtype.UUID) ([]qadb.QuestionReply, error) {
				return nil, nil
			},
			getDocumentRefFn: func(ctx context.Context, id pgtype.UUID) (qadb.GetDocumentForRefRow, error) {
				if docErr != nil {
					return qadb.GetDocumentForRefRow{}, docErr
				}
				return qadb.GetDocumentForRefRow{ID: id, WorkspaceID: mustUUID(t, uuidWS), FolderID: mustUUID(t, uuidFolder), Name: "doc live"}, nil
			},
		}
	}

	t.Run("guest with view gets the chip", func(t *testing.T) {
		res, err := newService(repoFor(withDocRef(), nil), nil).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		require.NoError(t, err)
		require.NotNil(t, res.DocumentRef)
		assert.Equal(t, uuidDoc, res.DocumentRef.ID)
		assert.Equal(t, uuidFolder, res.DocumentRef.FolderID)
		assert.Equal(t, "doc live", res.DocumentRef.Name)
	})

	t.Run("guest with revoked view loses the chip", func(t *testing.T) {
		checker := fakeChecker{allowFn: func(ctx context.Context, workspaceID, folderID, userID string) (bool, error) {
			return false, nil
		}}
		res, err := newService(repoFor(withDocRef(), nil), checker).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		require.NoError(t, err)
		assert.Nil(t, res.DocumentRef)
	})

	t.Run("manager keeps the chip regardless of folder access", func(t *testing.T) {
		checker := fakeChecker{allowFn: func(ctx context.Context, workspaceID, folderID, userID string) (bool, error) {
			return false, nil
		}}
		res, err := newService(repoFor(withDocRef(), nil), checker).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, managerActor())
		require.NoError(t, err)
		require.NotNil(t, res.DocumentRef)
	})

	t.Run("deleted document drops the chip silently", func(t *testing.T) {
		res, err := newService(repoFor(withDocRef(), pgx.ErrNoRows), nil).
			GetQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		require.NoError(t, err)
		assert.Nil(t, res.DocumentRef)
	})
}

func TestReplyQuestionGuards(t *testing.T) {
	t.Run("closed thread rejects reply", func(t *testing.T) {
		repo := &fakeRepo{
			getQuestionFn: func(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
				return sampleQuestion(t, uuidGroup, StatusClosed), nil
			},
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, true, nil), nil
			},
		}
		_, err := newService(repo, nil).ReplyQuestion(context.Background(),
			dto.ReplyQuestionRequest{WorkspaceID: uuidWS, QuestionID: uuidQuestion, Body: "b"}, guestActor())
		assert.ErrorIs(t, err, ErrQuestionClosed)
	})

	t.Run("disabled group rejects guest reply", func(t *testing.T) {
		repo := &fakeRepo{
			getQuestionFn: func(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
				return sampleQuestion(t, uuidGroup, StatusWaiting), nil
			},
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, false, nil), nil
			},
		}
		_, err := newService(repo, nil).ReplyQuestion(context.Background(),
			dto.ReplyQuestionRequest{WorkspaceID: uuidWS, QuestionID: uuidQuestion, Body: "b"}, guestActor())
		assert.ErrorIs(t, err, ErrQADisabled)
	})
}

func TestCloseQuestionGuards(t *testing.T) {
	t.Run("group mate cannot close someone else's question", func(t *testing.T) {
		q := sampleQuestion(t, uuidGroup, StatusWaiting)
		q.AuthorID = mustUUID(t, uuidGroupB)
		repo := &fakeRepo{
			getQuestionFn: func(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
				return q, nil
			},
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, true, nil), nil
			},
		}
		err := newService(repo, nil).CloseQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
		assert.ErrorIs(t, err, ErrCloseNotAllowed)
	})
}

func TestReopenQuestionGuards(t *testing.T) {
	err := newService(&fakeRepo{}, nil).ReopenQuestion(context.Background(), uuidWS, uuidQuestion, guestActor())
	assert.ErrorIs(t, err, ErrReopenNotAllowed)
}

func TestCountWaitingGuards(t *testing.T) {
	t.Run("guest forbidden", func(t *testing.T) {
		_, err := newService(&fakeRepo{}, nil).CountWaiting(context.Background(), uuidWS, guestActor())
		assert.ErrorIs(t, err, ErrQAForbidden)
	})

	t.Run("manager gets the count", func(t *testing.T) {
		repo := &fakeRepo{
			countWaitingFn: func(ctx context.Context, workspaceID pgtype.UUID) (int64, error) {
				return 9, nil
			},
		}
		res, err := newService(repo, nil).CountWaiting(context.Background(), uuidWS, managerActor())
		require.NoError(t, err)
		assert.EqualValues(t, 9, res.WaitingCount)
	})
}

func TestExportQuestions(t *testing.T) {
	t.Run("guest with disabled qa is rejected", func(t *testing.T) {
		repo := &fakeRepo{
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, false, nil), nil
			},
		}
		_, err := newService(repo, nil).ExportQuestions(context.Background(),
			dto.ExportQuestionsRequest{WorkspaceID: uuidWS}, guestActor())
		assert.ErrorIs(t, err, ErrQADisabled)
	})

	t.Run("guest silo forced and replies flatten under their question", func(t *testing.T) {
		var captured qadb.ListQuestionsParams
		repo := &fakeRepo{
			getMemberGroupQAFn: func(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error) {
				return memberGroup(t, true, nil), nil
			},
			listQuestionsFn: func(ctx context.Context, arg qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error) {
				captured = arg
				return []qadb.ListQuestionsRow{
					{
						ID:           mustUUID(t, uuidQuestion),
						WorkspaceID:  mustUUID(t, uuidWS),
						GroupID:      mustUUID(t, uuidGroup),
						GroupName:    "Grup A",
						Number:       3,
						AuthorName:   "Guest",
						Subject:      "subject",
						Body:         "body",
						Status:       StatusAnswered,
						DocumentName: "doc snapshot",
						CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
			listRepliesFn: func(ctx context.Context, questionID pgtype.UUID) ([]qadb.QuestionReply, error) {
				return []qadb.QuestionReply{
					{
						ID:         mustUUID(t, uuidGroupB),
						QuestionID: questionID,
						AuthorName: "Owner",
						AuthorRole: permission.RoleOwner,
						Body:       "answer",
						CreatedAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
					},
				}, nil
			},
		}

		page, err := newService(repo, nil).ExportQuestions(context.Background(),
			dto.ExportQuestionsRequest{WorkspaceID: uuidWS, GroupID: uuidGroupB}, guestActor())
		require.NoError(t, err)

		assert.Equal(t, uuidGroup, captured.GroupID.String())
		require.Len(t, page.Rows, 2)
		assert.Equal(t, "question", page.Rows[0].Type)
		assert.Equal(t, "doc snapshot", page.Rows[0].Document)
		assert.Equal(t, permission.RoleGuest, page.Rows[0].Role)
		assert.Equal(t, "reply", page.Rows[1].Type)
		assert.EqualValues(t, 3, page.Rows[1].Number)
		assert.Equal(t, "subject", page.Rows[1].Subject)
		assert.Equal(t, permission.RoleOwner, page.Rows[1].Role)
		assert.Empty(t, page.Rows[1].Document)
	})
}

func TestCreateFaqGuards(t *testing.T) {
	t.Run("guest forbidden", func(t *testing.T) {
		_, err := newService(&fakeRepo{}, nil).CreateFaq(context.Background(),
			dto.CreateFaqRequest{WorkspaceID: uuidWS, QuestionText: "q", AnswerText: "a"}, guestActor())
		assert.ErrorIs(t, err, ErrQAForbidden)
	})

	t.Run("unknown source question", func(t *testing.T) {
		repo := &fakeRepo{
			getQuestionFn: func(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error) {
				return qadb.Question{}, pgx.ErrNoRows
			},
		}
		_, err := newService(repo, nil).CreateFaq(context.Background(),
			dto.CreateFaqRequest{WorkspaceID: uuidWS, QuestionText: "q", AnswerText: "a", SourceQuestionID: uuidQuestion}, managerActor())
		assert.ErrorIs(t, err, ErrReferenceNotFound)
	})
}
