package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

// adminPassword is the password of the superadmin "admin" of newLoginServer.
const adminPassword = "correct horse battery"

// The slow password key derivation would take seconds per login under the race detector.
func init() {
	services.PasswordIterations = 1_000
}

// newLoginServer is a server whose superadmin "admin" logs in with adminPassword.
func newLoginServer(t *testing.T, configure ...func(settings *config.Config)) *restServer {
	t.Helper()

	return startLoginServer(t, storetest.Open(t), "admin", adminPassword, configure...)
}

func startLoginServer(t *testing.T, database *database.Store, username, password string, configure ...func(settings *config.Config)) *restServer {
	t.Helper()

	withAdmin := func(settings *config.Config) {
		settings.AdminUsername, settings.AdminPassword = username, password
		settings.SessionLifetime = config.DefaultSessionLifetime
	}

	return startRESTServer(t, database, append([]func(*config.Config){withAdmin}, configure...)...)
}

// behindProxy trusts httptest's peer, so a test picks the client address with X-Forwarded-For.
func behindProxy(settings *config.Config) {
	settings.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
}

// browser is a browser of the web app: it keeps the cookies the server sets.
type browser struct {
	server  *restServer
	cookies map[string]*http.Cookie
	headers []string // name, value, …: sent with every request
}

func (server *restServer) browser(headers ...string) *browser {
	return &browser{server: server, cookies: map[string]*http.Cookie{}, headers: headers}
}

// from is the client address of a browser behind behindProxy.
func from(index int) []string {
	return []string{"X-Forwarded-For", fmt.Sprintf("203.0.113.%d", index)}
}

// call sends a request as the web app does (JSON with the page's Origin for
// a change); target is a name with its query, if any.
func (client *browser) call(method, target string, input any, headers ...string) *httptest.ResponseRecorder {
	client.server.t.Helper()

	body := ""

	if input != nil {
		encoded, err := json.Marshal(input)

		if err != nil {
			client.server.t.Fatal(err)
		}

		body = string(encoded)
	}

	request := httptest.NewRequest(method, "/api/ui/"+target, strings.NewReader(body))

	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", uiOrigin)
	}

	for _, cookie := range client.cookies {
		request.AddCookie(cookie)
	}

	all := append(append([]string{}, client.headers...), headers...)

	for index := 0; index+1 < len(all); index += 2 {
		request.Header.Set(all[index], all[index+1])
	}

	response := serve(client.server, request)

	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(client.cookies, cookie.Name)
		} else {
			client.cookies[cookie.Name] = cookie
		}
	}

	return response
}

func (client *browser) login(username, password string, headers ...string) *httptest.ResponseRecorder {
	return client.call(http.MethodPost, "login", map[string]string{"username": username, "password": password}, headers...)
}

// loggedIn logs in and fails unless it worked.
func (client *browser) loggedIn(username, password string) *browser {
	client.server.t.Helper()

	if response := client.login(username, password); response.Code != http.StatusOK {
		client.server.t.Fatalf("login as %s: %d %s", username, response.Code, response.Body)
	}

	return client
}

// open reports whether the browser's session opens the web app's API.
func (client *browser) open() bool {
	return client.call(http.MethodGet, "get_session", nil).Code == http.StatusOK
}

func (client *browser) changePassword(current, password string, headers ...string) *httptest.ResponseRecorder {
	return client.call(http.MethodPost, "change_password",
		map[string]string{"current_password": current, "password": password, "confirm_password": password}, headers...)
}

// errorOf is the body of a refused request.
func errorOf(t *testing.T, response *httptest.ResponseRecorder) api.ErrorBody {
	t.Helper()

	var body api.ErrorBody

	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("%d %s: %v", response.Code, response.Body, err)
	}

	return body
}

