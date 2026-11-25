package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/storage/store"
)

// wantStatus checks a response's status and that its error says want.
func wantStatus(t *testing.T, label string, response *httptest.ResponseRecorder, status int, want string) {
	t.Helper()

	if response.Code != status || !strings.Contains(errorOf(t, response).Error, want) {
		t.Errorf("%s: %d %s, want %d with %q", label, response.Code, strings.TrimSpace(response.Body.String()), status, want)
	}
}

// The web app tells a record outside the user's projects (403) from one
// that does not exist (404), and a refusal of the role (403) from both.
func TestUIErrorCodes(t *testing.T) {
	server := newRESTServer(t)
	data := seedProjects(t, server.database)
	backend := server.webUserWith(store.RoleUser, store.OnlyProjects(1))
	missingID := store.NewID()

	for _, endpoint := range []string{"get_issue_details?issue_id=", "list_issue_events?issue_id="} {
		wantStatus(t, endpoint+"hidden", backend.call(http.MethodGet, endpoint+data.frontendIssue, ""), http.StatusForbidden, "is not in your projects")
		wantStatus(t, endpoint+"missing", backend.call(http.MethodGet, endpoint+missingID, ""), http.StatusNotFound, "not found")
	}

	if body := backend.call(http.MethodGet, "get_issue_details?issue_id="+data.frontendIssue, "").Body.String(); strings.Contains(body, "Frontend access error") {
		t.Errorf("a refusal shows the hidden issue: %s", body)
	}

	wantStatus(t, "list_issues of a hidden project", backend.call(http.MethodGet, "list_issues?project=frontend", ""), http.StatusForbidden, `project "frontend" is not in your projects`)
	wantStatus(t, "list_issues of a missing project", backend.call(http.MethodGet, "list_issues?project=nosuch", ""), http.StatusNotFound, `project "nosuch"`)
	wantStatus(t, "list_logs of a hidden project", backend.call(http.MethodGet, "list_logs?project=2", ""), http.StatusForbidden, "is not in your projects")

	// The role: an admin endpoint, deleting issues.
	wantStatus(t, "an admin endpoint", backend.call(http.MethodGet, "get_project_defaults", ""), http.StatusForbidden, "your role may not")
	wantStatus(t, "delete by a user", backend.call(http.MethodPost, "apply_issue_action", fmt.Sprintf(`{"action":"delete","issue_ids":[%q]}`, data.backendIssue)), http.StatusForbidden, "your role may not delete issues")

	// A temporary password opens only the account page.
	temporary := server.webUser(store.RoleAdmin, true)
	wantStatus(t, "temporary password", temporary.call(http.MethodGet, "list_issues", ""), http.StatusForbidden, "change your password first")
	wantStatus(t, "temporary password, plain request", temporary.call(http.MethodPost, "personal_token", "{}"), http.StatusForbidden, "change your password first")

	// Unknown names stay 404; a known name with another method is 405.
	wantStatus(t, "unknown endpoint", backend.call(http.MethodGet, "nope", ""), http.StatusNotFound, "not found")

	if response := backend.call(http.MethodPost, "list_issues", "{}"); response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("POST list_issues: %d %v", response.Code, response.Header())
	}

	// A limited admin: projects and connections outside their access.
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))
	wantStatus(t, "get_project of a hidden project", admin.call(http.MethodGet, "get_project?project=frontend", ""), http.StatusForbidden, `project "frontend" is not in your projects`)
	wantStatus(t, "get_project of a missing project", admin.call(http.MethodGet, "get_project?project=nosuch", ""), http.StatusNotFound, `project "nosuch"`)
	wantStatus(t, "get_project_setup of a hidden project", admin.call(http.MethodGet, "get_project_setup?project=2", ""), http.StatusForbidden, "is not in your projects")
	wantStatus(t, "get_project_setup of a missing project", admin.call(http.MethodGet, "get_project_setup?project=999", ""), http.StatusNotFound, "not found")

	// The superadmin is no user an admin may change.
	root := server.webUser(store.RoleSuperadmin, false)
	wantStatus(t, "reset the superadmin", server.webUser(store.RoleAdmin, false).call(http.MethodPost, "reset_user_password", fmt.Sprintf(`{"user_id":%q}`, root.id)),
		http.StatusForbidden, "your role may not change this user")
	wantStatus(t, "reset oneself", root.call(http.MethodPost, "reset_user_password", fmt.Sprintf(`{"user_id":%q}`, root.id)), http.StatusForbidden, "/account")
	wantStatus(t, "reset a missing user", root.call(http.MethodPost, "reset_user_password", fmt.Sprintf(`{"user_id":%q}`, missingID)), http.StatusNotFound, "not found")
}

