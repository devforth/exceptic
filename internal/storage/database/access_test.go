package database

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

func sameAccess(got, want store.Access) bool {
	return got.Limited == want.Limited && slices.Equal(got.ProjectIDs, want.ProjectIDs)
}

// A user stored before access existed sees every project.
func TestUserWithoutAccessSeesAll(t *testing.T) {
	var user userRecord

	if err := json.Unmarshal([]byte(`{"ID":"abcd1234","Username":"dev","Role":"user","password_hash":"x"}`), &user); err != nil {
		t.Fatal(err)
	}

	if !user.ProjectAccess().All() {
		t.Errorf("access: %+v", user.Access)
	}
}

func (database *Store) testUser(t *testing.T, name string, access store.Access) string {
	t.Helper()

	id, err := database.CreateUser(context.Background(), store.User{Email: name, Role: store.RoleAdmin, Access: access}, "x")

	if err != nil {
		t.Fatal(err)
	}

	return id
}

func (database *Store) accessOf(t *testing.T, id string) store.Access {
	t.Helper()

	user, err := database.GetUser(context.Background(), id)

	if err != nil {
		t.Fatal(err)
	}

	return user.Access
}

// A limited owner gets a new project in the same batch; a deleted project
// leaves every user's access, and an emptied one stays limited.
func TestProjectAccessFollowsProjects(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	limited := database.testUser(t, "limited", store.OnlyProjects(5))
	everyone := database.testUser(t, "everyone", store.Access{})

	id, err := database.CreateProject(ctx, store.Project{Name: "worker", PublicKey: "0123456789abcdef"}, time.Now(), limited)

	if err != nil {
		t.Fatal(err)
	}

	if access := database.accessOf(t, limited); !sameAccess(access, store.OnlyProjects(5, id)) {
		t.Errorf("the limited owner: %+v", access)
	}

	if _, err := database.CreateProject(ctx, store.Project{Name: "api", PublicKey: "fedcba9876543210"}, time.Now(), everyone); err != nil {
		t.Fatal(err)
	}

	if access := database.accessOf(t, everyone); !access.All() {
		t.Errorf("an owner with every project: %+v", access)
	}

	only := database.testUser(t, "only", store.OnlyProjects(id))

	if err := database.DeleteProject(ctx, id); err != nil {
		t.Fatal(err)
	}

	if access := database.accessOf(t, limited); !sameAccess(access, store.OnlyProjects(5)) {
		t.Errorf("after the delete: %+v", access)
	}

	if access := database.accessOf(t, only); !access.Limited || len(access.ProjectIDs) != 0 {
		t.Errorf("a user with only the deleted project: %+v", access)
	}
}

// Two limited administrators saving one user at once keep each other's part.
func TestUpdateUserMergesAccess(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	target := database.testUser(t, "target", store.OnlyProjects(3))

	var wait sync.WaitGroup

	for _, project := range []int{1, 2} {
		wait.Go(func() {
			change := store.UserChange{Email: "target", Name: "target", Role: store.RoleAdmin, By: store.OnlyProjects(project), Access: new(store.OnlyProjects(project))}

			if err := database.UpdateUser(ctx, target, change); err != nil {
				t.Error(err)
			}
		})
	}

	wait.Wait()

	if access := database.accessOf(t, target); !sameAccess(access, store.OnlyProjects(1, 2, 3)) {
		t.Errorf("access: %+v", access)
	}

	locked := store.UserChange{Email: "target", Name: "target", Role: store.RoleAdmin, By: store.OnlyProjects(1), Access: &store.Access{}}

	if err := database.UpdateUser(ctx, target, locked); !errors.Is(err, store.ErrAccessLocked) {
		t.Errorf("a limited administrator gave every project: %v", err)
	}

	// Name and role change only by someone who has all the user's projects.
	for _, change := range []store.UserChange{
		{Email: "target", Name: "target", Role: store.RoleUser},
		{Email: "target", Name: "renamed", Role: store.RoleAdmin},
		{Email: "target@example.com", Name: "target", Role: store.RoleAdmin},
	} {
		change.By = store.OnlyProjects(1, 2)

		if err := database.UpdateUser(ctx, target, change); !errors.Is(err, store.ErrTakeOverLocked) {
			t.Errorf("%+v by an administrator without project 3: %v", change, err)
		}
	}

	if err := database.UpdateUser(ctx, target, store.UserChange{Email: "target", Name: "renamed", Role: store.RoleUser, By: store.OnlyProjects(1, 2, 3)}); err != nil {
		t.Errorf("a role change by an administrator with all of them: %v", err)
	}
}

// The delivery log within some connections and projects; a message stored
// before messages had projects only with every project.
func TestOutboxChannels(t *testing.T) {
	ctx, now := context.Background(), time.Now()
	database := openTest(t)

	for channel, projectIDs := range map[string][]int{"first": {1}, "second": {1, 2}, "third": nil} {
		if err := database.EnqueueOutbox(ctx, []string{channel}, projectIDs, "notification", []byte(`{}`), now); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range []struct {
		filter store.OutboxFilter
		want   int
	}{
		{store.OutboxFilter{}, 3},
		{store.OutboxFilter{Channels: []string{}}, 0},
		{store.OutboxFilter{Channels: []string{"first", "third"}}, 2},
		{store.OutboxFilter{Channels: []string{"first", "third"}, Status: store.OutboxPending}, 2},
		{store.OutboxFilter{Channels: []string{"first", "third"}, Channel: "second"}, 0},
		{store.OutboxFilter{Channels: []string{"first"}, Channel: "first", Status: store.OutboxPending}, 1},
		{store.OutboxFilter{Access: store.OnlyProjects(1)}, 1},
		{store.OutboxFilter{Access: store.OnlyProjects(1, 2)}, 2},
		{store.OutboxFilter{Access: store.OnlyProjects(1, 2), Channel: "third"}, 0},
	} {
		count, err := database.CountOutbox(ctx, test.filter)
		records, listErr := database.ListOutbox(ctx, test.filter)

		if err != nil || listErr != nil || count != test.want || len(records) != test.want {
			t.Errorf("%+v: count %d, list %d, want %d (%v, %v)", test.filter, count, len(records), test.want, err, listErr)
		}
	}
}
