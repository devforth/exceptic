package ingest

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

// Same keys and IDs as the fixtures and the sentry-playground DSNs.
func testSettings() *config.Config {
	return &config.Config{
		Projects:       fixtures.Projects(),
		AllowedOrigins: []string{"*"},
	}
}

type testServer struct {
	t        *testing.T
	handler  *Handler
	pipeline *Pipeline
	db       *database.Store
	router   http.Handler
}

// newTestServer runs the handler with its pipeline on a new database, as the
// server does, until the test ends.
func newTestServer(t *testing.T, settings *config.Config) *testServer {
	t.Helper()

	return startTestServer(t, settings, StoreSink{Logger: slog.New(slog.DiscardHandler)})
}

func startTestServer(t *testing.T, settings *config.Config, sink StoreSink) *testServer {
	t.Helper()

	db := storetest.Open(t)
	pipeline := NewPipeline(slog.New(slog.DiscardHandler), settings, settings.Projects, db, sink)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() { defer close(done); pipeline.Run(ctx) }()

	// Before the store closes (cleanups run last added first).
	t.Cleanup(func() {
		cancel()
		<-done
	})

	router := http.NewServeMux()
	router.Handle("/api/{projectID}/envelope/{$}", pipeline.Handler())

	return &testServer{t: t, handler: pipeline.Handler(), pipeline: pipeline, db: db, router: router}
}

// waitStored waits until the pipeline has stored everything accepted so far.
func (server *testServer) waitStored() {
	server.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	if err := server.pipeline.WaitStored(ctx); err != nil {
		server.t.Fatalf("envelopes not stored: %v", err)
	}
}

// stored waits for the pipeline and returns the events and logs in the database.
func (server *testServer) stored() ([]store.EventSummary, []store.LogRecord) {
	server.t.Helper()
	server.waitStored()

	events, err := server.db.ListEvents(context.Background(), store.EventFilter{Limit: 1 << 20})

	if err != nil {
		server.t.Fatal(err)
	}

	page, err := server.db.ListLogs(context.Background(), store.LogFilter{Limit: 1 << 20})

	if err != nil {
		server.t.Fatal(err)
	}

	return events, page.Logs
}

func (server *testServer) send(request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)

	return response
}

func newRequest(method, target string, body []byte) *http.Request {
	return httptest.NewRequest(method, target, bytes.NewReader(body))
}

func envelopeURL(projectID, key string) string {
	target := "/api/" + projectID + "/envelope/"

	if key != "" {
		target += "?sentry_version=7&sentry_key=" + key
	}

	return target
}

const simpleEnvelope = "{\"event_id\":\"9ec79c33ec9942ab8353589fcb2e04dc\"}\n{\"type\":\"event\"}\n{\"message\":\"hello\"}\n"

func TestReplayRecordedSDKRequests(t *testing.T) {
	for _, fixture := range fixtures.All(t) {
		name := fixture.Name
		t.Run(name, func(t *testing.T) {
			server := newTestServer(t, testSettings())
			response := server.send(fixture.HTTP())

			if response.Code != http.StatusOK {
				t.Fatalf("got %d %s", response.Code, response.Body)
			}

			wantProject := 1 // backend

			if fixture.Browser() {
				wantProject = 2 // frontend

				if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3500" {
					t.Fatal("browser request without CORS header in the response")
				}
			}

			events, logs := server.stored()

			for _, event := range events {
				if event.ProjectID != wantProject {
					t.Fatalf("event stored for project %d, want %d", event.ProjectID, wantProject)
				}
			}

			for _, record := range logs {
				if record.ProjectID != wantProject {
					t.Fatalf("log stored for project %d, want %d", record.ProjectID, wantProject)
				}
			}

			var body map[string]string

			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}

			switch {
			case strings.Contains(name, "-event-"):
				if len(events) != 1 || body["id"] != events[0].EventID {
					t.Fatalf("events=%d response id=%q", len(events), body["id"])
				}
			case strings.HasSuffix(name, "-logs"):
				if len(logs) == 0 || len(events) != 0 {
					t.Fatalf("logs=%d events=%d", len(logs), len(events))
				}
			default: // spans and client reports are accepted and dropped
				if len(events)+len(logs) != 0 {
					t.Fatalf("%s must not be stored", name)
				}
			}
		})
	}
}

