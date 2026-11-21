package tests

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
)

// The old pages send a browser without a session to the login, and their
// Log out ends the session.
func TestLogin(t *testing.T) {
	server := newTestServer(t, password)

	if response := server.get("/issues"); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?next=%2Fissues" {
		t.Fatalf("anonymous: %d %q", response.Code, response.Header().Get("Location"))
	}

	htmx := httptest.NewRequest(http.MethodGet, "/issues?status=all", nil)
	htmx.Header.Set("HX-Request", "true")

	if response := server.send(htmx); response.Code != http.StatusUnauthorized || !strings.HasPrefix(response.Header().Get("HX-Redirect"), "/login") {
		t.Fatalf("anonymous htmx: %d %q", response.Code, response.Header().Get("HX-Redirect"))
	}

	server.login()

	if !strings.Contains(server.page("/issues"), "<h1>Issues</h1>") {
		t.Fatal("issues page after login")
	}

	cookie := server.cookie

	// The old pages' Log out (the web app's is a request of its API, TestUILogin).
	if response := server.post("/logout", nil); response.Code != http.StatusSeeOther {
		t.Fatalf("logout: %d", response.Code)
	}

	// The old cookie no longer works: the session is deleted on the server.
	server.cookie = cookie

	if response := server.get("/issues"); response.Code != http.StatusSeeOther {
		t.Fatalf("after logout: %d", response.Code)
	}
}

func TestCrossSiteRequestsRejected(t *testing.T) {
	server := newTestServer(t, password)
	server.ingestFixtures()
	server.login()

	id := server.unresolvedIDs()[0]

	response := server.post("/issues/"+id+"/action", url.Values{"action": {"resolve"}}, "Origin", "https://evil.test")

	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-site status change: %d", response.Code)
	}

	response = server.post("/issues/"+id+"/action", url.Values{"action": {"resolve"}}, "Origin", "http://example.com")

	if response.Code != http.StatusSeeOther {
		t.Fatalf("same-site status change: %d", response.Code)
	}
}

// Every dashboard route asks for a login, pages and actions alike; only
// static files are open. The web app's pages (/projects, /login) hold no
// data: its requests ask for the login (TestUIRequiresSession).
func TestDashboardNeedsLogin(t *testing.T) {
	server := newTestServer(t, password)

	for _, path := range []string{"/", "/issues", "/issues/1", "/projects/1", "/project-colors.css"} {
		if response := server.get(path); response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), "/login") {
			t.Errorf("GET %s: %d %q", path, response.Code, response.Header().Get("Location"))
		}
	}

	for _, path := range []string{"/issues/bulk", "/projects", "/projects/1/delete"} {
		if response := server.post(path, url.Values{}); response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), "/login") {
			t.Errorf("POST %s: %d %q", path, response.Code, response.Header().Get("Location"))
		}
	}
}

// A form sent after the session ended returns, after the login, to the page
// it was sent from: the login opens it with a GET, which a POST address
// answers with a 404.
func TestLoginAfterExpiredFormReturnsToPage(t *testing.T) {
	server := newTestServer(t, password)

	for _, test := range []struct {
		headers []string
		want    string
	}{
		{[]string{"Referer", "http://example.com/issues?status=all"}, "/login?next=" + url.QueryEscape("/issues?status=all")},
		{[]string{"HX-Current-URL", "http://example.com/logs?page=2", "Referer", "http://example.com/issues"}, "/login?next=" + url.QueryEscape("/logs?page=2")},
		{nil, "/login?next=%2Fissues"},
	} {
		response := server.post("/issues/bulk", url.Values{"action": {"resolve"}}, test.headers...)

		if location := response.Header().Get("Location"); location != test.want {
			t.Errorf("headers %v: redirected to %q, want %q", test.headers, location, test.want)
		}
	}
}

// ALLOWED_IPS covers every route of the UI, MCP and OAuth, whatever the
// method; only /health and ingest stay open.
func TestAllowedIPsCoverEveryRoute(t *testing.T) {
	server := newTestServerWith(t, func(settings *config.Config) {
		settings.AllowedIPs = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
		settings.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest's peer
	})
	paths := []string{
		"/", "/login", "/logout", "/account", "/issues", "/logs", "/projects", "/projects/cleanups", "/notifications",
		"/agents", "/static/app.js", "/project-colors.css", "/no-such-page", "/mcp", "/oauth/authorize", "/oauth/token",
		"/oauth/register", "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/openid-configuration", "/api/v1/list_issues", "/api/unknown", "/no-such-page/deeper", "/_app/immutable/x.js",
		"/api/ui/list_projects", "/api/ui/update_issue_status", "/api/ui/nope", "/api/ui/login", "/api/ui/login_state",
		"/api/ui/logout", "/api/ui/change_password", "/users", "/users/new",
	}

	for _, path := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodOptions} {
			for _, client := range []string{"203.0.113.5", "10.1.1.1, 203.0.113.5"} {
				request := httptest.NewRequest(method, path, nil)
				request.Header.Set("X-Forwarded-For", client)

				if response := server.send(request); response.Code != http.StatusForbidden {
					t.Errorf("%s %s from %s: %d, want 403", method, path, client, response.Code)
				}
			}
		}

		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Forwarded-For", "203.0.113.5, 10.1.1.1")

		if response := server.send(request); response.Code == http.StatusForbidden {
			t.Errorf("GET %s from 10.1.1.1: 403", path)
		}
	}

	if response := server.get("/health"); response.Code != http.StatusOK {
		t.Errorf("/health: %d", response.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/1/envelope/", strings.NewReader("{}\n"))
	request.Header.Set("X-Forwarded-For", "203.0.113.5")

	if response := server.send(request); response.Code == http.StatusForbidden {
		t.Error("ingest must stay open")
	}
}
