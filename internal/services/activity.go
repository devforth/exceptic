package services

import (
	"context"
	"errors"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// Activity is how a user signs in: their sessions, their sign-in log and the
// Google account linked to them.
type Activity struct {
	Sessions       []store.Session
	SignIns        []store.SignIn
	GoogleLinkedAt *time.Time
}

// Activity of user id, for the user themselves or a viewer above them
// (store.User.CanManage).
func (users *Users) Activity(ctx context.Context, viewer store.User, id string) (Activity, error) {
	target, err := users.ownOrManaged(ctx, viewer, id)

	if err != nil {
		return Activity{}, err
	}

	sessions, err := users.store.Sessions(ctx, target.ID, users.now())

	if err != nil {
		return Activity{}, err
	}

	signIns, err := users.store.SignIns(ctx, target.ID)

	return Activity{Sessions: sessions, SignIns: signIns, GoogleLinkedAt: target.GoogleLinkedAt}, err
}

// EndSession ends a session of user id: one's own but the one of current (that is
// logging out), or of a user the viewer may take over (store.User.CanTakeOver).
func (users *Users) EndSession(ctx context.Context, viewer store.User, id, sessionID, current string) error {
	target, err := users.takenOverOrOwn(ctx, viewer, id)

	if err != nil {
		return err
	}

	if target.ID == viewer.ID && sessionID == current {
		return Forbidden("this is the session you use: log out instead")
	}

	return users.store.EndUserSession(ctx, target.ID, sessionID)
}

// UnlinkGoogle drops the Google account of user id (one's own, or of a user
// the viewer may take over) and ends the sessions opened through it, but the
// viewer's current one.
func (users *Users) UnlinkGoogle(ctx context.Context, viewer store.User, id, current string) error {
	target, err := users.takenOverOrOwn(ctx, viewer, id)

	if err != nil {
		return err
	}

	keep := ""

	if target.ID == viewer.ID {
		keep = current
	}

	return users.store.UnlinkGoogle(ctx, target.ID, keep)
}

// ownOrManaged is the viewer (id empty or theirs) or a user they manage.
func (users *Users) ownOrManaged(ctx context.Context, viewer store.User, id string) (store.User, error) {
	if id == "" || id == viewer.ID {
		return users.Get(ctx, viewer.ID)
	}

	return users.manageable(ctx, viewer, id)
}

// takenOverOrOwn is the viewer, or a user they manage and may take over.
func (users *Users) takenOverOrOwn(ctx context.Context, viewer store.User, id string) (store.User, error) {
	target, err := users.ownOrManaged(ctx, viewer, id)

	if err == nil && target.ID != viewer.ID && !viewer.CanTakeOver(target) {
		err = Forbidden(TakeOverProblem)
	}

	return target, err
}

// GoogleSignIn finds the user of a Google account: the one it is linked to,
// else the user of its verified email, whom it is then linked to, unless
// another Google account is linked to them (result store.SignInNotLinked)
// or Google was unlinked from them (store.SignInUnlinked), each with the
// user, for their log. No user: store.ErrNotFound.
func (users *Users) GoogleSignIn(ctx context.Context, sub, email string) (store.User, string, error) {
	user, err := users.store.GoogleUser(ctx, sub)

	if err == nil || !errors.Is(err, store.ErrNotFound) {
		return user, store.SignInOK, err
	}

	user, err = users.store.FindUser(ctx, email)

	if err != nil {
		return store.User{}, "", err
	}

	// LinkGoogle checks the email and the link again in its batch.
	switch err := users.store.LinkGoogle(ctx, user.ID, email, sub, users.now()); {
	case errors.Is(err, store.ErrGoogleLinked):
		return user, store.SignInNotLinked, nil
	case errors.Is(err, store.ErrGoogleOff):
		return user, store.SignInUnlinked, nil
	case err != nil:
		return store.User{}, "", err
	}

	user, err = users.store.GetUser(ctx, user.ID)

	return user, store.SignInOK, err
}