func TestUILogin(t *testing.T) {
	server := newLoginServer(t)
	client := server.browser()

	// A wrong password and an unknown username get the same answer.
	wrong, unknown := client.login("admin", "wrong password!"), client.login("nobody", "wrong password!")

	if wrong.Code != http.StatusUnauthorized || wrong.Body.String() != unknown.Body.String() || unknown.Code != wrong.Code ||
		errorOf(t, wrong).Error != "Wrong username or password." {
		t.Fatalf("wrong password: %d %s, unknown user: %d %s", wrong.Code, wrong.Body, unknown.Code, unknown.Body)
	}

	if client.open() {
		t.Fatal("a failed login opened the app")
	}

	// Another site cannot log the browser in (login CSRF): Origin and Fetch Metadata.
	for _, headers := range [][]string{{"Origin", "https://evil.example"}, {"Sec-Fetch-Site", "cross-site"}, {"Content-Type", "text/plain"}} {
		if response := client.login("admin", adminPassword, headers...); response.Code < 400 || client.cookies[session.CookieName] != nil {
			t.Fatalf("login with %v: %d", headers, response.Code)
		}
	}

	response := client.login("admin", adminPassword)

	var output api.LoginOutput

	if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil || response.Code != http.StatusOK || output.Next != "/issues" {
		t.Fatalf("login: %d %s", response.Code, response.Body)
	}

	cookie := client.cookies[session.CookieName]

	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || client.cookies["exceptic_device"] == nil {
		t.Fatalf("cookies: %+v", client.cookies)
	}

	if !client.open() {
		t.Fatal("the login did not open the app")
	}

	// Logout ends the session on the server: the old cookie no longer works.
	if response := client.call(http.MethodPost, "logout", struct{}{}); response.Code != http.StatusNoContent || client.cookies[session.CookieName] != nil {
		t.Fatalf("logout: %d, cookies %v", response.Code, client.cookies)
	}

	client.cookies[session.CookieName] = cookie

	if client.open() {
		t.Fatal("the session survived the logout")
	}

	// Also without a session, as a second click does.
	if response := server.browser().call(http.MethodPost, "logout", struct{}{}); response.Code != http.StatusNoContent {
		t.Fatalf("logout without a session: %d", response.Code)
	}

	// The login is the web app's: not a request of the REST API.
	if response := server.do(http.MethodPost, "/api/v1/login", `{"username":"admin","password":"x"}`); response.Code != http.StatusNotFound {
		t.Errorf("REST login: %d", response.Code)
	}
}

// Only a local path is followed after the login.
func TestUILoginRedirectStaysOnSite(t *testing.T) {
	server := newLoginServer(t)

	for next, want := range map[string]string{
		"/logs?level=error": "/logs?level=error",
		"":                  "/issues",
		"https://evil.test": "/issues",
		"//evil.test/":      "/issues",
		"/\\evil.test":      "/issues",
		"/a b":              "/issues",
	} {
		client := server.browser()
		response := client.call(http.MethodPost, "login", map[string]string{"username": "admin", "password": adminPassword, "next": next})

		var output api.LoginOutput

		if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil || output.Next != want {
			t.Errorf("next=%q: %d %s, want %q", next, response.Code, response.Body, want)
		}

		// A logged-in browser is sent on by the login page.
		var state api.LoginState

		if err := json.Unmarshal(client.call(http.MethodGet, "login_state?next="+url.QueryEscape(next), nil).Body.Bytes(), &state); err != nil || state.Redirect != want {
			t.Errorf("login_state next=%q: %+v, want %q", next, state, want)
		}
	}

	var state api.LoginState

	if err := json.Unmarshal(server.browser().call(http.MethodGet, "login_state?next=/logs", nil).Body.Bytes(), &state); err != nil ||
		state.Redirect != "" || state.Disabled || state.AppName != server.app.Services.AppName {
		t.Errorf("login_state without a session: %+v", state)
	}
}

// Logging in again ends the session the browser had: a session cookie set
// by someone else (fixation) is never kept.
func TestUILoginReplacesSession(t *testing.T) {
	server := newLoginServer(t)
	client := server.browser().loggedIn("admin", adminPassword)
	previous := client.cookies[session.CookieName]

	client.loggedIn("admin", adminPassword)

	if client.cookies[session.CookieName].Value == previous.Value {
		t.Fatal("login must issue a new session token")
	}

	client.cookies[session.CookieName] = previous

	if client.open() {
		t.Fatal("the previous session still works")
	}
}

func TestUILoginRateLimit(t *testing.T) {
	server := newLoginServer(t)
	client := server.browser()

	for range session.MaxLoginFailures {
		client.login("admin", "guess guess guess")
	}

	// Even the right password is refused while the address is blocked.
	if response := client.login("admin", adminPassword); response.Code != http.StatusTooManyRequests || errorOf(t, response).Error != session.TooManyAttempts {
		t.Fatalf("got %d %s, want 429", response.Code, response.Body)
	}
}

// Guessing one account's password from many addresses is limited too, but
// those guesses cannot lock its owner out: a browser that logged in as the
// username before still may, typos included.
func TestUILoginRateLimitPerUsername(t *testing.T) {
	server := newLoginServer(t, behindProxy)
	owner := server.browser(from(0)...)

	owner.login("admin", adminPassword, from(100)...)

	if owner.cookies["exceptic_device"] == nil {
		t.Fatal("login did not mark the browser")
	}

	// The guesses use up the limits of the username and of the owner's network (a shared NAT).
	for range session.MaxLoginFailures {
		server.browser(from(0)...).login("Admin", "guess guess guess")
	}

	if response := server.browser(from(50)...).login("admin", adminPassword); response.Code != http.StatusTooManyRequests {
		t.Fatalf("right password from a new browser: %d, want 429", response.Code)
	}

	if response := owner.login("admin", "typo"); response.Code != http.StatusUnauthorized {
		t.Fatalf("typo from the owner's browser: %d, want 401", response.Code)
	}

	if response := owner.login("admin", adminPassword); response.Code != http.StatusOK {
		t.Fatalf("right password from the owner's browser: %d, want 200", response.Code)
	}

	if response := server.browser(from(51)...).login("dev", "guess guess guess"); response.Code == http.StatusTooManyRequests {
		t.Fatal("another username is blocked")
	}
}

