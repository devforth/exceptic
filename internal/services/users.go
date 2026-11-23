package services

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Users manages the panel's users. Who may change whom is decided by the
// methods of store.User; every method checks it again, whatever the page
// showed. Users change their own password through Auth.
type Users struct {
	logger   *slog.Logger
	store    store.Store
	projects config.ProjectList // nil: project ids are not checked
	now      func() time.Time
}

func NewUsers(logger *slog.Logger, database store.Store) *Users {
	return &Users{logger: logger, store: database, now: time.Now}
}

// EnsureSuperadmin creates the superadmin from ADMIN_USERNAME and
// ADMIN_PASSWORD when there is none: once, at the first start. Later the
// variables are not used.
func (users *Users) EnsureSuperadmin(ctx context.Context, username, password string) error {
	_, err := users.store.Superadmin(ctx)

	switch {
	case err == nil:
		return nil
	case !errors.Is(err, store.ErrNotFound):
		return err
	case password == "":
		users.logger.Warn("no superadmin yet: set ADMIN_PASSWORD (and ADMIN_USERNAME) to create one, the UI is closed until then")

		return nil
	}

	hash, err := hashPassword(password)

	if err != nil {
		return err
	}

	_, err = users.store.CreateUser(ctx, store.User{Username: username, Name: username, Role: store.RoleSuperadmin, CreatedAt: users.now()}, hash)

	if errors.Is(err, store.ErrUsernameTaken) {
		return fmt.Errorf("ADMIN_USERNAME %q belongs to an existing user: choose another username for the superadmin", username)
	}

	if err != nil {
		return err
	}

	users.logger.Info("superadmin created", "username", username)

	return nil
}

func (users *Users) List(ctx context.Context) ([]store.User, error) {
	return users.store.ListUsers(ctx)
}

// Get is the user id; an id of another shape is not found without a lookup.
func (users *Users) Get(ctx context.Context, id string) (store.User, error) {
	if !store.ValidID(id) {
		return store.User{}, store.ErrNotFound
	}

	return users.store.GetUser(ctx, id)
}

// Create adds a user with a temporary password, returned to be shown once.
func (users *Users) Create(ctx context.Context, viewer store.User, input UserInput) (string, string, Problems, error) {
	if !viewer.CanAdminister() {
		return "", "", nil, ErrForbidden
	}

	input.Username = strings.TrimSpace(input.Username)
	access := viewer.ProjectAccess()

	if input.Access != nil {
		access = *input.Access
	}

	problems := checkUserInput(input)
	users.checkAccess(problems, access)

	switch {
	case input.Username == "":
		problems["username"] = "Enter a username."
	case !config.ValidUsername(input.Username):
		problems["username"] = fmt.Sprintf("Up to %d letters, digits and . _ @ -.", config.MaxUsername)
	}

	forbidden := Problems{}
	checkRole(forbidden, viewer, input.Role)

	if !viewer.CanGiveAccess(access) {
		forbidden["projects"] = accessLockedProblem
	}

	if err := refused(problems, forbidden); err != nil || len(problems) > 0 {
		return "", "", problems, err
	}

	password := temporaryPassword()
	hash, err := hashPassword(password)

	if err != nil {
		return "", "", nil, err
	}

	user := store.User{Username: input.Username, Name: nameOr(input.Name, input.Username), Role: input.Role, MustChangePassword: true, CreatedAt: users.now(), Access: access}
	id, err := users.store.CreateUser(ctx, user, hash)

	if errors.Is(err, store.ErrUsernameTaken) {
		return "", "", nil, Refusal{Kind: ErrConflict, Problems: Problems{"username": "This username is taken."}}
	}

	return id, password, nil, err
}

