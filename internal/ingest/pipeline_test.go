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

	projects := benchProjects(1)
	notifier := &recordingNotifier{}
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), &config.Config{}, projects, database, StoreSink{Logger: slog.New(slog.DiscardHandler), Notifier: notifier})
	handler := pipeline.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { defer close(done); pipeline.Run(ctx) }()

	event, logs := fixtures.Envelope(t, "node-event-unhandled"), fixtures.Envelope(t, "node-logs")
	target := "/api/1/envelope/?sentry_key=" + projects[0].PublicKey
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

		if err := pipeline.WaitStored(waitCtx); err != nil || pipeline.stored.Load() != count {
			t.Fatalf("stored %d of %d: %v", pipeline.stored.Load(), count, err)
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
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), &config.Config{}, projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
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
			report, err := db.Cleanup(cleanupCtx, database.AllProjects, 3<<20)

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
		sent    atomic.Int64
		failed  atomic.Int64
		retried atomic.Int64
		group   sync.WaitGroup
	)

	// A 429 while a compaction part holds ingest's writes is retried, as an
	// SDK does after its Retry-After (here at once: the part is short).
	send := func(project config.Project, body []byte) {
		for {
			request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/%d/envelope/?sentry_key=%s", project.ID, project.PublicKey), bytes.NewReader(body))
			request.SetPathValue("projectID", strconv.Itoa(project.ID))

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			switch {
			case response.Code == http.StatusOK:
				sent.Add(1)
			case response.Code == http.StatusTooManyRequests && response.Header().Get("Retry-After") != "":
				retried.Add(1)
				time.Sleep(10 * time.Millisecond)

				continue
			default:
				failed.Add(1)
			}

			return
		}
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

	// On a slow machine the burst can end before the data reaches the limit:
	// the cleanups go on until one has deleted something.
	for deadline := time.Now().Add(30 * time.Second); cleanups.Load() == 0 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}

	// Run back to back with a 3 MB limit they would hold the writer's batches
	// for most of the time.
	stopCleanup()
	<-cleanupDone

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 60*time.Second)

	defer cancelWait()

	if err := pipeline.WaitStored(waitCtx); err != nil || pipeline.stored.Load() != sent.Load() {
		t.Fatalf("stored %d of %d, inbox %d bytes: %v", pipeline.stored.Load(), sent.Load(), db.InboxBytes(), err)
	}

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

	t.Logf("%d envelopes (%d retried after a 429), %d cleanups that deleted something", sent.Load(), retried.Load(), cleanups.Load())
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
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), &config.Config{}, projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
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
			t.Fatalf("stored %d of %d, inbox %d bytes", pipeline.stored.Load(), sent.Load(), db.InboxBytes())
		}

		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if pipeline.stored.Load() != sent.Load() || db.InboxBytes() != 0 {
		t.Fatalf("sent %d, stored %d, inbox %d bytes", sent.Load(), pipeline.stored.Load(), db.InboxBytes())
	}
}

