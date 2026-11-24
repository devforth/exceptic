package ingest

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/server"
)

// A client that stops sending is answered 408 after BodyReadTimeout, through the
// whole server (its request log wraps the writer), not at the server's
// longer ReadTimeout, and its part of the read budget is freed.
func TestStalledBodyTimesOutThroughServer(t *testing.T) {
	t.Parallel()

	ingest := newTestServer(t, testSettings())
	handler := server.New(server.Options{Logger: slog.New(slog.DiscardHandler), Database: ingest.db, Ingest: ingest.handler}).Handler()
	httpServer := httptest.NewUnstartedServer(handler)
	httpServer.Config.ReadTimeout = 30 * time.Second // as app.Run sets it
	httpServer.Start()

	defer httpServer.Close()

	conn, err := net.Dial("tcp", httpServer.Listener.Addr().String())

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = conn.Close() }()

	start := time.Now()

	_, err = fmt.Fprintf(conn, "POST %s HTTP/1.1\r\nHost: localhost\r\nContent-Length: 1000000\r\n\r\n%s",
		envelopeURL("1", fixtures.BackendKey), simpleEnvelope)

	if err != nil {
		t.Fatal(err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(BodyReadTimeout + 5*time.Second))

	response, err := http.ReadResponse(bufio.NewReader(conn), nil)

	if err != nil {
		t.Fatalf("no answer after %s: %v", time.Since(start).Round(time.Second), err)
	}

	_ = response.Body.Close()

	if response.StatusCode != http.StatusRequestTimeout {
		t.Errorf("got %d, want 408", response.StatusCode)
	}

	if dropped := fmt.Sprint(ingest.handler.dropped.Dropped(1)); dropped != "[{incomplete envelopes 1}]" {
		t.Errorf("dropped %s, want the envelope as not received whole", dropped)
	}

	ingest.handler.reading.mutex.Lock()
	defer ingest.handler.reading.mutex.Unlock()

	if used := ingest.handler.reading.used; used != 0 {
		t.Errorf("%d bytes still reserved", used)
	}
}