// Update changes a user's name, role and access. The access chosen
// replaces only the part the viewer sees (store.Access.Replace): the
// user's other projects stay. Name and role change only when the viewer
// may take the user over (store.User.CanTakeOver).
func (users *Users) Update(ctx context.Context, viewer store.User, id string, input UserInput) (Problems, error) {
	target, err := users.manageable(ctx, viewer, id)

	if err != nil {
		return nil, err
	}

	problems := checkUserInput(input)
	change := store.UserChange{Name: nameOr(input.Name, target.Username), Role: input.Role, By: viewer.ProjectAccess(), Access: input.Access}

	if input.Access != nil {
		users.checkAccess(problems, *input.Access)
	}

	forbidden := Problems{}

	if !viewer.CanTakeOver(target) {
		takeOverProblems(forbidden, target, change)
	}

	checkRole(forbidden, viewer, input.Role)

	if err := refused(problems, forbidden); err != nil || len(problems) > 0 {
		return problems, err
	}

	err = users.store.UpdateUser(ctx, id, change)

	// What the viewer may not give, decided again against the stored user.
	switch {
	case errors.Is(err, store.ErrAccessLocked):
		return nil, Refusal{Kind: ErrForbidden, Problems: Problems{"projects": accessLockedProblem}}
	case errors.Is(err, store.ErrTakeOverLocked):
		// The user gained a project meanwhile: say it at the fields changed.
		problems = Problems{"role": TakeOverProblem}
		takeOverProblems(problems, target, change)

		return nil, Refusal{Kind: ErrForbidden, Problems: problems}
	}

	return nil, err
}

// refused is the refusal of a form with forbidden problems (what the
// viewer may not give), the other problems shown with them; nil without.
func refused(problems, forbidden Problems) error {
	if len(forbidden) == 0 {
		return nil
	}

	maps.Copy(problems, forbidden)

	return Refusal{Kind: ErrForbidden, Problems: problems}
}

// checkRole adds a problem when viewer may not give role.
func checkRole(problems Problems, viewer store.User, role string) {
	if !viewer.CanAssign(role) {
		problems["role"] = "Your role may not give this role."
	}
}

// takeOverProblems adds a problem at the name and the role when change
// changes them.
func takeOverProblems(problems Problems, target store.User, change store.UserChange) {
	if change.Name != target.DisplayName() {
		problems["name"] = TakeOverProblem
	}

	if change.Role != target.Role {
		problems["role"] = TakeOverProblem
	}
}

// TakeOverProblem: why viewer may not rename, delete or reset the password
// of a user, or change their role (store.User.CanTakeOver).
const TakeOverProblem = "Only an admin with all of this user's projects can change it."

// AccessEditable says whether viewer may change target's access, and why
// not when they manage target but may not.
func (users *Users) AccessEditable(viewer, target store.User) (bool, string) {
	if viewer.CanChangeAccessOf(target) {
		return true, ""
	}

	if viewer.CanManage(target) {
		return false, accessLockedProblem
	}

	return false, ""
}

// accessLockedProblem: a change only someone with every project may make.
const accessLockedProblem = "Only someone with access to all projects can give or change access to all projects."

// checkAccess adds a problem when access names a project that does not exist.
func (users *Users) checkAccess(problems Problems, access store.Access) {
	if users.projects == nil || access.All() {
		return
	}

	for _, id := range access.ProjectIDs {
		if _, ok := users.projects.List().ByID(id); !ok {
			problems["projects"] = "Choose existing projects."

			return
		}
	}
}

// ResetPassword gives a user a temporary password (returned to be shown
// once); their sessions end and their agents are disconnected.
func (users *Users) ResetPassword(ctx context.Context, viewer store.User, id string) (string, error) {
	target, err := users.manageable(ctx, viewer, id)

	if err != nil {
		return "", err
	}

	if !viewer.CanTakeOver(target) {
		return "", Forbidden(TakeOverProblem)
	}

	return users.setTemporaryPassword(ctx, id, true)
}

// Delete removes a user with their sessions and agents; their name stays in
// the issue histories.
func (users *Users) Delete(ctx context.Context, viewer store.User, id string) error {
	target, err := users.manageable(ctx, viewer, id)

	if err != nil {
		return err
	}

	if !viewer.CanTakeOver(target) {
		return Forbidden(TakeOverProblem)
	}

	return users.store.DeleteUser(ctx, id)
}

// UpdateName changes the viewer's own name.
func (users *Users) UpdateName(ctx context.Context, viewer store.User, name string) (Problems, error) {
	problems := Problems{}

	if !checkName(problems, name) {
		return problems, nil
	}

	return nil, users.store.UpdateUserName(ctx, viewer.ID, nameOr(name, viewer.Username))
}

// SetPasswordByOperator is the reset-password command: run with the server
// stopped by whoever runs it, usually the superadmin who forgot the password.
// Sessions end, agents stay; the new password is temporary.
func (users *Users) SetPasswordByOperator(ctx context.Context, username string) (string, error) {
	user, err := users.store.FindUser(ctx, username)

	if err != nil {
		return "", err
	}

	return users.setTemporaryPassword(ctx, user.ID, false)
}

