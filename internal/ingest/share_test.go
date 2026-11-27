package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

// queueServer is the handler with a pipeline that does not run, so what it
// accepts stays in the inbox.
func queueServer(t *testing.T, projects config.Projects) *testServer {
	t.Helper()

	db := storetest.Open(t)
	storetest.AddProjects(t, db, projects)
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), testSettings(), projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
	pipeline.MinInboxShare = 0
	router := http.NewServeMux()
	router.Handle("/api/{projectID}/envelope/{$}", pipeline.Handler())

	return &testServer{t: t, handler: pipeline.Handler(), pipeline: pipeline, db: db, router: router}
}

// A project flooding the queue is refused once it holds its share, while
// the other connected project is still accepted.
func TestFloodHoldsOnlyItsShare(t *testing.T) {
	server := queueServer(t, fixtures.Projects())
	entry := int64(len(eventEnvelope()) + 20) // the body and the inbox header
	server.pipeline.MaxInboxBytes = 10 * entry

	expectAnswer(t, server.post("2", fixtures.BrowserKey, eventEnvelope()), http.StatusOK, "", "")

	accepted := 0

	for ; accepted < 20; accepted++ {
		if response := server.post("1", fixtures.BackendKey, eventEnvelope()); response.Code != http.StatusOK {
			expectAnswer(t, response, http.StatusTooManyRequests, pauseEverything(inboxFullRetry), inboxFullRetry)

			break
		}
	}

	if held := server.db.InboxBytesOf(1); accepted != 5 || held != 5*entry {
		t.Errorf("the flooding project queued %d envelopes, %d bytes; want 5, %d", accepted, held, 5*entry)
	}

	expectAnswer(t, server.post("2", fixtures.BrowserKey, eventEnvelope()), http.StatusOK, "", "")

	want := []store.Dropped{{Reason: store.DropQueueShare, Kind: "events", Count: 1}}

	if got := server.handler.dropped.Dropped(1); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dropped %v, want %v", got, want)
	}
}

// The queue is split among the active projects by their disk shares.
func TestInboxShares(t *testing.T) {
	projects := config.Projects{{ID: 1, Name: "one", PublicKey: "key1", DiskShare: 100}, {ID: 2, Name: "two", PublicKey: "key2", DiskShare: 50}, {ID: 3, Name: "three", PublicKey: "key3", Paused: true}, {ID: 4, Name: "four", PublicKey: "key4"}}
	server := queueServer(t, projects)
	pipeline := server.pipeline
	pipeline.MaxInboxBytes = 3000
	now := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

	expect := func(project config.Project, want int64) {
		t.Helper()

		if got := pipeline.inboxShare(project); got != want {
			t.Errorf("project %d: share %d, want %d", project.ID, got, want)
		}
	}

	// Alone (a paused project does not count), a project has the whole queue.
	_, _ = server.db.MarkContact(1, "key1", now)
	_, _ = server.db.MarkContact(3, "key3", now)

	expect(projects[0], 3000)

	// Connected, 50% weighs half as much as 100%.
	_, _ = server.db.MarkContact(2, "key2", now)

	expect(projects[0], 2000)
	expect(projects[1], 1000)

	// Data counts without a connection (a DSN reset), once read again.
	if err := server.db.SaveLogs(context.Background(), []store.NewLog{{ProjectID: 4, Timestamp: now, Level: "info", Body: "hello", Attributes: []byte("{}")}}, 0); err != nil {
		t.Fatal(err)
	}

	expect(projects[0], 2000)
	pipeline.readWithData(context.Background())
	expect(projects[0], 1200)
	expect(projects[1], 600)

	// A small share still takes one largest envelope.
	pipeline.MinInboxShare = 1000

	expect(projects[1], 1000)

	// Among many, a small share keeps config.MinInboxSharePercent of the queue:
	// 5% of 3000, more than its 5/255 of it.
	pipeline.MinInboxShare = 0

	expect(config.Project{ID: 5, Name: "five", PublicKey: "key5", DiskShare: 5}, 3000*config.MinInboxSharePercent/100)
}

// Every byte a project queued is freed once the pipeline stored its envelopes.
func TestQueuedBytesReturnToZero(t *testing.T) {
	server := newTestServer(t, testSettings())

	for range 5 {
		expectAnswer(t, server.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusOK, "", "")
		expectAnswer(t, server.post("2", fixtures.BrowserKey, logEnvelope(2, false)), http.StatusOK, "", "")
	}

	server.waitStored()

	for _, project := range []int{0, 1, 2} {
		if held := server.db.InboxBytesOf(project); held != 0 {
			t.Errorf("project %d still holds %d bytes", project, held)
		}
	}
}
