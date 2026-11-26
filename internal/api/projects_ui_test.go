package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

type uiProject struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	DSN        string   `json:"dsn"`
	FromEnv    bool     `json:"from_env"`
	Color      string   `json:"color"`
	ShownColor string   `json:"shown_color"`
	LogLevels  []string `json:"log_levels"`
	DiskShare  int      `json:"disk_share"`
}

// An unknown project is not found for the project page (the web app shows
// its 404 page); an agent reads which project it did not know.
func TestUIGetProjectUnknown(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	for _, call := range []struct{ method, name, body string }{
		{http.MethodGet, "get_project?project=99", ""}, {http.MethodGet, "get_project?project=nope", ""},
		{http.MethodPost, "update_project", `{"project":"99","color":"#123456"}`}, {http.MethodPost, "delete_project", `{"project":"nope"}`},
		{http.MethodGet, "get_project_setup?project=99", ""},
	} {
		if response := admin.call(call.method, call.name, call.body); response.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d %s", call.name, call.body, response.Code, response.Body)
		}
	}

	agent := newTestServer(t)

	if text, isError := agent.try("get_project", map[string]any{"project": "nope"}); !isError || text != `not found: project "nope" (see list_projects)` {
		t.Errorf("agent: %s", text)
	}
}

// The color a project shows: the palette's until it has its own, and
// choosing the palette color again follows the palette.
func TestUIGetProjectShownColor(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	var project uiProject

	admin.request(http.MethodGet, "get_project?project=1", "", &project)

	if project.Color != "" || project.ShownColor != config.ProjectPalette[0].Value || !project.FromEnv || !strings.Contains(project.DSN, fixtures.BackendKey) {
		t.Fatalf("%+v", project)
	}

	admin.request(http.MethodPost, "update_project", `{"project":"1","color":"#123456"}`, &project)

	if project.Color != "#123456" || project.ShownColor != "#123456" {
		t.Errorf("own color: %+v", project)
	}

	project = uiProject{}
	admin.request(http.MethodPost, "update_project", `{"project":"1","color":"`+config.ProjectPalette[0].Value+`"}`, &project)

	if project.Color != "" || project.ShownColor != config.ProjectPalette[0].Value {
		t.Errorf("the palette color again: %+v", project)
	}
}

// A new project's form starts from the next palette color and the levels
// recorded by default; only admins open it.
func TestGetProjectDefaults(t *testing.T) {
	server := newRESTServerWith(t, func(settings *config.Config) {
		settings.Recording = config.Recording{LogLevels: []string{"warn", "error"}}
	})

	var defaults struct {
		ShownColor string   `json:"shown_color"`
		LogLevels  []string `json:"log_levels"`
	}

	server.webUser(store.RoleAdmin, false).request(http.MethodGet, "get_project_defaults", "", &defaults)

	next := config.DefaultProjectColor(len(fixtures.Projects()) + 1)

	if defaults.ShownColor != next || !slices.Equal(defaults.LogLevels, []string{"error", "warn"}) {
		t.Errorf("%+v, want color %s", defaults, next)
	}

	// Users add projects too.
	if response := server.webUser(store.RoleUser, false).call(http.MethodGet, "get_project_defaults", ""); response.Code != http.StatusOK {
		t.Errorf("a user: %d", response.Code)
	}
}

// The project form shows each problem at its field.
func TestUIProjectFormErrors(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	for _, test := range []struct {
		name, body, field, want string
		status                  int
	}{
		{"create_project", `{"name":"Not Valid"}`, "name", "lowercase letters", http.StatusBadRequest},
		{"create_project", `{"name":"backend"}`, "name", "Another project already has this name.", http.StatusConflict},
		{"update_project", `{"project":"1","color":"red"}`, "color", "Choose a color.", http.StatusBadRequest},
		{"update_project", `{"project":"1","allowed_origins":["app.example.com"]}`, "allowed_origins", "Not a site address", http.StatusBadRequest},
	} {
		response := admin.call(http.MethodPost, test.name, test.body)

		var body api.ErrorBody

		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != test.status || !strings.Contains(body.Fields[test.field], test.want) {
			t.Errorf("%s %s: %d %s", test.name, test.body, response.Code, response.Body)
		}
	}
}