// Processors wait while parsed envelopes the writer has not stored fill ParseBytes.
func TestParsedEnvelopesFitParseBytes(t *testing.T) {
	db, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	projects := benchProjects(1)
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), &config.Config{}, projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
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

	projects := benchProjects(1)
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), &config.Config{}, projects, db, StoreSink{Logger: slog.New(slog.DiscardHandler)})
	envelope := func(body string, at time.Time) processed {
		return processed{
			Envelope: database.Envelope{Logs: []store.NewLog{{ProjectID: 1, Timestamp: at, Level: "info", Body: body, Attributes: []byte("{}")}}},
			project:  projects[0],
		}
	}
	now := time.Now().UTC()
	batch := []processed{envelope("before", now), envelope("broken", time.Date(20000, 1, 1, 0, 0, 0, 0, time.UTC)), envelope("after", now)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if _, stored := pipeline.storeBatch(ctx, batch); !stored {
		t.Fatal("the batch was never stored")
	}

	page, err := db.ListLogs(context.Background(), store.LogFilter{})
	logs := page.Logs

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

// panickingProjects panics in List while panicking is set.
type panickingProjects struct {
	projects  config.Projects
	panicking atomic.Bool
}

func (list *panickingProjects) List() config.Projects {
	if list.panicking.Load() {
		panic("damaged project list")
	}

	return list.projects
}

// panickingNotifier panics on every notification.
type panickingNotifier struct{}

func (panickingNotifier) Notify(notify.Notification) { panic("broken notifier") }

// lockedBuffer collects log output written from several goroutines.
type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (locked *lockedBuffer) Write(data []byte) (int, error) {
	locked.mutex.Lock()
	defer locked.mutex.Unlock()

	return locked.buffer.Write(data)
}

func (locked *lockedBuffer) String() string {
	locked.mutex.Lock()
	defer locked.mutex.Unlock()

	return locked.buffer.String()
}

// panicPipeline builds a pipeline of one project: accept posts an envelope,
// run starts storing, settle waits until the pipeline has stored what it took.
func panicPipeline(t *testing.T, projects config.ProjectList, sink StoreSink) (pipeline *Pipeline, accept func(body []byte), run, settle func()) {
	t.Helper()

	db, err := database.Open(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	pipeline = NewPipeline(sink.Logger, &config.Config{}, projects, db, sink)

	accept = func(body []byte) {
		t.Helper()

		request := httptest.NewRequest(http.MethodPost, "/api/1/envelope/?sentry_key="+projects.List()[0].PublicKey, bytes.NewReader(body))
		request.SetPathValue("projectID", "1")

		response := httptest.NewRecorder()
		pipeline.Handler().ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("%d %s", response.Code, response.Body)
		}
	}

	run = func() {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		go func() { defer close(done); pipeline.Run(ctx) }()

		t.Cleanup(func() { cancel(); <-done })
	}

	settle = func() {
		t.Helper()

		waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)

		defer cancelWait()

		if err := pipeline.WaitStored(waitCtx); err != nil {
			t.Fatalf("the pipeline did not settle: %v", err)
		}
	}

	return pipeline, accept, run, settle
}

// An envelope whose processing panics would panic again at every scan and
// every start: it is logged with the stack and dropped, and the pipeline goes on.
func TestPanickingEnvelopeIsDropped(t *testing.T) {
	var logs lockedBuffer

	projects := &panickingProjects{projects: benchProjects(1)}
	sink := StoreSink{Logger: slog.New(slog.NewTextHandler(&logs, nil))}
	pipeline, accept, run, settle := panicPipeline(t, projects, sink)
	event := fixtures.Envelope(t, "node-event-unhandled")

	// Accepted with a good list, processed with a damaged one.
	accept(varied(event, 0))
	projects.panicking.Store(true)
	run()
	settle()
	projects.panicking.Store(false)
	accept(varied(event, 1))
	settle()

	if stored := pipeline.stored.Load(); stored != 2 {
		t.Errorf("stored %d envelopes, want the dropped one and the next", stored)
	}

	if output := logs.String(); !strings.Contains(output, "damaged project list") || !strings.Contains(output, "pipeline_test.go") {
		t.Errorf("the panic was not logged with its stack:\n%s", output)
	}
}

// A panic while reporting a stored event loses only that report: the
// writer goes on with the next batches.
func TestPanickingReportKeepsTheWriter(t *testing.T) {
	var logs lockedBuffer

	sink := StoreSink{Logger: slog.New(slog.NewTextHandler(&logs, nil)), Notifier: panickingNotifier{}}
	pipeline, accept, run, settle := panicPipeline(t, benchProjects(1), sink)
	event := fixtures.Envelope(t, "node-event-unhandled")

	run()
	accept(varied(event, 0))
	settle()
	accept(varied(event, 1))
	settle()

	if stored := pipeline.stored.Load(); stored != 2 {
		t.Errorf("stored %d envelopes, want 2", stored)
	}

	if output := logs.String(); !strings.Contains(output, "broken notifier") || !strings.Contains(output, "pipeline_test.go") {
		t.Errorf("the panic was not logged with its stack:\n%s", output)
	}
}
