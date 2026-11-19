package services

import (
	"context"
	"errors"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// Sessions are the logins of the web UI, stored by the hash of their cookie.
type Sessions struct {
	store store.Store
}

func NewSessions(database store.Store) *Sessions {
	return &Sessions{store: database}
}

// Open reports whether a superadmin exists, so somebody can log in.
func (sessions *Sessions) Open(ctx context.Context) (bool, error) {
	_, err := sessions.store.Superadmin(ctx)

	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}

	return err == nil, err
}

// Start stores a new session of the user until expires and records the login.
func (sessions *Sessions) Start(ctx context.Context, tokenHash, userID string, now, expires time.Time) error {
	if err := sessions.store.CreateSession(ctx, tokenHash, userID, now, expires); err != nil {
		return err
	}

	return sessions.store.RecordLogin(ctx, userID, now)
}

func (sessions *Sessions) End(ctx context.Context, tokenHash string) error {
	return sessions.store.DeleteSession(ctx, tokenHash)
}

// User returns the user of a valid session; store.ErrNotFound otherwise.
func (sessions *Sessions) User(ctx context.Context, tokenHash string, now time.Time) (store.User, error) {
	return sessions.store.SessionUser(ctx, tokenHash, now)
}
