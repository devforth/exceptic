package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The data of the project access tests: an error in each project, and logs
// that share a trace and a span with the other project's error.
const (
	backendTrace    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // the backend error's; only a frontend log is in it
	frontendTrace   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" // the frontend error's; a backend log is in its span
	backendSpan     = "b0b0b0b0b0b0b0b0"
	frontendSpan    = "f0f0f0f0f0f0f0f0"
	frontendEventID = "ffffffffffffffffffffffffffffffff"
)

type accessData struct {
	backendIssue, frontendIssue string
}

// seedProjects stores a backend error (level error) and a frontend one
// (fatal, a level only frontend has), a frontend debug log in the backend
// error's span and a backend info log in the frontend error's span.
func seedProjects(t *testing.T, database *database.Store) accessData {
	t.Helper()

	ctx, now := context.Background(), time.Now()
	save := func(event store.NewEvent) string {
		event.Timestamp, event.ReceivedAt, event.Payload = now, now, []byte(`{"message":"`+event.Title+`"}`)
		result, err := database.SaveEvent(ctx, event)

		if err != nil {
			t.Fatal(err)
		}

		return result.IssueID
	}

	data := accessData{
		backendIssue: save(store.NewEvent{
			ProjectID: 1, EventID: strings.Repeat("e", 32), Fingerprint: "access-backend", Title: "Backend access error", Level: "error",
			TraceID: backendTrace, SpanID: backendSpan,
		}),
		frontendIssue: save(store.NewEvent{
			ProjectID: 2, EventID: frontendEventID, Fingerprint: "access-frontend", Title: "Frontend access error", Level: "fatal",
			TraceID: frontendTrace, SpanID: frontendSpan,
		}),
	}

	logs := []store.NewLog{
		{ProjectID: 2, Timestamp: now, Level: "debug", Body: "frontend log in the backend request", TraceID: backendTrace, SpanID: backendSpan},
		{ProjectID: 1, Timestamp: now, Level: "info", Body: "backend log in the frontend request", TraceID: frontendTrace, SpanID: frontendSpan},
	}

	if err := database.SaveLogs(ctx, logs, 0); err != nil {
		t.Fatal(err)
	}

	return data
}

// webUserWith adds a user with role and access, and a session for it.
func (server *restServer) webUserWith(role string, access store.Access) uiUser {
	server.t.Helper()

	return server.addWebUser(store.User{Role: role, Access: access})
}

type accessIssues struct {
	Items []struct {
		ID      string `json:"id"`
		Project string `json:"project"`
	} `json:"items"`
}

// accessList is a page of anything.
type accessList struct {
	Items []json.RawMessage `json:"items"`
}

func (list accessIssues) projects() []string {
	var names []string

	for _, item := range list.Items {
		if !slices.Contains(names, item.Project) {
			names = append(names, item.Project)
		}
	}

	slices.Sort(names)

	return names
}

// hiddenOrMissing checks that asking for something of a project outside
// the user's is forbidden, saying so, and for something that does not
// exist is missingStatus (404, or 400 for a form's field), without the
// other answer's words.
func hiddenOrMissing(t *testing.T, label string, hidden, missing func() (int, string), missingStatus int) {
	t.Helper()
	forbiddenOrMissing(t, label, hidden, missing, "not in your projects", missingStatus)
}

// forbiddenOrMissing checks that a refused request is 403 giving reason and
// one for something that does not exist missingStatus, without the reason.
func forbiddenOrMissing(t *testing.T, label string, forbidden, missing func() (int, string), reason string, missingStatus int) {
	t.Helper()

	forbiddenCode, forbiddenBody := forbidden()
	missingCode, missingBody := missing()

	if forbiddenCode != http.StatusForbidden || !strings.Contains(forbiddenBody, reason) {
		t.Errorf("%s: refused %d %s, want 403 %q", label, forbiddenCode, forbiddenBody, reason)
	}

	if missingCode != missingStatus || strings.Contains(missingBody, reason) {
		t.Errorf("%s: missing %d %s, want %d", label, missingCode, missingBody, missingStatus)
	}
}

func (user uiUser) answer(method, target, body string) func() (int, string) {
	return func() (int, string) {
		response := user.call(method, target, body)

		return response.Code, strings.TrimSpace(response.Body.String())
	}
}