func TestAuthentication(t *testing.T) {
	testCases := []struct {
		name       string
		request    func() *http.Request
		wantStatus int
	}{
		{"key in query", func() *http.Request {
			return newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(simpleEnvelope))
		}, http.StatusOK},
		{"key in X-Sentry-Auth", func() *http.Request {
			request := newRequest(http.MethodPost, envelopeURL("1", ""), []byte(simpleEnvelope))
			request.Header.Set("X-Sentry-Auth", "Sentry sentry_version=7, sentry_client=sentry.python/2.0, sentry_key="+fixtures.BackendKey)

			return request
		}, http.StatusOK},
		{"key in Authorization", func() *http.Request {
			request := newRequest(http.MethodPost, envelopeURL("1", ""), []byte(simpleEnvelope))
			request.Header.Set("Authorization", "sentry sentry_key="+fixtures.BackendKey)

			return request
		}, http.StatusOK},
		{"key in envelope dsn (tunnel)", func() *http.Request {
			body := "{\"dsn\":\"https://" + fixtures.BackendKey + "@errors.example.com/1\"}\n{\"type\":\"event\"}\n{}\n"
			return newRequest(http.MethodPost, envelopeURL("1", ""), []byte(body))
		}, http.StatusOK},
		{"envelope dsn for another project", func() *http.Request {
			body := "{\"dsn\":\"https://" + fixtures.BackendKey + "@errors.example.com/2\"}\n{\"type\":\"event\"}\n{}\n"
			return newRequest(http.MethodPost, envelopeURL("1", ""), []byte(body))
		}, http.StatusUnauthorized},
		{"no key", func() *http.Request {
			return newRequest(http.MethodPost, envelopeURL("1", ""), []byte(simpleEnvelope))
		}, http.StatusUnauthorized},
		{"wrong key", func() *http.Request {
			return newRequest(http.MethodPost, envelopeURL("1", fixtures.BrowserKey), []byte(simpleEnvelope))
		}, http.StatusForbidden},
		{"unknown project", func() *http.Request {
			return newRequest(http.MethodPost, envelopeURL("99", fixtures.BackendKey), []byte(simpleEnvelope))
		}, http.StatusForbidden},
		{"non-numeric project", func() *http.Request {
			return newRequest(http.MethodPost, envelopeURL("abc", fixtures.BackendKey), []byte(simpleEnvelope))
		}, http.StatusNotFound},
		{"wrong method", func() *http.Request {
			return newRequest(http.MethodGet, envelopeURL("1", fixtures.BackendKey), nil)
		}, http.StatusMethodNotAllowed},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := newTestServer(t, testSettings())
			response := server.send(testCase.request())

			if response.Code != testCase.wantStatus {
				t.Fatalf("got %d %s, want %d", response.Code, response.Body, testCase.wantStatus)
			}

			if events, logs := server.stored(); testCase.wantStatus != http.StatusOK && len(events)+len(logs) != 0 {
				t.Fatal("rejected request must not store anything")
			}

			if _, contacted := server.db.FirstContact(1); contacted != (testCase.wantStatus == http.StatusOK) {
				t.Fatalf("project marked connected: %v, status %d", contacted, testCase.wantStatus)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	settings := testSettings()
	settings.AllowedOrigins = []string{"https://app.example.com"}
	server := newTestServer(t, settings)

	t.Run("allowed origin", func(t *testing.T) {
		request := newRequest(http.MethodPost, envelopeURL("2", fixtures.BrowserKey), []byte(simpleEnvelope))
		request.Header.Set("Origin", "https://App.example.com")
		response := server.send(request)

		if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "https://App.example.com" {
			t.Fatalf("got %d, allow-origin %q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("preflight", func(t *testing.T) {
		request := newRequest(http.MethodOptions, envelopeURL("2", fixtures.BrowserKey), nil)
		request.Header.Set("Origin", "https://app.example.com")
		request.Header.Set("Access-Control-Request-Method", "POST")
		response := server.send(request)

		if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), "POST") {
			t.Fatalf("got %d, headers %v", response.Code, response.Header())
		}
	})

	t.Run("other origin", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodOptions} {
			request := newRequest(method, envelopeURL("2", fixtures.BrowserKey), []byte(simpleEnvelope))
			request.Header.Set("Origin", "https://evil.example.com")
			response := server.send(request)

			if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("%s: got %d, allow-origin %q", method, response.Code, response.Header().Get("Access-Control-Allow-Origin"))
			}
		}
	})

	t.Run("backend without origin", func(t *testing.T) {
		response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(simpleEnvelope)))

		if response.Code != http.StatusOK {
			t.Fatalf("got %d", response.Code)
		}
	})
}

