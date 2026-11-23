package store

import (
	"errors"
	"slices"
	"time"
)

// Roles of the users (RoleSummaries). What each may do is decided by the
// methods of User below, the one place for these rules: the catalog and
// services ask them, and the web app shows what they answer.
const (
	RoleSuperadmin = "superadmin"
	RoleAdmin      = "admin"
	RoleUser       = "user"
)

// RoleSummaries say what each role may do, for the user form; they follow
// the methods below.
var RoleSummaries = map[string]string{
	RoleSuperadmin: "everything",
	RoleAdmin:      "projects, notifications, users and deleting issues",
	RoleUser:       "issue statuses, logs, project DSNs and their own coding agents",
}

var ErrUsernameTaken = errors.New("username is taken")

// User is a user of the panel. The password hash is never read with it: only
// UserPassword and FindLogin return it, where a password is checked.
type User struct {
	ID                 string
	Username           string
	Name               string
	Role               string
	MustChangePassword bool
	CreatedAt          time.Time
	LastLoginAt        *time.Time
	// Access is the projects the user sees; ProjectAccess applies it.
	Access Access
}

// ProjectAccess is the projects the user sees: every one for the superadmin,
// whatever is stored.
func (user User) ProjectAccess() Access {
	if user.Role == RoleSuperadmin {
		return Access{}
	}

	return user.Access
}

// CanGiveAccess: user may give chosen to another user.
func (user User) CanGiveAccess(chosen Access) bool {
	return user.CanAdminister() && user.ProjectAccess().Covers(chosen)
}

// CanChangeAccessOf: user may change target's access. A target with every
// project is changed only by someone with every project.
func (user User) CanChangeAccessOf(target User) bool {
	return user.CanManage(target) && target.ProjectAccess().ChangeableBy(user.ProjectAccess())
}

// CoveredBy: viewer has every project user has.
func (user User) CoveredBy(viewer Access) bool {
	return viewer.Covers(user.ProjectAccess())
}

// CanTakeOver: user may rename, delete or reset the password of target,
// change their role and reach their agents. A password or role would give
// user, or target as an admin, target's projects, and the rest is for those
// who have them: only when user has them all.
func (user User) CanTakeOver(target User) bool {
	return user.CanManage(target) && target.CoveredBy(user.ProjectAccess())
}

// DisplayName is the name, or the username when no name is set.
func (user User) DisplayName() string {
	return displayName(user.Name, user.Username)
}

// CanAdminister: projects, notifications, users and deleting issues.
func (user User) CanAdminister() bool {
	return user.Role == RoleSuperadmin || user.Role == RoleAdmin
}

// CanManage reports whether user may change target at all (their access,
// and with CanTakeOver the rest), by the hierarchy: the superadmin every other
// user, an admin users of the role user, nobody themselves here (they have the
// account page).
func (user User) CanManage(target User) bool {
	if target.ID == user.ID {
		return false
	}

	switch user.Role {
	case RoleSuperadmin:
		return true
	case RoleAdmin:
		return target.Role == RoleUser
	}

	return false
}

// CanAccessAgentsOf: one's own MCP agents, or those of a user one may take
// over (disconnecting them is part of a password reset).
func (user User) CanAccessAgentsOf(owner User) bool {
	return owner.ID == user.ID || user.CanTakeOver(owner)
}

// AssignableRoles are the roles an administrator may give: there is one
// superadmin, so never that one.
var AssignableRoles = []string{RoleAdmin, RoleUser}

// AssignableRoles are the roles user may give: an admin only the role user,
// whom he then manages.
func (user User) AssignableRoles() []string {
	switch user.Role {
	case RoleSuperadmin:
		return slices.Clone(AssignableRoles)
	case RoleAdmin:
		return []string{RoleUser}
	}

	return nil
}

// CanAssign reports whether user may give role.
func (user User) CanAssign(role string) bool {
	return slices.Contains(user.AssignableRoles(), role)
}

// Actor is the user as the author of a change made through source.
func (user User) Actor(source string) Actor {
	return Actor{UserID: user.ID, Name: user.DisplayName(), Source: source}
}

// UserChange is a change of a user by an administrator whose access is By.
// Both are checked against the stored user under the write lock, so two
// administrators saving one user keep each other's projects: a new name or
// role only when By covers the user (User.CoveredBy, else
// ErrTakeOverLocked), and
// Access, when set (what By chose of what it was shown), merged with the
// stored access (Access.Replace).
type UserChange struct {
	Name, Role string
	By         Access
	Access     *Access
}

// PasswordChange is a new password hash and what it ends.
type PasswordChange struct {
	Hash               string
	MustChangePassword bool
	// KeepSession is the hash of the session that stays logged in (the user
	// changing their own password); every other session of the user ends.
	KeepSession string
	// RevokeAgents disconnects the user's agents: the password was changed
	// by someone else, so whoever had the account may have connected them.
	RevokeAgents bool
}

// displayName is the name to show: the name, or the username without one.
func displayName(name, username string) string {
	if name != "" {
		return name
	}

	return username
}
