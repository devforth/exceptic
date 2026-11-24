package database

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// dropIssueRecord deletes an issue's record alone, as no code of the store
// does, leaving its events and status changes behind.
func dropIssueRecord(t *testing.T, database *Store, issue *issueRecord) string {
	t.Helper()

	err := database.db.Update(true, func(batch *kv.Batch) error {
		_, err := database.issues.DeleteKey(batch, database.issues.KeyOf(*issue))

		return err
	})

	if err != nil {
		t.Fatal(err)
	}

	return issue.ID
}

func recordsOf(t *testing.T, database *Store, issueID string) (events, changes int) {
	t.Helper()

	events, _, err := database.events.Primary(kv.Text(issueID)).Count(database.db, 0)

	if err == nil {
		changes, _, err = database.status.Primary(kv.Text(issueID)).Count(database.db, 0)
	}

	if err != nil {
		t.Fatal(err)
	}

	return events, changes
}

func flipStatus(t *testing.T, database *Store, id string) {
	t.Helper()

	for _, status := range []string{store.StatusResolved, store.StatusUnresolved} {
		if _, err := database.SetIssuesStatus(context.Background(), []string{id}, status, store.Actor{Source: store.SourceUI}); err != nil {
			t.Fatal(err)
		}
	}
}

// projectEvents stores count events of one issue of a project.
func projectEvents(t *testing.T, database *Store, project int, fingerprint string, count int) {
	t.Helper()

	for index := range count {
		at := database.now().Add(time.Duration(index) * time.Second)

		_, err := database.SaveEvent(context.Background(), store.NewEvent{
			ProjectID: project, EventID: fmt.Sprintf("%d-%s-%d", project, fingerprint, index), Fingerprint: fingerprint, Title: fingerprint,
			Level: "error", Timestamp: at, ReceivedAt: at, Payload: []byte(`{"exception":{}}`),
		})

		if err != nil {
			t.Fatal(err)
		}
	}
}

// A size check under the limit leaves the records of missing issues alone;
// a run deletes them, over several batches, as its first step, and nothing else.
func TestCleanupDeletesRecordsOfMissingIssues(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	writeEvents(t, database, "lost", "error", 2*cleanupChunk+500)
	writeEvents(t, database, "small-lost", "error", 3)
	writeEvents(t, database, "kept", "error", 4)

	for _, fingerprint := range []string{"lost", "kept"} {
		flipStatus(t, database, storedIssue(database, fingerprint).ID)
	}

	kept := storedIssue(database, "kept").ID
	lost := []string{dropIssueRecord(t, database, storedIssue(database, "lost")), dropIssueRecord(t, database, storedIssue(database, "small-lost"))}

	if err := database.CheckSize(ctx, 1<<40, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}

	if events, changes := recordsOf(t, database, lost[0]); events != 2*cleanupChunk+500 || changes != 2 {
		t.Fatalf("a check under the limit deleted: %d events and %d status changes left", events, changes)
	}

	size, err := database.db.Size()

	if err != nil {
		t.Fatal(err)
	}

	runs := checkSize(t, database, size*9/10)

	if len(runs) != 1 || len(runs[0].Steps) == 0 {
		t.Fatalf("runs: %+v", runs)
	}

	first := runs[0].Steps[0]

	if first.Number != orphanStep || first.Projects[1].Events != 2*cleanupChunk+503 || first.Projects[1].StatusChanges != 2 {
		t.Errorf("first step %+v, counts %+v", first, first.Projects[1])
	}

	for _, id := range lost {
		if events, changes := recordsOf(t, database, id); events+changes > 0 {
			t.Errorf("missing issue %s keeps %d events and %d status changes", id, events, changes)
		}
	}

	if events, changes := recordsOf(t, database, kept); events != 4 || changes != 2 {
		t.Errorf("kept issue has %d events and %d status changes, want 4 and 2", events, changes)
	}
}

