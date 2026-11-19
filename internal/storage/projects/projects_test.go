package projects

import (
	"context"
	"errors"
	"log/slog"
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
	registry, err := Open(ctx, slog.New(slog.DiscardHandler), database, nil)

	if err != nil {
		t.Fatal(err)
	}

	database.hold.Store(true)

	stale := make(chan error, 1)

	go func() { stale <- registry.Reload(ctx) }()

	<-database.read

	created := make(chan error, 1)

	go func() {
		_, err := registry.Create(ctx, store.Project{Name: "worker"})

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
	registry, err := Open(ctx, slog.New(slog.DiscardHandler), database, nil)

	if err != nil {
		t.Fatal(err)
	}

	id, err := registry.Create(ctx, store.Project{Name: "worker"})

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

	if issues, _ := database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{id}}); issues != 0 {
		t.Fatalf("%d issues of the deleted project left", issues)
	}
}