var dsnPattern = regexp.MustCompile(`http://([0-9a-f]{32})@example\.com/(\d+)`)

// A project added in the web app works at once: its DSN stores what it
// records and drops other levels; levels and name change, the color stays
// when not given; deleting it removes its data and its DSN stops working.
func TestUIProjectLifecycle(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	var created uiProject

	admin.request(http.MethodPost, "create_project", `{"name":"mobile","color":"#123456","log_levels":["error","warn"]}`, &created)

	match := dsnPattern.FindStringSubmatch(created.DSN)

	if match == nil || created.Color != "#123456" || !slices.Equal(created.LogLevels, []string{"error", "warn"}) {
		t.Fatalf("created %+v", created)
	}

	envelopeURL := "/api/" + match[2] + "/envelope/?sentry_key=" + match[1]
	send := func(envelope string) int {
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, envelopeURL, strings.NewReader(envelope)))
		waitStored(t, server.app)

		return response.Code
	}

	if code := send("{}\n{\"type\":\"event\"}\n{\"message\":\"from mobile\"}"); code != http.StatusOK {
		t.Fatalf("envelope: %d", code)
	}

	send("{}\n{\"type\":\"event\"}\n{\"message\":\"mobile info\",\"level\":\"info\"}")

	ctx := context.Background()
	issues, err := server.database.ListIssues(ctx, store.IssueFilter{ProjectIDs: []int{created.ID}, Limit: 10})

	// Events of every level are stored: the info message is an issue too.
	if err != nil || len(issues) != 2 {
		t.Fatalf("issues of the new project: %+v %v", issues, err)
	}

	var updated uiProject

	admin.request(http.MethodPost, "update_project", `{"project":"`+strconv.Itoa(created.ID)+`","name":"mobile-app","log_levels":["error"]}`, &updated)

	if updated.Name != "mobile-app" || updated.Color != "#123456" || !slices.Equal(updated.LogLevels, []string{"error"}) {
		t.Errorf("updated %+v", updated)
	}

	admin.request(http.MethodPost, "delete_project", `{"project":"`+strconv.Itoa(created.ID)+`"}`, &struct{}{})

	if err := server.database.PurgeProjects(ctx); err != nil {
		t.Fatal(err)
	}

	if count, _, _ := server.database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{created.ID}}); count != 0 {
		t.Errorf("%d issues of the deleted project", count)
	}

	if code := send("{}\n{\"type\":\"event\"}\n{\"message\":\"late\"}"); code != http.StatusForbidden {
		t.Errorf("DSN of a deleted project: %d", code)
	}
}

// A project from PROJECTS can be renamed and change its color, but not be deleted.
func TestUIProjectFromEnv(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	var project uiProject

	admin.request(http.MethodPost, "update_project", `{"project":"1","name":"renamed","color":"#123456"}`, &project)

	if project.Name != "renamed" || project.Color != "#123456" || !project.FromEnv {
		t.Errorf("%+v", project)
	}

	// The Projects page's Change color sends the color alone; the menu's badges follow at once.
	admin.request(http.MethodPost, "update_project", `{"project":"1","color":"#654321"}`, &project)

	var session sessionAnswer

	admin.read("get_session", &session)

	if project.Name != "renamed" || project.Color != "#654321" || session.ProjectColors["renamed"] != "#654321" {
		t.Errorf("color alone: %+v, session colors %v", project, session.ProjectColors)
	}

	if response := admin.call(http.MethodPost, "delete_project", `{"project":"1"}`); response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "PROJECTS") {
		t.Errorf("delete: %d %s", response.Code, response.Body)
	}
}

// With PUBLIC_URL set, the DSNs of the project pages use it.
func TestUIDSNUsesPublicURL(t *testing.T) {
	server := newRESTServerWith(t, func(settings *config.Config) {
		settings.Notifications.PublicURL = "https://errors.example.com/monitoring"
	})
	want := "https://" + fixtures.BackendKey + "@errors.example.com/monitoring/1"

	var project uiProject

	admin := server.webUser(store.RoleAdmin, false)
	admin.request(http.MethodGet, "get_project?project=1", "", &project)

	var setup struct {
		DSN string `json:"dsn"`
	}

	admin.request(http.MethodGet, "get_project_setup?project=1", "", &setup)

	if project.DSN != want || setup.DSN != want {
		t.Errorf("DSN %s, setup %s, want %s", project.DSN, setup.DSN, want)
	}
}