// A user with some projects sees nothing of the others in the issue
// list, its counts and levels, the issue page and the menu.
func TestProjectAccessIssues(t *testing.T) {
	server := newRESTServer(t)
	data := seedProjects(t, server.database)
	everyone := server.webUser(store.RoleUser, false) // stored before access existed: every project
	backend := server.webUserWith(store.RoleUser, store.OnlyProjects(1))

	var list accessIssues

	everyone.read("list_issues", &list)

	if got := list.projects(); !slices.Equal(got, []string{"backend", "frontend"}) {
		t.Fatalf("a user without access set sees %v", got)
	}

	backend.read("list_issues", &list)

	if got := list.projects(); !slices.Equal(got, []string{"backend"}) {
		t.Errorf("list_issues: %v", got)
	}

	if count := backend.count("status=all"); count != 1 {
		t.Errorf("count_issues: %d", count)
	}

	hiddenOrMissing(t, "list_issues of a hidden project", backend.answer(http.MethodGet, "list_issues?project=frontend", ""), backend.answer(http.MethodGet, "list_issues?project=nosuch", ""), http.StatusNotFound)
	hiddenOrMissing(t, "count_issues of a hidden project", backend.answer(http.MethodGet, "count_issues?project=2", ""), backend.answer(http.MethodGet, "count_issues?project=999", ""), http.StatusNotFound)

	var levels struct {
		Levels []string `json:"levels"`
	}

	backend.read("list_issue_levels", &levels)

	if !slices.Equal(levels.Levels, []string{"error"}) {
		t.Errorf("list_issue_levels: %v", levels.Levels)
	}

	missingID := store.NewID()

	for _, endpoint := range []string{"get_issue_details?issue_id=", "list_issue_events?issue_id="} {
		hiddenOrMissing(t, endpoint, backend.answer(http.MethodGet, endpoint+data.frontendIssue, ""), backend.answer(http.MethodGet, endpoint+missingID, ""), http.StatusNotFound)
	}

	// A bulk action on a hidden issue skips it, as a deleted one.
	if code := backend.act("resolve", data.backendIssue, data.frontendIssue); code != http.StatusOK {
		t.Fatalf("resolve: %d", code)
	}

	everyone.read("list_issues?status=resolved", &list)

	if len(list.Items) != 1 || list.Items[0].ID != data.backendIssue {
		t.Errorf("resolved: %+v", list.Items)
	}

	var session struct {
		UnresolvedIssues int               `json:"unresolved_issues"`
		ProjectColors    map[string]string `json:"project_colors"`
	}

	backend.read("get_session", &session)

	if _, ok := session.ProjectColors["frontend"]; ok || session.UnresolvedIssues != 0 || len(session.ProjectColors) != 1 {
		t.Errorf("get_session: %+v", session)
	}

	// A limited admin's delete skips the hidden issue too.
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))

	if code := admin.act("delete", data.backendIssue, data.frontendIssue); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}

	everyone.read("list_issues?status=all", &list)

	if len(list.Items) != 1 || list.Items[0].ID != data.frontendIssue {
		t.Errorf("after the delete: %+v", list.Items)
	}
}

