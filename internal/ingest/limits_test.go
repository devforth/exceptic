package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/sentry"
)

// The SDK's names: events pause as error (Sentry adds default and security), logs as log_item.
const (
	eventsLimited = "46:error;default;security:project"
	logsLimited   = "46:log_item:project"
)

// eventEnvelope is an event with a new id each time (stored events are unique by id).
func eventEnvelope() []byte {
	return []byte(fmt.Sprintf("{\"event_id\":\"%032x\"}\n{\"type\":\"event\"}\n{\"message\":\"hello\"}\n", eventSequence.Add(1)))
}

var eventSequence atomic.Int64

// logEnvelope is a log item of count logs with distinct bodies, and an event when withEvent.
func logEnvelope(count int, withEvent bool) []byte {
	bodies := make([]string, count)

	for index := range bodies {
		bodies[index] = fmt.Sprintf(`{"timestamp":1763884800,"level":"info","body":"log %c"}`, 'a'+index)
	}

	envelope := fmt.Sprintf("{\"event_id\":\"%032x\"}\n", eventSequence.Add(1))

	if withEvent {
		payload := `{"message":"hello"}`
		envelope += fmt.Sprintf("{\"type\":\"event\",\"length\":%d}\n%s\n", len(payload), payload)
	}

	envelope += fmt.Sprintf("{\"type\":\"log\",\"item_count\":%d,\"content_type\":\"application/vnd.sentry.items.log+json\"}\n{\"items\":[%s]}\n", count, strings.Join(bodies, ","))

	return []byte(envelope)
}

func limitedServer(t *testing.T, events, logs int) (*testServer, *time.Time) {
	t.Helper()

	settings := testSettings()
	settings.RateLimitEventsPerMinute = events
	settings.RateLimitLogsPerMinute = logs
	server := newTestServer(t, settings)
	now := time.Date(2026, 11, 11, 10, 0, 15, 0, time.UTC)
	server.handler.now = func() time.Time { return now }

	return server, &now
}

func (server *testServer) post(projectID, key string, body []byte) *httptest.ResponseRecorder {
	return server.send(newRequest(http.MethodPost, envelopeURL(projectID, key), body))
}

func expectAnswer(t *testing.T, response *httptest.ResponseRecorder, status int, rateLimits, retryAfter string) {
	t.Helper()

	if response.Code != status || response.Header().Get("X-Sentry-Rate-Limits") != rateLimits || response.Header().Get("Retry-After") != retryAfter {
		t.Fatalf("got %d, X-Sentry-Rate-Limits %q, Retry-After %q (%s); want %d, %q, %q",
			response.Code, response.Header().Get("X-Sentry-Rate-Limits"), response.Header().Get("Retry-After"), response.Body, status, rateLimits, retryAfter)
	}
}

// A log flood pauses only logs: errors keep coming.
func TestRateLimitPerCategory(t *testing.T) {
	server, now := limitedServer(t, 2, 3)

	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(2, false)), http.StatusOK, "", "")
	// Two more logs do not fit: the item is dropped whole, and the SDK is told to pause logs only.
	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(2, false)), http.StatusTooManyRequests, logsLimited, "46")
	expectAnswer(t, server.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusOK, "", "")
	// Once limited, logs stay paused until the minute ends, even a batch that would fit.
	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(1, false)), http.StatusTooManyRequests, logsLimited, "46")
	expectAnswer(t, server.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusOK, "", "")
	expectAnswer(t, server.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusTooManyRequests, eventsLimited, "46")

	// The limits are per project, and a wrong key uses up nothing.
	expectAnswer(t, server.post("2", fixtures.BrowserKey, logEnvelope(3, false)), http.StatusOK, "", "")

	if response := server.post("1", fixtures.BrowserKey, logEnvelope(1, false)); response.Code != http.StatusForbidden {
		t.Fatalf("wrong key: got %d, want 403", response.Code)
	}

	if events, logs := server.stored(); len(events) != 2 || len(logs) != 5 {
		t.Fatalf("stored %d events, %d logs; want 2, 5", len(events), len(logs))
	}

	*now = now.Add(time.Minute)

	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(3, false)), http.StatusOK, "", "")
	expectAnswer(t, server.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusOK, "", "")
}

