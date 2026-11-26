package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

// envPassword is ADMIN_PASSWORD of newSetupServer, whose ADMIN_EMAIL is "admin".
const envPassword = "env password 123"

const setupCookie = "exceptic_setup"

// newSetupServer is a server without an admin, with ADMIN_EMAIL and ADMIN_PASSWORD set.
func newSetupServer(t *testing.T, configure ...func(settings *config.Config)) *restServer {
	t.Helper()

	withEnv := func(settings *config.Config) {
		settings.AdminEmail, settings.AdminPassword = "admin", envPassword
		settings.SessionLifetime = config.DefaultSessionLifetime
	}

	return startRESTServer(t, storetest.Open(t), append([]func(*config.Config){withEnv}, configure...)...)
}

func (client *browser) setupAdmin(email, password, confirm string, headers ...string) *httptest.ResponseRecorder {
	return client.call(http.MethodPost, "setup_admin", map[string]string{"email": email, "password": password, "confirm_password": confirm}, headers...)
}

// setupStarted logs in with the env credentials and fails unless the setup form opened.
func (client *browser) setupStarted() *browser {
	client.server.t.Helper()

	response := client.login("admin", envPassword)

	var output api.LoginOutput

	if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil || response.Code != http.StatusOK || !output.Setup {
		client.server.t.Fatalf("env login: %d %s", response.Code, response.Body)
	}

	return client
}

func (server *restServer) admins() []store.User {
	server.t.Helper()

	admins, err := server.database.Admins(server.t.Context())

	if err != nil {
		server.t.Fatal(err)
	}

	return admins
}

// The env credentials open the setup form, not the app; the form creates
// the first admin and logs them in; after it the env credentials are a
// wrong login like any other.
func TestSetupAdmin(t *testing.T) {
	server := newSetupServer(t)
	client := server.browser()

	var state api.LoginState

	if err := json.Unmarshal(client.call(http.MethodGet, "login_state", nil).Body.Bytes(), &state); err != nil || state.Disabled != "" {
		t.Fatalf("login_state: %+v %v", state, err)
	}

	client.setupStarted()

	cookie := client.cookies[setupCookie]

	if client.cookies[session.CookieName] != nil || client.open() {
		t.Fatal("the env login started a session")
	}

	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/api/ui/setup_admin" ||
		time.Until(cookie.Expires) > 10*time.Minute {
		t.Fatalf("setup cookie: %+v", cookie)
	}

	// Another site cannot post the form.
	if response := client.setupAdmin("owner@example.com", "owner password 1", "owner password 1", "Origin", "https://evil.example"); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin setup: %d", response.Code)
	}

	for label, test := range map[string]struct {
		email, password, confirm, field string
	}{
		"not an email":  {"owner", "owner password 1", "owner password 1", "email"},
		"short":         {"owner@example.com", "eleven char", "eleven char", "password"},
		"env password":  {"owner@example.com", envPassword, envPassword, "password"},
		"wrong confirm": {"owner@example.com", "owner password 1", "owner password 2", "confirm_password"},
	} {
		response := client.setupAdmin(test.email, test.password, test.confirm)

		if body := errorOf(t, response); response.Code != http.StatusBadRequest || body.Fields[test.field] == "" {
			t.Errorf("%s: %d %s", label, response.Code, response.Body)
		}
	}

	if response := client.setupAdmin(" Owner@Example.com ", "owner password 1", "owner password 1"); response.Code != http.StatusNoContent {
		t.Fatalf("setup: %d %s", response.Code, response.Body)
	}

	if client.cookies[setupCookie] != nil || !client.open() {
		t.Fatalf("after the setup: cookies %v, open %v", client.cookies, client.open())
	}

	admins := server.admins()

	if len(admins) != 1 || admins[0].Email != "owner@example.com" || admins[0].Name != store.DefaultName("owner@example.com") || admins[0].MustChangePassword {
		t.Fatalf("admins: %+v", admins)
	}

	// The cookie again, from a copy: the setup is over.
	replay := server.browser()
	replay.cookies[setupCookie] = cookie

	if response := replay.setupAdmin("other@example.com", "other password 1", "other password 1"); response.Code != http.StatusForbidden ||
		errorOf(t, response).Error != services.AdminExistsProblem {
		t.Fatalf("second setup: %d %s", response.Code, response.Body)
	}

	// The env credentials are now a wrong login, answered like one.
	envLogin, wrong := server.browser().login("admin", envPassword), server.browser().login("admin", "wrong password!")

	if envLogin.Code != http.StatusUnauthorized || envLogin.Body.String() != wrong.Body.String() || len(envLogin.Result().Cookies()) != 0 {
		t.Fatalf("env login after the setup: %d %s, a wrong password: %s", envLogin.Code, envLogin.Body, wrong.Body)
	}

	server.browser().loggedIn("owner@example.com", "owner password 1")

	if len(server.admins()) != 1 {
		t.Fatal("more than one admin")
	}
}

