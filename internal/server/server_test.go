package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDB struct{ err error }

func (fake fakeDB) Ping(context.Context) error { return fake.err }

func request(t *testing.T, handler http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))

	return response
}

func TestHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("ok", func(t *testing.T) {
		response := request(t, New(Options{Logger: logger, Database: fakeDB{}, Ingest: http.NotFoundHandler()}).Handler(), http.MethodGet, "/health")

		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok"`) {
			t.Fatalf("got %d %s", response.Code, response.Body)
		}

		if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("Content-Type = %q", contentType)
		}
	})

	t.Run("database down", func(t *testing.T) {
		response := request(t, New(Options{Logger: logger, Database: fakeDB{err: errors.New("boom")}, Ingest: http.NotFoundHandler()}).Handler(), http.MethodGet, "/health")

		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("got %d, want 503", response.Code)
		}

		if strings.Contains(response.Body.String(), "boom") {
			t.Fatal("internal error details must not leak to the response")
		}
	})

	t.Run("wrong method", func(t *testing.T) {
		response := request(t, New(Options{Logger: logger, Database: fakeDB{}, Ingest: http.NotFoundHandler()}).Handler(), http.MethodPost, "/health")

		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("got %d, want 405", response.Code)
		}
	})
}

func TestEnvelopeRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ingest := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "project "+r.PathValue("projectID"))
	})
	handler := New(Options{Logger: logger, Database: fakeDB{}, Ingest: ingest}).Handler()

	for _, path := range []string{"/api/7/envelope/", "/api/7/envelope"} {
		response := request(t, handler, http.MethodPost, path)

		if response.Code != http.StatusOK || response.Body.String() != "project 7" {
			t.Fatalf("%s: got %d %q", path, response.Code, response.Body)
		}
	}

	if response := request(t, handler, http.MethodPost, "/api/7/envelope/extra"); response.Code != http.StatusNotFound {
		t.Fatalf("subpath: got %d, want 404", response.Code)
	}
}

// routesFunc registers routes with a function.
type routesFunc func(router *http.ServeMux, protect func(http.Handler) http.Handler)

func (register routesFunc) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	register(router, protect)
}

// A handler that panics answers a JSON 500 without the panic's text; the
// panic is logged with its stack and the request.
func TestPanickingHandlerAnswers500(t *testing.T) {
	var output bytes.Buffer

	logger := slog.New(slog.NewTextHandler(&output, nil))
	routes := routesFunc(func(router *http.ServeMux, protect func(http.Handler) http.Handler) {
		router.Handle("GET /api/ui/boom", protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("secret detail")
		})))
	})
	server := httptest.NewServer(New(Options{Logger: logger, Database: fakeDB{}, Ingest: http.NotFoundHandler(), Web: routes}).Handler())

	defer server.Close()

	response, err := http.Get(server.URL + "/api/ui/boom")

	if err != nil {
		t.Fatalf("no answer: %v", err)
	}

	defer func() { _ = response.Body.Close() }()

	body, _ := io.ReadAll(response.Body)

	if response.StatusCode != http.StatusInternalServerError || strings.TrimSpace(string(body)) != `{"error":"internal error"}` {
		t.Fatalf("got %d %s", response.StatusCode, body)
	}

	logged := output.String()

	if !strings.Contains(logged, "secret detail") || !strings.Contains(logged, "/api/ui/boom") || !strings.Contains(logged, "server_test.go") {
		t.Errorf("the panic was not logged with its stack and path:\n%s", logged)
	}
}
