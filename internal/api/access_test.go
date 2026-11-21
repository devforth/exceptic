package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
)

// ALLOWED_IPS covers every route of the web app, its API, the REST API, MCP
// and OAuth, whatever the method; only /health and ingest stay open.
func TestAllowedIPsCoverEveryRoute(t *testing.T) {
	server := newRESTServerWith(t, func(settings *config.Config) {
		settings.AllowedIPs = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
		settings.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest's peer
	})
	paths := []string{
		"/", "/login", "/account", "/issues", "/logs", "/projects", "/notifications", "/agents", "/users",
		"/no-such-page", "/no-such-page/deeper", "/_app/immutable/x.js", "/mcp", "/oauth/authorize", "/oauth/token",
		"/oauth/register", "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/openid-configuration", "/api/v1/list_issues", "/api/unknown", "/api/ui/list_projects",
		"/api/ui/apply_issue_action", "/api/ui/nope", "/api/ui/login", "/api/ui/login_state", "/api/ui/logout",
		"/api/ui/change_password", "/api/ui/personal_token", "/api/ui/authorization_request",
	}

	send := func(method, path, client string) int {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("X-Forwarded-For", client)

		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)

		return response.Code
	}

	for _, path := range paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodHead, http.MethodOptions} {
			for _, client := range []string{"203.0.113.5", "10.1.1.1, 203.0.113.5"} {
				if code := send(method, path, client); code != http.StatusForbidden {
					t.Errorf("%s %s from %s: %d, want 403", method, path, client, code)
				}
			}
		}

		if code := send(http.MethodGet, path, "203.0.113.5, 10.1.1.1"); code == http.StatusForbidden {
			t.Errorf("GET %s from 10.1.1.1: 403", path)
		}
	}

	if code := send(http.MethodGet, "/health", "203.0.113.5"); code != http.StatusOK {
		t.Errorf("/health: %d", code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/1/envelope/", strings.NewReader("{}\n"))
	request.Header.Set("X-Forwarded-For", "203.0.113.5")

	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)

	if response.Code == http.StatusForbidden {
		t.Error("ingest must stay open")
	}
}
