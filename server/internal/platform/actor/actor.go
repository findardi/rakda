// Package actor holds the caller as every domain service sees it.
package actor

import "github.com/findardi/rakda/server/internal/platform/permission"

// Actor is identity from the JWT plus standing from the room membership.
// RoomStatus is empty for calls made outside a room.
type Actor struct {
	UserID     string
	Name       string
	Email      string
	Role       string
	RoomStatus string
}

// ManagesRoom reports owner or admin — the two roles that run a room.
func (a Actor) ManagesRoom() bool {
	return a.Role == permission.RoleOwner || a.Role == permission.RoleAdmin
}
