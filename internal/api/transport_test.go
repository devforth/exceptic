package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

func TestAllowedFor(t *testing.T) {
	user := store.User{ID: "user0001", Role: store.RoleUser}
	admin := store.User{ID: "admin001", Role: store.RoleAdmin}
	temporary := store.User{ID: "admin002", Role: store.RoleAdmin, MustChangePassword: true}
	everywhere := Endpoint{Transports: AllTransports}
	adminOnly := Endpoint{Transports: AllTransports, Admin: true}
	asUser := Endpoint{Transports: ActsAsUser}
	restOnly := Endpoint{Transports: TransportREST}
	open := Endpoint{Transports: TransportUI, OpenWithTemporaryPassword: true}

	cases := []struct {
		label    string
		endpoint Endpoint
		call     Call
		want     bool
	}{
		{"no transport", everywhere, Call{Viewer: admin}, false},
		{"token", everywhere, Call{Transport: TransportREST}, true},
		{"token, admin endpoint", adminOnly, Call{Transport: TransportREST}, true},
		{"token acts as no user", asUser, Call{Transport: TransportREST}, false},
		{"agent", asUser, Call{Transport: TransportMCP, Viewer: user}, true},
		{"agent without a user", everywhere, Call{Transport: TransportMCP}, false},
		{"agent, REST only", restOnly, Call{Transport: TransportMCP, Viewer: admin}, false},
		{"web app, REST only", restOnly, Call{Transport: TransportUI, Viewer: admin}, false},
		{"user, admin endpoint", adminOnly, Call{Transport: TransportUI, Viewer: user}, false},
		{"admin, admin endpoint", adminOnly, Call{Transport: TransportUI, Viewer: admin}, true},
		{"web app, temporary password", everywhere, Call{Transport: TransportUI, Viewer: temporary}, false},
		{"web app, temporary password, open", open, Call{Transport: TransportUI, Viewer: temporary}, true},
		{"agent, temporary password", everywhere, Call{Transport: TransportMCP, Viewer: temporary}, true}, // reset-password keeps agents
	}

	for _, test := range cases {
		if got := test.endpoint.AllowedFor(test.call); got != test.want {
			t.Errorf("%s: %v", test.label, got)
		}
	}

	// The catalog itself: nothing runs without a transport, nothing for admins runs for a user.
	for _, endpoint := range append(Endpoints(nil, nil), fetchSkill) {
		for _, transport := range []Transport{TransportMCP, TransportUI} {
			if endpoint.Admin && endpoint.AllowedFor(Call{Transport: transport, Viewer: user}) {
				t.Errorf("%s runs for a user over %s", endpoint.Name, transport)
			}
		}

		if endpoint.AllowedFor(Call{Viewer: admin}) {
			t.Errorf("%s runs without a transport", endpoint.Name)
		}
	}
}

func TestDefineNeedsTransports(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an endpoint without transports was defined")
		}
	}()

	Define(Endpoint{Name: "nowhere"}, func(context.Context, Call, struct{}) (struct{}, error) { return struct{}{}, nil })
}

// Names are path segments: an endpoint must not shadow an ingest path
// (/api/<project>/envelope) nor a plain request of the web app.
func TestEndpointNames(t *testing.T) {
	name := regexp.MustCompile(`^[a-z][a-z_]*$`)

	for _, endpoint := range append(Endpoints(nil, nil), fetchSkill) {
		isPlain := slices.ContainsFunc(PlainRequests, func(request PlainRequest) bool { return request.Name == endpoint.Name })

		if !name.MatchString(endpoint.Name) || endpoint.Name == "envelope" || isPlain {
			t.Errorf("endpoint name %q", endpoint.Name)
		}
	}
}

// Free text is redacted unless the reader is known not to be a model provider.
func TestForAgentFailsClosed(t *testing.T) {
	const text = "mail ops@example.com"

	for transport, redacted := range map[Transport]bool{0: true, TransportMCP: true, TransportREST: false, TransportUI: false} {
		if got := (Call{Transport: transport}).Text(text) != text; got != redacted {
			t.Errorf("transport %s: redacted %v", transport, got)
		}
	}
}

// Logins, the own password and personal tokens are never requests of the
// catalog: they are plain requests of the web app (PlainRequests, plain*.go),
// and logins and passwords go through the login's limits (session.Auth).
func TestCatalogNeverChangesCredentials(t *testing.T) {
	files, err := filepath.Glob("*.go")

	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || strings.HasPrefix(file, "plain") {
			continue
		}

		source, err := os.ReadFile(file)

		if err != nil {
			t.Fatal(err)
		}

		for _, method := range []string{"ChangeOwnPassword", "NewPersonalToken", "Authenticate", "SetupAdmin"} {
			if strings.Contains(string(source), method) {
				t.Errorf("%s uses %s", file, method)
			}
		}
	}
}

type failingSessions struct{}

func (failingSessions) User(*http.Request) (store.User, error) {
	return store.User{}, errors.New("database down")
}

