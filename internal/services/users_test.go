package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// lookupCounter is a store that only counts user lookups.
type lookupCounter struct {
	store.Store

	lookups int
}

func (counter *lookupCounter) GetUser(context.Context, string) (store.User, error) {
	counter.lookups++

	return store.User{}, store.ErrNotFound
}

func TestUsersRefuseMalformedIDsBeforeLookup(t *testing.T) {
	ctx := context.Background()
	counter := &lookupCounter{}
	users := NewUsers(slog.New(slog.NewTextHandler(io.Discard, nil)), counter)
	admin := store.User{ID: "admin001", Role: store.RoleSuperadmin}

	for _, id := range []string{"", "ADMIN001", "admin00", "../users", "admin0011"} {
		if _, err := users.Get(ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("Get(%q) = %v, want not found", id, err)
		}

		if _, err := users.ResetPassword(ctx, admin, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("ResetPassword(%q) = %v, want not found", id, err)
		}
	}

	if counter.lookups != 0 {
		t.Errorf("%d lookups of malformed ids, want none", counter.lookups)
	}
}

// A user stored without a valid password hash (as a password-less sign-in
// would store one) is opened by no password, the dummy hash's included.
func TestAuthenticateWithoutPasswordHash(t *testing.T) {
	iterations := PasswordIterations
	PasswordIterations = 1000

	t.Cleanup(func() { PasswordIterations = iterations })

	ctx := context.Background()
	database := openStore(t)
	users := NewUsers(slog.New(slog.DiscardHandler), database)

	for username, hash := range map[string]string{"sso": "", "broken": "x"} {
		if _, err := database.CreateUser(ctx, store.User{Email: username, Name: username, Role: store.RoleUser, CreatedAt: time.Now()}, hash); err != nil {
			t.Fatal(err)
		}

		for _, password := range []string{"unknown user", "", hash} {
			if _, ok, err := users.Authenticate(ctx, username, password); ok || err != nil {
				t.Errorf("user %q with hash %q: password %q logs in (err %v)", username, hash, password, err)
			}
		}
	}
}