// A project's run deletes the records of the project's missing issues
// alone; the run over all projects deletes the rest.
func TestCleanupOrphansOfAProject(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	seedShares(t, database, 50, 100)
	projectEvents(t, database, 1, "lost", 5)
	projectEvents(t, database, 2, "lost", 7)
	projectEvents(t, database, 2, "kept", 3)
	flipStatus(t, database, projectIssue(database, 2, "lost").ID)

	ownLost := dropIssueRecord(t, database, projectIssue(database, 1, "lost"))
	otherLost := dropIssueRecord(t, database, projectIssue(database, 2, "lost"))
	otherKept := projectIssue(database, 2, "kept").ID

	run, err := database.Cleanup(ctx, 1, 1)

	if err != nil || run.Status != cleanupDone {
		t.Fatal(run, err)
	}

	onlyProject(t, run, 1)

	if events, _ := recordsOf(t, database, ownLost); events != 0 {
		t.Errorf("project 1's missing issue keeps %d events", events)
	}

	if events, changes := recordsOf(t, database, otherLost); events != 7 || changes != 2 {
		t.Errorf("project 2's missing issue keeps %d events and %d status changes, want 7 and 2", events, changes)
	}

	if events, _ := recordsOf(t, database, otherKept); events != 3 {
		t.Errorf("project 2's issue keeps %d of its 3 events", events)
	}

	if run, err = database.Cleanup(ctx, AllProjects, 1); err != nil || run.Status != cleanupDone {
		t.Fatal(run, err)
	}

	if events, changes := recordsOf(t, database, otherLost); events+changes != 0 {
		t.Errorf("after the run over all projects %d events and %d status changes are left", events, changes)
	}

	if run.Steps[0].Number != orphanStep || run.Steps[0].Projects[2].Events != 7 {
		t.Errorf("first step %+v", run.Steps[0])
	}
}

// A batch stopped in the middle of a missing issue's events goes on after
// the last one it deleted, and a batch stopped between issues at the next.
func TestCleanupOrphansGoOn(t *testing.T) {
	database := openTest(t)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	writeEvents(t, database, "lost", "error", 30)
	writeEvents(t, database, "other", "error", 2)
	id := dropIssueRecord(t, database, storedIssue(database, "lost"))

	position, finished := runBatch(t, database, orphanStep, nil, cleanupChunk-10)

	if events, _ := recordsOf(t, database, id); finished || events == 30 || events == 0 {
		t.Fatalf("finished %v, %d events left", finished, events)
	}

	for !finished {
		position, finished = runBatch(t, database, orphanStep, position, cleanupChunk-10)
	}

	if events, _ := recordsOf(t, database, id); events != 0 || storedEvents(database, "other") != 2 {
		t.Fatalf("%d events left, other keeps %d", events, storedEvents(database, "other"))
	}
}

// countingReader counts the iterator moves of the walks it serves.
type countingReader struct {
	kv.Reader

	moves *int
}

func (reader countingReader) Iterate(lower, upper []byte) kv.Iterator {
	return countingIterator{reader.Reader.Iterate(lower, upper), reader.moves}
}

type countingIterator struct {
	kv.Iterator

	moves *int
}

func (it countingIterator) First() bool { *it.moves++; return it.Iterator.First() }

func (it countingIterator) Next() bool { *it.moves++; return it.Iterator.Next() }

func (it countingIterator) Seek(key []byte) bool { *it.moves++; return it.Iterator.Seek(key) }

// The walk reads a key per issue, not per event: 1 issue of 3000 events
// costs what 1 issue of 1 event does.
func TestOrphanWalkReadsAKeyPerIssue(t *testing.T) {
	database := openTest(t)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	writeEvents(t, database, "busy", "error", 3000)
	writeEvents(t, database, "quiet", "error", 1)
	writeEvents(t, database, "lost", "error", 1000)
	lost := dropIssueRecord(t, database, storedIssue(database, "lost"))

	var found []string

	moves := 0
	err := eachOrphan(countingReader{database.db, &moves}, database, database.events, "", func(id string, gone bool) (bool, error) {
		if gone {
			found = append(found, id)
		}

		return true, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 || found[0] != lost {
		t.Errorf("found %v, want only %s", found, lost)
	}

	// The first key, then a seek past each of the 3 issues.
	if moves > 4 {
		t.Errorf("%d moves over 3 issues of 4001 events", moves)
	}

	if _, err := database.Cleanup(context.Background(), AllProjects, 1); err != nil {
		t.Fatal(err)
	}
}

// Issues created while the step runs are never taken for missing: each
// batch checks the issue table under the write lock their writes take.
func TestOrphanStepSparesNewIssues(t *testing.T) {
	database := openTest(t)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))

	done := make(chan struct{})

	go func() {
		defer close(done)

		for index := range 200 {
			writeEvents(t, database, fmt.Sprint("new-", index), "error", 2)
		}
	}()

	for running := true; running; {
		select {
		case <-done:
			running = false
		default:
		}

		var position []byte

		for finished := false; !finished; {
			position, finished = runBatch(t, database, orphanStep, position, cleanupChunk-3)
		}
	}

	for index := range 200 {
		if events := storedEvents(database, fmt.Sprint("new-", index)); events != 2 {
			t.Fatalf("issue new-%d keeps %d of its 2 events", index, events)
		}
	}
}