// The form needs the cookie of the env login, signed by the server.
func TestSetupAdminNeedsCookie(t *testing.T) {
	server := newSetupServer(t)
	valid := server.browser().setupStarted().cookies[setupCookie]
	expires, tag, _ := strings.Cut(valid.Value, ".")
	later, _ := strconv.ParseInt(expires, 10, 64)

	for label, value := range map[string]string{
		"none":          "",
		"made up":       expires + "." + strings.Repeat("0", len(tag)),
		"extended":      strconv.FormatInt(later+3600, 10) + "." + tag,
		"without a tag": expires,
	} {
		client := server.browser()

		if value != "" {
			client.cookies[setupCookie] = &http.Cookie{Name: setupCookie, Value: value}
		}

		if response := client.setupAdmin("owner@example.com", "owner password 1", "owner password 1"); response.Code != http.StatusForbidden ||
			client.cookies[session.CookieName] != nil {
			t.Errorf("%s: %d %s", label, response.Code, response.Body)
		}
	}

	// Another ADMIN_PASSWORD ends the setups started with the old one.
	restarted := startRESTServer(t, server.database, func(settings *config.Config) {
		settings.AdminEmail, settings.AdminPassword = "admin", "changed env password"
	})
	client := restarted.browser()
	client.cookies[setupCookie] = valid

	if response := client.setupAdmin("owner@example.com", "owner password 1", "owner password 1"); response.Code != http.StatusForbidden {
		t.Errorf("after ADMIN_PASSWORD changed: %d %s", response.Code, response.Body)
	}

	if admins := server.admins(); len(admins) != 0 {
		t.Fatalf("admins: %+v", admins)
	}
}

// Two setups at once create one admin.
func TestSetupAdminOnce(t *testing.T) {
	server := newSetupServer(t)
	clients := []*browser{server.browser().setupStarted(), server.browser().setupStarted(), server.browser().setupStarted()}
	codes := make([]int, len(clients))

	var group sync.WaitGroup

	for index, client := range clients {
		group.Go(func() {
			email := "owner" + strconv.Itoa(index) + "@example.com"
			codes[index] = client.setupAdmin(email, "owner password 1", "owner password 1").Code
		})
	}

	group.Wait()

	created := 0

	for _, code := range codes {
		switch code {
		case http.StatusNoContent:
			created++
		case http.StatusForbidden:
		default:
			t.Errorf("answers %v", codes)
		}
	}

	if admins := server.admins(); created != 1 || len(admins) != 1 {
		t.Fatalf("answers %v, admins %+v", codes, admins)
	}
}

// The env login is a login attempt: counted before its check, so a blocked
// address cannot try it.
func TestSetupLoginLimit(t *testing.T) {
	server := newSetupServer(t)
	client := server.browser()

	for range session.MaxLoginFailures {
		if response := client.login("admin", "guess guess guess"); response.Code != http.StatusUnauthorized {
			t.Fatalf("guess: %d %s", response.Code, response.Body)
		}
	}

	if response := client.login("admin", envPassword); response.Code != http.StatusTooManyRequests || client.cookies[setupCookie] != nil {
		t.Fatalf("env login when blocked: %d %s", response.Code, response.Body)
	}
}