func compress(t *testing.T, encoding string, data []byte) []byte {
	t.Helper()

	var (
		buffer bytes.Buffer
		writer io.WriteCloser
	)

	switch encoding {
	case "gzip":
		writer = gzip.NewWriter(&buffer)
	case "br":
		writer = brotli.NewWriterLevel(&buffer, brotli.BestSpeed) // the default level takes seconds for the 20 MB bomb
	default:
		writer = zlib.NewWriter(&buffer)
	}

	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}

func TestBody(t *testing.T) {
	testCases := []struct {
		name       string
		encoding   string
		body       func(t *testing.T) []byte
		wantStatus int
	}{
		{"gzip", "gzip", func(t *testing.T) []byte { return compress(t, "gzip", []byte(simpleEnvelope)) }, http.StatusOK},
		{"deflate", "deflate", func(t *testing.T) []byte { return compress(t, "deflate", []byte(simpleEnvelope)) }, http.StatusOK},
		{"brotli", "br", func(t *testing.T) []byte { return compress(t, "br", []byte(simpleEnvelope)) }, http.StatusOK},
		{"broken gzip", "gzip", func(t *testing.T) []byte { return []byte("not gzip") }, http.StatusBadRequest},
		{"broken brotli", "br", func(t *testing.T) []byte { return []byte("not brotli") }, http.StatusBadRequest},
		{"unsupported encoding", "zstd", func(t *testing.T) []byte { return []byte(simpleEnvelope) }, http.StatusUnsupportedMediaType},
		{"too large", "", func(t *testing.T) []byte { return bytes.Repeat([]byte("x"), MaxBodyBytes+1) }, http.StatusRequestEntityTooLarge},
		// A small gzip body that expands past the limit (a "zip bomb").
		{"too large after decompression", "gzip", func(t *testing.T) []byte {
			return compress(t, "gzip", bytes.Repeat([]byte("x"), MaxBodyBytes+1))
		}, http.StatusRequestEntityTooLarge},
		{"too large after brotli", "br", func(t *testing.T) []byte {
			return compress(t, "br", bytes.Repeat([]byte("x"), MaxBodyBytes+1))
		}, http.StatusRequestEntityTooLarge},
		{"invalid envelope", "", func(t *testing.T) []byte { return []byte("not an envelope") }, http.StatusBadRequest},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := newTestServer(t, testSettings())
			request := newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), testCase.body(t))

			if testCase.encoding != "" {
				request.Header.Set("Content-Encoding", testCase.encoding)
			}

			response := server.send(request)

			if response.Code != testCase.wantStatus {
				t.Fatalf("got %d %s, want %d", response.Code, response.Body, testCase.wantStatus)
			}
		})
	}
}

// Items are parsed after the answer: a malformed one drops the whole
// envelope instead of storing half of it.
func TestMalformedItemDropsWholeEnvelope(t *testing.T) {
	server := newTestServer(t, testSettings())
	body := "{}\n{\"type\":\"event\"}\n{\"message\":\"ok\"}\n{\"type\":\"log\"}\nnot json\n"
	response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(body)))

	if response.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", response.Code)
	}

	if events, logs := server.stored(); len(events)+len(logs) != 0 {
		t.Fatal("the valid event must not be stored when another item is malformed")
	}
}

// An envelope the inbox cannot take is a 500 that does not tell why.
func TestInboxFailure(t *testing.T) {
	server := newTestServer(t, testSettings())

	if err := server.db.Close(); err != nil {
		t.Fatal(err)
	}

	response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(simpleEnvelope)))

	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "closed") {
		t.Fatalf("got %d %s", response.Code, response.Body)
	}
}

