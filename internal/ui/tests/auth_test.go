package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/storetest"

	"github.com/devforth/exceptic/internal/platform/config"
)

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

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {"wrong password!"}}); response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", response.Code)
	}

	server.login()

	if !strings.Contains(server.page("/issues"), "<h1>Issues</h1>") {
		t.Fatal("issues page after login")
	}

	cookie := server.cookie

	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags: %+v", cookie)
	}

	if response := server.post("/logout", nil); response.Code != http.StatusSeeOther {
		t.Fatalf("logout: %d", response.Code)
	}

	// The old cookie no longer works: the session is deleted on the server.
	server.cookie = cookie

	if response := server.get("/issues"); response.Code != http.StatusSeeOther {
		t.Fatalf("after logout: %d", response.Code)
	}
}

// ADMIN_USERNAME/ADMIN_PASSWORD create the superadmin at the first start
// only: later starts with other values change nothing, and sessions survive
// restarts.
func TestSuperadminFromEnvOnce(t *testing.T) {
	database := storetest.Open(t)

	first := startServer(t, database, "admin", password)
	first.login()

	for _, credentials := range [][2]string{{"admin", password}, {"admin", "another password"}, {"root", "another password"}} {
		restarted := startServer(t, database, credentials[0], credentials[1])
		restarted.cookie = first.cookie

		if response := restarted.get("/issues"); response.Code != http.StatusOK {
			t.Fatalf("restart with %q: %d", credentials[0], response.Code)
		}

		if response := restarted.post("/login", url.Values{"username": {credentials[0]}, "password": {"another password"}}); response.Code != http.StatusUnauthorized {
			t.Fatalf("the env password of a restart logs in: %d", response.Code)
		}
	}

	if users := first.page("/users"); strings.Count(users, `href="/users/`) != 1 || strings.Contains(users, ">root<") {
		t.Errorf("a restart created a user:\n%s", users)
	}
}

func TestLoginRedirectStaysOnSite(t *testing.T) {
	server := newTestServer(t, password)

	for next, want := range map[string]string{
		"/issues/5":          "/issues/5",
		"https://evil.test/": "/issues",
		"//evil.test/":       "/issues",
		"/\\evil.test":       "/issues",
	} {
		response := server.post("/login", url.Values{"username": {"admin"}, "password": {password}, "next": {next}})

		if location := response.Header().Get("Location"); location != want {
			t.Errorf("next=%q: redirected to %q, want %q", next, location, want)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	server := newTestServer(t, password)

	for range 10 {
		server.post("/login", url.Values{"username": {"admin"}, "password": {"guess guess guess"}})
	}

	// Even the right password is refused while the address is blocked.
	response := server.post("/login", url.Values{"username": {"admin"}, "password": {password}})

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", response.Code)
	}
}

// Guessing one account's password from many addresses is limited too, but
// those guesses cannot lock its owner out: a browser that logged in as the
// username before still may, typos included.
func TestLoginRateLimitPerUsername(t *testing.T) {
	server := newTestServerWith(t, func(settings *config.Config) {
		settings.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest's peer
	})

	from := func(index int, headers ...string) []string {
		return append([]string{"X-Forwarded-For", fmt.Sprintf("203.0.113.%d", index)}, headers...)
	}

	var device *http.Cookie

	for _, cookie := range server.post("/login", url.Values{"username": {"admin"}, "password": {password}}, from(100)...).Result().Cookies() {
		if cookie.Name == "exceptic_device" {
			device = cookie
		}
	}

	if device == nil {
		t.Fatal("login did not mark the browser")
	}

	// The guesses use up the limits of the username and of the owner's network (a shared NAT).
	for range 10 {
		server.post("/login", url.Values{"username": {"Admin"}, "password": {"guess guess guess"}}, from(0)...)
	}

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {password}}, from(50)...); response.Code != http.StatusTooManyRequests {
		t.Fatalf("right password from a new browser: %d, want 429", response.Code)
	}

	owner := from(0, "Cookie", device.Name+"="+device.Value)

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {"typo"}}, owner...); response.Code != http.StatusUnauthorized {
		t.Fatalf("typo from the owner's browser: %d, want 401", response.Code)
	}

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {password}}, owner...); response.Code != http.StatusSeeOther {
		t.Fatalf("right password from the owner's browser: %d, want 303", response.Code)
	}

	if response := server.post("/login", url.Values{"username": {"dev"}, "password": {"guess guess guess"}}, from(51)...); response.Code == http.StatusTooManyRequests {
		t.Fatal("another username is blocked")
	}
}

// The device cookie stops counting once the password changes, and the
// owner's own change gives the browser a new one.
func TestDeviceCookieFollowsPassword(t *testing.T) {
	server := newTestServerWith(t, func(settings *config.Config) {
		settings.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest's peer
	})

	from := func(index int, headers ...string) []string {
		return append([]string{"X-Forwarded-For", fmt.Sprintf("203.0.113.%d", index)}, headers...)
	}
	cookies := func(response *httptest.ResponseRecorder) map[string]*http.Cookie {
		named := map[string]*http.Cookie{}

		for _, cookie := range response.Result().Cookies() {
			named[cookie.Name] = cookie
		}

		return named
	}

	login := cookies(server.post("/login", url.Values{"username": {"admin"}, "password": {password}}, from(100)...))
	old := login["exceptic_device"]
	server.cookie = login[session.CookieName]

	const changed = "changed password 123"

	response := server.post("/account/password", url.Values{"current_password": {password}, "password": {changed}, "confirm_password": {changed}},
		from(100, "Cookie", old.Name+"="+old.Value)...)
	renewed := cookies(response)["exceptic_device"]

	if response.Code != http.StatusSeeOther || renewed == nil || renewed.Value == old.Value {
		t.Fatalf("own password change: %d, device cookie %v", response.Code, renewed)
	}

	server.cookie = nil

	for range 10 {
		server.post("/login", url.Values{"username": {"admin"}, "password": {"guess guess guess"}}, from(0)...)
	}

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {changed}}, from(0, "Cookie", old.Name+"="+old.Value)...); response.Code != http.StatusTooManyRequests {
		t.Fatalf("the cookie from before the password change: %d, want 429", response.Code)
	}

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {changed}}, from(0, "Cookie", renewed.Name+"="+renewed.Value)...); response.Code != http.StatusSeeOther {
		t.Fatalf("the renewed cookie: %d, want 303", response.Code)
	}
}

