package api

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"net/netip"
	"slices"

	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/store"
)

// SessionUsers reads the login session of a request (session.Auth):
// store.ErrNotFound without a valid one.
type SessionUsers interface {
	User(r *http.Request) (store.User, error)
}

// UI serves the catalog to the web app under /api/ui/<name>, as the user
// of the login session, not for scripts (they use the REST API's token).
type UI struct {
	logger         *slog.Logger
	endpoints      []Endpoint
	plain          map[string]http.Handler
	sessions       SessionUsers
	publicURL      string
	trustedProxies []netip.Prefix
	unknown        http.Handler
}

// uiPolicy: the answers are data, never a page to render or frame.
const uiPolicy = "default-src 'none'; frame-ancestors 'none'; sandbox"

// NewUI serves endpoints, and plain: the handlers of PlainRequests by name,
// each checking the session itself, behind the same checks of the request.
// A name of no plain request panics, a programming error found at startup.
func NewUI(logger *slog.Logger, endpoints []Endpoint, plain map[string]http.Handler, sessions SessionUsers, publicURL string, trustedProxies []netip.Prefix) *UI {
	for name := range plain {
		if !slices.ContainsFunc(PlainRequests, func(request PlainRequest) bool { return request.Name == name }) {
			panic("web app request " + name + " is not one of api.PlainRequests")
		}
	}

	handler := &UI{logger: logger, endpoints: endpoints, plain: plain, sessions: sessions, publicURL: publicURL, trustedProxies: trustedProxies}

	// Any other name: after the same checks.
	handler.unknown = handler.guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := handler.caller(w, r); ok {
			server.JSONError(w, http.StatusNotFound, ErrNotFound.Error())
		}
	}))

	return handler
}

// Register adds the routes, each wrapped with protect (the IP allowlist).
func (handler *UI) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	guarded := func(next http.Handler) http.Handler {
		return protect(handler.guard(next))
	}

	register(router, handler.logger, TransportUI, handler.endpoints, guarded, handler.caller)

	for _, request := range PlainRequests {
		if plain, ok := handler.plain[request.Name]; ok {
			router.Handle(request.Method+" "+server.UIAPIPrefix+request.Name, guarded(plain))
			router.Handle(server.UIAPIPrefix+request.Name, guarded(wrongMethod(request.Method, handler.caller)))
		}
	}
}

// ServeHTTP answers the addresses under server.UIAPIPrefix that are no route.
func (handler *UI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler.unknown.ServeHTTP(w, r)
}

// caller is the user of the session, as a call of the web app.
func (handler *UI) caller(w http.ResponseWriter, r *http.Request) (Call, bool) {
	viewer, ok := sessionViewer(handler.logger, handler.sessions, w, r)

	if !ok {
		return Call{}, false
	}

	return Call{Transport: TransportUI, Viewer: viewer, Actor: viewer.Actor(store.SourceUI),
		Address: server.PublicURL(r, handler.publicURL, handler.trustedProxies)}, true
}

// sessionViewer is the user of the request's session: without one it
// answers 401, so the app sends the browser to the login; 500 when it could
// not be checked, never a login loop.
func sessionViewer(logger *slog.Logger, sessions SessionUsers, w http.ResponseWriter, r *http.Request) (store.User, bool) {
	viewer, err := sessions.User(r)

	switch {
	case errors.Is(err, store.ErrNotFound):
		server.JSONError(w, http.StatusUnauthorized, "login required")

		return store.User{}, false
	case err != nil:
		logger.Error("web app session check failed", "path", r.URL.Path, "err", err)
		server.JSONError(w, http.StatusInternalServerError, "internal error")

		return store.User{}, false
	}

	return viewer, true
}

// guard: the headers of every answer, and only the web app's own pages may
// call. A page of another site cannot send JSON or forge Origin without a
// CORS preflight, which is never allowed; Fetch Metadata refuses it before.
func (handler *UI) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		server.SecurityHeaders(header, uiPolicy)
		header.Set("Cross-Origin-Resource-Policy", "same-origin")

		if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
			server.JSONError(w, http.StatusForbidden, "cross-site request rejected")

			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
				server.JSONError(w, http.StatusUnsupportedMediaType, "send the input as application/json")

				return
			}

			if !server.SameOrigin(r, handler.trustedProxies) {
				server.JSONError(w, http.StatusForbidden, "cross-site request rejected")

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