// A project is connected at its first request with its key, of any kind:
// SDKs send a session at start, long before an error. The time is written
// once; later envelopes write nothing.
func TestFirstContact(t *testing.T) {
	server := newTestServer(t, testSettings())
	first := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	server.handler.now = func() time.Time { return first }
	session := "{\"sent_at\":\"2026-11-23T09:00:00Z\"}\n{\"type\":\"session\"}\n{\"sid\":\"7c3a1f0e9d2b4c5a8e6f1a2b3c4d5e6f\",\"status\":\"ok\",\"init\":true}\n"

	if _, contacted := server.db.FirstContact(1); contacted {
		t.Fatal("connected before any request")
	}

	if response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(session))); response.Code != http.StatusOK {
		t.Fatalf("session: got %d %s", response.Code, response.Body)
	}

	if at, contacted := server.db.FirstContact(1); !contacted || !at.Equal(first) {
		t.Fatalf("after a session: %v %v, want %v", at, contacted, first)
	}

	if events, logs := server.stored(); len(events)+len(logs) != 0 {
		t.Fatalf("a session stored %d events, %d logs", len(events), len(logs))
	}

	server.handler.now = func() time.Time { return first.Add(time.Hour) }

	if response := server.send(fixtures.Load(t, "node-client-report").HTTP()); response.Code != http.StatusOK {
		t.Fatalf("client report: got %d %s", response.Code, response.Body)
	}

	if at, _ := server.db.FirstContact(1); !at.Equal(first) {
		t.Fatalf("first contact moved to %v", at)
	}

	if _, contacted := server.db.FirstContact(2); contacted {
		t.Fatal("another project marked connected")
	}
}

// A brotli stream's window, which its decoder may fill before any output,
// counts in the read budget.
func TestBrotliWindowIsBudgeted(t *testing.T) {
	var buffer bytes.Buffer

	writer := brotli.NewWriterOptions(&buffer, brotli.WriterOptions{Quality: 5, LGWin: 24})

	if _, err := writer.Write([]byte(simpleEnvelope)); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if window := brotliWindow(buffer.Bytes()[0]); window != 16<<20 {
		t.Fatalf("window %d, want 16 MB", window)
	}

	server := newTestServer(t, testSettings())
	server.handler.reading = newReadBudget(16<<20, 8<<20)

	request := newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), buffer.Bytes())
	request.Header.Set("Content-Encoding", "br")

	if response := server.send(request); response.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d %s, want 429", response.Code, response.Body)
	}

	if used := server.handler.reading.used; used != 0 {
		t.Errorf("%d bytes still reserved", used)
	}
}

// One project's share holds the largest body any encoding may take: a brotli
// body read whole, its window and its output, each near the size limit.
func TestLargestBrotliBodyFitsTheShare(t *testing.T) {
	attachment := make([]byte, 17<<20)
	_, _ = rand.Read(attachment) // does not compress

	var buffer bytes.Buffer

	writer := brotli.NewWriterOptions(&buffer, brotli.WriterOptions{Quality: 2, LGWin: 24})
	_, _ = fmt.Fprintf(writer, "{}\n{\"type\":\"attachment\",\"length\":%d}\n", len(attachment))
	_, _ = writer.Write(attachment)

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if window := brotliWindow(buffer.Bytes()[0]); window != 16<<20 || buffer.Len() <= 16<<20 {
		t.Fatalf("window %d, %d bytes", window, buffer.Len())
	}

	server := newTestServer(t, testSettings())
	request := newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), buffer.Bytes())
	request.Header.Set("Content-Encoding", "br")

	if response := server.send(request); response.Code != http.StatusOK {
		t.Fatalf("got %d %s, want 200", response.Code, response.Body)
	}
}