// An envelope only partly limited is stored without the limited items and
// answered 200, so no SDK counts the stored ones as lost; the header still
// pauses the limited category.
func TestRateLimitPartialEnvelope(t *testing.T) {
	server, _ := limitedServer(t, 1, 1)

	response := server.post("1", fixtures.BackendKey, logEnvelope(2, true))
	expectAnswer(t, response, http.StatusOK, logsLimited, "")

	if !strings.Contains(response.Body.String(), `"id":"`) {
		t.Fatalf("body %s", response.Body)
	}

	if events, logs := server.stored(); len(events) != 1 || len(logs) != 0 {
		t.Fatalf("stored %d events, %d logs; want 1, 0", len(events), len(logs))
	}

	// Events used up, logs limited: nothing left to store.
	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(1, true)), http.StatusTooManyRequests, "46:error;default;security;log_item:project", "46")

	other, _ := limitedServer(t, 1, 1)

	expectAnswer(t, other.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusOK, "", "")
	expectAnswer(t, other.post("1", fixtures.BackendKey, logEnvelope(1, true)), http.StatusOK, eventsLimited, "")

	if events, logs := other.stored(); len(events) != 1 || len(logs) != 1 {
		t.Fatalf("stored %d events, %d logs; want 1, 1", len(events), len(logs))
	}
}

func TestRateLimitOff(t *testing.T) {
	server, _ := limitedServer(t, 0, 0)

	for range 5 {
		expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(20, true)), http.StatusOK, "", "")
	}

	if events, logs := server.stored(); len(events) != 5 || len(logs) != 100 {
		t.Fatalf("stored %d events, %d logs", len(events), len(logs))
	}

	// One category off, the other limited.
	logsOnly, _ := limitedServer(t, 0, 1)

	expectAnswer(t, logsOnly.post("1", fixtures.BackendKey, logEnvelope(1, true)), http.StatusOK, "", "")
	expectAnswer(t, logsOnly.post("1", fixtures.BackendKey, logEnvelope(1, true)), http.StatusOK, logsLimited, "")
}

// Envelopes without events or logs (spans with tracing on, one per trace)
// neither count nor get a 429.
func TestRateLimitCountsOnlyStoredItems(t *testing.T) {
	server, _ := limitedServer(t, 1, 1)

	for range 3 {
		if response := server.send(fixtures.Load(t, "node-spans").HTTP()); response.Code != http.StatusOK || response.Header().Get("X-Sentry-Rate-Limits") != "" {
			t.Fatalf("spans: got %d %s", response.Code, response.Body)
		}
	}

	expectAnswer(t, server.post("1", fixtures.BackendKey, eventEnvelope()), http.StatusOK, "", "")

	if events, logs := server.stored(); len(events) != 1 || len(logs) != 0 {
		t.Fatalf("stored %d events, %d logs", len(events), len(logs))
	}
}

// Items the inbox refused (server busy) are not counted.
func TestRateLimitCountsOnlyAccepted(t *testing.T) {
	server, _ := limitedServer(t, 1, 2)
	server.pipeline.MaxInboxBytes = -1

	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(2, true)), http.StatusTooManyRequests, "10::project", "10")

	server.pipeline.MaxInboxBytes = 1 << 30

	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(2, true)), http.StatusOK, "", "")
}

type warnCounter struct {
	slog.Handler
	warnings atomic.Int32
}

func (counter *warnCounter) Handle(_ context.Context, record slog.Record) error {
	if record.Level == slog.LevelWarn {
		counter.warnings.Add(1)
	}

	return nil
}

func (counter *warnCounter) Enabled(context.Context, slog.Level) bool { return true }