// The setup page: every platform's code with the project's DSN, a prompt
// for an agent, whether the project is connected (any request with its key,
// a session at the SDK's start too) and whether it has stored anything; open
// to every role, as the DSN is on the Projects page.
func TestUIProjectSetup(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	type setupAnswer struct {
		Name        string     `json:"name"`
		DSN         string     `json:"dsn"`
		Received    bool       `json:"received"`
		Connected   bool       `json:"connected"`
		ConnectedAt *time.Time `json:"connected_at"`
		Platforms   []struct {
			ID          string `json:"id"`
			Init        string `json:"init"`
			AgentPrompt string `json:"agent_prompt"`
		} `json:"platforms"`
	}

	var setup setupAnswer

	user.request(http.MethodGet, "get_project_setup?project=1", "", &setup)

	if setup.Name != "backend" || setup.Received || setup.Connected || setup.ConnectedAt != nil || len(setup.Platforms) < 6 || setup.Platforms[0].ID != "node" {
		t.Fatalf("%+v", setup)
	}

	for _, platform := range setup.Platforms {
		if !strings.Contains(platform.Init, setup.DSN) || !strings.Contains(platform.AgentPrompt, setup.DSN) || strings.Contains(platform.Init, "{dsn}") {
			t.Errorf("%s: %q, prompt %q", platform.ID, platform.Init, platform.AgentPrompt)
		}
	}

	server.sendEnvelope("{}\n{\"type\":\"session\"}\n{\"sid\":\"7c3a1f0e9d2b4c5a8e6f1a2b3c4d5e6f\",\"status\":\"ok\",\"init\":true}\n")
	user.request(http.MethodGet, "get_project_setup?project=1", "", &setup)

	if !setup.Connected || setup.ConnectedAt == nil || setup.Received {
		t.Errorf("after a session: connected %v at %v, received %v", setup.Connected, setup.ConnectedAt, setup.Received)
	}

	server.ingestFixtures()
	user.request(http.MethodGet, "get_project_setup?project=1", "", &setup)

	if !setup.Received || !setup.Connected {
		t.Error("a project with errors has received nothing")
	}
}

// Every role sees each project's part of the database and when it last received.
func TestUIStorageByProject(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	var storage struct {
		UsedBytes  int64 `json:"used_bytes"`
		OtherBytes int64 `json:"other_bytes"`
		Counting   bool  `json:"counting"`
		Projects   []struct {
			ID           int        `json:"id"`
			Bytes        int64      `json:"bytes"`
			Size         string     `json:"size"`
			LastReceived *time.Time `json:"last_received"`
		} `json:"projects"`
	}

	server.webUser(store.RoleUser, false).request(http.MethodGet, "get_storage", "", &storage)

	if storage.Counting || len(storage.Projects) != len(fixtures.Projects()) {
		t.Fatalf("%+v", storage)
	}

	total := storage.OtherBytes

	for _, project := range storage.Projects {
		if project.Bytes <= 0 || project.Size == "" || project.LastReceived == nil {
			t.Errorf("project %+v", project)
		}

		total += project.Bytes
	}

	if total != storage.UsedBytes || storage.OtherBytes < 0 {
		t.Errorf("parts add up to %d, used %d", total, storage.UsedBytes)
	}
}