// A brotli stream that declares a 16 MB window and stalls holds only the
// bytes it sent, not the window.
func TestStalledBrotliHoldsOnlyItsBytes(t *testing.T) {
	server := newTestServer(t, testSettings())
	reader, writer := io.Pipe()
	request := httptest.NewRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), reader)
	request.Header.Set("Content-Encoding", "br")

	done := make(chan *httptest.ResponseRecorder)

	go func() { done <- server.send(request) }()

	if _, err := writer.Write([]byte{0x0f}); err != nil { // WBITS 24: a 16 MB window
		t.Fatal(err)
	}

	used := func() int64 {
		server.handler.reading.mutex.Lock()
		defer server.handler.reading.mutex.Unlock()

		return server.handler.reading.used
	}

	for deadline := time.Now().Add(200 * time.Millisecond); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if held := used(); held > 64<<10 {
			t.Errorf("a stalled stream holds %d bytes", held)

			break
		}
	}

	writer.CloseWithError(io.ErrUnexpectedEOF)

	if response := <-done; response.Code != http.StatusBadRequest {
		t.Errorf("got %d %s, want 400", response.Code, response.Body)
	}

	if held := used(); held != 0 {
		t.Errorf("%d bytes still reserved", held)
	}
}

// Credentials of the monitored app's users never reach the database (and so
// the panel, the API or notifications).
func TestCredentialsAreScrubbed(t *testing.T) {
	server := newTestServer(t, testSettings())
	body := "{}\n" +
		`{"type":"event"}` + "\n" +
		`{"message":"hello","request":{"headers":{"Cookie":"sid=cookie-value","Authorization":"Bearer header-token-value"},"cookies":{"sid":"cookie-object-value"}}}` + "\n" +
		`{"type":"log"}` + "\n" +
		`{"items":[{"timestamp":1731400000,"level":"info","body":"request","attributes":{"http.request.header.authorization":{"value":"Bearer log-token-value","type":"string"}}}]}` + "\n"

	if response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(body))); response.Code != http.StatusOK {
		t.Fatalf("got %d %s", response.Code, response.Body)
	}

	events, logs := server.stored()

	if len(events) != 1 || len(logs) != 1 {
		t.Fatalf("stored %d events, %d logs", len(events), len(logs))
	}

	event, err := server.db.GetEvent(context.Background(), events[0].ProjectID, events[0].EventID)

	if err != nil {
		t.Fatal(err)
	}

	stored := string(event.Payload) + string(logs[0].Attributes)

	for _, secret := range []string{"cookie-value", "header-token-value", "cookie-object-value", "log-token-value"} {
		if strings.Contains(stored, secret) {
			t.Errorf("%q reached the database:\n%s", secret, stored)
		}
	}
}

// SDKs send request bodies as text: a JSON body inside a string gets the
// key rules of the payload, and so does everything under a secret's key.
func TestCredentialsInJSONTextAreScrubbed(t *testing.T) {
	server := newTestServer(t, testSettings())
	body := "{}\n" +
		`{"type":"event"}` + "\n" +
		`{"message":"hello","request":{"method":"POST","url":"https://app.example.com/login","data":"{\"username\":\"root\",\"password\":\"string-body-password\",\"api_key\":\"string-body-key\"}"},` +
		`"breadcrumbs":[{"message":"{\"cookie\":\"sid=breadcrumb-cookie\",\"token\":\"breadcrumb-token\"}"}],` +
		`"extra":{"body":"{\"cookie\":\"sid=extra-cookie\",\"token\":\"extra-token\"}","auth":{"value":{"user":7},"token":"sibling-token-value","password":"sibling-password"}}}` + "\n" +
		`{"type":"log"}` + "\n" +
		`{"items":[{"timestamp":1731400000,"level":"info","body":"{\"cookie\":\"sid=log-body-cookie\",\"token\":\"log-body-token\"}","attributes":{"request.body":{"value":"{\"cookie\":\"sid=attribute-cookie\",\"token\":\"attribute-token\"}","type":"string"}}}]}` + "\n"

	if response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(body))); response.Code != http.StatusOK {
		t.Fatalf("got %d %s", response.Code, response.Body)
	}

	events, logs := server.stored()

	if len(events) != 1 || len(logs) != 1 {
		t.Fatalf("stored %d events, %d logs", len(events), len(logs))
	}

	event, err := server.db.GetEvent(context.Background(), events[0].ProjectID, events[0].EventID)

	if err != nil {
		t.Fatal(err)
	}

	stored := string(event.Payload) + logs[0].Body + string(logs[0].Attributes)

	for _, secret := range []string{
		"string-body-password", "string-body-key", "breadcrumb-cookie", "breadcrumb-token", "extra-cookie", "extra-token",
		"sibling-token-value", "sibling-password", "log-body-cookie", "log-body-token", "attribute-cookie", "attribute-token",
	} {
		if strings.Contains(stored, secret) {
			t.Errorf("%q reached the database:\n%s", secret, stored)
		}
	}

	if !strings.Contains(stored, `\"username\":\"root\"`) {
		t.Errorf("the username was removed:\n%s", stored)
	}
}

