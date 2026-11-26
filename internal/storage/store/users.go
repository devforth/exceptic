package store

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Roles of the users (RoleSummaries). What each may do is decided by the
// methods of User below, the one place for these rules: the catalog and
// services ask them, and the web app shows what they answer.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// RoleSummaries say what each role may do, for the user form; they follow
// the methods below.
var RoleSummaries = map[string]string{
	RoleAdmin: "everything: every project, users, settings, deleting",
	RoleUser:  "issue statuses, logs, project DSNs and their own coding agents",
}

var ErrEmailTaken = errors.New("email is taken")

// ErrNoProjects: a user of the role user would see no project, and so nothing.
var ErrNoProjects = errors.New("a user needs a project")

// ErrLastAdmin: the change would leave no admin, and with none the server
// opens the first-admin setup to whoever knows ADMIN_EMAIL and ADMIN_PASSWORD.
var ErrLastAdmin = errors.New("the last admin")

// ErrAdminExists: the first-admin setup is over, an admin exists.
var ErrAdminExists = errors.New("an admin exists")

// User is a user of the panel. The password hash is never read with it: only
// UserPassword and FindLogin return it, where a password is checked.
type User struct {
	ID string
	// Email is the login, unique in any case: an email, or the username of a
	// user stored before logins were emails, until it is changed. Stored
	// under its former name.
	Email              string `json:"Username"`
	Name               string
	Role               string
	MustChangePassword bool
	CreatedAt          time.Time
	LastLoginAt        *time.Time
	// LastActiveAt is the last request of any of the user's sessions (written at most every SessionActiveEvery).
	LastActiveAt *time.Time `json:",omitempty"`
	// Access is the projects the user sees; ProjectAccess applies it.
	Access Access
	// Theme is the web app's color theme the user chose (one of Themes); empty until the first login records the system's.
	Theme string `json:",omitempty"`
	// GoogleSub is the Google account linked to the user (its subject), set at
	// the first Google sign-in with the user's email; a changed email drops it.
	GoogleSub      string     `json:",omitempty"`
	GoogleLinkedAt *time.Time `json:",omitempty"`
	// GoogleLinkOff: Google was unlinked, so no Google sign-in links again by
	// the email until the user signs in with the password.
	GoogleLinkOff bool `json:",omitempty"`
}

// Themes are the web app's color themes.
var Themes = []string{"light", "dark"}

// ProjectAccess is the projects the user sees: every one for an admin,
// whatever is stored; none for a role this code does not know.
func (user User) ProjectAccess() Access {
	switch user.Role {
	case RoleAdmin:
		return Access{}
	case RoleUser:
		return user.Access
	}

	return OnlyProjects()
}

// CanGiveAccess: user may give chosen to another user: only projects they
// have, every project only an admin.
func (user User) CanGiveAccess(chosen Access) bool {
	return user.ProjectAccess().Covers(chosen)
}

// SeesUser: target is in user's list of users: for an admin everyone, for
// a user themselves and the users of the role user they share a project
// with (an admin, whom a user cannot change, is not shown).
func (user User) SeesUser(target User) bool {
	switch {
	case user.Role == RoleAdmin || target.ID == user.ID:
		return true
	case target.Role != RoleUser:
		return false
	}

	return user.ProjectAccess().Shares(target.ProjectAccess())
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

// DisplayName is the name, or the email when no name is set.
func (user User) DisplayName() string {
	return displayName(user.Name, user.Email)
}

// CanAdminister: projects, notifications, users and deleting issues.
func (user User) CanAdminister() bool {
	return user.Role == RoleAdmin
}

// CanManage reports whether user may change target at all (their access
// within user's projects, and with CanTakeOver the rest): an admin every
// other user, admins included; a user the users they see, never an admin;
// nobody themselves here (they have the account page).
func (user User) CanManage(target User) bool {
	switch {
	case target.ID == user.ID:
		return false
	case user.Role == RoleAdmin:
		return true
	}

	return target.Role == RoleUser && user.SeesUser(target)
}

// CanAccessAgentsOf: one's own MCP agents, or those of a user one may take
// over (disconnecting them is part of a password reset).
func (user User) CanAccessAgentsOf(owner User) bool {
	return owner.ID == user.ID || user.CanTakeOver(owner)
}

// AssignableRoles are every role, in the order the user form offers them.
var AssignableRoles = []string{RoleAdmin, RoleUser}

// AssignableRoles are the roles user may give: an admin every one, a user
// only the role user.
func (user User) AssignableRoles() []string {
	if user.Role == RoleAdmin {
		return slices.Clone(AssignableRoles)
	}

	return []string{RoleUser}
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
// role or email only when By covers the user (User.CoveredBy, else
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

// NameAfterEmail is the name to save when a user's email changes from
// previous to email: a name that was the email (saved when none was given)
// or none follows it.
func NameAfterEmail(name, previous, email string) string {
	if name == "" || name == previous || name == DefaultName(previous) {
		return DefaultName(email)
	}

	return name
}

// DefaultName is a user's name when none is given: the email before the @
// (the whole login when it is no email), so other users never see the address.
func DefaultName(email string) string {
	local, _, _ := strings.Cut(email, "@")

	return local
}

// displayName is the name to show: the name, or the default one without it.
func displayName(name, email string) string {
	if name != "" {
		return name
	}

	return DefaultName(email)
}
