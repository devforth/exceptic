package database

import (
	"context"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

func usage(t *testing.T, database *Store) store.ProjectUsage {
	t.Helper()

	usage, err := database.ProjectUsage(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	return usage
}

// sum adds up the values of a map.
func sum[K comparable](values map[K]int64) int64 {
	var total int64

	for _, value := range values {
		total += value
	}

	return total
}

// Each project's issues, events and logs are counted apart, together as
// many bytes as the cleanup's counters hold; deleting a project takes its
// count away, and the newest issue or log tells when it last received.
func TestProjectUsage(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

	database.SetClock(func() time.Time { return now })

	for _, project := range []store.Project{{ID: 1, Name: "backend", PublicKey: "0123456789abcdef"}, {ID: 2, Name: "frontend", PublicKey: "fedcba9876543210"}} {
		if err := database.SeedProject(ctx, project, now); err != nil {
			t.Fatal(err)
		}
	}

	if got := usage(t, database); len(got.Bytes) != 0 || len(got.LastReceived) != 0 || got.Counting {
		t.Fatalf("empty database: %+v", got)
	}

	writeEvents(t, database, "crash", "error", 3) // project 1
	writeLogs(t, database, "info", 4, "line")     // projects 1 and 2

	got := usage(t, database)

	if got.Bytes[1] == 0 || got.Bytes[2] == 0 || got.Bytes[1] <= got.Bytes[2] || sum(got.Bytes) != database.logicalSize() {
		t.Errorf("bytes %v, the counters hold %d", got.Bytes, database.logicalSize())
	}

	if !got.LastReceived[1].Equal(now.Add(2*time.Second)) || !got.LastReceived[2].Equal(now) {
		t.Errorf("last received %v", got.LastReceived)
	}

	if err := database.DeleteProject(ctx, 2); err != nil {
		t.Fatal(err)
	}

	if err := database.PurgeProjects(ctx); err != nil {
		t.Fatal(err)
	}

	if got := usage(t, database); got.Bytes[2] != 0 || len(got.Bytes) != 1 || sum(got.Bytes) != database.logicalSize() {
		t.Errorf("after deleting project 2: %v, the counters hold %d", got.Bytes, database.logicalSize())
	}
}

// What each project keeps, by the kinds the cleanup deletes: the oldest log
// below error, error or fatal log and event; none when there is none.
func TestProjectUsageOldest(t *testing.T) {
	database := openTest(t)
	seedShares(t, database, 100, 100)

	first := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	second := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

	clockAt(database, first)
	writeLogs(t, database, "debug", 2, "debug") // projects 1 and 2
	writeEvents(t, database, "crash", "warning", 1)
	clockAt(database, second)
	writeLogs(t, database, "fatal", 1, "fatal") // project 1
	writeLogs(t, database, "info", 2, "info")
	writeEvents(t, database, "crash", "error", 1)

	got := usage(t, database)

	if want := (store.OldestKept{LogsBelowError: first, ErrorLogs: second, Errors: first}); got.Oldest[1] != want {
		t.Errorf("project 1: %+v, want %+v", got.Oldest[1], want)
	}

	if want := (store.OldestKept{LogsBelowError: first}); got.Oldest[2] != want {
		t.Errorf("project 2: %+v, want %+v", got.Oldest[2], want)
	}
}

// A project's first contact is written once, kept across a restart and
// deleted with the project; once known, marking it again writes nothing.
func TestFirstContact(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	first := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)

	if err := database.SeedProject(ctx, store.Project{ID: 1, Name: "backend", PublicKey: "0123456789abcdef"}, first); err != nil {
		t.Fatal(err)
	}

	if written, err := database.MarkContact(1, first); err != nil || !written {
		t.Fatalf("first mark: written %v, %v", written, err)
	}

	// Removed behind the store's back: a second mark must not write it again.
	if err := database.db.Update(true, func(batch *kv.Batch) error {
		return database.contacts.Delete(batch, projectContact{ProjectID: 1})
	}); err != nil {
		t.Fatal(err)
	}

	if written, err := database.MarkContact(1, first.Add(time.Hour)); err != nil || written {
		t.Fatalf("second mark: written %v, %v", written, err)
	}

	if stored, _ := database.contacts.Primary().All(database.db); len(stored) != 0 {
		t.Fatalf("second mark wrote %+v", stored)
	}

	// Forgotten in memory too, it is written again.
	database.contactsMutex.Lock()
	delete(database.contacted, 1)
	database.contactsMutex.Unlock()

	if _, err := database.MarkContact(1, first); err != nil {
		t.Fatal(err)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	if at, ok := reopened.FirstContact(1); !ok || !at.Equal(first) {
		t.Fatalf("after a restart: %v %v", at, ok)
	}

	if got := usage(t, reopened).FirstContact[1]; !got.Equal(first) {
		t.Fatalf("usage first contact %v", got)
	}

	if err := reopened.DeleteProject(ctx, 1); err != nil {
		t.Fatal(err)
	}

	if _, ok := reopened.FirstContact(1); ok {
		t.Fatal("first contact kept after the project's deletion")
	}

	if stored, _ := reopened.contacts.Primary().All(reopened.db); len(stored) != 0 {
		t.Fatalf("deletion kept %+v", stored)
	}
}
