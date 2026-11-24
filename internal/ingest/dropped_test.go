package ingest

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// What the limits refuse and what the SDK reports dropping are counted per
// project, by reason and kind; refusals before the key is checked are not.
func TestDroppedIsCountedPerProject(t *testing.T) {
	server, now := limitedServer(t, 0, 3)

	expectAnswer(t, server.post("1", fixtures.BackendKey, logEnvelope(5, false)), http.StatusTooManyRequests, logsLimited, "46")
	server.post("1", fixtures.BackendKey, logEnvelope(2, true)) // the event is stored, the 2 logs dropped
	server.post("1", fixtures.BackendKey, []byte("not an envelope"))
	server.post("1", "wrong-key", logEnvelope(4, false))

	report := `{"discarded_events":[{"reason":"ratelimit_backoff","category":"log_item","quantity":40},` +
		`{"reason":"sample_rate","category":"error","quantity":3},{"reason":"queue_overflow","category":"error","quantity":2},` +
		`{"reason":"network_error","category":"span","quantity":9},{"reason":"network_error","category":"error","quantity":4}]}`
	server.post("1", fixtures.BackendKey, fmt.Appendf(nil, "{}\n{\"type\":\"client_report\"}\n%s\n", report))

	want := []store.Dropped{
		{Reason: store.DropSDKRateLimit, Kind: "logs", Count: 40},
		{Reason: store.DropRateLimit, Kind: "logs", Count: 7},
		{Reason: store.DropSDKOverflow, Kind: "events", Count: 2},
		{Reason: store.DropInvalid, Kind: "envelopes", Count: 1},
	}

	if got := server.handler.dropped.counts(1, *now); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dropped %v, want %v", got, want)
	}

	if got := server.handler.dropped.counts(2, *now); got != nil {
		t.Errorf("project 2 dropped %v", got)
	}
}

// A refusal is counted once: the SDK reports it again as a failed send
// (sentry-python's network_error for any answer but 2xx and 429), and the
// server cannot tell those from real network failures, so they count not.
func TestDroppedRefusalCountsOnce(t *testing.T) {
	server, now := limitedServer(t, 0, 0)

	server.post("1", fixtures.BackendKey, []byte("not an envelope"))
	server.post("1", fixtures.BackendKey, []byte(`{}`+"\n"+`{"type":"client_report"}`+"\n"+
		`{"discarded_events":[{"reason":"network_error","category":"error","quantity":1},{"reason":"send_error","category":"log_item","quantity":3}]}`+"\n"))

	want := []store.Dropped{{Reason: store.DropInvalid, Kind: "envelopes", Count: 1}}

	if got := server.handler.dropped.counts(1, *now); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dropped %v, want %v", got, want)
	}
}

// A body cut off on its way (the client hung up) is not received whole, not
// invalid.
func TestDroppedCutOff(t *testing.T) {
	server := newTestServer(t, testSettings())
	request := httptest.NewRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey),
		io.MultiReader(strings.NewReader(simpleEnvelope[:10]), iotest.ErrReader(io.ErrUnexpectedEOF)))

	if response := server.send(request); response.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", response.Code)
	}

	want := []store.Dropped{{Reason: store.DropIncomplete, Kind: "envelopes", Count: 1}}

	if got := server.handler.dropped.Dropped(1); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("dropped %v, want %v", got, want)
	}
}

// A full inbox drops the events and logs of the envelope.
func TestDroppedWhenBusy(t *testing.T) {
	server := newTestServer(t, testSettings())
	server.pipeline.MaxInboxBytes = -1

	if response := server.post("1", fixtures.BackendKey, logEnvelope(3, true)); response.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d", response.Code)
	}

	want := "[{busy logs 3} {busy events 1}]"

	if got := server.handler.dropped.Dropped(1); fmt.Sprint(got) != want {
		t.Errorf("dropped %v, want %s", got, want)
	}
}

// The counts cover the last 24 hours by the hour, and a project with
// nothing in them is forgotten.
func TestDroppedRollsOverADay(t *testing.T) {
	counter := newDropCounter(slog.New(slog.DiscardHandler))
	start := time.Date(2026, 11, 11, 10, 30, 0, 0, time.UTC)
	backend, frontend := config.Project{ID: 1, Name: "backend"}, config.Project{ID: 2, Name: "frontend"}

	counter.add(backend, dropRateLimit, dropLogs, 5, start)
	counter.add(backend, dropRateLimit, dropLogs, 7, start.Add(23*time.Hour))

	if got := counter.counts(1, start.Add(23*time.Hour+20*time.Minute)); len(got) != 1 || got[0].Count != 12 {
		t.Errorf("within a day: %v", got)
	}

	if got := counter.counts(1, start.Add(24*time.Hour)); len(got) != 1 || got[0].Count != 7 {
		t.Errorf("a day later: %v", got)
	}

	// The bucket of an hour a day later starts again from zero.
	counter.add(backend, dropRateLimit, dropLogs, 1, start.Add(24*time.Hour))

	if got := counter.counts(1, start.Add(24*time.Hour)); len(got) != 1 || got[0].Count != 8 {
		t.Errorf("bucket reused: %v", got)
	}

	counter.add(frontend, dropBusy, dropEnvelopes, 1, start.Add(72*time.Hour))

	if _, kept := counter.projects[1]; kept {
		t.Error("a project without drops in the last day is kept")
	}
}

// Ingest is public: a project's drops log one warning an hour, not one per envelope.
func TestDroppedWarnsOncePerHour(t *testing.T) {
	server, now := limitedServer(t, 1, 0)
	counter := &warnCounter{Handler: slog.DiscardHandler}
	server.handler.dropped.logger = slog.New(counter)

	for range 5 {
		server.post("1", fixtures.BackendKey, eventEnvelope())
		server.post("1", fixtures.BackendKey, []byte("not an envelope"))
	}

	if warnings := counter.warnings.Load(); warnings != 1 {
		t.Fatalf("%d warnings, want 1", warnings)
	}

	*now = now.Add(time.Hour)

	server.post("1", fixtures.BackendKey, []byte("not an envelope"))

	if warnings := counter.warnings.Load(); warnings != 2 {
		t.Fatalf("%d warnings in the next hour, want 2", warnings)
	}
}
