package repository

import (
	"context"
	"errors"

	accessdb "github.com/findardi/rakda/server/internal/access/repository/sqlc"
	"github.com/findardi/rakda/server/internal/platform/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResolveMembership is the one MemberResolver every {workspaceID} module hands
// to middleware.RequireMember. It lives on the repository rather than in a
// module package so content, activity, qa, and workspace can share it without
// pulling the access handlers and their mail dependency into their import
// graphs. One copy matters for more than style: a resolver that forgets a
// field leaves a whole route subtree unguarded without a symptom (U-11).
// A lapsed membership never reaches here — GetMembershipWithPermissions
// filters expires_at in SQL, so it reads as "not a member".
func (r *Repository) ResolveMembership(ctx context.Context, workspaceID, userID string) (*middleware.Membership, error) {
	var wID, uID pgtype.UUID
	if err := wID.Scan(workspaceID); err != nil {
		return nil, middleware.ErrResourceNotFound
	}
	if err := uID.Scan(userID); err != nil {
		return nil, middleware.ErrResourceNotFound
	}

	row, err := r.GetMembershipWithPermissions(ctx, accessdb.GetMembershipWithPermissionsParams{
		WorkspaceID: wID,
		UserID:      uID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, middleware.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}

	return &middleware.Membership{
		Role:            row.RoleName,
		Permissions:     row.Permissions,
		Status:          row.Status,
		WorkspaceStatus: row.WorkspaceStatus,
	}, nil
}
