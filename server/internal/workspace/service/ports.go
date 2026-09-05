package service

import (
	"context"
	"io"

	activityservice "github.com/findardi/rakda/server/internal/activity/service"
	workspacedb "github.com/findardi/rakda/server/internal/workspace/repository/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type WorkspaceRepository interface {
	CreateWorkspace(ctx context.Context, arg workspacedb.CreateWorkspaceParams) (workspacedb.Workspace, error)

	DeleteWorkspace(ctx context.Context, id pgtype.UUID) error

	GetWorkspaceByNameAndOwner(ctx context.Context, arg workspacedb.GetWorkspaceByNameAndOwnerParams) (workspacedb.Workspace, error)
	GetWorkspacesByOwner(ctx context.Context, ownerID pgtype.UUID) ([]workspacedb.Workspace, error)
	GetWorkspaces(ctx context.Context, userID pgtype.UUID) ([]workspacedb.GetWorkspacesRow, error)
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (workspacedb.Workspace, error)
	GetWorkspaceForMember(ctx context.Context, arg workspacedb.GetWorkspaceForMemberParams) (workspacedb.Workspace, error)
	GetMemberRoleName(ctx context.Context, arg workspacedb.GetMemberRoleNameParams) (string, error)
	GetWorkspaceSummary(ctx context.Context, workspaceID pgtype.UUID) (workspacedb.GetWorkspaceSummaryRow, error)

	UpdateWorkspace(ctx context.Context, arg workspacedb.UpdateWorkspaceParams) (workspacedb.Workspace, error)
	UpdateWorkspaceStatus(ctx context.Context, arg workspacedb.UpdateWorkspaceStatusParams) (workspacedb.Workspace, error)

	ExecTx(ctx context.Context, fn func(*workspacedb.Queries, pgx.Tx) error) error
}

type ActivityRecorder interface {
	RecordTx(ctx context.Context, tx pgx.Tx, e activityservice.Entry) error
}

// Provisioner seeds a freshly created room inside the creating transaction.
// access (roles, default group) and content (General folder) both implement it.
type Provisioner interface {
	ProvisionWorkspace(ctx context.Context, tx pgx.Tx, workspaceID, ownerID pgtype.UUID) error
}

// AssetStore is the slice of object storage a room's own assets need. Branding
// pictures are the first workspace-owned objects, so this port is deliberately
// tiny — no presign, no multipart: the bytes always pass through the server.
type AssetStore interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}
