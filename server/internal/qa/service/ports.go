package service

import (
	"context"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type QARepository interface {
	GetMemberGroupQA(ctx context.Context, arg qadb.GetMemberGroupQAParams) (qadb.GetMemberGroupQARow, error)
	ListQuestions(ctx context.Context, arg qadb.ListQuestionsParams) ([]qadb.ListQuestionsRow, error)
	CountQuestions(ctx context.Context, arg qadb.CountQuestionsParams) (int64, error)
	CountWaitingQuestions(ctx context.Context, workspaceID pgtype.UUID) (int64, error)
	GetQuestion(ctx context.Context, arg qadb.GetQuestionParams) (qadb.Question, error)
	ListQuestionReplies(ctx context.Context, questionID pgtype.UUID) ([]qadb.QuestionReply, error)
	ListFaqs(ctx context.Context, workspaceID pgtype.UUID) ([]qadb.Faq, error)
	CountFaqs(ctx context.Context, workspaceID pgtype.UUID) (int64, error)
	GetDocumentForRef(ctx context.Context, id pgtype.UUID) (qadb.GetDocumentForRefRow, error)
	GetFolderForRef(ctx context.Context, id pgtype.UUID) (qadb.GetFolderForRefRow, error)

	ExecTxTx(ctx context.Context, fn func(*qadb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
}

type ContentAccessChecker interface {
	CanUserViewFolder(ctx context.Context, workspaceID, folderID, userID string) (bool, error)
}