func (users *Users) setTemporaryPassword(ctx context.Context, id string, revokeAgents bool) (string, error) {
	password := temporaryPassword()

	return password, setPassword(ctx, users.store, id, password, store.PasswordChange{MustChangePassword: true, RevokeAgents: revokeAgents})
}

// manageable returns user id when the viewer may change them (ErrForbidden
// otherwise).
func (users *Users) manageable(ctx context.Context, viewer store.User, id string) (store.User, error) {
	target, err := users.Get(ctx, id)

	switch {
	case err != nil:
	case target.ID == viewer.ID:
		err = Forbidden(OwnAccountProblem)
	case !viewer.CanManage(target):
		err = Forbidden("your role may not change this user")
	}

	return target, err
}

// OwnAccountProblem: nobody changes themselves as a user they manage.
const OwnAccountProblem = "change your own account on /account (update_account_name)"

// Authenticate checks a login: the user with username and password, ok
// false when either is wrong. An unknown username is checked against a dummy
// hash, so the response time does not tell which usernames exist; so is a
// user without a password hash, whom no password opens.
func (users *Users) Authenticate(ctx context.Context, username, password string) (store.User, bool, error) {
	user, hash, err := users.store.FindLogin(ctx, strings.TrimSpace(username))

	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.User{}, false, err
	}

	if !checkPassword(cmp.Or(hash, dummyPasswordHash()), password) || user.ID == "" || hash == "" {
		return store.User{}, false, nil
	}

	return user, true, nil
}

// DeviceStamp is a hash of the login of username that changes with its
// password, role or deletion ("" for no such user): the login limit's
// device cookie is bound to it.
func (users *Users) DeviceStamp(ctx context.Context, username string) (string, error) {
	user, hash, err := users.store.FindLogin(ctx, strings.TrimSpace(username))

	if errors.Is(err, store.ErrNotFound) || (err == nil && user.ID == "") {
		return "", nil
	}

	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte("device stamp\x00" + user.ID + "\x00" + user.Role + "\x00" + hash))

	return hex.EncodeToString(sum[:16]), nil
}

// ChangeOwnPassword replaces the viewer's own password after checking the
// current one (wrongCurrent counts as a failed login). The other sessions
// end, keepSession (a hash) stays; agents stay connected.
func (users *Users) ChangeOwnPassword(ctx context.Context, viewer store.User, current, password, confirm, keepSession string) (problems Problems, wrongCurrent bool, err error) {
	hash, err := users.store.UserPassword(ctx, viewer.ID)

	if err != nil {
		return nil, false, err
	}

	problems = Problems{}

	if !checkPassword(hash, current) {
		wrongCurrent = true
		problems["current_password"] = "The current password is wrong."
	}

	checkNewPassword(problems, current, password, confirm)

	if len(problems) > 0 {
		return problems, wrongCurrent, nil
	}

	return nil, false, setPassword(ctx, users.store, viewer.ID, password, store.PasswordChange{KeepSession: keepSession})
}

// nameOr is the display name as saved: the username when none is given.
func nameOr(name, username string) string {
	return cmp.Or(strings.TrimSpace(name), username)
}

// setPassword stores a new password hash with what the change ends. It
// checks no role: callers are the methods of Users, which do.
func setPassword(ctx context.Context, database store.Store, id string, password string, change store.PasswordChange) error {
	hash, err := hashPassword(password)

	if err != nil {
		return err
	}

	change.Hash = hash

	return database.SetPassword(ctx, id, change)
}

func checkUserInput(input UserInput) Problems {
	problems := Problems{}
	checkName(problems, input.Name)

	return problems
}

// checkName adds a problem when a display name is too long.
func checkName(problems Problems, name string) bool {
	if utf8.RuneCountInString(strings.TrimSpace(name)) > config.MaxDisplayName {
		problems["name"] = fmt.Sprintf("At most %d characters.", config.MaxDisplayName)

		return false
	}

	return true
}

// checkNewPassword adds a problem when a new password is not acceptable.
func checkNewPassword(problems Problems, current, password, confirm string) {
	switch {
	case !config.ValidPasswordLength(password):
		problems["password"] = fmt.Sprintf("%d to %d characters.", config.MinPasswordLength, config.MaxPasswordLength)
	case password == current:
		problems["password"] = "Choose a password different from the current one."
	case password != confirm:
		problems["confirm_password"] = "The passwords do not match."
	}
}
