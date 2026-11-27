package projects

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

// heldStore holds a ListProjects, once hold is set, after it has read the
// list, until release is closed.
type heldStore struct {
	store.Store
	hold    atomic.Bool
	read    chan struct{}
	release chan struct{}
}

func (database *heldStore) ListProjects(ctx context.Context) ([]store.Project, error) {
	projects, err := database.Store.ListProjects(ctx)

	if database.hold.CompareAndSwap(true, false) {
		close(database.read)
		<-database.release
	}

	return projects, err
}

// A reload that read the list before a change cannot store it after the
// reload of that change.
func TestReloadKeepsTheNewestList(t *testing.T) {
	ctx := context.Background()
	database := &heldStore{Store: storetest.Open(t), read: make(chan struct{}), release: make(chan struct{})}
	registry, err := Open(ctx, database)

	if err != nil {
		t.Fatal(err)
	}

	database.hold.Store(true)

	stale := make(chan error, 1)

	go func() { stale <- registry.Reload(ctx) }()

	<-database.read

	created := make(chan error, 1)

	go func() {
		_, err := registry.Create(ctx, store.Project{Name: "worker"}, "")

		created <- err
	}()

	// Without the lock the create's reload finishes first; with it, it waits
	// for the stale one, so the wait only gives the bug its chance.
	select {
	case err := <-created:
		created <- err
	case <-time.After(100 * time.Millisecond):
	}

	close(database.release)

	if err := errors.Join(<-stale, <-created); err != nil {
		t.Fatal(err)
	}

	if _, listed := registry.List().Find("worker"); !listed {
		t.Fatal("a stale reload dropped the project just created")
	}
}

// A delete whose request ends early still removes the project from the list
// ingest checks and deletes its data.
func TestDeleteOutlivesTheRequest(t *testing.T) {
	ctx := context.Background()
	database := storetest.Open(t)
	registry, err := Open(ctx, database)

	if err != nil {
		t.Fatal(err)
	}

	id, err := registry.Create(ctx, store.Project{Name: "worker"}, "")

	if err != nil {
		t.Fatal(err)
	}

	if _, err := database.SaveEvent(ctx, store.NewEvent{ProjectID: id, EventID: "first", Fingerprint: "first", Title: "Error", Level: "error",
		Timestamp: time.Now(), ReceivedAt: time.Now(), Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}

	ended, cancel := context.WithCancel(ctx)
	cancel()

	if err := registry.Delete(ended, id); err != nil {
		t.Fatal(err)
	}

	if _, listed := registry.List().ByID(id); listed {
		t.Fatal("a deleted project is still listed, so its DSN is accepted")
	}

	if issues, _, _ := database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{id}}); issues != 0 {
		t.Fatalf("%d issues of the deleted project left", issues)
	}
}

// Open seeds nothing: a new database has no project, and one stored with
// its own id and key is listed as stored and can be deleted like any other.
func TestOpenListsStoredProjects(t *testing.T) {
	ctx := context.Background()
	database := storetest.Open(t)
	registry, err := Open(ctx, database)

	if err != nil {
		t.Fatal(err)
	}

	if listed := registry.List(); len(listed) != 0 {
		t.Fatalf("a new database lists %+v", listed)
	}

	if err := database.AddProjects(ctx, time.Now(), store.Project{ID: 1, Name: "backend", PublicKey: "0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}

	if registry, err = Open(ctx, database); err != nil {
		t.Fatal(err)
	}

	if project, listed := registry.List().ByID(1); !listed || project.Name != "backend" || project.PublicKey != "0123456789abcdef" {
		t.Fatalf("stored project: %+v, %v", project, listed)
	}

	if err := registry.Delete(ctx, 1); err != nil {
		t.Fatal(err)
	}

	if _, listed := registry.List().ByID(1); listed {
		t.Fatal("the deleted project is still listed")
	}
}
