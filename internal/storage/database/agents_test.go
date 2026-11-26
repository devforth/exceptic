package database

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

func TestOAuthClientCapacityAndCleanup(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	// A full table of clients that never connected, client "1" the oldest.
	addClients := func(prefix string, at func(index int) time.Time) {
		t.Helper()

		err := database.db.Update(true, func(batch *kv.Batch) error {
			for index := 1; index <= maxOAuthClients; index++ {
				if err := database.oauthClients.Insert(batch, store.OAuthClient{ID: fmt.Sprintf("%s%d", prefix, index), Name: "Client", CreatedAt: at(index)}); err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			t.Fatal(err)
		}
	}

	count := func() int {
		clients, err := database.oauthClients.Primary().All(database.db)

		if err != nil {
			t.Fatal(err)
		}

		return len(clients)
	}

	addClients("", func(index int) time.Time { return now.Add(time.Duration(index) * time.Second) })

	user, err := database.CreateUser(ctx, store.User{Email: "admin", Role: store.RoleAdmin, CreatedAt: now}, "x")

	if err != nil {
		t.Fatal(err)
	}

	// Client "2" is being authorized: a live code keeps it.
	if err := database.CreateOAuthCode(ctx, "pending-code", store.OAuthCode{UserID: user, ClientID: "2", ExpiresAt: now.Add(26 * time.Hour)}, now); err != nil {
		t.Fatal(err)
	}

	// Full: the oldest unused client makes room for a new one.
	if err := database.RegisterOAuthClient(ctx, store.OAuthClient{ID: "new-client", CreatedAt: now.Add(time.Minute)}); err != nil {
		t.Fatalf("full table refused a registration: %v", err)
	}

	if _, err := database.GetOAuthClient(ctx, "1"); !errors.Is(err, store.ErrNotFound) || count() != maxOAuthClients {
		t.Fatalf("oldest unused client kept (%v), %d clients", err, count())
	}

	// After unusedClientAge the unused ones go; the one being authorized stays.
	later := now.Add(unusedClientAge + time.Hour)

	if err := database.RegisterOAuthClient(ctx, store.OAuthClient{ID: "later-client", CreatedAt: later}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.GetOAuthClient(ctx, "2"); err != nil {
		t.Fatal("pending authorization lost its client", err)
	}

	if clients := count(); clients != 2 {
		t.Fatalf("unused clients not cleaned up: %d", clients)
	}

	// Every client in use: the table is full for real.
	addClients("used-", func(int) time.Time { return later })

	for index := 1; index <= maxOAuthClients; index++ {
		code := store.OAuthCode{UserID: user, ClientID: fmt.Sprintf("used-%d", index), ExpiresAt: later.Add(time.Hour)}

		if err := database.CreateOAuthCode(ctx, fmt.Sprintf("code-%d", index), code, later); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.RegisterOAuthClient(ctx, store.OAuthClient{ID: "one-more", CreatedAt: later}); !errors.Is(err, store.ErrOAuthCapacity) {
		t.Fatalf("capacity with every client in use: %v", err)
	}
}

// Emails are unique in any case, the last admin stays, and deleting a user
// ends their sessions and disconnects their agents.
func TestUserRules(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	root, err := database.CreateUser(ctx, store.User{Email: "Root", Role: store.RoleAdmin, CreatedAt: now}, "hash")

	if err != nil {
		t.Fatal(err)
	}

	if _, err := database.CreateUser(ctx, store.User{Email: "root", Role: store.RoleUser, CreatedAt: now}, "hash"); !errors.Is(err, store.ErrEmailTaken) {
		t.Fatalf("same email in another case: %v", err)
	}

	if err := database.DeleteUser(ctx, root); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("deleting the last admin: %v", err)
	}

	if err := database.UpdateUser(ctx, root, store.UserChange{Name: "Root", Role: store.RoleUser}); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("demoting the last admin: %v", err)
	}

	second, err := database.CreateUser(ctx, store.User{Email: "second", Role: store.RoleAdmin, CreatedAt: now}, "hash")

	if err != nil {
		t.Fatal(err)
	}

	// With another admin, one may go.
	if err := database.UpdateUser(ctx, second, store.UserChange{Name: "second", Role: store.RoleUser}); err != nil {
		t.Fatalf("demoting one of two admins: %v", err)
	}

	user, err := database.CreateUser(ctx, store.User{Email: "dev", Role: store.RoleUser, CreatedAt: now}, "hash")

	if err != nil {
		t.Fatal(err)
	}

	if found, err := database.FindUser(ctx, "DEV"); err != nil || found.ID != user {
		t.Fatalf("find in another case: %+v, %v", found, err)
	}

	for _, hash := range []string{"session-a", "session-b"} {
		if err := database.CreateSession(ctx, store.NewSession{TokenHash: hash, UserID: user, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.ReplacePersonalToken(ctx, user, "token", now); err != nil {
		t.Fatal(err)
	}

	// Changing the own password keeps this session and the agents.
	if err := database.SetPassword(ctx, user, store.PasswordChange{Hash: "new", KeepSession: "session-a"}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.SessionUser(ctx, "session-b", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("the other session survived a password change")
	}

	if has, _ := database.HasPersonalToken(ctx, user); !has {
		t.Fatal("the own password change disconnected the agents")
	}

	if err := database.DeleteUser(ctx, user); err != nil {
		t.Fatal(err)
	}

	if _, err := database.SessionUser(ctx, "session-a", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("a session of a deleted user still works")
	}

	if _, err := database.UseGrant(ctx, "token", now); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("a token of a deleted user still works")
	}

	if admins, err := database.Admins(ctx); err != nil || len(admins) != 1 || admins[0].ID != root {
		t.Fatalf("admins: %+v, %v", admins, err)
	}
}

// Users are listed by role, admins first, then by email.
func TestListUsersByRole(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	for username, role := range map[string]string{"bob": store.RoleAdmin, "admin": store.RoleAdmin, "alice": store.RoleUser, "carol": store.RoleUser} {
		if _, err := database.CreateUser(ctx, store.User{Email: username, Role: role, CreatedAt: now}, "hash"); err != nil {
			t.Fatal(err)
		}
	}

	users, err := database.ListUsers(ctx)

	if err != nil {
		t.Fatal(err)
	}

	var order []string

	for _, user := range users {
		order = append(order, user.Email)
	}

	if want := []string{"admin", "bob", "alice", "carol"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

// Agents are listed newest first, and disconnecting one drops only the codes
// its user allowed: a client id published as a URL is shared by every user.
func TestGrantsOfASharedClient(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	client := "https://client.example/metadata.json"
	alice, _ := database.CreateUser(ctx, store.User{Email: "alice", Role: store.RoleUser, CreatedAt: now}, "hash")
	bob, _ := database.CreateUser(ctx, store.User{Email: "bob", Role: store.RoleUser, CreatedAt: now}, "hash")

	var created []string

	for index := range 10 {
		tokens := store.GrantTokens{AccessHash: fmt.Sprintf("access-%d", index), AccessExpiresAt: now.Add(time.Hour)}
		id, err := database.CreateGrant(ctx, store.Grant{UserID: alice, Kind: store.GrantOAuth, ClientID: client, CreatedAt: now.Add(time.Duration(index) * time.Minute)}, tokens)

		if err != nil {
			t.Fatal(err)
		}

		created = append([]string{id}, created...)
	}

	grants, err := database.ListGrants(ctx, alice, now)

	if err != nil {
		t.Fatal(err)
	}

	var listed []string

	for _, grant := range grants {
		listed = append(listed, grant.ID)
	}

	if !slices.Equal(listed, created) {
		t.Errorf("grants %v, want newest first %v", listed, created)
	}

	for user, hash := range map[string]string{alice: "alice-code", bob: "bob-code"} {
		if err := database.CreateOAuthCode(ctx, hash, store.OAuthCode{UserID: user, ClientID: client, ExpiresAt: now.Add(time.Hour)}, now); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.DeleteGrant(ctx, created[0]); err != nil {
		t.Fatal(err)
	}

	if _, err := database.TakeOAuthCode(ctx, "alice-code", now); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the disconnected user's code still works: %v", err)
	}

	if _, err := database.TakeOAuthCode(ctx, "bob-code", now); err != nil {
		t.Errorf("another user's code of the same client: %v", err)
	}
}

// A refresh token works once; presented again, it revokes its grant.
func TestRefreshTokenReplayRevokesTheGrant(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	user, _ := database.CreateUser(ctx, store.User{Email: "admin", Role: store.RoleAdmin, CreatedAt: now}, "x")
	tokens := store.GrantTokens{AccessHash: "access-1", AccessExpiresAt: now.Add(time.Hour), RefreshHash: "refresh-1", RefreshExpiresAt: now.Add(24 * time.Hour)}
	grant, err := database.CreateGrant(ctx, store.Grant{UserID: user, Kind: store.GrantOAuth, ClientID: "client", CreatedAt: now}, tokens)

	if err != nil {
		t.Fatal(err)
	}

	next := store.GrantTokens{AccessHash: "access-2", AccessExpiresAt: now.Add(time.Hour), RefreshHash: "refresh-2", RefreshExpiresAt: now.Add(24 * time.Hour)}

	if err := database.RotateGrant(ctx, "other-client", "refresh-1", next, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another client rotated the grant: %v", err)
	}

	if err := database.RotateGrant(ctx, "client", "refresh-1", next, now); err != nil {
		t.Fatal(err)
	}

	if _, err := database.UseGrant(ctx, "access-2", now); err != nil {
		t.Fatal("the new access token does not work", err)
	}

	if err := database.RotateGrant(ctx, "client", "refresh-1", next, now); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("a spent refresh token was accepted: %v", err)
	}

	if _, err := database.GetGrant(ctx, grant); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("a replayed refresh token did not revoke the grant")
	}
}
