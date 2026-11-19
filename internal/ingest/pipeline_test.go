package ingest

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

// varied returns envelope with a new event id and, for issue >= 0, an error
// type that makes it that issue (the type is part of the grouping).
func varied(envelope []byte, issue int) []byte {
	header := string(envelope[:bytes.IndexByte(envelope, '\n')])
	start := strings.Index(header, `"event_id":"`)
	id := make([]byte, 16)
	_, _ = rand.Read(id)

	if start >= 0 {
		envelope = bytes.ReplaceAll(envelope, []byte(header[start+12:start+44]), []byte(hex.EncodeToString(id)))
	}

	if issue >= 0 {
		envelope = bytes.Replace(envelope, []byte(`"type":"Error"`), fmt.Appendf(nil, `"type":"LoadTest%dError"`, issue), 1)
	}

	return envelope
}

func benchProjects(count int) config.Projects {
	var projects config.Projects

	for id := 1; id <= count; id++ {
		projects = append(projects, config.Project{ID: id, Name: fmt.Sprintf("project-%d", id), PublicKey: fmt.Sprintf("key%029d", id)})
	}

	return projects
}

func TestPipelineStoresEnvelopes(t *testing.T) {
	database, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = database.Close() })

	settings := &config.Config{Projects: benchProjects(1), AllowedOrigins: []string{"*"}}
	notifier := &recordingNotifier{}
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), settings, settings.Projects, database, StoreSink{Logger: slog.New(slog.DiscardHandler), Notifier: notifier})
	handler := pipeline.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { defer close(done); pipeline.Run(ctx) }()

	event, logs := fixtures.Envelope(t, "node-event-unhandled"), fixtures.Envelope(t, "node-logs")
	target := "/api/1/envelope/?sentry_key=" + settings.Projects[0].PublicKey
	send := func(body []byte) {
		request := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
		request.SetPathValue("projectID", "1")

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("%d %s", response.Code, response.Body)
		}
	}

	waitStored := func(count int64) {
		t.Helper()

		waitCtx, cancelWait := context.WithTimeout(context.Background(), 10*time.Second)

		defer cancelWait()

		if err := pipeline.WaitStored(waitCtx); err != nil || pipeline.Stored() != count {
			t.Fatalf("stored %d of %d: %v", pipeline.Stored(), count, err)
		}
	}

	// A retry of the SDK is recognized while its event is stored.
	duplicate := varied(event, 0)
	send(duplicate)
	waitStored(1)
	send(duplicate)
	waitStored(2)

	for range 5 {
		send(varied(event, 0))
	}

	send(varied(event, 1))
	send(logs)
	send(logs)
	send(logs)

	waitStored(11)

	// A resolved issue that happens again is notified as a regression.
	issues, err := database.ListIssues(context.Background(), store.IssueFilter{Sort: "event_count", Limit: 1})

	if err != nil || len(issues) != 1 {
		t.Fatal(issues, err)
	}

	if _, err := database.SetIssuesStatus(context.Background(), []string{issues[0].ID}, store.StatusResolved, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	send(varied(event, 0))
	waitStored(12)
	cancel()
	<-done

	kinds := map[notify.Kind]int{}

	for _, notification := range notifier.notifications {
		kinds[notification.Kind]++
	}

	if len(notifier.notifications) != 3 || kinds[notify.NewIssue] != 2 || kinds[notify.Regression] != 1 {
		t.Errorf("notifications: %v", kinds)
	}

	counts := map[int]int{}
	issues, err = database.ListIssues(context.Background(), store.IssueFilter{})

	if err != nil {
		t.Fatal(err)
	}

	for _, issue := range issues {
		counts[issue.EventCount]++
	}

	// 7 distinct events of one issue (a retry is not counted), 1 of another.
	if len(counts) != 2 || counts[7] != 1 || counts[1] != 1 {
		t.Errorf("event counts: %v", counts)
	}

	if database.InboxBytes() != 0 {
		t.Errorf("inbox holds %d bytes", database.InboxBytes())
	}
}

// A burst through the pipeline while the cleanup deletes and compacts under
// a small limit: every accepted envelope is stored, the inbox empties, and
// the stored data stays consistent (indexes, byte counters, issue counts).
func TestBurstWithCleanup(t *testing.T) {
	db, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	projects := benchProjects(10)
	settings := &config.Config{Projects: projects, AllowedOrigins: []string{"*"}}
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), settings, projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
	handler := pipeline.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { defer close(done); pipeline.Run(ctx) }()

	cleanupCtx, stopCleanup := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{})

	var (
		cleanups   atomic.Int64
		cleanupErr error
	)

	go func() {
		defer close(cleanupDone)

		for cleanupCtx.Err() == nil {
			report, err := db.Cleanup(cleanupCtx, 3<<20)

			if err != nil && cleanupCtx.Err() == nil {
				cleanupErr = err

				return
			}

			if len(report.Steps) > 0 {
				cleanups.Add(1)
			}

			time.Sleep(20 * time.Millisecond)
		}
	}()

	event, logs := fixtures.Envelope(t, "node-event-unhandled"), fixtures.Envelope(t, "node-logs")

	var (
		sent   atomic.Int64
		failed atomic.Int64
		group  sync.WaitGroup
	)

	send := func(project config.Project, body []byte) {
		request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/%d/envelope/?sentry_key=%s", project.ID, project.PublicKey), bytes.NewReader(body))
		request.SetPathValue("projectID", strconv.Itoa(project.ID))

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			failed.Add(1)

			return
		}

		sent.Add(1)
	}

	for _, project := range projects {
		group.Add(1)

		go func() {
			defer group.Done()

			for index := range 200 {
				body := varied(event, index%25)
				send(project, body)

				if index%10 == 0 {
					send(project, body) // a retry of the SDK
				}

				if index%5 == 0 {
					send(project, logs)
				}
			}
		}()
	}

	group.Wait()

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 60*time.Second)

	defer cancelWait()

	if err := pipeline.WaitStored(waitCtx); err != nil || pipeline.Stored() != sent.Load() {
		t.Fatalf("stored %d of %d, inbox %d bytes: %v", pipeline.Stored(), sent.Load(), db.InboxBytes(), err)
	}

	stopCleanup()
	<-cleanupDone
	cancel()
	<-done

	if failed.Load() > 0 || cleanupErr != nil || cleanups.Load() == 0 {
		t.Fatalf("%d answers not 200, cleanup error %v, %d cleanups", failed.Load(), cleanupErr, cleanups.Load())
	}

	if pending, err := db.Pending(0, 10, nil); err != nil || len(pending) > 0 {
		t.Fatalf("inbox not empty: %d entries, %v", len(pending), err)
	}

	if problems, err := db.Verify(); err != nil || len(problems) > 0 {
		t.Fatalf("verify: %v %v", problems, err)
	}

	t.Logf("%d envelopes, %d cleanups that deleted something", sent.Load(), cleanups.Load())
}