// The issue page and the logs show no log of a hidden project, by trace or
// span, and no issue of one next to a log.
func TestProjectAccessLogs(t *testing.T) {
	server := newRESTServer(t)
	data := seedProjects(t, server.database)
	everyone := server.webUser(store.RoleUser, false)
	backend := server.webUserWith(store.RoleUser, store.OnlyProjects(1))

	type page struct {
		EventState string `json:"event_state"`
		Event      *struct {
			TraceHasLogs bool `json:"trace_has_logs"`
			RelatedLogs  []struct {
				Body string `json:"body"`
			} `json:"related_logs"`
		} `json:"event"`
		AgentPrompt string `json:"agent_prompt"`
	}

	var full, scoped page

	everyone.read("get_issue_details?issue_id="+data.backendIssue, &full)
	backend.read("get_issue_details?issue_id="+data.backendIssue, &scoped)

	if full.Event == nil || len(full.Event.RelatedLogs) != 1 || !full.Event.TraceHasLogs {
		t.Fatalf("every project: %+v", full.Event)
	}

	if scoped.Event == nil || len(scoped.Event.RelatedLogs) != 0 || scoped.Event.TraceHasLogs || strings.Contains(scoped.AgentPrompt, "frontend log") {
		t.Errorf("backend only: %+v", scoped.Event)
	}

	type logs struct {
		Items []struct {
			ID      int64  `json:"id"`
			Project string `json:"project"`
			Body    string `json:"body"`
			Issue   *struct {
				ID string `json:"id"`
			} `json:"issue"`
		} `json:"items"`
	}

	var all, own logs

	everyone.read("list_logs", &all)
	backend.read("list_logs", &own)

	if len(all.Items) != 2 {
		t.Fatalf("every project: %+v", all.Items)
	}

	if len(own.Items) != 1 || own.Items[0].Project != "backend" || own.Items[0].Issue != nil {
		t.Errorf("backend only: %+v", own.Items)
	}

	for _, item := range all.Items {
		if item.Project == "backend" && (item.Issue == nil || item.Issue.ID != data.frontendIssue) {
			t.Errorf("the backend log's request issue: %+v", item)
		}
	}

	backend.read("list_logs?trace_id="+backendTrace, &own)

	if len(own.Items) != 0 {
		t.Errorf("a trace with only frontend logs: %+v", own.Items)
	}

	var count struct {
		Count int `json:"count"`
	}

	backend.read("count_logs", &count)

	if count.Count != 1 {
		t.Errorf("count_logs: %d", count.Count)
	}

	var levels struct {
		Levels []string `json:"levels"`
	}

	backend.read("list_log_levels", &levels)

	if !slices.Equal(levels.Levels, []string{"info"}) {
		t.Errorf("list_log_levels: %v", levels.Levels)
	}

	hiddenOrMissing(t, "list_logs of a hidden project", backend.answer(http.MethodGet, "list_logs?project=frontend", ""), backend.answer(http.MethodGet, "list_logs?project=nosuch", ""), http.StatusNotFound)

	// A limited admin's delete of both logs deletes only his project's.
	var ids []int64

	for _, item := range all.Items {
		ids = append(ids, item.ID)
	}

	body, err := json.Marshal(map[string]any{"log_ids": ids})

	if err != nil {
		t.Fatal(err)
	}

	var deleted struct {
		Deleted int `json:"deleted"`
	}

	server.webUserWith(store.RoleAdmin, store.OnlyProjects(1)).request(http.MethodPost, "delete_logs", string(body), &deleted)
	everyone.read("list_logs", &all)

	if deleted.Deleted != 1 || len(all.Items) != 1 || all.Items[0].Project != "frontend" {
		t.Errorf("after deleting both: deleted %d, left %+v", deleted.Deleted, all.Items)
	}
}

// The Projects page shows only the user's projects: the list, setup, a
// project's settings and the parts of the storage; the database total stays.
func TestProjectAccessProjects(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	everyone := server.webUser(store.RoleUser, false)
	backend := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))

	var projects accessList

	backend.read("list_projects", &projects)

	if len(projects.Items) != 1 {
		t.Errorf("list_projects: %+v", projects.Items)
	}

	hiddenOrMissing(t, "get_project_setup", backend.answer(http.MethodGet, "get_project_setup?project=2", ""), backend.answer(http.MethodGet, "get_project_setup?project=999", ""), http.StatusNotFound)
	hiddenOrMissing(t, "get_project", backend.answer(http.MethodGet, "get_project?project=frontend", ""), backend.answer(http.MethodGet, "get_project?project=nosuch", ""), http.StatusNotFound)
	hiddenOrMissing(t, "update_project", backend.answer(http.MethodPost, "update_project", `{"project":"frontend","name":"renamed"}`), backend.answer(http.MethodPost, "update_project", `{"project":"nosuch","name":"renamed"}`), http.StatusNotFound)
	hiddenOrMissing(t, "delete_project", backend.answer(http.MethodPost, "delete_project", `{"project":"frontend"}`), backend.answer(http.MethodPost, "delete_project", `{"project":"nosuch"}`), http.StatusNotFound)

	type storage struct {
		UsedBytes  int64 `json:"used_bytes"`
		OtherBytes int64 `json:"other_bytes"`
		Projects   []struct {
			ID    int   `json:"id"`
			Bytes int64 `json:"bytes"`
		} `json:"projects"`
	}

	var full, scoped storage

	everyone.read("get_storage", &full)
	backend.read("get_storage", &scoped)

	if len(full.Projects) != 2 || len(scoped.Projects) != 1 || scoped.Projects[0].ID != 1 {
		t.Fatalf("parts: %+v, %+v", full.Projects, scoped.Projects)
	}

	if scoped.UsedBytes != full.UsedBytes || scoped.OtherBytes != scoped.UsedBytes-scoped.Projects[0].Bytes {
		t.Errorf("total %d (every project %d), other %d", scoped.UsedBytes, full.UsedBytes, scoped.OtherBytes)
	}
}