// The device cookie stops counting once the password changes, and the
// owner's own change gives the browser a new one.
func TestUIDeviceCookieFollowsPassword(t *testing.T) {
	server := newLoginServer(t, behindProxy)
	owner := server.browser(from(100)...).loggedIn("admin", adminPassword)
	old := *owner.cookies["exceptic_device"]

	const changed = "changed password 123"

	if response := owner.changePassword(adminPassword, changed); response.Code != http.StatusNoContent || owner.cookies["exceptic_device"].Value == old.Value {
		t.Fatalf("own password change: %d %s, device cookie %v", response.Code, response.Body, owner.cookies["exceptic_device"])
	}

	renewed := *owner.cookies["exceptic_device"]

	for range session.MaxLoginFailures {
		server.browser(from(0)...).login("admin", "guess guess guess")
	}

	withDevice := func(device http.Cookie) *browser {
		client := server.browser(from(0)...)
		client.cookies[device.Name] = &device

		return client
	}

	if response := withDevice(old).login("admin", changed); response.Code != http.StatusTooManyRequests {
		t.Fatalf("the cookie from before the password change: %d, want 429", response.Code)
	}

	if response := withDevice(renewed).login("admin", changed); response.Code != http.StatusOK {
		t.Fatalf("the renewed cookie: %d, want 200", response.Code)
	}
}

// Only the right password makes a browser a known device; behind an HTTPS
// proxy the session and device cookies are Secure.
func TestUIDeviceCookieOnlyAfterSuccess(t *testing.T) {
	server := newLoginServer(t, behindProxy)
	client := server.browser()

	if client.login("admin", "wrong password"); client.cookies["exceptic_device"] != nil {
		t.Fatal("a wrong password set the device cookie")
	}

	client.loggedIn("admin", adminPassword)
	delete(client.cookies, "exceptic_device")

	if client.changePassword("wrong", "new password 456"); client.cookies["exceptic_device"] != nil {
		t.Fatal("a wrong current password set the device cookie")
	}

	for proto, secure := range map[string]bool{"https": true, "http": false} {
		other := server.browser("X-Forwarded-Proto", proto, "Origin", proto+"://example.com")
		other.loggedIn("admin", adminPassword)

		for _, name := range []string{session.CookieName, "exceptic_device"} {
			if cookie := other.cookies[name]; cookie == nil || cookie.Secure != secure {
				t.Errorf("%s over %s: %+v, want Secure %v", name, proto, cookie, secure)
			}
		}
	}
}

// Without a superadmin nobody can log in; the login page says why.
func TestUIDisabledWithoutPassword(t *testing.T) {
	server := startLoginServer(t, storetest.Open(t), "admin", "")
	client := server.browser()

	var state api.LoginState

	if err := json.Unmarshal(client.call(http.MethodGet, "login_state", nil).Body.Bytes(), &state); err != nil || !state.Disabled {
		t.Fatalf("login_state: %+v %v", state, err)
	}

	if response := client.login("admin", ""); response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "ADMIN_PASSWORD") {
		t.Fatalf("login: %d %s", response.Code, response.Body)
	}
}

// ADMIN_USERNAME/ADMIN_PASSWORD create the superadmin at the first start
// only: later starts with other values change nothing, and sessions survive
// restarts.
func TestUISuperadminFromEnvOnce(t *testing.T) {
	database := storetest.Open(t)
	first := startLoginServer(t, database, "admin", adminPassword).browser().loggedIn("admin", adminPassword)

	for _, credentials := range [][2]string{{"admin", adminPassword}, {"admin", "another password"}, {"root", "another password"}} {
		restarted := startLoginServer(t, database, credentials[0], credentials[1])
		first.server = restarted

		if !first.open() {
			t.Fatalf("restart with %q: the session ended", credentials[0])
		}

		if response := restarted.browser().login(credentials[0], "another password"); response.Code != http.StatusUnauthorized {
			t.Fatalf("the env password of a restart logs in: %d", response.Code)
		}
	}

	users, err := first.server.app.Services.Users.List(context.Background())

	if err != nil || len(users) != 1 || users[0].Username != "admin" {
		t.Errorf("a restart created a user: %+v %v", users, err)
	}
}

