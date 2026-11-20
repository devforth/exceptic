package api_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
)

// uiOrigin is where httptest sends requests from (httptest.NewRequest uses example.com).
const uiOrigin = "http://example.com"

// uiUser is a user of the web app: its session cookie.
type uiUser struct {
	server *restServer
	id     string
	cookie string
}

// webUser adds a user with role and a session for it.
func (server *restServer) webUser(role string, mustChangePassword bool) uiUser {
	server.t.Helper()

	ctx := context.Background()
	name := role + "-" + store.NewID()
	id, err := server.database.CreateUser(ctx, store.User{Username: name, Name: "Web " + role, Role: role, MustChangePassword: mustChangePassword, CreatedAt: time.Now()}, "x")

	if err != nil {
		server.t.Fatal(err)
	}

	token := store.NewID() + store.NewID()

	if err := server.database.CreateSession(ctx, secret.Hash(token), id, time.Now(), time.Now().Add(time.Hour)); err != nil {
		server.t.Fatal(err)
	}

	return uiUser{server: server, id: id, cookie: token}
}

// call sends a request as the web app does: the session cookie, and for a
// change JSON with the page's Origin.
func (user uiUser) call(method, name, body string) *httptest.ResponseRecorder {
	user.server.t.Helper()

	request := httptest.NewRequest(method, "/api/ui/"+name, strings.NewReader(body))

	if user.cookie != "" {
		request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})
	}

	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", uiOrigin)
	}

	response := httptest.NewRecorder()
	user.server.handler.ServeHTTP(response, request)

	return response
}

func (user uiUser) callRoute(endpoint api.Endpoint, body string) *httptest.ResponseRecorder {
	method, _, _ := strings.Cut(api.Route(api.TransportUI, endpoint), " ")

	return user.call(method, endpoint.Name, body)
}

// Only a valid session opens the web app's API; no other credential does,
// and the session opens nothing else.
func TestUIRequiresSession(t *testing.T) {
	server := newRESTServer(t)
	anonymous := uiUser{server: server}

	expired := server.webUser(store.RoleAdmin, false)

	if err := server.database.CreateSession(context.Background(), secret.Hash("expired-token"), expired.id, time.Now(), time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	for label, user := range map[string]uiUser{
		"no cookie": anonymous, "unknown session": {server: server, cookie: "no-such-session"},
		"expired session": {server: server, cookie: "expired-token"},
	} {
		response := user.call(http.MethodGet, "list_projects", "")

		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "login required") {
			t.Errorf("%s: %d %s", label, response.Code, response.Body)
		}

		for name, want := range map[string]string{"X-Content-Type-Options": "nosniff", "Cross-Origin-Resource-Policy": "same-origin", "Cache-Control": "no-store"} {
			if got := response.Header().Get(name); got != want {
				t.Errorf("%s: %s %q", label, name, got)
			}
		}

		if !strings.Contains(response.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s: policy %q", label, response.Header().Get("Content-Security-Policy"))
		}
	}

	// The API token does not open the web app's API.
	request := httptest.NewRequest(http.MethodGet, "/api/ui/list_projects", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	if response := serve(server, request); response.Code != http.StatusUnauthorized {
		t.Errorf("API token on /api/ui: %d", response.Code)
	}

	// A session opens neither the REST API nor MCP.
	user := server.webUser(store.RoleSuperadmin, false)

	// An agent's token does not open the web app's API either.
	personal, err := server.app.OAuth.NewPersonalToken(context.Background(), user.id)

	if err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/ui/list_projects", nil)
	request.Header.Set("Authorization", "Bearer "+personal)

	if response := serve(server, request); response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "login required") {
		t.Errorf("MCP token on /api/ui: %d %s", response.Code, response.Body)
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/list_issues", nil),
		httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)),
	} {
		request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", uiOrigin)

		if response := serve(server, request); response.Code != http.StatusUnauthorized {
			t.Errorf("session on %s: %d", request.URL.Path, response.Code)
		}
	}
}

func serve(server *restServer, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)

	return response
}