// Envelopes still in the fresh queue when a scan of the inbox stores them are
// not stored a second time, and the inbox size stays exact.
func TestScanDoesNotStoreTwice(t *testing.T) {
	db, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	projects := benchProjects(1)
	settings := &config.Config{Projects: projects, AllowedOrigins: []string{"*"}}
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), settings, projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
	handler := pipeline.Handler()
	pipeline.LogsPerMessage = 0
	pipeline.Rescan = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { defer close(done); pipeline.Run(ctx) }()

	logs := fixtures.Envelope(t, "node-logs")
	target := fmt.Sprintf("/api/%d/envelope/?sentry_key=%s", projects[0].ID, projects[0].PublicKey)

	var (
		sent  atomic.Int64
		group sync.WaitGroup
	)

	for range 16 {
		group.Add(1)

		go func() {
			defer group.Done()

			for range 60 {
				request := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(logs))
				request.SetPathValue("projectID", strconv.Itoa(projects[0].ID))

				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)

				if response.Code == http.StatusOK {
					sent.Add(1)
				}
			}
		}()
	}

	group.Wait()

	for deadline := time.Now().Add(60 * time.Second); db.InboxBytes() > 0; {
		if time.Now().After(deadline) {
			t.Fatalf("stored %d of %d, inbox %d bytes", pipeline.Stored(), sent.Load(), db.InboxBytes())
		}

		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if pipeline.Stored() != sent.Load() || db.InboxBytes() != 0 {
		t.Fatalf("sent %d, stored %d, inbox %d bytes", sent.Load(), pipeline.Stored(), db.InboxBytes())
	}
}

// Processors wait while parsed envelopes the writer has not stored fill ParseBytes.
func TestParsedEnvelopesFitParseBytes(t *testing.T) {
	db, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	settings := &config.Config{Projects: benchProjects(1)}
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), settings, settings.Projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
	entries := make(chan database.InboxEntry, 3)
	results := make(chan processed, 3)

	for range 3 {
		entry, err := db.Append(1, time.Now(), fixtures.Envelope(t, "node-logs"))

		if err != nil {
			t.Fatal(err)
		}

		entries <- entry

		pipeline.ParseBytes = int64(entry.Size)
	}

	close(entries)

	budget := semaphore.NewWeighted(pipeline.ParseBytes)

	go pipeline.processEntries(entries, results, budget)

	waitResults := func(count int) {
		t.Helper()

		for deadline := time.Now().Add(5 * time.Second); len(results) < count; {
			if time.Now().After(deadline) {
				t.Fatalf("%d results of %d", len(results), count)
			}

			time.Sleep(time.Millisecond)
		}
	}

	waitResults(1)
	time.Sleep(50 * time.Millisecond)

	if len(results) != 1 {
		t.Fatalf("%d envelopes parsed with room for one", len(results))
	}

	budget.Release(pipeline.ParseBytes)
	waitResults(2)
}

// A record JSON cannot encode (a year past 9999) fails every retry: it is
// dropped with its envelope instead of stopping the writer, and the rest of
// the batch is stored.
func TestUnencodableEnvelopeIsDropped(t *testing.T) {
	db, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	settings := &config.Config{Projects: benchProjects(1)}
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), settings, settings.Projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
	envelope := func(body string, at time.Time) processed {
		return processed{
			Envelope: database.Envelope{Logs: []store.NewLog{{ProjectID: 1, Timestamp: at, Level: "info", Body: body, Attributes: []byte("{}")}}},
			project:  settings.Projects[0],
		}
	}
	now := time.Now().UTC()
	batch := []processed{envelope("before", now), envelope("broken", time.Date(20000, 1, 1, 0, 0, 0, 0, time.UTC)), envelope("after", now)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if _, stored := pipeline.storeBatch(ctx, batch); !stored {
		t.Fatal("the batch was never stored")
	}

	logs, err := db.ListLogs(context.Background(), store.LogFilter{})

	if err != nil {
		t.Fatal(err)
	}

	bodies := make([]string, 0, len(logs))

	for _, record := range logs {
		bodies = append(bodies, record.Body)
	}

	if strings.Join(bodies, ",") != "after,before" {
		t.Errorf("stored logs: %v", bodies)
	}
}