// The own password: the current one is checked against the login limit;
// this session stays, the others end, connected agents keep working.
func TestUIChangePassword(t *testing.T) {
	server := newLoginServer(t)

	if response := server.browser().changePassword(adminPassword, "new password 456"); response.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", response.Code)
	}

	client := server.browser().loggedIn("admin", adminPassword)
	other := server.browser().loggedIn("admin", adminPassword)
	root := superadmin(t, server)
	token, err := server.app.OAuth.NewPersonalToken(context.Background(), root)

	if err != nil {
		t.Fatal(err)
	}

	for label, test := range map[string]struct {
		input map[string]string
		field string
	}{
		"wrong current": {map[string]string{"current_password": "wrong", "password": "new password 456", "confirm_password": "new password 456"}, "current_password"},
		"mismatch":      {map[string]string{"current_password": adminPassword, "password": "new password 456", "confirm_password": "new password 457"}, "confirm_password"},
		"same":          {map[string]string{"current_password": adminPassword, "password": adminPassword, "confirm_password": adminPassword}, "password"},
		"short":         {map[string]string{"current_password": adminPassword, "password": "eleven char", "confirm_password": "eleven char"}, "password"},
	} {
		response := client.call(http.MethodPost, "change_password", test.input)

		if body := errorOf(t, response); response.Code != http.StatusBadRequest || body.Fields[test.field] == "" {
			t.Errorf("%s: %d %s", label, response.Code, response.Body)
		}
	}

	if response := client.changePassword(adminPassword, "new password 456"); response.Code != http.StatusNoContent {
		t.Fatalf("change: %d %s", response.Code, response.Body)
	}

	if !client.open() || other.open() {
		t.Errorf("after the change: this session %v, the other %v", client.open(), other.open())
	}

	if response := mcpTools(server, token); response.Code != http.StatusOK {
		t.Errorf("the agent was disconnected: %d", response.Code)
	}

	// A stolen session must not become a way to guess the password.
	for range session.MaxLoginFailures {
		client.changePassword("guess guess guess", "another password 789")
	}

	if response := client.changePassword("new password 456", "another password 789"); errorOf(t, response).Fields["current_password"] != session.TooManyAttempts {
		t.Errorf("after the guesses: %d %s", response.Code, response.Body)
	}
}

// A temporary password opens only get_session, change_password and logout;
// choosing a password opens the rest.
func TestUITemporaryPassword(t *testing.T) {
	server := newLoginServer(t)
	admin := server.browser().loggedIn("admin", adminPassword)

	var created struct {
		User              struct{ ID string } `json:"user"`
		TemporaryPassword string              `json:"temporary_password"`
	}

	if err := json.Unmarshal(admin.call(http.MethodPost, "create_user", map[string]string{"username": "dev", "role": "user"}).Body.Bytes(), &created); err != nil || created.TemporaryPassword == "" {
		t.Fatalf("create_user: %+v %v", created, err)
	}

	user := server.browser().loggedIn("dev", created.TemporaryPassword)

	var session sessionAnswer

	if err := json.Unmarshal(user.call(http.MethodGet, "get_session", nil).Body.Bytes(), &session); err != nil || !session.User.MustChangePassword {
		t.Fatalf("get_session: %+v %v", session, err)
	}

	if response := user.call(http.MethodPost, "update_account_name", map[string]string{"name": "Dev"}); response.Code != http.StatusNotFound {
		t.Errorf("update_account_name with a temporary password: %d", response.Code)
	}

	if response := user.changePassword(created.TemporaryPassword, "user password 123"); response.Code != http.StatusNoContent {
		t.Fatalf("change_password: %d %s", response.Code, response.Body)
	}

	session = sessionAnswer{}

	if err := json.Unmarshal(user.call(http.MethodGet, "get_session", nil).Body.Bytes(), &session); err != nil || session.User.MustChangePassword {
		t.Fatalf("after the change: %+v %v", session, err)
	}

	if response := user.call(http.MethodPost, "update_account_name", map[string]string{"name": "Dev"}); response.Code != http.StatusOK {
		t.Errorf("update_account_name: %d %s", response.Code, response.Body)
	}

	if response := user.call(http.MethodPost, "logout", struct{}{}); response.Code != http.StatusNoContent || user.open() {
		t.Errorf("logout: %d", response.Code)
	}
}

// superadmin is the id of the superadmin "admin".
func superadmin(t *testing.T, server *restServer) string {
	t.Helper()

	user, err := server.database.FindUser(context.Background(), "admin")

	if err != nil {
		t.Fatal(err)
	}

	return user.ID
}

// mcpTools lists the MCP tools with an agent's token.
func mcpTools(server *restServer, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, api.MCPPath, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+token)

	return serve(server, request)
}
