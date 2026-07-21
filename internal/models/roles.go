package models

// Membership roles for a hiveshare. Exactly two capabilities:
//   - RoleAll:  invite, read, write memory, edit hiveshare, manage members
//   - RoleView: read-only (list/get/search/stream/metrics); no writes, no invites
//
// Legacy DB values owner/member map to all; viewer maps to view.
const (
	RoleAll  = "all"
	RoleView = "view"
)

// NormalizeRole maps stored role strings onto all|view.
func NormalizeRole(role string) string {
	switch role {
	case RoleAll, "owner", "member":
		return RoleAll
	case RoleView, "viewer":
		return RoleView
	default:
		return role
	}
}

// CanWrite returns true if the role may create/update/delete memory and invite.
func CanWrite(role string) bool {
	return NormalizeRole(role) == RoleAll
}

// CanView returns true if the user is a member at all.
func CanView(role string) bool {
	n := NormalizeRole(role)
	return n == RoleAll || n == RoleView
}