// The cleanup history leaves out runs of a hidden project and shows a run
// over the database without the hidden projects' counts.
func TestProjectAccessCleanups(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)

	ctx := context.Background()

	if _, err := server.database.Cleanup(ctx, 2, 1); err != nil {
		t.Fatal(err)
	}

	seedProjects(t, server.database)

	if _, err := server.database.Cleanup(ctx, database.AllProjects, 1); err != nil {
		t.Fatal(err)
	}

	type history struct {
		Runs []struct {
			Project *struct {
				Name string `json:"name"`
			} `json:"project"`
			Steps []struct {
				Projects []struct {
					Project string `json:"project"`
				} `json:"projects"`
			} `json:"steps"`
		} `json:"runs"`
	}

	projectsOf := func(runs history) []string {
		var names []string

		for _, run := range runs.Runs {
			if run.Project != nil {
				names = append(names, "run of "+run.Project.Name)
			}

			for _, step := range run.Steps {
				for _, project := range step.Projects {
					if !slices.Contains(names, project.Project) {
						names = append(names, project.Project)
					}
				}
			}
		}

		slices.Sort(names)

		return names
	}

	var full, scoped history

	server.webUser(store.RoleUser, false).read("list_cleanups", &full)
	server.webUserWith(store.RoleUser, store.OnlyProjects(1)).read("list_cleanups", &scoped)

	if got := projectsOf(full); !slices.Equal(got, []string{"backend", "frontend", "run of frontend"}) {
		t.Fatalf("every project: %v", got)
	}

	if got := projectsOf(scoped); len(scoped.Runs) != 1 || !slices.Equal(got, []string{"backend"}) {
		t.Errorf("backend only: %d runs, %v", len(scoped.Runs), got)
	}
}

// A user whose list is empty sees no project, never every one.
func TestProjectAccessNone(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	nobody := server.webUserWith(store.RoleAdmin, store.OnlyProjects())

	var list accessList

	for _, endpoint := range []string{"list_issues?status=all", "list_projects", "list_logs"} {
		if nobody.read(endpoint, &list); len(list.Items) != 0 {
			t.Errorf("%s: %+v", endpoint, list.Items)
		}
	}

	var levels struct {
		Levels []string `json:"levels"`
	}

	for _, endpoint := range []string{"list_issue_levels", "list_log_levels"} {
		if nobody.read(endpoint, &levels); len(levels.Levels) != 0 {
			t.Errorf("%s: %v", endpoint, levels.Levels)
		}
	}

	var session struct {
		UnresolvedIssues int               `json:"unresolved_issues"`
		ProjectColors    map[string]string `json:"project_colors"`
	}

	if nobody.read("get_session", &session); session.UnresolvedIssues != 0 || len(session.ProjectColors) != 0 {
		t.Errorf("get_session: %+v", session)
	}

	var storage struct {
		Projects []json.RawMessage `json:"projects"`
	}

	if nobody.read("get_storage", &storage); len(storage.Projects) != 0 {
		t.Errorf("get_storage: %d parts", len(storage.Projects))
	}
}

// The REST API's token sees every project, whatever users are limited to.
func TestProjectAccessREST(t *testing.T) {
	server := newRESTServer(t)
	data := seedProjects(t, server.database)
	server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))

	var list accessIssues

	server.get("/api/v1/list_issues", &list)

	if got := list.projects(); !slices.Equal(got, []string{"backend", "frontend"}) {
		t.Errorf("list_issues: %v", got)
	}

	if response := server.do(http.MethodGet, "/api/v1/get_issue?"+url.Values{"issue_id": {data.frontendIssue}}.Encode(), ""); response.Code != http.StatusOK {
		t.Errorf("get_issue: %d %s", response.Code, response.Body)
	}
}

