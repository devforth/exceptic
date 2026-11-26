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
	"slices"
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

// EnsureAdmin creates an admin from ADMIN_EMAIL (the login as given, an
// email or not) and ADMIN_PASSWORD while there is none. Later the variables
// are not used.
func (users *Users) EnsureAdmin(ctx context.Context, email, password string) error {
	admins, err := users.store.Admins(ctx)

	switch {
	case err != nil:
		return err
	case len(admins) > 0:
		return nil
	case password == "":
		users.logger.Warn("no admin yet: set ADMIN_PASSWORD (and ADMIN_EMAIL) to create one, the UI is closed until then")

		return nil
	}

	hash, err := hashPassword(password)

	if err != nil {
		return err
	}

	_, err = users.store.CreateUser(ctx, store.User{Email: email, Name: store.DefaultName(email), Role: store.RoleAdmin, CreatedAt: users.now()}, hash)

	if errors.Is(err, store.ErrEmailTaken) {
		return fmt.Errorf("ADMIN_EMAIL %q belongs to an existing user: choose another email for the admin", email)
	}

	if err != nil {
		return err
	}

	users.logger.Info("admin created", "email", email)

	return nil
}

// List is the users viewer sees (store.User.SeesUser).
func (users *Users) List(ctx context.Context, viewer store.User) ([]store.User, error) {
	all, err := users.store.ListUsers(ctx)

	return slices.DeleteFunc(all, func(user store.User) bool { return !viewer.SeesUser(user) }), err
}