func TestUIDisabledWithoutPassword(t *testing.T) {
	server := newTestServer(t, "")

	if body := server.page("/login"); !strings.Contains(body, "ADMIN_PASSWORD") {
		t.Fatal("login page must explain that the UI is disabled")
	}

	if response := server.post("/login", url.Values{"username": {"admin"}, "password": {""}}); response.Code != http.StatusForbidden {
		t.Fatalf("login with empty password: %d", response.Code)
	}
}

func TestCrossSiteRequestsRejected(t *testing.T) {
	server := newTestServer(t, password)
	server.ingestFixtures()
	server.login()

	id := issueIDs(server.page("/issues"))[0]

	response := server.post("/issues/"+id+"/action", url.Values{"action": {"resolve"}}, "Origin", "https://evil.test")

	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-site status change: %d", response.Code)
	}

	response = server.post("/issues/"+id+"/action", url.Values{"action": {"resolve"}}, "Origin", "http://example.com")

	if response.Code != http.StatusSeeOther {
		t.Fatalf("same-site status change: %d", response.Code)
	}
}

// Logging in again ends the session the browser had: a session cookie set
// by someone else (fixation) is never kept.
func TestLoginReplacesSession(t *testing.T) {
	server := newTestServer(t, password)
	server.login()

	previous := server.cookie
	server.login()

	if server.cookie.Value == previous.Value {
		t.Fatal("login must issue a new session token")
	}

	server.cookie = previous

	if response := server.get("/issues"); response.Code != http.StatusSeeOther {
		t.Fatalf("the previous session still works: %d", response.Code)
	}
}

// Every dashboard route asks for a login, pages and actions alike; only the
// login itself and static files are open. The web app's pages (/projects)
// hold no data: its requests ask for the login (TestUIRequiresSession).
func TestDashboardNeedsLogin(t *testing.T) {
	server := newTestServer(t, password)

	for _, path := range []string{"/", "/issues", "/issues/1", "/logs", "/projects/1", "/notifications", "/notifications/log", "/notifications/1", "/project-colors.css"} {
		if response := server.get(path); response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), "/login") {
			t.Errorf("GET %s: %d %q", path, response.Code, response.Header().Get("Location"))
		}
	}

	for _, path := range []string{"/issues/bulk", "/projects", "/projects/1/delete", "/notifications", "/notifications/1/retry", "/notifications/email/start"} {
		if response := server.post(path, url.Values{}); response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), "/login") {
			t.Errorf("POST %s: %d %q", path, response.Code, response.Header().Get("Location"))
		}
	}

	if response := server.get("/login"); response.Code != http.StatusOK {
		t.Errorf("the login page must be open: %d", response.Code)
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
		"/", "/login", "/logout", "/account", "/issues", "/logs", "/projects", "/projects/cleanups", "/notifications", "/users",
		"/agents", "/static/app.js", "/project-colors.css", "/no-such-page", "/mcp", "/oauth/authorize", "/oauth/token",
		"/oauth/register", "/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource/mcp",
		"/.well-known/openid-configuration", "/api/v1/list_issues", "/api/unknown", "/no-such-page/deeper", "/_app/immutable/x.js",
		"/api/ui/list_projects", "/api/ui/update_issue_status", "/api/ui/nope",
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

// Only the right password makes a browser a known device; behind an HTTPS
// proxy the session and device cookies are Secure.
func TestDeviceCookieOnlyAfterSuccess(t *testing.T) {
	server := newTestServerWith(t, func(settings *config.Config) {
		settings.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest's peer
	})
	cookies := func(response *httptest.ResponseRecorder) map[string]*http.Cookie {
		named := map[string]*http.Cookie{}

		for _, cookie := range response.Result().Cookies() {
			named[cookie.Name] = cookie
		}

		return named
	}

	failed := server.post("/login", url.Values{"username": {"admin"}, "password": {"wrong password"}})

	if _, found := cookies(failed)["exceptic_device"]; found {
		t.Fatal("a wrong password set the device cookie")
	}

	server.login()

	wrongCurrent := server.post("/account/password", url.Values{"current_password": {"wrong"}, "password": {"new password 456"}, "confirm_password": {"new password 456"}})

	if _, found := cookies(wrongCurrent)["exceptic_device"]; found {
		t.Fatal("a wrong current password set the device cookie")
	}

	for proto, secure := range map[string]bool{"https": true, "http": false} {
		response := server.post("/login", url.Values{"username": {"admin"}, "password": {password}}, "X-Forwarded-Proto", proto)
		named := cookies(response)

		for _, name := range []string{session.CookieName, "exceptic_device"} {
			if cookie := named[name]; cookie == nil || cookie.Secure != secure {
				t.Errorf("%s over %s: %+v, want Secure %v", name, proto, cookie, secure)
			}
		}
	}
}
