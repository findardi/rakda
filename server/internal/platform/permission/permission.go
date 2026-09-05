package permission

import "slices"

const (
	PermWorkspaceView   = "workspace:view"
	PermWorkspaceEdit   = "workspace:edit"
	PermWorkspaceDelete = "workspace:delete"

	PermMemberView   = "member:view"
	PermMemberAdd    = "member:add"
	PermMemberEdit   = "member:edit"
	PermMemberDelete = "member:delete"

	PermRoleView = "role:view"

	PermGroupView   = "group:view"
	PermGroupCreate = "group:create"
	PermGroupEdit   = "group:edit"
	PermGroupDelete = "group:delete"
	PermGroupAssign = "group:assign"

	PermFolderView   = "folder:view"
	PermFolderCreate = "folder:create"
	PermFolderEdit   = "folder:edit"
	PermFolderDelete = "folder:delete"

	PermDocumentView     = "document:view"
	PermDocumentUpload   = "document:upload"
	PermDocumentDownload = "document:download"
	PermDocumentEdit     = "document:edit"
	PermDocumentDelete   = "document:delete"
)

var All = []string{
	PermWorkspaceView, PermWorkspaceEdit, PermWorkspaceDelete,
	PermMemberView, PermMemberAdd, PermMemberEdit, PermMemberDelete,
	PermRoleView,
	PermGroupView, PermGroupCreate, PermGroupEdit, PermGroupDelete, PermGroupAssign,
	PermFolderView, PermFolderCreate, PermFolderEdit, PermFolderDelete,
	PermDocumentView, PermDocumentUpload, PermDocumentDownload, PermDocumentEdit, PermDocumentDelete,
}

const (
	RoleOwner = "owner"
	RoleAdmin = "admin"
	RoleGuest = "guest"
)

const (
	RoomPrepare = "prepare"
	RoomActive  = "active"
	RoomArchive = "archive"
)

type SystemRole struct {
	Name        string
	Permissions []string
}

func DefaultSystemRoles() []SystemRole {
	return []SystemRole{
		{Name: RoleOwner, Permissions: slices.Clone(All)},
		{Name: RoleAdmin, Permissions: []string{
			PermWorkspaceView, PermWorkspaceEdit,
			PermMemberView, PermMemberAdd, PermMemberEdit, PermMemberDelete,
			PermRoleView,
			PermGroupView, PermGroupCreate, PermGroupEdit, PermGroupDelete, PermGroupAssign,
			PermFolderView, PermFolderCreate, PermFolderEdit, PermFolderDelete,
			PermDocumentView, PermDocumentUpload, PermDocumentDownload, PermDocumentEdit, PermDocumentDelete,
		}},
		{Name: RoleGuest, Permissions: []string{
			PermWorkspaceView,
			PermFolderView,
			PermDocumentView,
			PermDocumentDownload,
		}},
	}
}
