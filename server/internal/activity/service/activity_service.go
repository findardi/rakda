package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	activitydb "github.com/findardi/rakda/server/internal/activity/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ActionFolderCreated       = "folder_created"
	ActionFolderRenamed       = "folder_renamed"
	ActionFolderMoved         = "folder_moved"
	ActionFolderDeleted       = "folder_deleted"
	ActionFolderRestored      = "folder_restored"
	ActionFolderPurged        = "folder_purged"
	ActionDocumentUploaded    = "document_uploaded"
	ActionDocumentMoved       = "document_moved"
	ActionDocumentDeleted     = "document_deleted"
	ActionDocumentRestored    = "document_restored"
	ActionDocumentPurged      = "document_purged"
	ActionDocumentDownloaded  = "document_downloaded"
	ActionDocumentViewed      = "document_viewed"
	ActionVersionUploaded     = "version_uploaded"
	ActionVersionRestored     = "version_restored"
	ActionRenditionRetried    = "rendition_retried"
	ActionInviteSent          = "invite_sent"
	ActionInviteAccepted      = "invite_accepted"
	ActionInviteRejected      = "invite_rejected"
	ActionInviteResent        = "invite_resent"
	ActionInviteRevoked       = "invite_revoked"
	ActionMemberRemoved       = "member_removed"
	ActionRoleChanged         = "role_changed"
	ActionGroupCreated        = "group_created"
	ActionGroupUpdated        = "group_updated"
	ActionGroupDeleted        = "group_deleted"
	ActionGroupAssigned       = "group_assigned"
	ActionGroupUnassigned     = "group_unassigned"
	ActionFolderAccessChanged = "folder_access_changed"
	ActionFolderAccessRemoved = "folder_access_removed"
	ActionSearchPerformed     = "search_performed"
	ActionQuestionSubmitted   = "question_submitted"
	ActionQuestionReplied     = "question_replied"
	ActionQuestionAnswered    = "question_answered"
	ActionQuestionClosed      = "question_closed"
	ActionQuestionReopened    = "question_reopened"
	ActionFaqPublished        = "faq_published"
	ActionQaSettingsChanged   = "qa_settings_changed"
)

const (
	TargetFolder       = "folder"
	TargetDocument     = "document"
	TargetVersion      = "version"
	TargetMember       = "member"
	TargetGroup        = "group"
	TargetInvitation   = "invitation"
	TargetFolderAccess = "folder_access"
	TargetSearch       = "search"
	TargetQuestion     = "question"
	TargetFaq          = "faq"
)

const (
	EventViewPage     = "view_page"
	EventPageDuration = "page_duration"
)

type Entry struct {
	WorkspaceID string
	ActorID     string
	ActorName   string
	ActorRole   string
	Action      string
	TargetType  string
	TargetID    string
	TargetName  string
	Metadata    map[string]any
}

type PageEvent struct {
	WorkspaceID  string
	DocumentID   string
	DocumentName string
	VersionID    string
	PageNo       int32
	EventType    string
	DurationMs   int32
	ActorID      string
	ActorEmail   string
}

type ActivityService struct {
	repo ActivityRepository
}

func NewActivityService(repo ActivityRepository) *ActivityService {
	return &ActivityService{
		repo: repo,
	}
}

func (s *ActivityService) Record(ctx context.Context, e Entry) {
	params, err := logParams(e)
	if err != nil {
		log.Printf("activity: record %s: %v", e.Action, err)
		return
	}

	if err := s.repo.InsertActivityLog(ctx, params); err != nil {
		log.Printf("activity: record %s: %v", e.Action, err)
	}
}

func (s *ActivityService) RecordTx(ctx context.Context, tx pgx.Tx, e Entry) error {
	params, err := logParams(e)
	if err != nil {
		return fmt.Errorf("activity log %s: %w", e.Action, err)
	}

	if err := activitydb.New(tx).InsertActivityLog(ctx, params); err != nil {
		return fmt.Errorf("activity log %s: %w", e.Action, err)
	}

	return nil
}

func (s *ActivityService) RecordPageEvent(ctx context.Context, ev PageEvent) {
	params, err := eventParams(ev)
	if err != nil {
		log.Printf("activity: %s event: %v", ev.EventType, err)
		return
	}

	if err := s.repo.InsertContentEvent(ctx, params); err != nil {
		log.Printf("activity: %s event: %v", ev.EventType, err)
	}
}

func logParams(e Entry) (activitydb.InsertActivityLogParams, error) {
	workspaceID, err := pgUUID(e.WorkspaceID)
	if err != nil {
		return activitydb.InsertActivityLogParams{}, fmt.Errorf("workspace id: %w", err)
	}

	actorID, err := pgUUID(e.ActorID)
	if err != nil {
		return activitydb.InsertActivityLogParams{}, fmt.Errorf("actor id: %w", err)
	}

	targetID, err := pgUUID(e.TargetID)
	if err != nil {
		return activitydb.InsertActivityLogParams{}, fmt.Errorf("target id: %w", err)
	}

	metadata := []byte("{}")
	if len(e.Metadata) > 0 {
		metadata, err = json.Marshal(e.Metadata)
		if err != nil {
			return activitydb.InsertActivityLogParams{}, fmt.Errorf("marshal metadata: %w", err)
		}
	}

	return activitydb.InsertActivityLogParams{
		WorkspaceID: workspaceID,
		ActorID:     actorID,
		ActorName:   e.ActorName,
		ActorRole:   e.ActorRole,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    targetID,
		TargetName:  e.TargetName,
		Metadata:    metadata,
	}, nil
}

func eventParams(ev PageEvent) (activitydb.InsertContentEventParams, error) {
	workspaceID, err := pgUUID(ev.WorkspaceID)
	if err != nil {
		return activitydb.InsertContentEventParams{}, fmt.Errorf("workspace id: %w", err)
	}
	documentID, err := pgUUID(ev.DocumentID)
	if err != nil {
		return activitydb.InsertContentEventParams{}, fmt.Errorf("document id: %w", err)
	}
	versionID, err := pgUUID(ev.VersionID)
	if err != nil {
		return activitydb.InsertContentEventParams{}, fmt.Errorf("version id: %w", err)
	}
	actorID, err := pgUUID(ev.ActorID)
	if err != nil {
		return activitydb.InsertContentEventParams{}, fmt.Errorf("actor id: %w", err)
	}

	pageNo := ev.PageNo
	var durationMS *int32
	if ev.EventType == EventPageDuration {
		durationMS = &ev.DurationMs
	}

	return activitydb.InsertContentEventParams{
		WorkspaceID:  workspaceID,
		DocumentID:   documentID,
		DocumentName: ev.DocumentName,
		VersionID:    versionID,
		PageNo:       &pageNo,
		EventType:    ev.EventType,
		DurationMs:   durationMS,
		ActorID:      actorID,
		ActorEmail:   ev.ActorEmail,
	}, nil
}

func pgUUID(u string) (pgtype.UUID, error) {
	var id pgtype.UUID

	if u == "" {
		return id, nil
	}

	if err := id.Scan(u); err != nil {
		return id, fmt.Errorf("parse uuid %q: %w", u, err)
	}

	return id, nil
}
