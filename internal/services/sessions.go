package services

import (
	"context"
	"errors"
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Sessions are the logins of the web UI, stored by the hash of their cookie.
type Sessions struct {
	store store.Store
}

func NewSessions(database store.Store) *Sessions {
	return &Sessions{store: database}
}

// Open reports whether an admin exists, so somebody can log in.
func (sessions *Sessions) Open(ctx context.Context) (bool, error) {
	admins, err := sessions.store.Admins(ctx)

	return len(admins) > 0, err
}

// Start stores a new session and records the login, in the user's sign-in log too.
func (sessions *Sessions) Start(ctx context.Context, login store.NewSession) error {
	login.Browser = textutil.Shorten(login.Browser, store.MaxBrowserLength)

	if err := sessions.store.CreateSession(ctx, login); err != nil {
		return err
	}

	if err := sessions.store.RecordLogin(ctx, login.UserID, login.Method, login.CreatedAt); err != nil {
		return err
	}

	return sessions.store.RecordSignIn(ctx, store.SignIn{
		UserID: login.UserID, At: login.CreatedAt, Method: login.Method, Result: store.SignInOK, Address: login.Address, Browser: login.Browser,
	})
}

// Refused records a refused attempt at the user of email in their sign-in
// log; an email of nobody records nothing.
func (sessions *Sessions) Refused(ctx context.Context, email string, attempt store.SignIn) error {
	user, err := sessions.store.FindUser(ctx, email)

	if errors.Is(err, store.ErrNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	attempt.UserID, attempt.Browser = user.ID, textutil.Shorten(attempt.Browser, store.MaxBrowserLength)

	return sessions.store.RecordSignIn(ctx, attempt)
}

// Touch records a session's activity.
func (sessions *Sessions) Touch(ctx context.Context, tokenHash string, now time.Time) error {
	return sessions.store.TouchSession(ctx, tokenHash, now)
}

func (sessions *Sessions) End(ctx context.Context, tokenHash string) error {
	return sessions.store.DeleteSession(ctx, tokenHash)
}

// User returns the user of a valid session; store.ErrNotFound otherwise.
func (sessions *Sessions) User(ctx context.Context, tokenHash string, now time.Time) (store.User, error) {
	return sessions.store.SessionUser(ctx, tokenHash, now)
}
