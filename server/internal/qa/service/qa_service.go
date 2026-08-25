package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	"github.com/findardi/rakda/server/internal/platform/permission"
	qadb "github.com/findardi/rakda/server/internal/qa/repository/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	StatusWaiting  = "waiting"
	StatusAnswered = "answered"
	StatusClosed   = "closed"
)

const (
	defaultQuestionPageSize = 50
	maxQuestionPageSize     = 100
)

var (
	ErrQuestionNotFound     = errors.New("question not found")
	ErrQADisabled           = errors.New("qa is disabled for this group")
	ErrQuestionLimitReached = errors.New("question limit reached")
	ErrOnlyGuestCanAsk      = errors.New("only guests can submit questions")
	ErrQuestionClosed       = errors.New("question is closed")
	ErrCloseNotAllowed      = errors.New("only the asker or a room manager can close")
	ErrReopenNotAllowed     = errors.New("only a room manager can reopen")
	ErrQuestionNotClosed    = errors.New("question is not closed")
	ErrQAForbidden          = errors.New("forbidden")
	ErrInvalidCursor        = errors.New("invalid cursor")
	ErrInvalidFilter        = errors.New("invalid filter")
	ErrReferenceNotFound    = errors.New("reference not found")
)

type Actor struct {
	UserID string
	Name   string
	Email  string
	Role   string
}

func (a Actor) managesRoom() bool {
	return a.Role == permission.RoleOwner || a.Role == permission.RoleAdmin
}

type QAService struct {
	repo     QARepository
	content  ContentAccessChecker
	activity ActivityRecorder
}

func NewQAService(repo QARepository, content ContentAccessChecker, activity ActivityRecorder) *QAService {
	return &QAService{
		repo:     repo,
		content:  content,
		activity: activity,
	}
}

func (s *QAService) activityEntry(workspaceID string, actor Actor, action, targetType, targetID, targetName string, metadata map[string]any) activityservice.Entry {
	return activityservice.Entry{
		WorkspaceID: workspaceID,
		ActorID:     actor.UserID,
		ActorName:   actor.Name,
		ActorRole:   actor.Role,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		TargetName:  targetName,
		Metadata:    metadata,
	}
}

func nextStatusAfterReply(authorRole string) string {
	if authorRole == permission.RoleGuest {
		return StatusWaiting
	}
	return StatusAnswered
}

func validStatusFilter(status string) bool {
	return status == StatusWaiting || status == StatusAnswered || status == StatusClosed
}

func questionCursor(createdAt time.Time, id string) string {
	return fmt.Sprintf("%d_%s", createdAt.UnixMicro(), id)
}

func parseQuestionCursor(cursor string) (pgtype.Timestamptz, pgtype.UUID, error) {
	var createdAt pgtype.Timestamptz
	var id pgtype.UUID

	micros, rawID, found := strings.Cut(cursor, "_")
	if !found {
		return createdAt, id, ErrInvalidCursor
	}
	v, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return createdAt, id, ErrInvalidCursor
	}
	if err := id.Scan(rawID); err != nil {
		return createdAt, id, ErrInvalidCursor
	}

	createdAt = pgtype.Timestamptz{Time: time.UnixMicro(v), Valid: true}
	return createdAt, id, nil
}

func questionVisibleTo(q qadb.Question, actor Actor, actorGroupID pgtype.UUID) bool {
	if actor.managesRoom() {
		return true
	}
	return q.GroupID.Valid && actorGroupID.Valid && q.GroupID == actorGroupID
}

func uuidString(u pgtype.UUID) string {
	v, err := u.Value()
	if err != nil || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
