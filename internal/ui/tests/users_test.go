package tests

import (
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/storetest"

	"github.com/devforth/exceptic/internal/ingest/grouping"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

const userPassword = "user password 123"

// createUser adds a user through the web app's API and lets them choose
// their password, as they must at the first login. It returns their id and
// a client logged in as them.
func (server *testServer) createUser(username, role string) (string, *testServer) {
	server.t.Helper()

	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		TemporaryPassword string `json:"temporary_password"`
	}

	response := server.call("create_user", map[string]string{"username": username, "role": role})

	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || response.Code != http.StatusOK {
		server.t.Fatalf("create %s: %d %s", username, response.Code, response.Body)
	}

	user := server.as(username, created.TemporaryPassword)

	// A temporary password leads the old pages to the account page.
	if response := user.get("/issues"); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/account" {
		server.t.Fatalf("temporary password: %d %q", response.Code, response.Header().Get("Location"))
	}

	response = user.call("change_password", map[string]string{"current_password": created.TemporaryPassword, "password": userPassword, "confirm_password": userPassword})

	if response.Code != http.StatusNoContent {
		server.t.Fatalf("choose a password: %d %s", response.Code, response.Body)
	}

	user.page("/issues")

	return created.User.ID, user
}

// What each role may open: users get issues, logs, the projects (read-only)
// and MCP; admins also the settings.
func TestRoles(t *testing.T) {
	server := newTestServer(t, password)
	server.ingestFixtures()
	server.login()

	_, admin := server.createUser("ops", store.RoleAdmin)
	_, user := server.createUser("dev", store.RoleUser)

	for _, path := range []string{"/issues", "/logs", "/agents"} {
		user.page(path)
	}

	for _, path := range []string{"/notifications", "/notifications/new", "/notifications/log", "/projects/new", "/projects/1"} {
		if response := user.get(path); response.Code != http.StatusNotFound {
			t.Errorf("user GET %s: %d", path, response.Code)
		}

		admin.page(path)
	}

	// The menu shows only what the role allows.
	menu := user.page("/agents")

	for _, hidden := range []string{`href="/notifications"`, `href="/users"`} {
		if strings.Contains(menu, hidden) {
			t.Errorf("a user sees %s", hidden)
		}
	}

	if response := user.post("/projects", url.Values{"name": {"sneaky"}}); response.Code != http.StatusNotFound {
		t.Errorf("a user created a project: %d", response.Code)
	}
}

// Users change statuses but cannot delete issues: no button, and the server
// refuses it from the issue page and the list.
func TestUsersCannotDeleteIssues(t *testing.T) {
	server := newTestServer(t, password)
	server.ingestFixtures()
	server.login()

	_, user := server.createUser("dev", store.RoleUser)
	id := user.unresolvedIDs()[0]

	for _, page := range []string{user.page("/issues"), user.page("/issues/" + id)} {
		if strings.Contains(page, `value="delete"`) {
			t.Error("a user sees the Delete button")
		}
	}

	if !strings.Contains(server.page("/issues/"+id), `value="delete"`) {
		t.Error("the superadmin has no Delete button")
	}

	if response := user.post("/issues/"+id+"/action", url.Values{"action": {"delete"}}); response.Code != http.StatusNotFound {
		t.Errorf("delete from the issue page: %d", response.Code)
	}

	if response := user.post("/issues/bulk", url.Values{"action": {"delete"}, "issue": {id}}); response.Code != http.StatusNotFound {
		t.Errorf("bulk delete: %d", response.Code)
	}

	user.page("/issues/" + id)

	if response := user.post("/issues/"+id+"/action", url.Values{"action": {"resolve"}}); response.Code != http.StatusSeeOther {
		t.Errorf("a user cannot resolve: %d", response.Code)
	}
}

// personalToken generates the personal token of the client's user.
func personalToken(t *testing.T, client *testServer) string {
	t.Helper()

	match := personalTokenField.FindStringSubmatch(client.post("/agents/token", nil).Body.String())

	if match == nil {
		t.Fatal("no personal token")
	}

	return match[1]
}

