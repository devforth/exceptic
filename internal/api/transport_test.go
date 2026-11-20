package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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

// Names are path segments: an endpoint must not shadow an ingest path (/api/<project>/envelope).
func TestEndpointNames(t *testing.T) {
	name := regexp.MustCompile(`^[a-z][a-z_]*$`)

	for _, endpoint := range append(Endpoints(nil, nil), fetchSkill) {
		if !name.MatchString(endpoint.Name) || endpoint.Name == "envelope" {
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

// The own password and personal tokens are never requests of the catalog:
// they go through the login's limits (session.Auth) and its pages.
func TestCatalogNeverChangesCredentials(t *testing.T) {
	files, err := filepath.Glob("*.go")

	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		source, err := os.ReadFile(file)

		if err != nil {
			t.Fatal(err)
		}

		for _, method := range []string{"ChangeOwnPassword", "NewPersonalToken"} {
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
	handler := NewUI(slog.New(slog.DiscardHandler), Endpoints(nil, nil), failingSessions{}, "", nil)
	router := http.NewServeMux()
	handler.Register(router, func(next http.Handler) http.Handler { return next })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/ui/list_projects", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
}
