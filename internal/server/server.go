// Package server wires HTTP routes and middleware.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/logging"
)

// Pinger reports whether the storage is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Routes is a group of routes (API, web app) registered with a protection wrapper.
type Routes interface {
	Register(router *http.ServeMux, protect func(http.Handler) http.Handler)
}

// UIRoutes are the routes of the web app's API; as a handler it answers
// every other address under UIAPIPrefix, with the same checks as its routes.
type UIRoutes interface {
	Routes
	http.Handler
}

// Where the catalog of requests (package api) is served: to the API token,
// and to the web app with its login session.
const (
	RESTPrefix  = "/api/v1/"
	UIAPIPrefix = "/api/ui/"
)

// Options are the server's dependencies.
type Options struct {
	Logger   *slog.Logger
	Database Pinger
	// Ingest handles SDK envelopes (see package ingest). Open to everyone.
	Ingest http.Handler
	// API adds the admin REST API routes, wrapped with the given protection.
	API Routes
	// MCP adds the endpoint for coding agents and OAuth the endpoints they
	// sign in with; both check their own tokens, the server adds the IP allowlist.
	MCP   Routes
	OAuth Routes
	// Web adds the web app (package web) at every other address, and UIAPI
	// the requests it makes with its login; the server adds the IP allowlist.
	Web   Routes
	UIAPI UIRoutes
	// SignIn adds the sign-in with Google (session.Auth); the server adds the IP allowlist.
	SignIn Routes
	// Unsubscribe adds the links that stop emails: open to everyone like
	// ingest (mail providers request them from their servers), signed.
	Unsubscribe    interface{ Register(router *http.ServeMux) }
	APIToken       string
	AllowedIPs     []netip.Prefix
	TrustedProxies []netip.Prefix
}

type Server struct {
	Options
}

func New(options Options) *Server {
	return &Server{Options: options}
}

func (server *Server) Handler() http.Handler {
	router := http.NewServeMux()
	router.HandleFunc("GET /health", server.health)
	// SDKs derive this URL from the DSN. No method in the pattern: the ingest
	// handler answers CORS preflight (OPTIONS) itself. {$} means an exact match.
	router.Handle("/api/{projectID}/envelope/{$}", server.Ingest)
	router.Handle("/api/{projectID}/envelope", server.Ingest)

	if server.API != nil {
		server.API.Register(router, server.protectAPI)
	}

	if server.MCP != nil {
		server.MCP.Register(router, server.protectUI)
	}

	if server.OAuth != nil {
		server.OAuth.Register(router, server.protectUI)
	}

	if server.Web != nil {
		server.Web.Register(router, server.protectUI)
	}

	if server.UIAPI != nil {
		server.UIAPI.Register(router, server.protectUI)
	}

	if server.SignIn != nil {
		server.SignIn.Register(router, server.protectUI)
	}

	if server.Unsubscribe != nil {
		server.Unsubscribe.Register(router)
	}

	router.Handle("/api/", server.unknownAPI())

	return server.logRequests(server.recoverPanics(router))
}

// unknownAPI answers any other /api/ address with JSON, not the web app:
// under UIAPIPrefix the web app's API itself (a login, then 404), elsewhere
// a 404, behind the token when the REST API is served. Less specific than
// the ingest route /api/{projectID}/envelope/, so it does not overlap it.
func (server *Server) unknownAPI() http.Handler {
	var rest http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		JSONError(w, http.StatusNotFound, "unknown API endpoint: see "+RESTPrefix+"<name> in docs/api.md")
	})

	if server.API != nil {
		rest = server.protectAPI(rest)
	}

	var web http.Handler

	if server.UIAPI != nil {
		web = server.protectUI(server.UIAPI)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if web != nil && strings.HasPrefix(r.URL.Path, UIAPIPrefix) {
			web.ServeHTTP(w, r)

			return
		}

		rest.ServeHTTP(w, r)
	})
}

func (server *Server) protectUI(next http.Handler) http.Handler {
	return RequireAllowedIP(server.Logger, server.AllowedIPs, server.TrustedProxies, next)
}

// protectAPI: the IP check runs first, so clients outside the allowlist
// cannot even probe tokens.
func (server *Server) protectAPI(next http.Handler) http.Handler {
	return RequireAllowedIP(server.Logger, server.AllowedIPs, server.TrustedProxies, RequireToken(server.APIToken, next))
}

func (server *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)

	defer cancel()

	if err := server.Database.Ping(ctx); err != nil {
		server.Logger.Error("health check: database unavailable", "err", err)
		WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "error": "database unavailable"})

		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the connection (the read
// deadline of ingest bodies).
func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

// recoverPanics logs a handler's panic with its stack and answers a JSON 500
// without its text, where net/http would only drop the connection. Once the
// answer has begun, the connection is dropped all the same.
func (server *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracker := &startTracker{ResponseWriter: w}

		defer func() {
			recovered := recover()

			if recovered == nil {
				return
			}

			if recovered == http.ErrAbortHandler { //nolint:errorlint // the value given to panic, never wrapped
				panic(recovered)
			}

			server.Logger.Error("http handler panicked", "method", r.Method, "path", r.URL.Path, "route", r.Pattern, "err", logging.PanicError(recovered))

			if tracker.started {
				panic(http.ErrAbortHandler)
			}

			JSONError(w, http.StatusInternalServerError, "internal error")
		}()

		next.ServeHTTP(tracker, r)
	})
}

// startTracker notes whether the answer has begun.
type startTracker struct {
	http.ResponseWriter
	started bool
}

func (tracker *startTracker) WriteHeader(status int) {
	tracker.started = true
	tracker.ResponseWriter.WriteHeader(status)
}

func (tracker *startTracker) Write(data []byte) (int, error) {
	tracker.started = true

	return tracker.ResponseWriter.Write(data)
}

func (tracker *startTracker) Unwrap() http.ResponseWriter { return tracker.ResponseWriter }

func (server *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		server.Logger.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration", time.Since(start),
		)
	})
}