// Conflicts with stored records are 409, with the field the form shows.
func TestUIConflicts(t *testing.T) {
	server := newNotificationsServer(t)
	root := server.admin

	response := root.call(http.MethodPost, "create_user", `{"email":"taken@example.com","role":"user"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("create_user: %d %s", response.Code, response.Body)
	}

	response = root.call(http.MethodPost, "create_user", `{"email":"taken@example.com","role":"user"}`)

	if body := errorOf(t, response); response.Code != http.StatusConflict || body.Fields["email"] != "This email is taken." {
		t.Errorf("a taken email: %d %s", response.Code, response.Body)
	}

	response = root.call(http.MethodPost, "create_project", `{"name":"backend"}`)

	if body := errorOf(t, response); response.Code != http.StatusConflict || body.Fields["name"] == "" {
		t.Errorf("a taken project name: %d %s", response.Code, response.Body)
	}

	wantStatus(t, "a project of PROJECTS", root.call(http.MethodPost, "delete_project", `{"project":"1"}`), http.StatusConflict, "PROJECTS")

	webhook := `{"type":"slack","webhook_url":"https://hooks.slack.com/services/T0/B0/conflict","all_projects":true}`

	if response := root.call(http.MethodPost, "create_connection", webhook); response.Code != http.StatusOK {
		t.Fatalf("create_connection: %d %s", response.Code, response.Body)
	}

	response = root.call(http.MethodPost, "create_connection", webhook)

	if body := errorOf(t, response); response.Code != http.StatusConflict || body.Duplicate == nil {
		t.Errorf("a duplicate connection: %d %s", response.Code, response.Body)
	}
}

// Bodies over the limit are 413, over REST and in the web app.
func TestBodyTooLarge(t *testing.T) {
	server := newRESTServer(t)
	large := `{"issue_ids":["` + strings.Repeat("a", 1<<17) + `"],"status":"resolved"}`

	if response := server.do(http.MethodPost, "/api/v1/update_issue_status", large); response.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("REST: %d %s", response.Code, response.Body)
	}

	user := server.webUser(store.RoleUser, false)

	if response := user.call(http.MethodPost, "apply_issue_action", large); response.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("web app: %d %s", response.Code, response.Body)
	}

	if response := user.call(http.MethodPost, "change_password", large); response.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("plain request: %d %s", response.Code, response.Body)
	}
}

// The REST API: missing issues are 404, a wrong method 405.
func TestRESTErrorCodes(t *testing.T) {
	server := newRESTServer(t)
	missingID := store.NewID()

	if response := server.do(http.MethodPost, "/api/v1/update_issue_status", fmt.Sprintf(`{"issue_ids":[%q],"status":"resolved"}`, missingID)); response.Code != http.StatusNotFound ||
		!strings.Contains(response.Body.String(), missingID) {
		t.Errorf("update_issue_status of a missing issue: %d %s", response.Code, response.Body)
	}

	if response := server.do(http.MethodGet, "/api/v1/update_issue_status", ""); response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "POST" {
		t.Errorf("GET update_issue_status: %d %v", response.Code, response.Header())
	}

	if response := server.do(http.MethodGet, "/api/v1/nope", ""); response.Code != http.StatusNotFound {
		t.Errorf("unknown endpoint: %d", response.Code)
	}
}

// An agent tells a hidden issue from a deleted one by the tool's text,
// which names the projects it sees.
func TestMCPErrorCodes(t *testing.T) {
	server := newTestServer(t)
	data := seedProjects(t, server.database)
	user := server.asWith(store.RoleUser, store.OnlyProjects(1))
	missingID := store.NewID()

	checks := []struct {
		tool      string
		arguments map[string]any
		want      string
	}{
		{"get_issue", map[string]any{"issue_id": data.frontendIssue}, "forbidden: issue " + data.frontendIssue + " is not in your projects (backend)"},
		{"get_issue", map[string]any{"issue_id": missingID}, "not found: issue " + missingID},
		{"list_issue_events", map[string]any{"issue_id": data.frontendIssue}, "forbidden: issue " + data.frontendIssue + " is not in your projects (backend)"},
		{"get_event", map[string]any{"project": "frontend", "event_id": frontendEventID}, `forbidden: project "frontend" is not in your projects (backend)`},
		{"get_event", map[string]any{"project": "nosuch", "event_id": frontendEventID}, `not found: project "nosuch"`},
		{"update_issue_status", map[string]any{"issue_ids": []string{data.frontendIssue, missingID}, "status": "resolved"}, "forbidden: issue " + data.frontendIssue + " is not in your projects (backend)"},
		{"update_issue_status", map[string]any{"issue_ids": []string{missingID}, "status": "resolved"}, "not found: issue " + missingID},
		{"list_issues", map[string]any{"project": []string{"frontend"}}, `forbidden: project "frontend" is not in your projects (backend)`},
		{"list_issues", map[string]any{"since": "yesterday"}, "invalid: since must be"},
	}

	for _, check := range checks {
		if text, isError := user.try(check.tool, check.arguments); !isError || !strings.HasPrefix(text, check.want) {
			t.Errorf("%s %v: %q, want %q", check.tool, check.arguments, text, check.want)
		}
	}

	var batch struct {
		Issues    []map[string]any `json:"issues"`
		Missing   []string         `json:"missing_issue_ids"`
		Forbidden []string         `json:"forbidden_issue_ids"`
	}

	user.decode("get_issues", map[string]any{"issue_ids": []string{data.backendIssue, data.frontendIssue, missingID}}, &batch)

	if len(batch.Issues) != 1 || len(batch.Missing) != 1 || batch.Missing[0] != missingID || len(batch.Forbidden) != 1 || batch.Forbidden[0] != data.frontendIssue {
		t.Errorf("get_issues: %+v", batch)
	}

	// A role refusal inside a tool.
	admin, _ := server.as(store.RoleAdmin)
	root, err := server.database.Superadmin(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	if text, isError := admin.try("delete_user", map[string]any{"user_id": root.ID}); !isError || !strings.HasPrefix(text, "forbidden: your role may not change this user") {
		t.Errorf("delete the superadmin: %q", text)
	}
}
