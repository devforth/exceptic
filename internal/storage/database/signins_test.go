package database

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// The log keeps the newest SignInsKept sign-ins and as many refusals apart,
// so a flood of refusals never pushes a sign-in out; a deleted user gets none.
func TestSignInLogKeepsEachKind(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	id, err := database.CreateUser(ctx, store.User{Email: "dev@example.com", Role: store.RoleUser, CreatedAt: now}, "hash")

	if err != nil {
		t.Fatal(err)
	}

	record := func(result string, count int) {
		for index := range count {
			if err := database.RecordSignIn(ctx, store.SignIn{UserID: id, At: now.Add(time.Duration(index) * time.Second), Method: store.SignInPassword, Result: result}); err != nil {
				t.Fatal(err)
			}
		}
	}

	record(store.SignInOK, 3)
	record(store.SignInBlocked, store.SignInsKept+5)

	signIns, err := database.SignIns(ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	results := map[string]int{}

	for _, signIn := range signIns {
		results[signIn.Result]++
	}

	if results[store.SignInOK] != 3 || results[store.SignInBlocked] != store.SignInsKept {
		t.Errorf("kept %v, want 3 ok and %d blocked", results, store.SignInsKept)
	}

	if err := database.DeleteUser(ctx, id); err != nil {
		t.Fatal(err)
	}

	record(store.SignInWrongPassword, 1)

	if signIns, _ := database.SignIns(ctx, id); len(signIns) != 0 {
		t.Errorf("a deleted user's log got %d rows", len(signIns))
	}
}

// LinkGoogle checks in its batch what the sign-in found: an email changed
// meanwhile or another account linked meanwhile refuses it; an unlink
// disconnects the Google sessions and stops linking by email until a
// password sign-in.
func TestLinkGoogleChecksInItsBatch(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	id, err := database.CreateUser(ctx, store.User{Email: "dev@example.com", Role: store.RoleUser, CreatedAt: now}, "hash")

	if err != nil {
		t.Fatal(err)
	}

	if err := database.LinkGoogle(ctx, id, "old@example.com", "google-1", now); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("another email: %v", err)
	}

	if err := database.LinkGoogle(ctx, id, "Dev@Example.com", "google-1", now); err != nil {
		t.Fatal(err)
	}

	if err := database.LinkGoogle(ctx, id, "dev@example.com", "google-2", now); !errors.Is(err, store.ErrGoogleLinked) {
		t.Errorf("a second account: %v", err)
	}

	session := func(method string, index int) string {
		hash := fmt.Sprintf("%016d%048d", index, 0)

		if err := database.CreateSession(ctx, store.NewSession{TokenHash: hash, UserID: id, Method: method, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}

		return store.SessionID(hash)
	}

	password, google, current := session(store.SignInPassword, 1), session(store.SignInGoogle, 2), session(store.SignInGoogle, 3)

	if err := database.UnlinkGoogle(ctx, id, current); err != nil {
		t.Fatal(err)
	}

	sessions, err := database.Sessions(ctx, id, now)

	if err != nil {
		t.Fatal(err)
	}

	left := map[string]bool{}

	for _, session := range sessions {
		left[session.ID] = true
	}

	if !left[password] || left[google] || !left[current] || len(left) != 2 {
		t.Errorf("after the unlink %v: want the password and the current one", left)
	}

	if err := database.LinkGoogle(ctx, id, "dev@example.com", "google-1", now); !errors.Is(err, store.ErrGoogleOff) {
		t.Errorf("relinking before a password sign-in: %v", err)
	}

	if err := database.RecordLogin(ctx, id, store.SignInGoogle, now); err != nil {
		t.Fatal(err)
	}

	if err := database.LinkGoogle(ctx, id, "dev@example.com", "google-1", now); !errors.Is(err, store.ErrGoogleOff) {
		t.Errorf("a Google login turned linking on again: %v", err)
	}

	if err := database.RecordLogin(ctx, id, store.SignInPassword, now); err != nil {
		t.Fatal(err)
	}

	if err := database.LinkGoogle(ctx, id, "dev@example.com", "google-1", now); err != nil {
		t.Errorf("after a password sign-in: %v", err)
	}

	// An admin's change of the email drops the link and its sessions.
	google = session(store.SignInGoogle, 4)
	user, _ := database.GetUser(ctx, id)

	if err := database.UpdateUser(ctx, id, store.UserChange{Email: "ops@example.com", Name: user.Name, Role: user.Role, By: store.Access{}}); err != nil {
		t.Fatal(err)
	}

	sessions, _ = database.Sessions(ctx, id, now)

	for _, session := range sessions {
		if session.ID == google {
			t.Error("a Google session outlived the email change")
		}
	}

	if user, _ := database.GetUser(ctx, id); user.GoogleSub != "" {
		t.Error("the link outlived the email change")
	}
}