// The Projects page shows what each project's limits dropped in the last
// day, only for the projects that dropped something.
func TestUIStorageShowsDropped(t *testing.T) {
	server := newRESTServerWith(t, func(settings *config.Config) { settings.RateLimitEventsPerMinute = 1 })

	for index := range 3 {
		body := fmt.Sprintf("{\"event_id\":\"%032x\"}\n{\"type\":\"event\"}\n{\"message\":\"hello\"}\n", index+1)
		request := httptest.NewRequest(http.MethodPost, fixtures.BackendEnvelopeURL, strings.NewReader(body))
		server.handler.ServeHTTP(httptest.NewRecorder(), request)
	}

	var storage struct {
		Projects []struct {
			ID      int `json:"id"`
			Dropped []struct {
				Kind       string `json:"kind"`
				Count      int64  `json:"count"`
				Reason     string `json:"reason"`
				ReasonText string `json:"reason_text"`
			} `json:"dropped"`
		} `json:"projects"`
	}

	server.webUser(store.RoleUser, false).request(http.MethodGet, "get_storage", "", &storage)

	for _, project := range storage.Projects {
		got := fmt.Sprint(project.Dropped)

		if project.ID == 1 && got != "[{events 2 rate_limit rate limit}]" || project.ID != 1 && len(project.Dropped) != 0 {
			t.Errorf("project %d dropped %s", project.ID, got)
		}
	}
}

