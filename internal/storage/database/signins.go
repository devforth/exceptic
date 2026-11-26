package database

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// signInRecord is an entry of a user's sign-in log, keyed by its sequence.
type signInRecord struct {
	store.SignIn
	Sequence uint64 `json:"sequence"`
}

// RecordSignIn adds an attempt to the log of a user still there and keeps
// the newest store.SignInsKept of its kind (sign-ins or refusals).
func (database *Store) RecordSignIn(_ context.Context, signIn store.SignIn) error {
	sequence, err := database.db.Next()

	if err != nil {
		return err
	}

	signIn.At = signIn.At.UTC()

	return database.db.Update(false, func(batch *kv.Batch) error {
		// Under the write lock: a user deleted meanwhile gets no orphan rows.
		if _, err := database.user(batch, signIn.UserID); err != nil {
			return notFoundIsNil(err)
		}

		// The walk sees only stored rows, so one fewer makes room for this one.
		ok := signIn.Result == store.SignInOK
		kind := func(record signInRecord) bool { return (record.Result == store.SignInOK) == ok }

		if err := database.deleteSignInsOf(batch, signIn.UserID, store.SignInsKept-1, kind); err != nil {
			return err
		}

		return database.signIns.Insert(batch, signInRecord{SignIn: signIn, Sequence: sequence})
	})
}

// deleteSignInsOf deletes a user's sign-in attempts of kind (nil: all) but the newest keep.
func (database *Store) deleteSignInsOf(batch *kv.Batch, userID string, keep int, kind func(signInRecord) bool) error {
	kept := 0

	return database.signIns.Primary(kv.Text(userID)).Newest().Each(batch, func(entry kv.Entry[signInRecord]) (bool, error) {
		if kind != nil && !kind(entry.Record) {
			return true, nil
		}

		if kept < keep {
			kept++

			return true, nil
		}

		database.signIns.DeleteEntry(batch, entry)

		return true, nil
	})
}

func (database *Store) SignIns(_ context.Context, userID string) ([]store.SignIn, error) {
	signIns := []store.SignIn{}

	err := database.signIns.Primary(kv.Text(userID)).Newest().Each(database.db, func(entry kv.Entry[signInRecord]) (bool, error) {
		signIns = append(signIns, entry.Record.SignIn)

		return true, nil
	})

	return signIns, err
}

// TouchSession records a session's activity; a session gone meanwhile is no error.
func (database *Store) TouchSession(_ context.Context, tokenHash string, now time.Time) error {
	return database.db.Update(false, func(batch *kv.Batch) error {
		session, err := database.sessions.Get(batch, textKey(tokenHash))

		if errors.Is(err, kv.ErrNotFound) {
			return nil
		}

		if err != nil {
			return err
		}

		session.LastActiveAt = now.UTC()

		if err := database.sessions.Save(batch, session); err != nil {
			return err
		}

		user, err := database.user(batch, session.UserID)

		if err != nil {
			return notFoundIsNil(err)
		}

		at := now.UTC()
		user.LastActiveAt = &at

		return database.users.Save(batch, user)
	})
}

// notFoundIsNil: a record gone meanwhile is no error here.
func notFoundIsNil(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, kv.ErrNotFound) {
		return nil
	}

	return err
}

func (database *Store) Sessions(_ context.Context, userID string, now time.Time) ([]store.Session, error) {
	sessions := []store.Session{}

	err := database.sessions.By("user", kv.Text(userID)).Each(database.db, func(entry kv.Entry[sessionRecord]) (bool, error) {
		if record := entry.Record; record.ExpiresAt.After(now) {
			sessions = append(sessions, store.Session{
				ID: store.SessionID(record.TokenHash), Method: record.Method, Address: record.Address, Browser: record.Browser,
				// A session stored before activity was recorded: active when it began.
				CreatedAt: record.CreatedAt, LastActiveAt: cmp.Or(record.LastActiveAt, record.CreatedAt), ExpiresAt: record.ExpiresAt,
			})
		}

		return true, nil
	})

	slices.SortFunc(sessions, func(first, second store.Session) int { return second.CreatedAt.Compare(first.CreatedAt) })

	return sessions, err
}

func (database *Store) EndUserSession(_ context.Context, userID, sessionID string) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		ended := false

		err := database.sessions.By("user", kv.Text(userID)).Each(batch, func(entry kv.Entry[sessionRecord]) (bool, error) {
			if store.SessionID(entry.Record.TokenHash) == sessionID {
				database.sessions.DeleteEntry(batch, entry)

				ended = true
			}

			return !ended, nil
		})

		if err == nil && !ended {
			err = store.ErrNotFound
		}

		return err
	})
}

func (database *Store) GoogleUser(_ context.Context, sub string) (store.User, error) {
	user, err := database.users.GetBy(database.db, "google", kv.K(kv.Text(sub)))

	return user.User, notFound(err)
}

func (database *Store) LinkGoogle(_ context.Context, userID, email, sub string, now time.Time) error {
	err := database.changeUser(userID, func(_ *kv.Batch, user *userRecord) error {
		switch {
		case !strings.EqualFold(user.Email, email):
			return store.ErrNotFound
		case user.GoogleSub == sub:
			return nil
		case user.GoogleSub != "":
			return store.ErrGoogleLinked
		case user.GoogleLinkOff:
			return store.ErrGoogleOff
		}

		at := now.UTC()
		user.GoogleSub, user.GoogleLinkedAt = sub, &at

		return nil
	})

	if conflict := (kv.ErrConflict{}); errors.As(err, &conflict) && conflict.Index == "google" {
		return store.ErrGoogleTaken
	}

	return err
}

func (database *Store) UnlinkGoogle(_ context.Context, userID, keepSession string) error {
	return database.changeUser(userID, func(batch *kv.Batch, user *userRecord) error {
		user.GoogleSub, user.GoogleLinkedAt, user.GoogleLinkOff = "", nil, true

		return database.deleteGoogleSessions(batch, userID, keepSession)
	})
}