// asWith is the server seen by an agent of a new user with role and access.
func (server *testServer) asWith(role string, access store.Access) *testServer {
	server.t.Helper()

	ctx := context.Background()
	user := store.User{Username: role + "-" + store.NewID(), Role: role, Access: access, CreatedAt: time.Now()}
	id, err := server.database.CreateUser(ctx, user, "x")

	if err != nil {
		server.t.Fatal(err)
	}

	token, err := server.authorization.NewPersonalToken(ctx, id)

	if err != nil {
		server.t.Fatal(err)
	}

	other := *server
	other.token, other.session = token, nil

	return &other
}

// The tools of an agent of a limited user see only their projects; asking
// for another's records is forbidden, told apart from missing ones.
func TestProjectAccessMCP(t *testing.T) {
	server := newTestServer(t)
	data := seedProjects(t, server.database)
	user := server.asWith(store.RoleUser, store.OnlyProjects(1))
	admin := server.asWith(store.RoleAdmin, store.OnlyProjects(1))

	var list accessIssues

	user.decode("list_issues", map[string]any{"status": "all", "limit": 100}, &list)

	if got := list.projects(); !slices.Equal(got, []string{"backend"}) {
		t.Errorf("list_issues: %v", got)
	}

	var projects accessList

	user.decode("list_projects", nil, &projects)

	if len(projects.Items) != 1 {
		t.Errorf("list_projects: %d", len(projects.Items))
	}

	var batch struct {
		Issues    []json.RawMessage `json:"issues"`
		Missing   []string          `json:"missing_issue_ids"`
		Forbidden []string          `json:"forbidden_issue_ids"`
	}

	user.decode("get_issues", map[string]any{"issue_ids": []string{data.backendIssue, data.frontendIssue}}, &batch)

	if len(batch.Issues) != 1 || len(batch.Missing) != 0 || !slices.Equal(batch.Forbidden, []string{data.frontendIssue}) {
		t.Errorf("get_issues: %d issues, missing %v, forbidden %v", len(batch.Issues), batch.Missing, batch.Forbidden)
	}

	// A tool error's kind as the status it stands for.
	tool := func(server *testServer, name string, arguments map[string]any) func() (int, string) {
		return func() (int, string) {
			text, isError := server.try(name, arguments)

			switch {
			case !isError:
				return http.StatusOK, text
			case strings.HasPrefix(text, "forbidden: "):
				return http.StatusForbidden, text
			case strings.HasPrefix(text, "not found"):
				return http.StatusNotFound, text
			default:
				return http.StatusBadRequest, text
			}
		}
	}

	missingID := store.NewID()

	for _, name := range []string{"get_issue", "list_issue_events"} {
		hiddenOrMissing(t, name, tool(user, name, map[string]any{"issue_id": data.frontendIssue}), tool(user, name, map[string]any{"issue_id": missingID}), http.StatusNotFound)
	}

	hiddenOrMissing(t, "get_event", tool(user, "get_event", map[string]any{"project": "frontend", "event_id": frontendEventID}), tool(user, "get_event", map[string]any{"project": "nosuch", "event_id": frontendEventID}), http.StatusNotFound)
	hiddenOrMissing(t, "update_issue_status", tool(user, "update_issue_status", map[string]any{"issue_ids": []string{data.frontendIssue}, "status": "resolved"}), tool(user, "update_issue_status", map[string]any{"issue_ids": []string{missingID}, "status": "resolved"}), http.StatusNotFound)
	hiddenOrMissing(t, "delete_issue", tool(admin, "delete_issue", map[string]any{"issue_id": data.frontendIssue}), tool(admin, "delete_issue", map[string]any{"issue_id": missingID}), http.StatusNotFound)
	hiddenOrMissing(t, "get_project", tool(admin, "get_project", map[string]any{"project": "frontend"}), tool(admin, "get_project", map[string]any{"project": "nosuch"}), http.StatusNotFound)

	var logs struct {
		Items []struct {
			Project string `json:"project"`
		} `json:"items"`
	}

	user.decode("list_logs", map[string]any{"limit": 100}, &logs)

	for _, item := range logs.Items {
		if item.Project != "backend" {
			t.Errorf("list_logs: a %s log", item.Project)
		}
	}

	// Nothing changed: the frontend issue is still there and unresolved.
	server.decode("get_issues", map[string]any{"issue_ids": []string{data.frontendIssue}}, &batch)

	if len(batch.Issues) != 1 || !strings.Contains(string(batch.Issues[0]), `"unresolved"`) {
		t.Errorf("the frontend issue: %s", batch.Issues)
	}
}
