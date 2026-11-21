package database

import (
	"bytes"
	"context"
	"maps"
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

// Data stored before the project counters is counted once in the
// background: until then the page says so, and what arrives meanwhile is
// counted right; afterwards the counts and the events' project index are
// what a database written with them holds. A new database needs no counting.
func TestBackfillProjectSizes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	if usage(t, database).Counting {
		t.Fatal("a new database counts")
	}

	writeEvents(t, database, "crash", "error", 5)
	writeLogs(t, database, "info", 6, "line")

	want := usage(t, database).Bytes

	// As written before: no group counters, no project index of events, no backfill done.
	err = database.db.Update(true, func(batch *kv.Batch) error {
		for _, prefix := range []string{"group/", "event.project/", "meta/"} {
			iterator := database.db.Iterate([]byte(prefix), append([]byte(prefix), 0xff))

			for valid := iterator.First(); valid; valid = iterator.Next() {
				if prefix != "meta/" || bytes.Contains(iterator.Key(), []byte("backfill")) {
					batch.Delete(bytes.Clone(iterator.Key()))
				}
			}

			if err := iterator.Close(); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database = reopenWithoutProjectIndex(t, dir)

	if got := usage(t, database); !got.Counting || len(got.Bytes) != 0 {
		t.Fatalf("before counting: %+v", got)
	}

	checkCollections(t, database)

	writeEvents(t, database, "late", "error", 2)
	writeLogs(t, database, "warn", 2, "late")
	checkCollections(t, database)

	if err := database.Backfill(ctx); err != nil {
		t.Fatal(err)
	}

	checkCollections(t, database)

	got := usage(t, database)

	if got.Counting || got.Bytes[1] <= want[1] || got.Bytes[2] <= want[2] || sum(got.Bytes) != database.logicalSize() {
		t.Errorf("after counting %+v (before the late data %v), the counters hold %d", got, want, database.logicalSize())
	}

	if events, _, err := database.events.By("project", kv.U32(1)).Count(database.db, 0); err != nil || events != 7 {
		t.Errorf("events by project: %d %v", events, err)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	if again := usage(t, reopened); again.Counting || !maps.Equal(again.Bytes, got.Bytes) {
		t.Errorf("after a restart: %+v, want %v", again, got.Bytes)
	}
}

// reopenWithoutProjectIndex opens dir, whose event counters still count the
// project index keys just deleted, and takes those bytes back, as a database
// of the version before the index holds them.
func reopenWithoutProjectIndex(t *testing.T, dir string) *Store {
	t.Helper()

	database, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = database.Close() })

	err = database.db.Update(true, func(batch *kv.Batch) error {
		return database.events.Primary().Head().Each(database.db, func(entry kv.Entry[eventRecord]) (bool, error) {
			indexKey := kv.K(kv.U32(uint32(entry.Record.ProjectID)), kv.U8(levelRank(entry.Record.Level)), kv.U64(entry.Record.Sequence))
			size := len("event.project/") + len(indexKey) + len("event/") + len(entry.Key)
			batch.Count(sizeCounter(kindEvent, levelRank(entry.Record.Level), dayOf(entry.Record.ReceivedAt)), -int64(size))

			return true, nil
		})
	})

	if err != nil {
		t.Fatal(err)
	}

	return database
}
