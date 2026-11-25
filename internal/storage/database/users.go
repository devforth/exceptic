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

// userRecord is a user with the password hash, which only FindLogin and UserPassword return.
type userRecord struct {
	store.User
	PasswordHash string `json:"password_hash"`
}

// sessionRecord is a login to the UI; the cookie itself is never stored.
type sessionRecord struct {
	TokenHash string    `json:"token_hash"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// freeID returns a new random ID that no record of collection has.
func freeID[T any](reader kv.Reader, collection *kv.Collection[T]) (string, error) {
	for {
		id := store.NewID()

		if _, err := collection.Get(reader, textKey(id)); errors.Is(err, kv.ErrNotFound) {
			return id, nil
		} else if err != nil {
			return "", err
		}
	}
}

func (database *Store) user(reader kv.Reader, id string) (userRecord, error) {
	user, err := database.users.Get(reader, textKey(id))

	return user, notFound(err)
}

// CreateUser stores a new user; ErrUsernameTaken when the username is taken in any case.
func (database *Store) CreateUser(_ context.Context, user store.User, passwordHash string) (string, error) {
	err := database.db.Update(true, func(batch *kv.Batch) error {
		var err error

		if user.ID, err = freeID(batch, database.users); err != nil {
			return err
		}

		if user.Access.Limited {
			user.Access = store.OnlyProjects(user.Access.ProjectIDs...)
		}

		return database.users.Insert(batch, userRecord{User: user, PasswordHash: passwordHash})
	})

	if conflict := (kv.ErrConflict{}); errors.As(err, &conflict) && conflict.Index == "name" {
		return "", store.ErrUsernameTaken
	}

	return user.ID, err
}

func (database *Store) GetUser(_ context.Context, id string) (store.User, error) {
	user, err := database.user(database.db, id)

	return user.User, err
}

// FindUser finds a user by username, in any case.
func (database *Store) FindUser(_ context.Context, username string) (store.User, error) {
	user, err := database.users.GetBy(database.db, "name", usernameKey(username))

	return user.User, notFound(err)
}

// FindLogin finds a user by username with their password hash, for a login.
func (database *Store) FindLogin(_ context.Context, username string) (store.User, string, error) {
	user, err := database.users.GetBy(database.db, "name", usernameKey(username))

	return user.User, user.PasswordHash, notFound(err)
}

// UserPassword returns a user's password hash, to check a password.
func (database *Store) UserPassword(_ context.Context, id string) (string, error) {
	user, err := database.user(database.db, id)

	return user.PasswordHash, err
}

// Superadmin returns the superadmin; ErrNotFound when there is none yet.
func (database *Store) Superadmin(context.Context) (store.User, error) {
	user, err := database.users.GetBy(database.db, "superadmin", kv.K(kv.U8(0)))

	return user.User, notFound(err)
}

// ListUsers lists the users by role (superadmin first), then username.
func (database *Store) ListUsers(context.Context) ([]store.User, error) {
	records, err := database.users.Primary().All(database.db)

	if err != nil {
		return nil, err
	}

	// Ranks start at 1, so a missing one (0) is the last rank, users'.
	rank := map[string]int{store.RoleSuperadmin: 1, store.RoleAdmin: 2}
	rankOf := func(user store.User) int { return cmp.Or(rank[user.Role], 3) }
	users := make([]store.User, 0, len(records))

	for _, record := range records {
		users = append(users, record.User)
	}

	slices.SortFunc(users, func(first, second store.User) int {
		return cmp.Or(cmp.Compare(rankOf(first), rankOf(second)), strings.Compare(strings.ToLower(first.Username), strings.ToLower(second.Username)))
	})

	return users, nil
}

// changeUser rewrites a user in one batch; change may also write other records.
func (database *Store) changeUser(id string, change func(batch *kv.Batch, user *userRecord) error) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		user, err := database.user(batch, id)

		if err != nil {
			return err
		}

		if err := change(batch, &user); err != nil {
			return err
		}

		return database.users.Save(batch, user)
	})
}

// UpdateUser changes a user's name and role, and their access when given,
// merged with the stored one (store.Access.Replace).
func (database *Store) UpdateUser(_ context.Context, id string, change store.UserChange) error {
	return database.changeUser(id, func(_ *kv.Batch, user *userRecord) error {
		if (user.Role != change.Role || user.DisplayName() != change.Name) && !user.CoveredBy(change.By) {
			return store.ErrTakeOverLocked
		}

		user.Name, user.Role = change.Name, change.Role

		if change.Access == nil {
			return nil
		}

		access, err := user.Access.Replace(change.By, *change.Access)
		user.Access = access

		return err
	})
}

// dropProjectFromUsers removes a deleted project from the users who have
// it; one left without projects sees none.
func (database *Store) dropProjectFromUsers(batch *kv.Batch, projectID int) error {
	return database.users.Primary().Each(batch, func(entry kv.Entry[userRecord]) (bool, error) {
		if !entry.Record.Access.Limited || !slices.Contains(entry.Record.Access.ProjectIDs, projectID) {
			return true, nil
		}

		user := entry.Record
		user.Access.ProjectIDs = slices.DeleteFunc(slices.Clone(user.Access.ProjectIDs), func(id int) bool { return id == projectID })

		return true, database.users.Save(batch, user)
	})
}

// addProjectToUser gives a limited user a project they created.
func (database *Store) addProjectToUser(batch *kv.Batch, userID string, projectID int) error {
	user, err := database.user(batch, userID)

	if err != nil || user.ProjectAccess().All() {
		return err
	}

	user.Access = store.OnlyProjects(append(slices.Clone(user.Access.ProjectIDs), projectID)...)

	return database.users.Save(batch, user)
}

// UpdateUserName changes only the name, so a role changed meanwhile stays.
func (database *Store) UpdateUserName(_ context.Context, id string, name string) error {
	return database.changeUser(id, func(_ *kv.Batch, user *userRecord) error {
		user.Name = name

		return nil
	})
}

// UpdateUserTheme changes only the web app's theme of a user.
func (database *Store) UpdateUserTheme(_ context.Context, id string, theme string) error {
	return database.changeUser(id, func(_ *kv.Batch, user *userRecord) error {
		user.Theme = theme

		return nil
	})
}

// SetPassword replaces a user's password, ends their other sessions and, if asked, disconnects their agents.
func (database *Store) SetPassword(_ context.Context, id string, change store.PasswordChange) error {
	return database.changeUser(id, func(batch *kv.Batch, user *userRecord) error {
		user.PasswordHash, user.MustChangePassword = change.Hash, change.MustChangePassword

		err := database.deleteSessionsOf(batch, id, change.KeepSession)

		if err == nil && change.RevokeAgents {
			err = database.deleteAgentsOf(batch, id)
		}

		return err
	})
}

// DeleteUser removes a user with their sessions and agents; the status changes they made keep their name.
func (database *Store) DeleteUser(_ context.Context, id string) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		user, err := database.user(batch, id)

		if err != nil {
			return err
		}

		if err := database.users.Delete(batch, user); err != nil {
			return err
		}

		if err := database.deleteSessionsOf(batch, id, ""); err != nil {
			return err
		}

		return database.deleteAgentsOf(batch, id)
	})
}

// RecordLogin notes the time of a user's login.
func (database *Store) RecordLogin(_ context.Context, id string, now time.Time) error {
	return database.changeUser(id, func(_ *kv.Batch, user *userRecord) error {
		at := now.UTC()
		user.LastLoginAt = &at

		return nil
	})
}

// CreateSession stores a login, first dropping expired sessions (login is rare).
func (database *Store) CreateSession(_ context.Context, tokenHash string, userID string, now, expiresAt time.Time) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		err := database.sessions.Primary().Where(func(session sessionRecord) bool { return !session.ExpiresAt.After(now) }).
			Each(batch, func(entry kv.Entry[sessionRecord]) (bool, error) {
				database.sessions.DeleteEntry(batch, entry)

				return true, nil
			})

		if err != nil {
			return err
		}

		return database.sessions.Insert(batch, sessionRecord{TokenHash: tokenHash, UserID: userID, CreatedAt: now.UTC(), ExpiresAt: expiresAt.UTC()})
	})
}

// SessionUser returns the user of a session that has not expired; ErrNotFound otherwise.
func (database *Store) SessionUser(_ context.Context, tokenHash string, now time.Time) (store.User, error) {
	session, err := database.sessions.Get(database.db, textKey(tokenHash))

	if err != nil {
		return store.User{}, notFound(err)
	}

	if !session.ExpiresAt.After(now) {
		return store.User{}, store.ErrNotFound
	}

	user, err := database.user(database.db, session.UserID)

	return user.User, err
}

func (database *Store) DeleteSession(_ context.Context, tokenHash string) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		_, err := database.sessions.DeleteKey(batch, textKey(tokenHash))

		return err
	})
}

// deleteSessionsOf ends a user's sessions but keep.
func (database *Store) deleteSessionsOf(batch *kv.Batch, userID string, keep string) error {
	return database.sessions.By("user", kv.Text(userID)).Each(batch, func(entry kv.Entry[sessionRecord]) (bool, error) {
		if entry.Record.TokenHash != keep {
			database.sessions.DeleteEntry(batch, entry)
		}

		return true, nil
	})
}