// A session that cannot be checked is a server error, not a logout: a 401
// would send the browser to the login again and again.
func TestUISessionCheckFails(t *testing.T) {
	handler := NewUI(slog.New(slog.DiscardHandler), Endpoints(nil, nil), nil, failingSessions{}, "", nil)
	router := http.NewServeMux()
	handler.Register(router, func(next http.Handler) http.Handler { return next })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/ui/list_projects", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
}

// Plain requests of the web app pass the same checks as the catalog's.
func TestUIPlainRequests(t *testing.T) {
	reached := 0
	login := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached++

		w.WriteHeader(http.StatusNoContent)
	})
	handler := NewUI(slog.New(slog.DiscardHandler), Endpoints(nil, nil), map[string]http.Handler{"login": login}, failingSessions{}, "", nil)

	send := func(protect func(http.Handler) http.Handler, method, contentType, origin string) *httptest.ResponseRecorder {
		router := http.NewServeMux()
		handler.Register(router, protect)
		router.Handle("/api/ui/", handler) // as the server mounts the rest

		request := httptest.NewRequest(method, "/api/ui/login", strings.NewReader("{}"))
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Origin", origin)

		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		return response
	}
	open := func(next http.Handler) http.Handler { return next }
	closed := func(http.Handler) http.Handler { return http.NotFoundHandler() }

	response := send(open, http.MethodPost, "application/json", "http://example.com")

	if response.Code != http.StatusNoContent || reached != 1 || !strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("same origin: %d %v, reached %d", response.Code, response.Header(), reached)
	}

	for label, response := range map[string]*httptest.ResponseRecorder{
		"other site":    send(open, http.MethodPost, "application/json", "https://evil.example"),
		"form post":     send(open, http.MethodPost, "text/plain", "http://example.com"),
		"not protected": send(closed, http.MethodPost, "application/json", "http://example.com"),
		"other method":  send(open, http.MethodGet, "", ""),
	} {
		if response.Code < 400 {
			t.Errorf("%s: %d", label, response.Code)
		}
	}

	if reached != 1 {
		t.Errorf("refused requests reached the handler: %d", reached)
	}
}

func TestUIPlainNamesOnly(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a plain handler named as an endpoint was mounted")
		}
	}()

	NewUI(slog.New(slog.DiscardHandler), nil, map[string]http.Handler{"list_issues": http.NotFoundHandler()}, failingSessions{}, "", nil)
}

// Over HTTP a refused input names its fields and the record that already
// exists, so a form shows them, with the status of its kind; MCP gets the
// kind and the one message naming them.
func TestInputErrorBody(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
		want   ErrorBody
		text   string
	}{
		"fields": {
			problemsError(map[string]string{"name": "taken", "log_level": "unknown level"}, projectFormFields), http.StatusBadRequest,
			ErrorBody{Error: "log_levels: unknown level; name: taken", Fields: map[string]string{"name": "taken", "log_levels": "unknown level"}},
			"invalid: log_levels: unknown level; name: taken",
		},
		"duplicate": {
			duplicateError(services.Duplicate{ID: "conn0001", Name: "alerts"}, "sends there"), http.StatusConflict,
			ErrorBody{Error: "connection conn0001 (alerts) already sends there: update it instead", Duplicate: &DuplicateOutput{ID: "conn0001", Name: "alerts"}},
			"conflict: connection conn0001 (alerts) already sends there: update it instead",
		},
		"refusal": {
			services.Refusal{Kind: services.ErrConflict, Problems: services.Problems{"email": "taken"}}, http.StatusConflict,
			ErrorBody{Error: "email: taken", Fields: map[string]string{"email": "taken"}}, "conflict: email: taken",
		},
		"hidden":  {services.Hidden("issue", "abcd1234"), http.StatusForbidden, ErrorBody{Error: "issue abcd1234 is not in your projects"}, "forbidden: issue abcd1234 is not in your projects"},
		"missing": {services.Missing("issue", "abcd1234"), http.StatusNotFound, ErrorBody{Error: "not found: issue abcd1234"}, "not found: issue abcd1234"},
		"role":    {services.ErrForbidden, http.StatusForbidden, ErrorBody{Error: "your role may not do this"}, "forbidden: your role may not do this"},
		"plain":   {Invalid("offset out of range"), http.StatusBadRequest, ErrorBody{Error: "offset out of range"}, "invalid: offset out of range"},
		"unknown": {errors.New("disk failed"), http.StatusInternalServerError, ErrorBody{Error: "internal error"}, "internal error"},
	}

	for label, test := range cases {
		endpoint := Define(Endpoint{Name: "refuse", Transports: TransportREST}, func(context.Context, Call, struct{}) (struct{}, error) {
			return struct{}{}, test.err
		})
		caller := func(http.ResponseWriter, *http.Request) (Call, bool) { return Call{Transport: TransportREST}, true }
		response := httptest.NewRecorder()
		serve(slog.New(slog.DiscardHandler), TransportREST, endpoint, caller).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/refuse", nil))

		var body ErrorBody

		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != test.status || !reflect.DeepEqual(body, test.want) {
			t.Errorf("%s: %d %s", label, response.Code, response.Body)
		}

		if text := OutcomeOf(test.err).ToolText(); text != test.text {
			t.Errorf("%s: MCP text %q", label, text)
		}
	}
}