// Without a key in the request only the envelope header is read before the
// key is checked: a small gzip body that expands to 20 MB costs no more.
func TestKeylessBodyIsNotReadBeforeAuth(t *testing.T) {
	server := newTestServer(t, testSettings())
	padding := bytes.Repeat([]byte("x"), MaxBodyBytes-100)
	body := compress(t, "gzip", append([]byte("{}\n{\"type\":\"event\",\"length\":"+strconv.Itoa(len(padding))+"}\n"), padding...))

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)

	request := newRequest(http.MethodPost, envelopeURL("1", ""), body)
	request.Header.Set("Content-Encoding", "gzip")
	response := server.send(request)

	runtime.ReadMemStats(&after)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("got %d %s", response.Code, response.Body)
	}

	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 2<<20 {
		t.Errorf("allocated %d bytes for a request without a key", allocated)
	}
}

// Bodies being read share one memory budget: past it the SDK gets 429 with
// Retry-After, and a finished request returns what it held.
func TestReadBudget(t *testing.T) {
	server := newTestServer(t, testSettings())
	large := []byte("{}\n{\"type\":\"event\"}\n{\"message\":\"" + strings.Repeat("x", 1<<20) + "\"}\n")

	server.handler.reading = newReadBudget(512<<10, 256<<10)

	response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), large))

	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d %s", response.Code, response.Body)
	}

	if response := server.send(newRequest(http.MethodPost, envelopeURL("1", fixtures.BackendKey), []byte(simpleEnvelope))); response.Code != http.StatusOK {
		t.Fatalf("small envelope: %d %s", response.Code, response.Body)
	}

	if used := server.handler.reading.used; used != 0 {
		t.Errorf("%d bytes still reserved", used)
	}
}

// With the key in the request nothing of the body is read before the key
// and the project are checked.
func TestBodyIsReadAfterAuth(t *testing.T) {
	server := newTestServer(t, testSettings())
	request := newRequest(http.MethodPost, envelopeURL("1", "wrong"), []byte("not zstd"))
	request.Header.Set("Content-Encoding", "zstd")

	if response := server.send(request); response.Code != http.StatusForbidden {
		t.Fatalf("got %d %s, want 403", response.Code, response.Body)
	}
}

// One project may hold half of the budget, so the public key of a browser
// app cannot stop the others.
func TestReadBudgetShare(t *testing.T) {
	budget := newReadBudget(100, 50)

	if !budget.reserve(1, 50) || budget.reserve(1, 1) {
		t.Fatal("project 1 is not held to its half")
	}

	if !budget.reserve(2, 50) || budget.reserve(3, 1) {
		t.Fatal("the total is not enforced")
	}

	budget.release(1, 50)
	budget.release(2, 50)

	if budget.used != 0 || len(budget.usedBy) != 0 {
		t.Errorf("used %d, by project %v", budget.used, budget.usedBy)
	}
}

// A brotli body can only be read whole, so without a key in the request it
// is refused before anything of it is read.
func TestKeylessBrotliIsRefusedUnread(t *testing.T) {
	server := newTestServer(t, testSettings())
	body := compress(t, "br", append([]byte("{\"dsn\":\"https://"+fixtures.BackendKey+"@errors.example.com/1\"}\n"), bytes.Repeat([]byte("x"), 1<<20)...))
	request := newRequest(http.MethodPost, envelopeURL("1", ""), body)
	request.Header.Set("Content-Encoding", "br")

	if response := server.send(request); response.Code != http.StatusUnauthorized {
		t.Fatalf("got %d %s, want 401", response.Code, response.Body)
	}

	if used := server.handler.reading.used; used != 0 {
		t.Errorf("%d bytes still reserved", used)
	}
}