// Every role reads the cleanup history.
func TestListCleanups(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	type history struct {
		StopPercent int `json:"stop_percent"`
		Kept        int `json:"kept"`
		Runs        []struct {
			Status string `json:"status"`
			Result string `json:"result"`
			Steps  []struct {
				Name     string `json:"name"`
				Projects []struct {
					Project string `json:"project"`
					Deleted string `json:"deleted"`
				} `json:"projects"`
			} `json:"steps"`
		} `json:"runs"`
	}

	var empty history

	user.request(http.MethodGet, "list_cleanups", "", &empty)

	if empty.Runs == nil || len(empty.Runs) != 0 || empty.StopPercent != config.CleanupStopPercent || empty.Kept != store.CleanupRunsKept {
		t.Fatalf("%+v", empty)
	}

	ctx := context.Background()

	if err := server.database.SaveLogs(ctx, []store.NewLog{{ProjectID: 1, Timestamp: time.Now(), Level: "info", Body: "a log to clean"}}, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := server.database.Cleanup(ctx, database.AllProjects, 1); err != nil {
		t.Fatal(err)
	}

	var runs history

	user.request(http.MethodGet, "list_cleanups", "", &runs)

	if len(runs.Runs) != 1 || runs.Runs[0].Status != store.CleanupDone || runs.Runs[0].Result == "" || len(runs.Runs[0].Steps) == 0 {
		t.Fatalf("%+v", runs)
	}

	step := runs.Runs[0].Steps[0]

	if !strings.Contains(step.Name, "logs below error") || len(step.Projects) != 1 || step.Projects[0].Project != "backend" || step.Projects[0].Deleted != "1 logs" {
		t.Errorf("step %+v", step)
	}
}

// A project's disk share: 100 unless set, 5 to 100 in steps of 5, each
// problem at its field; get_storage gives its limit and what it keeps.
func TestUIProjectDiskShare(t *testing.T) {
	const maxBytes = 1 << 30

	server := newRESTServerWith(t, func(settings *config.Config) { settings.Retention.MaxDatabaseBytes = maxBytes })
	server.ingestFixtures()

	admin := server.webUser(store.RoleAdmin, false)

	var defaults struct {
		DiskShare int `json:"disk_share"`
	}

	admin.request(http.MethodGet, "get_project_defaults", "", &defaults)

	var stored, created, updated uiProject

	admin.read("get_project?project=backend", &stored)
	admin.request(http.MethodPost, "create_project", `{"name":"mobile","disk_share":35}`, &created)
	admin.request(http.MethodPost, "update_project", `{"project":"backend","disk_share":20}`, &updated)

	if defaults.DiskShare != 100 || stored.DiskShare != 100 || created.DiskShare != 35 || updated.DiskShare != 20 {
		t.Fatalf("defaults %d, stored %d, created %d, updated %d", defaults.DiskShare, stored.DiskShare, created.DiskShare, updated.DiskShare)
	}

	for body, field := range map[string]bool{`{"project":"backend","disk_share":42}`: true, `{"project":"backend","disk_share":3}`: false, `{"project":"backend","disk_share":105}`: false} {
		response := admin.call(http.MethodPost, "update_project", body)

		var refused api.ErrorBody

		if err := json.Unmarshal(response.Body.Bytes(), &refused); err != nil || response.Code != http.StatusBadRequest || (refused.Fields["disk_share"] != "") != field {
			t.Errorf("%s: %d %s", body, response.Code, response.Body)
		}
	}

	var storage struct {
		Projects []struct {
			Name       string `json:"name"`
			LimitBytes int64  `json:"limit_bytes"`
			Oldest     struct {
				LogsBelowError *time.Time `json:"logs_below_error"`
				ErrorLogs      *time.Time `json:"error_logs"`
				Errors         *time.Time `json:"errors"`
			} `json:"oldest"`
		} `json:"projects"`
	}

	admin.request(http.MethodGet, "get_storage", "", &storage)

	for _, project := range storage.Projects {
		switch project.Name {
		case "backend":
			if project.LimitBytes != maxBytes/5 || project.Oldest.Errors == nil {
				t.Errorf("backend: %+v", project)
			}
		case "mobile":
			if project.LimitBytes != maxBytes*35/100 || project.Oldest.Errors != nil || project.Oldest.LogsBelowError != nil || project.Oldest.ErrorLogs != nil {
				t.Errorf("mobile: %+v", project)
			}
		default:
			if project.LimitBytes != 0 {
				t.Errorf("%s at 100%%: limit %d", project.Name, project.LimitBytes)
			}
		}
	}
}

// A run of one project names it; a run over all projects names none.
func TestListCleanupsOfAProject(t *testing.T) {
	server := newRESTServer(t)
	ctx := context.Background()

	if err := server.database.SaveLogs(ctx, []store.NewLog{{ProjectID: 1, Timestamp: time.Now(), Level: "info", Body: "a log to clean"}}, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := server.database.Cleanup(ctx, 1, 1); err != nil {
		t.Fatal(err)
	}

	if _, err := server.database.Cleanup(ctx, database.AllProjects, 1); err != nil {
		t.Fatal(err)
	}

	var history struct {
		Runs []struct {
			Project *struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"project"`
		} `json:"runs"`
	}

	server.webUser(store.RoleUser, false).request(http.MethodGet, "list_cleanups", "", &history)

	if len(history.Runs) != 2 || history.Runs[0].Project != nil || history.Runs[1].Project == nil || *history.Runs[1].Project != struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}{1, "backend"} {
		t.Fatalf("%+v", history.Runs)
	}
}

// A project's allowed domains are saved as typed in any case and apply to
// ingest at once: a browser of another site is refused, a backend SDK is
// not checked; an empty list takes any site again.
func TestUIProjectAllowedOrigins(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	var project struct {
		AllowedOrigins []string `json:"allowed_origins"`
	}

	admin.request(http.MethodPost, "update_project", `{"project":"1","allowed_origins":["https://App.example.com","https://*.example.org"," "]}`, &project)

	if strings.Join(project.AllowedOrigins, " ") != "https://app.example.com https://*.example.org" {
		t.Fatalf("saved %q", project.AllowedOrigins)
	}

	send := func(origin string) int {
		request := httptest.NewRequest(http.MethodPost, fixtures.BackendEnvelopeURL, strings.NewReader("{}\n"))

		if origin != "" {
			request.Header.Set("Origin", origin)
		}

		return serve(server, request).Code
	}

	for origin, allowed := range map[string]bool{"https://app.example.com": true, "https://eu.example.org": true, "https://evil.example.com": false, "": true} {
		if code := send(origin); (code != http.StatusForbidden) != allowed {
			t.Errorf("origin %q: %d", origin, code)
		}
	}

	admin.request(http.MethodPost, "update_project", `{"project":"1","allowed_origins":[]}`, &project)

	if len(project.AllowedOrigins) != 0 || send("https://evil.example.com") == http.StatusForbidden {
		t.Errorf("cleared: %q", project.AllowedOrigins)
	}
}

// Resetting a DSN: the old key is refused at once, the new one works, the
// setup says when; a project from PROJECTS keeps the key it has there, and
// a user resets only a project they have.
func TestUIResetProjectKey(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	var created uiProject

	admin.request(http.MethodPost, "create_project", `{"name":"mobile"}`, &created)

	send := func(dsn string) int {
		match := dsnPattern.FindStringSubmatch(dsn)

		if match == nil {
			t.Fatalf("DSN %q", dsn)
		}

		response := httptest.NewRecorder()
		url := "/api/" + match[2] + "/envelope/?sentry_key=" + match[1]
		server.handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, url, strings.NewReader("{}\n{\"type\":\"event\"}\n{\"message\":\"from mobile\"}")))

		return response.Code
	}

	var setup struct {
		DSN          string     `json:"dsn"`
		KeyChangedAt *time.Time `json:"key_changed_at"`
		CreatedAt    time.Time  `json:"created_at"`
	}

	id := strconv.Itoa(created.ID)
	admin.request(http.MethodGet, "get_project_setup?project="+id, "", &setup)
	old := setup.DSN

	if setup.KeyChangedAt != nil || setup.CreatedAt.IsZero() || send(old) != http.StatusOK {
		t.Fatalf("before the reset: %+v", setup)
	}

	user := server.webUserWith(store.RoleUser, store.OnlyProjects(created.ID))
	user.request(http.MethodPost, "reset_project_key", `{"project":`+id+`}`, &setup)

	if setup.DSN == old || setup.KeyChangedAt == nil || send(old) != http.StatusForbidden || send(setup.DSN) != http.StatusOK {
		t.Errorf("after the reset: %+v, the old one %d", setup, send(old))
	}

	// Settings saved afterwards keep the new key.
	admin.request(http.MethodPost, "update_project", `{"project":"mobile","color":"#654321"}`, &struct{}{})

	if send(setup.DSN) != http.StatusOK {
		t.Error("a settings save brought the old key back")
	}

	// A project from PROJECTS resets too.
	if response := admin.call(http.MethodPost, "reset_project_key", `{"project":1}`); response.Code != http.StatusOK || strings.Contains(response.Body.String(), fixtures.BackendKey) {
		t.Errorf("a project from PROJECTS: %d %s", response.Code, response.Body)
	}

	if response := server.webUserWith(store.RoleUser, store.OnlyProjects(1)).call(http.MethodPost, "reset_project_key", `{"project":`+id+`}`); response.Code != http.StatusForbidden {
		t.Errorf("a user without the project: %d", response.Code)
	}
}

// A paused project's apps are answered but nothing is stored until it is
// resumed; a user pauses a project they have, not another.
func TestUIPauseProject(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	var created uiProject

	admin.request(http.MethodPost, "create_project", `{"name":"mobile"}`, &created)

	match := dsnPattern.FindStringSubmatch(created.DSN)
	url := "/api/" + match[2] + "/envelope/?sentry_key=" + match[1]
	stored := func(message string) int {
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, url, strings.NewReader("{}\n{\"type\":\"event\"}\n{\"message\":\""+message+"\"}")))
		waitStored(t, server.app)

		if response.Code != http.StatusOK {
			t.Fatalf("envelope %q: %d", message, response.Code)
		}

		var list struct {
			Items []struct {
				Title string `json:"title"`
			} `json:"items"`
		}

		admin.request(http.MethodGet, "list_issues?project=mobile&query="+message, "", &list)

		return len(list.Items)
	}

	user := server.webUserWith(store.RoleUser, store.OnlyProjects(created.ID))

	var project struct {
		Paused bool `json:"paused"`
	}

	user.request(http.MethodPost, "set_project_active", `{"project":"mobile","active":false}`, &project)

	if !project.Paused || stored("while-paused") != 0 {
		t.Errorf("paused: %+v, stored while paused", project)
	}

	var resumed struct {
		Paused bool `json:"paused"`
	}

	user.request(http.MethodPost, "set_project_active", `{"project":"mobile","active":true}`, &resumed)

	if resumed.Paused || stored("after-resume") != 1 {
		t.Errorf("resumed: %+v", resumed)
	}

	if response := server.webUserWith(store.RoleUser, store.OnlyProjects(1)).call(http.MethodPost, "set_project_active", `{"project":"mobile","active":false}`); response.Code != http.StatusForbidden {
		t.Errorf("a user without the project: %d", response.Code)
	}
}
