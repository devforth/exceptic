// Tests run the whole server (ingest + UI) against a real database filled by
// replaying the recorded SDK requests, and drive the UI like a browser.
package tests

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/devforth/exceptic/internal/app"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/database"

	"github.com/devforth/exceptic/internal/storage/storetest"

	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/secret"
)

const (
	password = "correct horse battery"
)

// testLog sends server logs to t.Log: they are printed only for failed tests.
type testLog struct{ t *testing.T }

func (log testLog) Write(data []byte) (int, error) {
	log.t.Log(strings.TrimSpace(string(data)))

	return len(data), nil
}

type testServer struct {
	t       *testing.T
	app     *app.App
	handler http.Handler
	cookie  *http.Cookie
	// mails receives what the server sent through its (fake) SMTP server.
	mails <-chan fixtures.SMTPSession
}

const secretKey = "0123456789abcdef0123456789abcdef"

func newTestServer(t *testing.T, adminPassword string) *testServer {
	t.Helper()

	return startServer(t, storetest.Open(t), "admin", adminPassword)
}

// newTestServerWith is newTestServer with settings changed by configure.
func newTestServerWith(t *testing.T, configure func(settings *config.Config)) *testServer {
	t.Helper()

	return startServer(t, storetest.Open(t), "admin", password, configure)
}

// startServer builds a server on an existing database, like a restart of the
// real server with possibly different settings.
func startServer(t *testing.T, database *database.Store, adminUsername, adminPassword string, configure ...func(settings *config.Config)) *testServer {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(testLog{t}, nil))
	smtp, mails := fixtures.FakeSMTP(t)
	settings := &config.Config{
		AppName:         config.DefaultAppName,
		Projects:        fixtures.Projects(),
		AllowedOrigins:  []string{"*"},
		AdminUsername:   adminUsername,
		AdminPassword:   adminPassword,
		SessionLifetime: config.DefaultSessionLifetime,
		// Every connection type is available: SMTP for email, a key for chat tokens.
		Notifications: config.Notifications{SecretKey: secretKey, SMTP: smtp},
		// Shown on the Projects page; nothing is deleted by the UI.
		Retention: config.Retention{MaxDatabaseBytes: 2 << 30},
	}

	for _, change := range configure {
		change(settings)
	}

	box, err := secret.New(secretKey)

	if err != nil {
		t.Fatal(err)
	}

	// A fake Telegram API: tests never reach the real one.
	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getChat") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":-100,"type":"group","title":"Ops chat"}}`))

			return
		}

		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(telegram.Close)

	built, err := app.New(context.Background(), app.Options{
		Settings: settings, Logger: logger, Store: database, Version: "test", Web: fixtures.WebApp(t),
		Factory: notify.Factory{SMTP: smtp, Box: box, TelegramURL: telegram.URL},
	})

	if err != nil {
		t.Fatal(err)
	}

	// Its pipeline stores what the server accepts; it stops before the store closes (cleanups run last added first).
	t.Cleanup(built.Start())

	return &testServer{t: t, app: built, handler: built.Handler, mails: mails}
}

func (server *testServer) send(request *http.Request) *httptest.ResponseRecorder {
	if server.cookie != nil {
		request.AddCookie(server.cookie)
	}

	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)

	return response
}

func (server *testServer) get(target string) *httptest.ResponseRecorder {
	return server.send(httptest.NewRequest(http.MethodGet, target, nil))
}

func (server *testServer) post(target string, form url.Values, headers ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	for index := 0; index+1 < len(headers); index += 2 {
		request.Header.Set(headers[index], headers[index+1])
	}

	return server.send(request)
}

// page fetches target as a logged-in user and fails unless it is 200.
func (server *testServer) page(target string) string {
	server.t.Helper()

	response := server.get(target)

	if response.Code != http.StatusOK {
		server.t.Fatalf("GET %s: %d %s", target, response.Code, response.Body)
	}

	return response.Body.String()
}

// call sends a change to the web app's API (/api/ui/<name>) as its pages do:
// JSON with the page's Origin.
func (server *testServer) call(name string, input any) *httptest.ResponseRecorder {
	server.t.Helper()

	body, err := json.Marshal(input)

	if err != nil {
		server.t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/ui/"+name, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.com")

	return server.send(request)
}

// login logs in as the superadmin created from the env of the test server.
func (server *testServer) login() {
	server.t.Helper()
	server.loginAs("admin", password)
}

func (server *testServer) loginAs(username, userPassword string) {
	server.t.Helper()

	response := server.call("login", map[string]string{"username": username, "password": userPassword})

	if response.Code != http.StatusOK {
		server.t.Fatalf("login as %s: %d %s", username, response.Code, response.Body)
	}

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == session.CookieName && cookie.Value != "" {
			server.cookie = cookie
		}
	}

	if server.cookie == nil {
		server.t.Fatal("login did not set a session cookie")
	}
}

// as is another browser on the same server, logged in as username.
func (server *testServer) as(username, userPassword string) *testServer {
	server.t.Helper()

	other := &testServer{t: server.t, app: server.app, handler: server.handler, mails: server.mails}
	other.loginAs(username, userPassword)

	return other
}

func (server *testServer) ingestFixtures() {
	server.t.Helper()

	for _, fixture := range fixtures.All(server.t) {
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, fixture.HTTP())

		if response.Code != http.StatusOK {
			server.t.Fatalf("%s: ingest got %d %s", fixture.Name, response.Code, response.Body)
		}
	}

	server.waitStored()
}

// waitStored waits until the pipeline has stored everything the server accepted.
func (server *testServer) waitStored() {
	server.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	if err := server.app.WaitStored(ctx); err != nil {
		server.t.Fatalf("envelopes not stored: %v", err)
	}
}

var (
	// The personal token just generated, and the disconnect buttons, on the MCP page.
	personalTokenField = regexp.MustCompile(`id="mcp-personal-token" value="Bearer ([^"]+)"`)
	disconnectAction   = regexp.MustCompile(`action="(/agents/[a-z0-9]{8}/disconnect)"`)
)

// unresolvedIDs lists the unresolved issues the way the web app reads them
// (/api/ui/list_issues), as the user of the server's session.
func (server *testServer) unresolvedIDs() []string {
	server.t.Helper()

	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}

	if err := json.Unmarshal([]byte(server.page("/api/ui/list_issues?limit=100")), &page); err != nil {
		server.t.Fatal(err)
	}

	ids := make([]string, 0, len(page.Items))

	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}

	return ids
}

// applyIssueAction runs an action on issues as the web app does (its
// /api/ui request), and fails unless it is done.
func (server *testServer) applyIssueAction(action string, ids ...string) {
	server.t.Helper()

	body, err := json.Marshal(map[string]any{"action": action, "issue_ids": ids})

	if err != nil {
		server.t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/ui/apply_issue_action", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://example.com")

	if response := server.send(request); response.Code != http.StatusOK {
		server.t.Fatalf("%s %v: %d %s", action, ids, response.Code, response.Body)
	}
}
