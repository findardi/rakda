package service

import (
	"context"

	accessdb "github.com/findardi/Riksa-App/server/internal/access/repository/sqlc"
	activityservice "github.com/findardi/Riksa-App/server/internal/activity/service"
	authdto "github.com/findardi/Riksa-App/server/internal/auth/dto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AccessRepository interface {
	AddMember(ctx context.Context, arg accessdb.AddMemberParams) (accessdb.WorkspaceMember, error)
	CreateGroup(ctx context.Context, arg accessdb.CreateGroupParams) (accessdb.WorkspaceGroup, error)

	DeleteMember(ctx context.Context, id pgtype.UUID) error
	DeleteGroup(ctx context.Context, id pgtype.UUID) error
	DeleteGroupMember(ctx context.Context, arg accessdb.DeleteGroupMemberParams) error

	UpdateRole(ctx context.Context, arg accessdb.UpdateRoleParams) (accessdb.WorkspaceMember, error)

	GetRole(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceRole, error)
	GetRoles(ctx context.Context, workspaceID pgtype.UUID) ([]accessdb.WorkspaceRole, error)
	GetMember(ctx context.Context, id pgtype.UUID) (accessdb.GetMemberRow, error)
	GetMembers(ctx context.Context, workspaceID pgtype.UUID) ([]accessdb.GetMembersRow, error)
	GetMemberByWorkspaceUser(ctx context.Context, arg accessdb.GetMemberByWorkspaceUserParams) (accessdb.WorkspaceMember, error)
	GetWorkspaceInvitation(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceUserInvitation, error)
	GetGroups(ctx context.Context, workspaceID pgtype.UUID) ([]accessdb.WorkspaceGroup, error)
	GetGroup(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceGroup, error)
	GetGroupMembers(ctx context.Context, groupID pgtype.UUID) ([]accessdb.GetGroupMembersRow, error)
	GetInvitationByCodeHashDetailed(ctx context.Context, codeHash string) (accessdb.GetInvitationByCodeHashDetailedRow, error)

	UpdateGroup(ctx context.Context, arg accessdb.UpdateGroupParams) (accessdb.WorkspaceGroup, error)

	InsertRole(ctx context.Context, arg accessdb.InsertRoleParams) (accessdb.WorkspaceRole, error)
	InsertWorkspaceInvitation(ctx context.Context, arg accessdb.InsertWorkspaceInvitationParams) (accessdb.WorkspaceUserInvitation, error)
	InsertGroupMember(ctx context.Context, arg accessdb.InsertGroupMemberParams) (accessdb.WorkspaceGroupMember, error)
	MoveMemberToDefaultGroup(ctx context.Context, memberID pgtype.UUID) (int64, error)
	ListWorkspaceInvitations(ctx context.Context, arg accessdb.ListWorkspaceInvitationsParams) ([]accessdb.ListWorkspaceInvitationsRow, error)

	RevokeWorkspaceInvitation(ctx context.Context, id pgtype.UUID) (accessdb.WorkspaceUserInvitation, error)
	ResendInvitation(ctx context.Context, arg accessdb.ResendInvitationParams) (accessdb.WorkspaceUserInvitation, error)
	ReinviteWorkspaceInvitation(ctx context.Context, arg accessdb.ReinviteWorkspaceInvitationParams) (accessdb.WorkspaceUserInvitation, error)

	ExecTx(ctx context.Context, fn func(q *accessdb.Queries) error) error
	ExecTxTx(ctx context.Context, fn func(*accessdb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	Record(ctx context.Context, e activityservice.Entry)
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
}

type MailService interface {
	Send(ctx context.Context, to, subject, body string) error
}

type Tokenizer interface {
	Generate() string
	Hash(code string) string
}

type AuthService interface {
	UserExists(ctx context.Context, email string) (authdto.UserResponse, error)
}