// Admins list the connected agents of any user; users only their own.
func TestAgentsOfOtherUsers(t *testing.T) {
	server := newTestServer(t, password)
	server.login()

	id, user := server.createUser("dev", store.RoleUser)
	token := personalToken(t, user)
	agentAccess(t, server, token, http.StatusOK)

	own := server.page("/agents")

	if strings.Contains(own, "/disconnect") || !strings.Contains(own, `<select name="user"`) {
		t.Error("the superadmin's own list shows another user's agent, or no user select")
	}

	selected := regexp.MustCompile(`<option value="([a-z0-9]{8})" selected>`).FindStringSubmatch(own)

	if selected == nil {
		t.Fatalf("the superadmin is not the selected owner:\n%s", own)
	}

	root := selected[1]

	page := server.page("/agents?user=" + id)
	disconnect := disconnectAction.FindStringSubmatch(page)

	if disconnect == nil {
		t.Fatalf("the user's agent is not listed:\n%s", page)
	}

	// The client tabs keep whose apps are listed.
	if !strings.Contains(page, `href="/agents?client=codex&amp;user=`+id+`" role="tab"`) || !strings.Contains(own, `href="/agents?client=codex" role="tab"`) {
		t.Error("a client tab drops whose apps are listed")
	}

	if strings.Contains(user.page("/agents"), `<select name="user"`) {
		t.Error("a user can choose whose agents to list")
	}

	if response := user.get("/agents?user=" + root); !strings.Contains(response.Body.String(), disconnect[1]) {
		t.Error("?user= changed what a user sees")
	}

	ownerOnly := server.as("admin", password)
	ownToken := personalToken(t, ownerOnly)
	agentAccess(t, server, ownToken, http.StatusOK)

	ownGrant := disconnectAction.FindStringSubmatch(server.page("/agents"))

	if response := user.post(ownGrant[1], nil); response.Code != http.StatusNotFound {
		t.Errorf("a user disconnected the superadmin's agent: %d", response.Code)
	}

	if response := server.post(disconnect[1], url.Values{"user": {id}}); response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/agents?client=claude&user="+id {
		t.Fatalf("disconnect another user's agent: %d %q", response.Code, response.Header().Get("Location"))
	}

	agentAccess(t, server, token, http.StatusUnauthorized)
}

// Every status change is in the issue's history: who, when and how; a
// resolved issue that happens again is reopened by its event.
func TestStatusHistory(t *testing.T) {
	database := storetest.Open(t)
	server := startServer(t, database, "admin", password)
	server.ingestFixtures()
	server.login()

	_, user := server.createUser("dev", store.RoleUser)
	id := server.unresolvedIDs()[0]

	if response := user.call("update_account_name", map[string]string{"name": "Dev One"}); response.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", response.Code, response.Body)
	}

	user.post("/issues/"+id+"/action", url.Values{"action": {"resolve"}})
	server.post("/issues/bulk", url.Values{"action": {"ignore"}, "issue": {id}})
	server.post("/issues/"+id+"/action", url.Values{"action": {"ignore"}}) // no change, no entry

	page := html.UnescapeString(server.page("/issues/" + id))
	history := page[strings.Index(page, "<h2>Status history</h2>"):]
	history = history[:strings.Index(history, "</section>")]

	if strings.Count(history, "<li>") != 2 || !strings.Contains(history, "by admin") || !strings.Contains(history, "by Dev One") ||
		strings.Index(history, "by admin") > strings.Index(history, "by Dev One") {
		t.Fatalf("history:\n%s", history)
	}

	if !strings.Contains(page, `class="status status-ignored"`) || !strings.Contains(page, "by admin ·") {
		t.Error("the header does not say who changed the status")
	}

	// A resolved issue that happens again: reopened by its event.
	ctx := context.Background()

	if _, err := database.SetIssuesStatus(ctx, []string{id}, store.StatusResolved, store.Actor{Name: "API token", Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	issue, err := database.GetIssue(ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	// The fingerprint of the issue, from its stored event as ingest computes it.
	latest, err := database.LatestEvent(ctx, id)

	if err != nil {
		t.Fatal(err)
	}

	parsed, err := sentry.ParseEvent(latest.Payload, latest.EventID, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	fingerprint := grouping.Fingerprint(parsed)

	again := store.NewEvent{
		ProjectID: issue.ProjectID, EventID: "0123456789abcdef0123456789abcdef", Fingerprint: fingerprint,
		Title: issue.Title, Level: issue.Level, Timestamp: time.Now(), ReceivedAt: time.Now(), Payload: []byte(`{}`),
	}

	if result, err := database.SaveEvent(ctx, again); err != nil || !result.Regression {
		t.Fatal("not a regression", result, err)
	}

	page = html.UnescapeString(server.page("/issues/" + id))

	if !strings.Contains(page, "reopened: happened again") || !strings.Contains(page, "by API token") {
		t.Errorf("reopening not in the history:\n%s", page)
	}
}

// Admins manage the agents of users and admins, not the superadmin's.
func TestAdminsCannotTouchSuperadminAgents(t *testing.T) {
	server := newTestServer(t, password)
	server.login()

	agentAccess(t, server, personalToken(t, server), http.StatusOK)

	_, admin := server.createUser("ops", store.RoleAdmin)
	root := superadminID(t, admin)
	superadminGrant := disconnectAction.FindStringSubmatch(server.page("/agents"))

	if page := admin.page("/agents?user=" + root); strings.Contains(page, superadminGrant[1]) || strings.Contains(page, `<option value="`+root+`"`) {
		t.Errorf("an admin sees the superadmin's agents:\n%s", page)
	}

	if response := admin.post(superadminGrant[1], nil); response.Code != http.StatusNotFound {
		t.Errorf("an admin disconnected the superadmin's agent: %d", response.Code)
	}
}

// superadminID is the id of the superadmin ("admin").
func superadminID(t *testing.T, server *testServer) string {
	t.Helper()

	users, err := server.app.Services.Users.List(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	for _, user := range users {
		if user.Role == store.RoleSuperadmin {
			return user.ID
		}
	}

	t.Fatal("no superadmin")

	return ""
}