// Ingest is public: a limited project logs one warning per category a minute, not one per envelope.
func TestRateLimitWarnsOncePerMinute(t *testing.T) {
	server, now := limitedServer(t, 1, 1)
	counter := &warnCounter{Handler: slog.DiscardHandler}
	server.handler.logger = slog.New(counter)

	for range 5 {
		server.post("1", fixtures.BackendKey, logEnvelope(1, false))
	}

	if warnings := counter.warnings.Load(); warnings != 1 {
		t.Fatalf("%d warnings, want 1", warnings)
	}

	for range 3 {
		server.post("1", fixtures.BackendKey, eventEnvelope())
	}

	if warnings := counter.warnings.Load(); warnings != 2 {
		t.Fatalf("%d warnings after events, want 2", warnings)
	}

	*now = now.Add(time.Minute)

	for range 3 {
		server.post("1", fixtures.BackendKey, logEnvelope(1, false))
	}

	if warnings := counter.warnings.Load(); warnings != 3 {
		t.Fatalf("%d warnings in the next minute, want 3", warnings)
	}
}

func TestRateLimiterUnderConcurrency(t *testing.T) {
	limiter := newRateLimiter(0, 20)
	now := time.Date(2026, 11, 11, 10, 0, 15, 0, time.UTC)

	var (
		granted atomic.Int32
		workers sync.WaitGroup
	)

	for range 50 {
		workers.Go(func() {
			if decision := limiter.take(1, itemCounts{categoryLogs: 1}, now); !decision.limited[categoryLogs] {
				granted.Add(1)
			}
		})
	}

	workers.Wait()

	if granted.Load() != 20 {
		t.Fatalf("granted %d, want 20", granted.Load())
	}
}

func TestCountItems(t *testing.T) {
	tests := []struct {
		name   string
		body   []byte
		events int
		logs   int
	}{
		{"event", []byte(simpleEnvelope), 1, 0},
		{"logs and event", logEnvelope(3, true), 1, 3},
		// item_count is the SDK's word; the logs are counted.
		{"item_count lies", []byte("{}\n{\"type\":\"log\",\"item_count\":1}\n{\"items\":[{},{},{}],\"other\":[1,2]}\n"), 0, 3},
		// A log item without logs still counts once, so empty items cannot flood the inbox.
		{"no logs", []byte("{}\n{\"type\":\"log\"}\n{\"items\":[]}\n"), 0, 1},
		{"malformed", []byte("{}\n{\"type\":\"log\"}\n{\"items\":[1,\n"), 0, 1},
		{"spans", []byte("{}\n{\"type\":\"span\"}\n{}\n"), 0, 0},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			envelope := parseTestEnvelope(t, testCase.body)
			counts := countItems(envelope)

			if counts[categoryEvents] != testCase.events || counts[categoryLogs] != testCase.logs {
				t.Fatalf("counted %v, want %d events, %d logs", counts, testCase.events, testCase.logs)
			}
		})
	}
}

func parseTestEnvelope(t *testing.T, body []byte) *sentry.Envelope {
	t.Helper()

	envelope, err := sentry.ParseEnvelope(body)

	if err != nil {
		t.Fatal(err)
	}

	return envelope
}

// A request that read the time before the minute ended must not move the
// project's window back: the next minute's counts and closed kinds stay.
func TestRateLimiterWindowNeverGoesBack(t *testing.T) {
	limiter := newRateLimiter(0, 10)
	late := time.Date(2026, 11, 11, 10, 0, 59, 990_000_000, time.UTC)
	next := time.Date(2026, 11, 11, 10, 1, 0, 10_000_000, time.UTC)

	early := limiter.take(1, itemCounts{categoryLogs: 4}, late)
	limiter.take(1, itemCounts{categoryLogs: 11}, next)

	if decision := limiter.take(1, itemCounts{categoryLogs: 1}, late); !decision.limited[categoryLogs] {
		t.Fatal("a late request reopened the closed kind of the next minute")
	}

	limiter.refund(1, early.counted(itemCounts{categoryLogs: 4}), late)

	if decision := limiter.take(1, itemCounts{categoryLogs: 1}, next.Add(time.Second)); !decision.limited[categoryLogs] {
		t.Fatal("the next minute's window was replaced")
	}

	limiter = newRateLimiter(0, 10)
	limiter.take(1, itemCounts{categoryLogs: 4}, late)
	limiter.take(1, itemCounts{categoryLogs: 6}, next)
	limiter.refund(1, itemCounts{categoryLogs: 4}, late)

	if decision := limiter.take(1, itemCounts{categoryLogs: 5}, next); !decision.limited[categoryLogs] {
		t.Fatal("a refund of the previous minute was taken off the next one")
	}
}