// Every endpoint the web app may use is served on its method; the other
// method, other transports' endpoints and unknown names answer 404.
func TestUIRoutesFollowTheCatalog(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleSuperadmin, false)

	for _, endpoint := range api.Endpoints(nil, nil) {
		method, _, _ := strings.Cut(api.Route(api.TransportUI, endpoint), " ")
		other := map[string]string{http.MethodGet: http.MethodPost, http.MethodPost: http.MethodGet}[method]

		if !endpoint.Serves(api.TransportUI) {
			if response := user.call(method, endpoint.Name, "{}"); response.Code != http.StatusNotFound {
				t.Errorf("%s (%s) is served to the web app: %d", endpoint.Name, endpoint.Transports, response.Code)
			}

			continue
		}

		// Empty input: answered by the endpoint (a result or an input mistake), not by the routing.
		if response := user.call(method, endpoint.Name, "{}"); response.Code != http.StatusOK && response.Code != http.StatusBadRequest {
			t.Errorf("%s %s: %d %s", method, endpoint.Name, response.Code, response.Body)
		}

		if response := user.call(other, endpoint.Name, "{}"); response.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d, want 404", other, endpoint.Name, response.Code)
		}
	}

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if response := user.call(method, "nope", "{}"); response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"not found"`) {
			t.Errorf("%s unknown: %d %s", method, response.Code, response.Body)
		}
	}
}

// What a user may not use answers exactly as an unknown endpoint: an admin
// endpoint to a user, and every endpoint while the password is temporary
// unless it is open then.
func TestUIRefusalsLookUnknown(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)
	temporary := server.webUser(store.RoleAdmin, true)

	same := func(label string, got, want *httptest.ResponseRecorder) {
		t.Helper()

		if got.Code != want.Code || got.Body.String() != want.Body.String() || !maps.EqualFunc(got.Header(), want.Header(), slices.Equal) {
			t.Errorf("%s: %d %s %v, want %d %s %v", label, got.Code, got.Body, got.Header(), want.Code, want.Body, want.Header())
		}
	}

	for _, endpoint := range api.Endpoints(nil, nil) {
		if !endpoint.Serves(api.TransportUI) {
			continue
		}

		method, _, _ := strings.Cut(api.Route(api.TransportUI, endpoint), " ")
		unknown := user.call(method, "nope", "{}")

		if endpoint.Admin {
			same(endpoint.Name+" as user", user.callRoute(endpoint, "{}"), unknown)
		}

		if !endpoint.OpenWithTemporaryPassword {
			same(endpoint.Name+" with a temporary password", temporary.callRoute(endpoint, "{}"), temporary.call(method, "nope", "{}"))
		}
	}

	admin := server.webUser(store.RoleAdmin, false)

	if response := admin.call(http.MethodGet, "list_users", ""); response.Code != http.StatusOK {
		t.Errorf("admin list_users: %d %s", response.Code, response.Body)
	}
}

// Only the web app's own pages may call: changes need JSON and the page's
// Origin, and Fetch Metadata refuses other sites first.
func TestUICrossSite(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	send := func(method, contentType, origin, site string) int {
		request := httptest.NewRequest(method, "/api/ui/list_projects", strings.NewReader("{}"))

		if method == http.MethodPost {
			request.URL.Path = "/api/ui/update_issue_status"
		}

		request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})

		for name, value := range map[string]string{"Content-Type": contentType, "Origin": origin, "Sec-Fetch-Site": site} {
			if value != "" {
				request.Header.Set(name, value)
			}
		}

		return serve(server, request).Code
	}

	cases := []struct {
		label                             string
		method, contentType, origin, site string
		want                              int
	}{
		{"foreign origin", http.MethodPost, "application/json", "https://evil.example", "", http.StatusForbidden},
		{"no origin", http.MethodPost, "application/json", "", "", http.StatusForbidden},
		{"form post", http.MethodPost, "text/plain", uiOrigin, "", http.StatusUnsupportedMediaType},
		{"cross-site read", http.MethodGet, "", "", "cross-site", http.StatusForbidden},
		{"same-site read", http.MethodGet, "", "", "same-site", http.StatusForbidden},
		{"same-origin read", http.MethodGet, "", "", "same-origin", http.StatusOK},
		{"same-origin change", http.MethodPost, "application/json; charset=utf-8", uiOrigin, "same-origin", http.StatusBadRequest}, // reaches the endpoint: no issue IDs
	}

	for _, test := range cases {
		if got := send(test.method, test.contentType, test.origin, test.site); got != test.want {
			t.Errorf("%s: %d, want %d", test.label, got, test.want)
		}
	}

	// Over HTTPS a page of the same host served over plain HTTP may not act.
	for origin, want := range map[string]int{"http://example.com": http.StatusForbidden, "https://example.com": http.StatusBadRequest} {
		request := httptest.NewRequest(http.MethodPost, "https://example.com/api/ui/update_issue_status", strings.NewReader("{}"))
		request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", origin)

		if got := serve(server, request).Code; got != want {
			t.Errorf("HTTPS request from %s: %d, want %d", origin, got, want)
		}
	}
}

// A refused form names each field by its input name, so the page shows the
// error next to it.
func TestUIFormErrors(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)
	response := admin.call(http.MethodPost, "create_project", `{"name":"Not Valid"}`)

	var body api.ErrorBody

	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", response.Code, response.Body)
	}

	if body.Fields["name"] == "" || !strings.Contains(body.Error, "name: "+body.Fields["name"]) {
		t.Errorf("fields %v, error %q", body.Fields, body.Error)
	}
}

// Changes are recorded as the user, from the web app.
func TestUIActsAsTheUser(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	user := server.webUser(store.RoleUser, false)

	var issues struct {
		Items []restIssue `json:"items"`
	}

	if err := json.Unmarshal(user.call(http.MethodGet, "list_issues", "").Body.Bytes(), &issues); err != nil || len(issues.Items) == 0 {
		t.Fatalf("issues: %v %v", issues, err)
	}

	issueID := issues.Items[0].ID

	if response := user.call(http.MethodPost, "update_issue_status", `{"issue_ids":["`+issueID+`"],"status":"resolved"}`); response.Code != http.StatusOK {
		t.Fatalf("update: %d %s", response.Code, response.Body)
	}

	history, err := server.database.IssueStatusHistory(context.Background(), issueID)

	if err != nil || len(history) == 0 {
		t.Fatalf("history %v, %v", history, err)
	}

	if latest := history[len(history)-1]; latest.Source != store.SourceUI || latest.UserID != user.id || latest.Name != "Web user" {
		t.Errorf("recorded as %+v", latest.Actor)
	}
}

// DSNs name the server as the browser reached it.
func TestUIProjects(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)
	request := httptest.NewRequest(http.MethodGet, "/api/ui/list_projects", nil)
	request.Host = "panel.example:8443"
	request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})
	response := serve(server, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "@panel.example:8443/") {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
}

type sessionAnswer struct {
	User struct {
		ID                 string `json:"id"`
		MustChangePassword bool   `json:"must_change_password"`
	} `json:"user"`
	DisplayName      string            `json:"display_name"`
	CanAdminister    bool              `json:"can_administer"`
	AppName          string            `json:"app_name"`
	UnresolvedIssues int               `json:"unresolved_issues"`
	ProjectColorsURL string            `json:"project_colors_url"`
	ProjectColors    map[string]string `json:"project_colors"`
}

// The web app's menu: who is logged in, what they may do, the counter and the colors.
func TestGetSession(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	unresolved, err := server.app.Services.Issues.Unresolved(context.Background())

	if err != nil || unresolved == 0 {
		t.Fatalf("unresolved %d, %v", unresolved, err)
	}

	for role, canAdminister := range map[string]bool{store.RoleUser: false, store.RoleAdmin: true, store.RoleSuperadmin: true} {
		user := server.webUser(role, false)
		response := user.call(http.MethodGet, "get_session", "")

		var session sessionAnswer

		if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", role, response.Code, response.Body)
		}

		if session.User.ID != user.id || session.DisplayName != "Web "+role || session.CanAdminister != canAdminister ||
			session.AppName != server.app.Services.AppName || session.UnresolvedIssues != unresolved ||
			session.ProjectColorsURL != server.app.Services.Projects.ColorsURL() {
			t.Errorf("%s: %+v", role, session)
		}

		for _, project := range server.app.Services.Projects.List() {
			if color := session.ProjectColors[project.Name]; color != project.Color || !config.ValidColor(color) {
				t.Errorf("%s: color of %s %q, want %q", role, project.Name, color, project.Color)
			}
		}
	}

	// Open with a temporary password, so the app can send the user to choose one.
	temporary := server.webUser(store.RoleUser, true)

	var session sessionAnswer

	if err := json.Unmarshal(temporary.call(http.MethodGet, "get_session", "").Body.Bytes(), &session); err != nil || !session.User.MustChangePassword {
		t.Errorf("temporary password: %+v %v", session, err)
	}
}

// Every role sees how each project records, as the old Projects page shows it.
func TestUIProjectLevels(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	var projects struct {
		Items []struct {
			Name        string   `json:"name"`
			FromEnv     bool     `json:"from_env"`
			EventLevels []string `json:"event_levels"`
			LogLevels   []string `json:"log_levels"`
		} `json:"items"`
	}

	if err := json.Unmarshal(user.call(http.MethodGet, "list_projects", "").Body.Bytes(), &projects); err != nil || len(projects.Items) == 0 {
		t.Fatalf("%+v %v", projects, err)
	}

	// The test settings record every level: explicit lists, highest first.
	for _, project := range projects.Items {
		if !project.FromEnv || !slices.Equal(project.EventLevels, []string{"fatal", "error", "warning", "info", "debug"}) || len(project.LogLevels) != 6 {
			t.Errorf("%+v", project)
		}
	}

	// A project's own levels, in any order, come back highest first; none stays none.
	admin := server.webUser(store.RoleAdmin, false)

	if response := admin.call(http.MethodPost, "update_project", `{"project":"backend","event_levels":["warning","fatal"],"log_levels":[]}`); response.Code != http.StatusOK {
		t.Fatalf("update_project: %d %s", response.Code, response.Body)
	}

	if err := json.Unmarshal(user.call(http.MethodGet, "list_projects", "").Body.Bytes(), &projects); err != nil {
		t.Fatal(err)
	}

	for _, project := range projects.Items {
		if project.Name == "backend" && (!slices.Equal(project.EventLevels, []string{"fatal", "warning"}) || project.LogLevels == nil || len(project.LogLevels) != 0) {
			t.Errorf("own levels: %+v", project)
		}
	}
}

// Every role sees how much the database holds, as the Projects page shows it.
func TestGetStorage(t *testing.T) {
	server := newRESTServer(t)
	response := server.webUser(store.RoleUser, false).call(http.MethodGet, "get_storage", "")

	var storage struct {
		UsedBytes          int64  `json:"used_bytes"`
		Used               string `json:"used"`
		Limit              string `json:"limit"`
		CleanupStopPercent int    `json:"cleanup_stop_percent"`
	}

	if err := json.Unmarshal(response.Body.Bytes(), &storage); err != nil || response.Code != http.StatusOK ||
		storage.UsedBytes == 0 || storage.Used == "" || storage.Limit != "" || storage.CleanupStopPercent == 0 {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
}

// The Logs page: several projects and levels at once, numbered pages, the
// count and the level choices, and the issue a log belongs to.
func TestUILogs(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	user := server.webUser(store.RoleUser, false)

	type logRow struct {
		Body  string `json:"body"`
		Level string `json:"level"`
		Issue *struct {
			ID string `json:"id"`
		} `json:"issue"`
	}

	type logs struct {
		Items []logRow `json:"items"`
	}

	read := func(name, query string, result any) {
		t.Helper()

		response := user.call(http.MethodGet, name+query, "")

		if response.Code != http.StatusOK {
			t.Fatalf("%s%s: %d %s", name, query, response.Code, response.Body)
		}

		if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
			t.Fatal(err)
		}
	}

	var all, both, second, pair logs

	read("list_logs", "?limit=100", &all)
	read("list_logs", "?limit=100&project=backend&project=frontend", &both)

	if len(all.Items) < 3 || len(both.Items) != len(all.Items) {
		t.Fatalf("every log %d, of both projects %d", len(all.Items), len(both.Items))
	}

	var count struct {
		Count int `json:"count"`
	}

	read("count_logs", "?project=backend,frontend", &count)

	if count.Count != len(all.Items) {
		t.Errorf("count %d, want %d", count.Count, len(all.Items))
	}

	read("list_logs", "?limit=2", &pair)
	read("list_logs", "?limit=1&offset=1", &second)

	if len(second.Items) != 1 || second.Items[0].Body != pair.Items[1].Body {
		t.Errorf("offset 1 gave %+v, want the second of %+v", second.Items, pair.Items)
	}

	if response := user.call(http.MethodGet, "list_logs?offset=1&cursor=5", ""); response.Code != http.StatusBadRequest {
		t.Errorf("offset with cursor: %d", response.Code)
	}

	var warnAndInfo logs

	read("list_logs", "?limit=100&level=warn&level=info", &warnAndInfo)

	if len(warnAndInfo.Items) == 0 || slices.ContainsFunc(warnAndInfo.Items, func(item logRow) bool { return item.Level != "warn" && item.Level != "info" }) {
		t.Errorf("levels warn and info: %+v", warnAndInfo.Items)
	}

	var levels struct {
		Levels []string `json:"levels"`
	}

	read("list_log_levels", "", &levels)

	for _, item := range all.Items {
		if !slices.Contains(levels.Levels, item.Level) {
			t.Errorf("level %s of a log is not a choice: %v", item.Level, levels.Levels)
		}
	}

	if !slices.ContainsFunc(all.Items, func(item logRow) bool { return item.Issue != nil && item.Issue.ID != "" }) {
		t.Error("no log names its issue")
	}
}