// Visible is the user id as viewer sees them: one outside their list is hidden.
func (users *Users) Visible(ctx context.Context, viewer store.User, id string) (store.User, error) {
	target, err := users.Get(ctx, id)

	if err == nil && !viewer.SeesUser(target) {
		err = Hidden("user", id)
	}

	return target, err
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
	email, emailOK := config.ParseEmail(input.Email)
	access := viewer.ProjectAccess()

	if input.Access != nil {
		access = *input.Access
	}

	problems := checkUserInput(input)
	users.checkAccess(problems, input.Role, access)

	if !emailOK {
		problems["email"] = enterEmail
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

	user := store.User{Email: email, Name: nameOr(input.Name, email), Role: input.Role, MustChangePassword: true, CreatedAt: users.now(), Access: access}
	id, err := users.store.CreateUser(ctx, user, hash)

	if errors.Is(err, store.ErrEmailTaken) {
		return "", "", nil, emailTaken()
	}

	return id, password, nil, err
}

const enterEmail = "Enter a valid email address."

// emailTaken: emails are unique; telling so names a user only to
// administrators, or to a user who has just typed their own password.
func emailTaken() error {
	return Refusal{Kind: ErrConflict, Problems: Problems{"email": emailTakenProblem}}
}

const emailTakenProblem = "This email is taken."

// newEmail is the email a form sets: the stored one when unchanged (in any
// case; a login stored before emails stays), else a valid email.
func newEmail(problems Problems, stored, entered string) string {
	entered = strings.TrimSpace(entered)

	if strings.EqualFold(entered, stored) {
		return stored
	}

	email, ok := config.ParseEmail(entered)

	if !ok {
		problems["email"] = enterEmail
	}

	return email
}

// Update changes a user's email, name, role and access. The access chosen
// replaces only the part the viewer sees (store.Access.Replace): the
// user's other projects stay. Email, name and role change only when the
// viewer may take the user over (store.User.CanTakeOver).
func (users *Users) Update(ctx context.Context, viewer store.User, id string, input UserInput) (Problems, error) {
	target, err := users.manageable(ctx, viewer, id)

	if err != nil {
		return nil, err
	}

	problems := checkUserInput(input)
	email := newEmail(problems, target.Email, input.Email)
	change := store.UserChange{Email: email, Name: store.NameAfterEmail(strings.TrimSpace(input.Name), target.Email, email), Role: input.Role, By: viewer.ProjectAccess(), Access: input.Access}

	if input.Access != nil {
		users.checkAccess(problems, "", *input.Access) // empty is checked merged, in the batch
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
	case errors.Is(err, store.ErrEmailTaken):
		return nil, emailTaken()
	case errors.Is(err, store.ErrAccessLocked):
		return nil, Refusal{Kind: ErrForbidden, Problems: Problems{"projects": accessLockedProblem}}
	case errors.Is(err, store.ErrNoProjects):
		return Problems{"projects": noProjectsProblem}, nil
	case errors.Is(err, store.ErrLastAdmin):
		return nil, Refusal{Kind: ErrForbidden, Problems: Problems{"role": lastAdminProblem}}
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

// takeOverProblems adds a problem at the email, the name and the role when
// change changes them.
func takeOverProblems(problems Problems, target store.User, change store.UserChange) {
	if change.Email != target.Email {
		problems["email"] = TakeOverProblem
	}

	if change.Name != target.DisplayName() {
		problems["name"] = TakeOverProblem
	}

	if change.Role != target.Role {
		problems["role"] = TakeOverProblem
	}
}

// TakeOverProblem: why viewer may not rename, delete or reset the password
// of a user, or change their email or role (store.User.CanTakeOver).
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
func (users *Users) checkAccess(problems Problems, role string, access store.Access) {
	if access.All() {
		return
	}

	if role == store.RoleUser && len(access.ProjectIDs) == 0 {
		problems["projects"] = noProjectsProblem

		return
	}

	if users.projects == nil {
		return
	}

	for _, id := range access.ProjectIDs {
		if _, ok := users.projects.List().ByID(id); !ok {
			problems["projects"] = "Choose existing projects."

			return
		}
	}
}

// noProjectsProblem: a user without projects would see nothing.
const noProjectsProblem = "Choose at least one project."

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

	err = users.store.DeleteUser(ctx, id)

	if errors.Is(err, store.ErrLastAdmin) {
		return Forbidden(lastAdminProblem)
	}

	return err
}

// lastAdminProblem: without an admin nobody could manage the server.
const lastAdminProblem = "This is the last admin: make another user an admin first."

// UpdateName changes the viewer's own name.
func (users *Users) UpdateName(ctx context.Context, viewer store.User, name string) (Problems, error) {
	problems := Problems{}

	if !checkName(problems, name) {
		return problems, nil
	}

	return nil, users.store.UpdateUserName(ctx, viewer.ID, nameOr(name, viewer.Email))
}

// SetTheme saves the viewer's own theme of the web app (one of store.Themes).
func (users *Users) SetTheme(ctx context.Context, viewer store.User, theme string) error {
	// The catalog offers only these (Choices); anything else is a caller's bug.
	if !slices.Contains(store.Themes, theme) {
		return fmt.Errorf("unknown theme %q", theme)
	}

	return users.store.UpdateUserTheme(ctx, viewer.ID, theme)
}

// SetPasswordByOperator is the reset-password command: run with the server
// stopped by whoever runs it, usually an admin who forgot the password.
// Sessions end, agents stay; the new password is temporary.
func (users *Users) SetPasswordByOperator(ctx context.Context, email string) (string, error) {
	user, err := users.store.FindUser(ctx, email)

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
	target, err := users.Visible(ctx, viewer, id)

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

// Authenticate checks a login: the user with email and password, ok false
// when either is wrong. An unknown email is checked against a dummy hash, so
// the response time does not tell which emails exist; so is a user without
// a password hash, whom no password opens.
func (users *Users) Authenticate(ctx context.Context, email, password string) (store.User, bool, error) {
	user, hash, err := users.store.FindLogin(ctx, strings.TrimSpace(email))

	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.User{}, false, err
	}

	if !checkPassword(cmp.Or(hash, dummyPasswordHash()), password) || user.ID == "" || hash == "" {
		return store.User{}, false, nil
	}

	return user, true, nil
}

// DeviceStamp is a hash of the login of email that changes with its
// password, role or deletion ("" for no such user): the login limit's
// device cookie is bound to it.
func (users *Users) DeviceStamp(ctx context.Context, email string) (string, error) {
	user, hash, err := users.store.FindLogin(ctx, strings.TrimSpace(email))

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

// ChangeOwnEmail replaces the viewer's own email after checking the current
// password (wrongCurrent counts as a failed login) and returns the email
// the user has now. Sessions and agents stay.
func (users *Users) ChangeOwnEmail(ctx context.Context, viewer store.User, current, entered string) (problems Problems, wrongCurrent bool, email string, err error) {
	hash, err := users.store.UserPassword(ctx, viewer.ID)

	if err != nil {
		return nil, false, viewer.Email, err
	}

	problems = Problems{}

	if !checkPassword(hash, current) {
		wrongCurrent = true
		problems["current_password"] = "The current password is wrong."
	}

	email = newEmail(problems, viewer.Email, entered)

	if len(problems) > 0 || email == viewer.Email {
		return problems, wrongCurrent, viewer.Email, nil
	}

	err = users.store.UpdateUserEmail(ctx, viewer.ID, email)

	if errors.Is(err, store.ErrEmailTaken) {
		return Problems{"email": emailTakenProblem}, false, viewer.Email, nil
	}

	if err != nil {
		return nil, false, viewer.Email, err
	}

	return nil, false, email, nil
}

// nameOr is the display name as saved: the email when none is given.
func nameOr(name, email string) string {
	return cmp.Or(strings.TrimSpace(name), store.DefaultName(email))
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
