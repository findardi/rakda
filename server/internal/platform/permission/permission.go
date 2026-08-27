package permission

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

var set = func() map[string]struct{} {
	m := make(map[string]struct{}, len(All))
	for _, p := range All {
		m[p] = struct{}{}
	}
	return m
}()

func IsValid(p string) bool {
	_, ok := set[p]
	return ok
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

func IsValidRoomStatus(s string) bool {
	switch s {
	case RoomPrepare, RoomActive, RoomArchive:
		return true
	}
	return false
}

type SystemRole struct {
	Name        string
	Permissions []string
}

func GetOwner() []string {
	return append([]string{}, All...)
}

func GetAdmin() []string {
	return []string{
		PermWorkspaceView, PermWorkspaceEdit,
		PermMemberView, PermMemberAdd, PermMemberEdit, PermMemberDelete,
		PermRoleView,
		PermGroupView, PermGroupCreate, PermGroupEdit, PermGroupDelete, PermGroupAssign,
		PermFolderView, PermFolderCreate, PermFolderEdit, PermFolderDelete,
		PermDocumentView, PermDocumentUpload, PermDocumentDownload, PermDocumentEdit, PermDocumentDelete,
	}
}

func GetGuest() []string {
	return []string{
		PermWorkspaceView,
		PermFolderView,
		PermDocumentView,
		PermDocumentDownload,
	}
}

func DefaultSystemRoles() []SystemRole {
	return []SystemRole{
		{Name: RoleOwner, Permissions: GetOwner()},
		{Name: RoleAdmin, Permissions: GetAdmin()},
		{Name: RoleGuest, Permissions: GetGuest()},
	}
}
