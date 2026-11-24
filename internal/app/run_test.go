package app

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
)

// serve runs serveLimited on a local port and returns its address.
func serve(t *testing.T, most int) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	httpServer, limited := serveLimited(handler, listener, most)

	go func() { _ = httpServer.Serve(limited) }()

	t.Cleanup(func() { _ = httpServer.Close() })

	return "http://" + listener.Addr().String()
}

func get(t *testing.T, client *http.Client, address string, header http.Header) int {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, address+"/health", nil)

	if err != nil {
		t.Fatal(err)
	}

	request.Header = header
	response, err := client.Do(request)

	if err != nil {
		t.Fatal(err)
	}

	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	return response.StatusCode
}

func TestHeadersOverTheLimit(t *testing.T) {
	address := serve(t, config.MaxConnections)
	client := &http.Client{Timeout: 5 * time.Second}

	if status := get(t, client, address, http.Header{"X-Large": {strings.Repeat("a", 60<<10)}}); status != http.StatusOK {
		t.Fatalf("60 KiB of headers: %d", status)
	}

	if status := get(t, client, address, http.Header{"X-Large": {strings.Repeat("a", 80<<10)}}); status != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("80 KiB of headers: %d, want 431", status)
	}
}

// With every connection taken by idle keep-alive ones, a new client (the
// healthcheck) is served at once: the idle ones are closed to make room.
func TestIdleConnectionsMakeRoom(t *testing.T) {
	address := serve(t, 2)

	for range 2 {
		idle := &http.Client{Transport: &http.Transport{}, Timeout: 5 * time.Second}

		if status := get(t, idle, address, nil); status != http.StatusOK {
			t.Fatal(status)
		}
	}

	healthcheck := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 3 * time.Second}

	if status := get(t, healthcheck, address, nil); status != http.StatusOK {
		t.Fatal(status)
	}
}
