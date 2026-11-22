package api_test

import (
	"context"
	"encoding/json"
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
	"github.com/devforth/exceptic/internal/storage/store"
)

type uiProject struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	DSN         string   `json:"dsn"`
	FromEnv     bool     `json:"from_env"`
	Color       string   `json:"color"`
	ShownColor  string   `json:"shown_color"`
	EventLevels []string `json:"event_levels"`
	LogLevels   []string `json:"log_levels"`
}

// An unknown project is not found for the project page (the web app shows
// its 404 page); an agent still reads which project it did not know.
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

	if text, isError := agent.try("get_project", map[string]any{"project": "nope"}); !isError || !strings.Contains(text, `unknown project "nope" (see list_projects)`) {
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
		settings.Recording = config.Recording{EventLevels: []string{"error", "fatal"}, LogLevels: []string{"warn", "error"}}
	})

	var defaults struct {
		ShownColor  string   `json:"shown_color"`
		EventLevels []string `json:"event_levels"`
		LogLevels   []string `json:"log_levels"`
	}

	server.webUser(store.RoleAdmin, false).request(http.MethodGet, "get_project_defaults", "", &defaults)

	next := config.DefaultProjectColor(len(fixtures.Projects()) + 1)

	if defaults.ShownColor != next || !slices.Equal(defaults.EventLevels, []string{"fatal", "error"}) || !slices.Equal(defaults.LogLevels, []string{"error", "warn"}) {
		t.Errorf("%+v, want color %s", defaults, next)
	}

	if response := server.webUser(store.RoleUser, false).call(http.MethodGet, "get_project_defaults", ""); response.Code != http.StatusNotFound {
		t.Errorf("a user: %d", response.Code)
	}
}

// The project form shows each problem at its field.
func TestUIProjectFormErrors(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)

	for _, test := range []struct{ name, body, field, want string }{
		{"create_project", `{"name":"Not Valid"}`, "name", "lowercase letters"},
		{"create_project", `{"name":"backend"}`, "name", "Another project already has this name."},
		{"update_project", `{"project":"1","color":"red"}`, "color", "Choose a color."},
	} {
		response := admin.call(http.MethodPost, test.name, test.body)

		var body api.ErrorBody

		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusBadRequest || !strings.Contains(body.Fields[test.field], test.want) {
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

	admin.request(http.MethodPost, "create_project", `{"name":"mobile","color":"#123456","event_levels":["fatal","error"],"log_levels":["error","warn"]}`, &created)

	match := dsnPattern.FindStringSubmatch(created.DSN)

	if match == nil || created.Color != "#123456" || !slices.Equal(created.EventLevels, []string{"fatal", "error"}) {
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

	if err != nil || len(issues) != 1 || issues[0].Title != "from mobile" {
		t.Fatalf("issues of the new project: %+v %v", issues, err)
	}

	var updated uiProject

	admin.request(http.MethodPost, "update_project", `{"project":"`+strconv.Itoa(created.ID)+`","name":"mobile-app","event_levels":["error"]}`, &updated)

	if updated.Name != "mobile-app" || updated.Color != "#123456" || !slices.Equal(updated.EventLevels, []string{"error"}) {
		t.Errorf("updated %+v", updated)
	}

	admin.request(http.MethodPost, "delete_project", `{"project":"`+strconv.Itoa(created.ID)+`"}`, &struct{}{})

	if err := server.database.PurgeProjects(ctx); err != nil {
		t.Fatal(err)
	}

	if count, _ := server.database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{created.ID}}); count != 0 {
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

	if response := admin.call(http.MethodPost, "delete_project", `{"project":"1"}`); response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "PROJECTS") {
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
// for an agent, and whether the project has received anything; open to
// every role, as the DSN is on the Projects page.
func TestUIProjectSetup(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	type setupAnswer struct {
		Name      string `json:"name"`
		DSN       string `json:"dsn"`
		Received  bool   `json:"received"`
		Platforms []struct {
			ID          string `json:"id"`
			Init        string `json:"init"`
			AgentPrompt string `json:"agent_prompt"`
		} `json:"platforms"`
	}

	var setup setupAnswer

	user.request(http.MethodGet, "get_project_setup?project=1", "", &setup)

	if setup.Name != "backend" || setup.Received || len(setup.Platforms) < 6 || setup.Platforms[0].ID != "node" {
		t.Fatalf("%+v", setup)
	}

	for _, platform := range setup.Platforms {
		if !strings.Contains(platform.Init, setup.DSN) || !strings.Contains(platform.AgentPrompt, setup.DSN) || strings.Contains(platform.Init, "{dsn}") {
			t.Errorf("%s: %q, prompt %q", platform.ID, platform.Init, platform.AgentPrompt)
		}
	}

	server.ingestFixtures()
	user.request(http.MethodGet, "get_project_setup?project=1", "", &setup)

	if !setup.Received {
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

	if _, err := server.database.Cleanup(ctx, 1); err != nil {
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
