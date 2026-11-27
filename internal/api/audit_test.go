package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/storage/store"
)

// auditEntries reads the whole audit log, newest first.
func (server *restServer) auditEntries() []store.AuditEntry {
	server.t.Helper()

	entries, err := server.database.ListAudit(context.Background(), store.AuditFilter{})

	if err != nil {
		server.t.Fatal(err)
	}

	return entries
}

// newAudit is what the audit log gained since it held before entries.
func (server *restServer) newAudit(before int) []store.AuditEntry {
	server.t.Helper()

	entries := server.auditEntries()

	return entries[:len(entries)-before]
}

// field is a diff's field as a string, its JSON text otherwise.
func field(fields map[string]json.RawMessage, name string) string {
	var text string

	if json.Unmarshal(fields[name], &text) == nil {
		return text
	}

	return string(fields[name])
}

// expectOne: the change wrote one entry of resource and action, by the admin through the web app.
func (server *notificationsServer) expectOne(before int, what, resource, action string) store.AuditEntry {
	server.t.Helper()

	added := server.newAudit(before)

	if len(added) != 1 || added[0].Resource != resource || added[0].Action != action {
		server.t.Fatalf("%s: audit entries %+v, want one %s %s", what, added, resource, action)
	}

	entry := added[0]

	if entry.Actor.UserID != server.admin.id || entry.Actor.Role != store.RoleAdmin || entry.Actor.Email == "" ||
		entry.Source != store.SourceUI || entry.Agent != "" || entry.Address == "" {
		server.t.Errorf("%s: actor %+v, source %q, agent %q, address %q", what, entry.Actor, entry.Source, entry.Agent, entry.Address)
	}

	return entry
}

// Each change of an admin in the web app writes one entry with only the
// fields it changed; no secret is ever in one, and own-account changes,
// test messages and refusals write none.
func TestAuditRecordsChanges(t *testing.T) {
	server := newNotificationsServer(t)
	server.ingestFixtures()

	ctx := context.Background()
	secrets := []string{telegramToken, notificationsKey}

	// Projects.
	var project struct {
		ID  int    `json:"id"`
		DSN string `json:"dsn"`
	}

	server.must("create_project", map[string]any{"name": "audited"}, &project)
	entry := server.expectOne(0, "create project", store.AuditProject, store.AuditCreate)

	if entry.RecordName != "audited" || entry.RecordID != fmt.Sprint(project.ID) || len(entry.ProjectIDs) != 1 || entry.Before != nil ||
		field(entry.After, "name") != "audited" || field(entry.After, "paused") != "false" {
		t.Errorf("create project: %+v", entry)
	}

	count := len(server.auditEntries())

	server.must("update_project", map[string]any{"project": "audited", "color": "#123456"}, nil)
	entry = server.expectOne(count, "edit project", store.AuditProject, store.AuditEdit)

	if len(entry.Before) != 1 || len(entry.After) != 1 || field(entry.After, "color") != "#123456" || field(entry.Before, "color") == "" {
		t.Errorf("edit project: only the color changed: %+v %+v", entry.Before, entry.After)
	}

	count++

	// Saving it unchanged, and a refused save, write nothing.
	server.must("update_project", map[string]any{"project": "audited", "color": "#123456"}, nil)
	server.refused("update_project", map[string]any{"project": "audited", "color": "red"})

	server.must("set_project_active", map[string]any{"project": "audited", "active": false}, nil)
	entry = server.expectOne(count, "pause project", store.AuditProject, store.AuditPause)

	if field(entry.Before, "paused") != "false" || field(entry.After, "paused") != "true" {
		t.Errorf("pause: %+v %+v", entry.Before, entry.After)
	}

	count++

	var setup struct {
		DSN string `json:"dsn"`
	}

	server.must("reset_project_key", map[string]any{"project": project.ID}, &setup)
	entry = server.expectOne(count, "reset key", store.AuditProject, store.AuditResetKey)

	// A DSN is public: the old and the new one are kept.
	if field(entry.Before, "dsn") != project.DSN || field(entry.After, "dsn") != setup.DSN || project.DSN == setup.DSN {
		t.Errorf("reset key: %+v %+v", entry.Before, entry.After)
	}

	count++

	// Connections.
	connectionID := server.create(map[string]any{"type": "telegram", "name": "Ops", "bot_token": telegramToken, "chat_id": "-100"})
	entry = server.expectOne(count, "create connection", store.AuditConnection, store.AuditCreate)

	if entry.RecordID != connectionID || field(entry.After, "destination") != "chat -100" || entry.ProjectIDs != nil {
		t.Errorf("create connection: %+v", entry)
	}

	count++
	newToken := "654321:ZYXWVUTSRQPONMLKJIHGFEDCBA"
	secrets = append(secrets, newToken)

	server.must("update_connection", map[string]any{"connection_id": connectionID, "bot_token": newToken, "name": "Ops room"}, nil)
	entry = server.expectOne(count, "edit connection", store.AuditConnection, store.AuditEdit)

	if field(entry.Before, "name") != "Ops" || field(entry.After, "name") != "Ops room" || field(entry.After, "token") != store.AuditReplaced || len(entry.After) != 2 {
		t.Errorf("edit connection: %+v %+v", entry.Before, entry.After)
	}

	count++

	server.must("set_connection_active", map[string]any{"connection_id": connectionID, "active": false}, nil)
	server.expectOne(count, "pause connection", store.AuditConnection, store.AuditPause)

	count++

	server.must("retry_connection", map[string]any{"connection_id": connectionID}, nil)
	server.expectOne(count, "retry", store.AuditConnection, store.AuditRetry)

	count++

	server.must("test_chat", map[string]any{"connection_id": connectionID}, nil)

	if added := server.newAudit(count); len(added) != 0 {
		t.Errorf("a test message is no change: %+v", added)
	}

	server.must("delete_connection", map[string]any{"connection_id": connectionID}, nil)
	entry = server.expectOne(count, "delete connection", store.AuditConnection, store.AuditDelete)

	if entry.After != nil || field(entry.Before, "name") != "Ops room" {
		t.Errorf("delete connection: %+v", entry)
	}

	count++

	server.must("start_email_verification", map[string]any{"email": "ops@example.com"}, nil)

	code, _ := server.receiveCode()
	secrets = append(secrets, code)

	server.must("confirm_email", map[string]any{"email": "ops@example.com", "code": code}, nil)
	entry = server.expectOne(count, "confirm email", store.AuditConnection, store.AuditConfirm)

	if entry.RecordName != "ops@example.com" || field(entry.After, "confirmed") != "true" {
		t.Errorf("confirm email: %+v", entry)
	}

	count++

	// Users.
	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
		TemporaryPassword string `json:"temporary_password"`
	}

	server.must("create_user", map[string]any{"email": "dev@example.com", "role": "user", "projects": []string{"backend"}}, &created)
	entry = server.expectOne(count, "create user", store.AuditUser, store.AuditCreate)

	secrets = append(secrets, created.TemporaryPassword)

	if entry.RecordID != created.User.ID || field(entry.After, "projects") != `["backend"]` || fmt.Sprint(entry.ProjectIDs) != "[1]" {
		t.Errorf("create user: %+v", entry)
	}

	count++

	server.must("update_user", map[string]any{"user_id": created.User.ID, "name": "Dev"}, nil)
	entry = server.expectOne(count, "edit user", store.AuditUser, store.AuditEdit)

	if field(entry.After, "name") != "Dev" || len(entry.After) != 1 {
		t.Errorf("edit user: %+v", entry.After)
	}

	count++

	var reset struct {
		TemporaryPassword string `json:"temporary_password"`
	}

	server.must("reset_user_password", map[string]any{"user_id": created.User.ID}, &reset)
	entry = server.expectOne(count, "reset password", store.AuditUser, store.AuditResetPassword)

	secrets = append(secrets, reset.TemporaryPassword)

	if field(entry.After, "password") != store.AuditReplaced {
		t.Errorf("reset password: %+v", entry.After)
	}

	count++
	sessionToken := store.NewID() + store.NewID()

	if err := server.database.CreateSession(ctx, store.NewSession{
		TokenHash: secret.Hash(sessionToken), UserID: created.User.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Browser: "Firefox",
	}); err != nil {
		t.Fatal(err)
	}

	sessionID := store.SessionID(secret.Hash(sessionToken))

	server.must("end_session", map[string]any{"user_id": created.User.ID, "session_id": sessionID}, nil)
	entry = server.expectOne(count, "end session", store.AuditSession, store.AuditDelete)

	if entry.RecordID != sessionID || entry.RecordName != "dev@example.com" || field(entry.Before, "browser") != "Firefox" {
		t.Errorf("end session: %+v", entry)
	}

	count++

	if err := server.database.LinkGoogle(ctx, created.User.ID, "dev@example.com", "google-sub", time.Now()); err != nil {
		t.Fatal(err)
	}

	server.must("unlink_google", map[string]any{"user_id": created.User.ID}, nil)
	entry = server.expectOne(count, "unlink Google", store.AuditUser, store.AuditUnlinkGoogle)

	if field(entry.Before, "google_linked") != "true" || field(entry.After, "google_linked") != "false" {
		t.Errorf("unlink Google: %+v %+v", entry.Before, entry.After)
	}

	count++

	agentToken, err := server.app.OAuth.NewPersonalToken(ctx, created.User.ID)

	if err != nil {
		t.Fatal(err)
	}

	secrets = append(secrets, agentToken)
	grants, err := server.database.ListGrants(ctx, created.User.ID, time.Now())

	if err != nil || len(grants) != 1 {
		t.Fatalf("grants %+v, %v", grants, err)
	}

	server.must("disconnect_agent", map[string]any{"grant_id": grants[0].ID}, nil)
	entry = server.expectOne(count, "disconnect agent", store.AuditAgent, store.AuditDelete)

	if entry.RecordID != grants[0].ID || field(entry.Before, "user") != "dev@example.com" || fmt.Sprint(entry.ProjectIDs) != "[1]" {
		t.Errorf("disconnect agent: %+v", entry)
	}

	count++

	server.must("delete_user", map[string]any{"user_id": created.User.ID}, nil)
	entry = server.expectOne(count, "delete user", store.AuditUser, store.AuditDelete)

	if field(entry.Before, "email") != "dev@example.com" || entry.After != nil {
		t.Errorf("delete user: %+v", entry)
	}

	count++

	// Own account: nothing.
	server.must("update_account_name", map[string]any{"name": "Root"}, nil)
	server.must("set_theme", map[string]any{"theme": "dark"}, nil)

	if _, err := server.app.OAuth.NewPersonalToken(ctx, server.admin.id); err != nil {
		t.Fatal(err)
	}

	ownGrants, err := server.database.ListGrants(ctx, server.admin.id, time.Now())

	if err != nil || len(ownGrants) != 1 {
		t.Fatalf("own grants %+v, %v", ownGrants, err)
	}

	server.must("disconnect_agent", map[string]any{"grant_id": ownGrants[0].ID}, nil)

	if added := server.newAudit(count); len(added) != 0 {
		t.Errorf("own account changes are not audited: %+v", added)
	}

	// Issues and logs.
	issues, err := server.database.ListIssues(ctx, store.IssueFilter{ProjectIDs: []int{1}, Limit: 2})

	if err != nil || len(issues) < 2 {
		t.Fatalf("issues %v, %v", issues, err)
	}

	server.must("apply_issue_action", map[string]any{"action": "resolve", "issue_ids": []string{issues[0].ID}}, nil)
	entry = server.expectOne(count, "resolve", store.AuditIssue, store.AuditEdit)

	if field(entry.Before, "status") != store.StatusUnresolved || field(entry.After, "status") != store.StatusResolved || fmt.Sprint(entry.ProjectIDs) != "[1]" {
		t.Errorf("resolve: %+v", entry)
	}

	count++

	server.must("apply_issue_action", map[string]any{"action": "delete", "issue_ids": []string{issues[1].ID}}, nil)
	entry = server.expectOne(count, "delete issue", store.AuditIssue, store.AuditDelete)

	if entry.RecordID != issues[1].ID || field(entry.Before, "title") == "" {
		t.Errorf("delete issue: %+v", entry)
	}

	count++

	logs, err := server.database.ListLogs(ctx, store.LogFilter{Limit: 1})

	if err != nil || len(logs.Logs) != 1 {
		t.Fatalf("logs %+v, %v", logs, err)
	}

	server.must("delete_logs", map[string]any{"log_ids": []int64{logs.Logs[0].ID}}, nil)
	entry = server.expectOne(count, "delete log", store.AuditLog, store.AuditDelete)

	if entry.RecordID != fmt.Sprint(logs.Logs[0].ID) || field(entry.Before, "level") != logs.Logs[0].Level {
		t.Errorf("delete log: %+v", entry)
	}

	count++

	server.must("delete_project", map[string]any{"project": "audited"}, nil)
	server.expectOne(count, "delete project", store.AuditProject, store.AuditDelete)

	// Deleting the project kept its entries.
	stored := server.auditEntries()

	if projectEntries := countWhere(stored, func(entry store.AuditEntry) bool { return entry.Resource == store.AuditProject }); projectEntries != 5 {
		t.Errorf("%d project entries after its delete, want 5", projectEntries)
	}

	encoded, err := json.Marshal(stored)

	if err != nil {
		t.Fatal(err)
	}

	for _, value := range secrets {
		if value != "" && strings.Contains(string(encoded), value) {
			t.Errorf("the audit log holds the secret %q", value)
		}
	}
}

func countWhere(entries []store.AuditEntry, match func(store.AuditEntry) bool) int {
	count := 0

	for _, entry := range entries {
		if match(entry) {
			count++
		}
	}

	return count
}

type auditRow struct {
	ID         int64          `json:"id"`
	ItemCount  int            `json:"item_count"`
	RecordName string         `json:"record_name"`
	UserName   string         `json:"user_name"`
	UserID     string         `json:"user_id"`
	Email      string         `json:"user_email"`
	IP         string         `json:"ip"`
	Source     string         `json:"source"`
	Agent      string         `json:"agent"`
	Resource   string         `json:"resource"`
	Action     string         `json:"action"`
	RecordID   string         `json:"record_id"`
	Projects   []string       `json:"projects"`
	Before     map[string]any `json:"before"`
	After      map[string]any `json:"after"`
	Exists     bool           `json:"exists"`
}

type auditPage struct {
	Items      []auditRow `json:"items"`
	NextCursor string     `json:"next_cursor"`
}

// An admin lists every entry. A user sees an entry of one record only when
// all its projects are theirs, and one of several records when one of its
// items is, with only those items (AuditEntry.ShownTo); entries of no
// project, such as creating a user, are for admins only. Nobody deletes them.
func TestAuditVisibility(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	admin := server.webUser(store.RoleAdmin, false)
	user := server.addWebUser(store.User{Role: store.RoleUser, Access: store.OnlyProjects(1)})

	issues, err := server.database.ListIssues(context.Background(), store.IssueFilter{Limit: 100})

	if err != nil {
		t.Fatal(err)
	}

	backend, frontend := "", ""

	for _, issue := range issues {
		switch {
		case issue.ProjectID == 1 && backend == "":
			backend = issue.ID
		case issue.ProjectID == 2 && frontend == "":
			frontend = issue.ID
		}
	}

	admin.request(http.MethodPost, "apply_issue_action", `{"action":"resolve","issue_ids":["`+backend+`","`+frontend+`"]}`, &struct{}{})
	admin.request(http.MethodPost, "create_user", `{"email":"all@example.com","role":"admin"}`, &struct{}{})
	// A user's own change of an issue of theirs.
	user.request(http.MethodPost, "apply_issue_action", `{"action":"reopen","issue_ids":["`+backend+`"]}`, &struct{}{})

	var page auditPage

	page = auditPage{}
	admin.read("list_audit", &page)

	// One request is one entry: the bulk resolve of two issues too.
	if len(page.Items) != 3 || page.Items[0].UserID != user.id || page.Items[2].Resource != store.AuditIssue {
		t.Fatalf("admin's list: %+v", page.Items)
	}

	if !page.Items[0].Exists || page.Items[0].Before["status"] != store.StatusResolved {
		t.Errorf("newest entry: %+v", page.Items[0])
	}

	bulk := page.Items[2]

	if bulk.ItemCount != 2 || bulk.RecordID != "" || bulk.RecordName != "2 issues" || !slices.Equal(bulk.Projects, []string{"backend", "frontend"}) {
		t.Errorf("the bulk entry: %+v", bulk)
	}

	type entryItems struct {
		Items []struct {
			RecordID string         `json:"record_id"`
			Project  string         `json:"project"`
			Before   map[string]any `json:"before"`
			After    map[string]any `json:"after"`
		} `json:"items"`
	}

	var entry entryItems

	admin.read("get_audit_entry?id="+strconv.FormatInt(bulk.ID, 10), &entry)

	if len(entry.Items) != 2 || entry.Items[0].After["status"] != store.StatusResolved {
		t.Errorf("admin's bulk items: %+v", entry.Items)
	}

	adminEmail := bulk.Email
	createUser := page.Items[1].ID

	page = auditPage{}
	user.read("list_audit", &page)

	if len(page.Items) != 2 || page.Items[0].RecordID != backend {
		t.Fatalf("user's list: %+v", page.Items)
	}

	// The user sees the bulk entry with only the issue of their project; the
	// admin who made it is shown by role only, and neither the search nor the
	// user filter finds the admin or the other project's issue.
	if hidden := page.Items[1]; adminEmail == "" || bulk.IP == "" || hidden.UserName != "Admin" || hidden.UserID != "" || hidden.Email != "" || hidden.IP != "" ||
		hidden.ItemCount != 1 || hidden.RecordName != "1 issue" || !slices.Equal(hidden.Projects, []string{"backend"}) {
		t.Errorf("the admin's bulk entry shown to a user: %+v", hidden)
	}

	if own := page.Items[0]; own.UserID != user.id || own.IP == "" {
		t.Errorf("the user's own entry: %+v", own)
	}

	entry = entryItems{}
	user.read("get_audit_entry?id="+strconv.FormatInt(bulk.ID, 10), &entry)

	if len(entry.Items) != 1 || entry.Items[0].RecordID != backend || entry.Items[0].Project != "backend" {
		t.Errorf("user's bulk items: %+v", entry.Items)
	}

	if response := user.call(http.MethodGet, "get_audit_entry?id="+strconv.FormatInt(createUser, 10), ""); response.Code != http.StatusForbidden {
		t.Errorf("an entry the user may not see: %d %s", response.Code, response.Body)
	}

	for _, query := range []string{"query=" + url.QueryEscape(adminEmail), "user_id=" + admin.id, "query=" + frontend} {
		page = auditPage{}
		user.read("list_audit?"+query, &page)

		if len(page.Items) != 0 {
			t.Errorf("%s found hidden entries for a user: %+v", query, page.Items)
		}
	}

	var count struct {
		Count int `json:"count"`
	}

	user.read("count_audit", &count)

	if count.Count != 2 {
		t.Errorf("user's count %d, want 2", count.Count)
	}

	admin.read("count_audit?resource=user", &count)

	if count.Count != 1 {
		t.Errorf("admin's count of user entries %d, want 1", count.Count)
	}

	page = auditPage{}
	admin.read("list_audit?project=frontend&limit=1", &page)

	if len(page.Items) != 1 || page.Items[0].ID != bulk.ID || page.NextCursor != "" {
		t.Errorf("frontend entries: %+v", page)
	}

	// The user who acted is gone; their entries stay with their name.
	admin.request(http.MethodPost, "delete_user", `{"user_id":"`+user.id+`"}`, &struct{}{})

	page = auditPage{}
	admin.read("list_audit?action=edit", &page)

	if len(page.Items) != 2 || page.Items[0].UserName != "Web user" {
		t.Errorf("after deleting the user: %+v", page.Items)
	}
}

// agentOf connects an agent to server with userID's personal token.
func (server *restServer) agentOf(userID string) *testServer {
	server.t.Helper()

	token, err := server.app.OAuth.NewPersonalToken(context.Background(), userID)

	if err != nil {
		server.t.Fatal(err)
	}

	httpServer := httptest.NewServer(server.handler)

	server.t.Cleanup(httpServer.Close)

	return &testServer{t: server.t, url: httpServer.URL + api.MCPPath, database: server.database, authorization: server.app.OAuth, token: token}
}

// A user who does not administer sees who acted only for the users they
// see on the Users page (who share a project with them); anyone else by
// role, without id, email, address or agent, and never finds them by query
// or user_id. Agents never get an address; get_audit_entry over MCP gives
// the items the agent's user sees and refuses an entry they do not see.
func TestAuditActors(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	issues, err := server.database.ListIssues(context.Background(), store.IssueFilter{Limit: 100})

	if err != nil {
		t.Fatal(err)
	}

	backend, frontend := "", ""

	for _, issue := range issues {
		switch {
		case issue.ProjectID == 1 && backend == "":
			backend = issue.ID
		case issue.ProjectID == 2 && frontend == "":
			frontend = issue.ID
		}
	}

	admin := server.webUser(store.RoleAdmin, false)
	viewer := server.addWebUser(store.User{Role: store.RoleUser, Access: store.OnlyProjects(1)})
	fellow := server.addWebUser(store.User{Role: store.RoleUser, Access: store.OnlyProjects(1)})
	stranger := server.addWebUser(store.User{Role: store.RoleUser, Access: store.OnlyProjects(1)})

	fellow.request(http.MethodPost, "apply_issue_action", `{"action":"resolve","issue_ids":["`+backend+`"]}`, &struct{}{})
	stranger.request(http.MethodPost, "apply_issue_action", `{"action":"reopen","issue_ids":["`+backend+`"]}`, &struct{}{})

	// The stranger moves to the other project: the viewer no longer sees
	// them, while their change of the viewer's issue stays in the log.
	moved := store.OnlyProjects(2)

	if err := server.database.UpdateUser(context.Background(), stranger.id, store.UserChange{Name: "Web user", Role: store.RoleUser, Access: &moved}); err != nil {
		t.Fatal(err)
	}

	agent := server.agentOf(admin.id)
	agent.call("update_issue_status", map[string]any{"issue_ids": []string{backend, frontend}, "status": "resolved"})
	agent.call("update_issue_status", map[string]any{"issue_ids": []string{frontend}, "status": "ignored"})

	entries := server.auditEntries()

	if len(entries) != 4 || entries[1].Address == "" || entries[1].Agent == "" || entries[2].Address == "" || entries[3].Address == "" {
		t.Fatalf("stored entries: %+v", entries)
	}

	bulk, strangerEmail, fellowEmail := entries[1].ID, entries[2].Actor.Email, entries[3].Actor.Email

	var page auditPage

	viewer.read("list_audit", &page)

	if len(page.Items) != 3 {
		t.Fatalf("viewer's list: %+v", page.Items)
	}

	if row := page.Items[0]; row.UserName != "Admin" || row.UserID != "" || row.Email != "" || row.IP != "" || row.Agent != "" || row.Source != store.SourceMCP {
		t.Errorf("an admin's agent shown to a user: %+v", row)
	}

	if row := page.Items[1]; row.UserName != "User" || row.UserID != "" || row.Email != "" || row.IP != "" || row.RecordID != backend {
		t.Errorf("a user the viewer does not see: %+v", row)
	}

	if row := page.Items[2]; row.UserName != "Web user" || row.UserID != fellow.id || row.Email != fellowEmail || row.IP == "" {
		t.Errorf("a user of the viewer's project: %+v", row)
	}

	for query, want := range map[string]int{
		"user_id=" + fellow.id: 1, "query=" + url.QueryEscape(fellowEmail): 1,
		"user_id=" + stranger.id: 0, "query=" + url.QueryEscape(strangerEmail): 0, "user_id=" + admin.id: 0, "query=Personal": 0,
	} {
		page = auditPage{}
		viewer.read("list_audit?"+query, &page)

		if len(page.Items) != want {
			t.Errorf("%s: %d entries, want %d: %+v", query, len(page.Items), want, page.Items)
		}
	}

	page = auditPage{}
	agent.decode("list_audit", nil, &page)

	if len(page.Items) != 4 || page.Items[1].Agent != "Personal token" || page.Items[3].UserID != fellow.id {
		t.Fatalf("the admin's agent's list: %+v", page.Items)
	}

	for _, row := range page.Items {
		if row.IP != "" {
			t.Errorf("an agent got an address: %+v", row)
		}
	}

	type auditEntry struct {
		auditRow

		Items []struct {
			RecordID string `json:"record_id"`
			Project  string `json:"project"`
		} `json:"items"`
	}

	var entry auditEntry

	agent.decode("get_audit_entry", map[string]any{"id": bulk}, &entry)

	if len(entry.Items) != 2 || entry.RecordName != "2 issues" || entry.IP != "" || entry.Agent != "Personal token" {
		t.Errorf("the admin's agent's entry: %+v", entry)
	}

	viewerAgent := server.agentOf(viewer.id)
	entry = auditEntry{}
	viewerAgent.decode("get_audit_entry", map[string]any{"id": bulk}, &entry)

	if len(entry.Items) != 1 || entry.Items[0].RecordID != backend || entry.Items[0].Project != "backend" ||
		entry.UserName != "Admin" || entry.Agent != "" || entry.IP != "" || entry.Email != "" {
		t.Errorf("the viewer's agent's entry: %+v", entry)
	}

	if text, isError := viewerAgent.try("get_audit_entry", map[string]any{"id": entries[0].ID}); !isError || !strings.HasPrefix(text, "forbidden:") {
		t.Errorf("an entry of another project for an agent: %v %s", isError, text)
	}
}

// MCP entries name the agent that acted for the user, REST ones the API token.
func TestAuditTransports(t *testing.T) {
	server := newTestServer(t)
	issues, err := server.database.ListIssues(context.Background(), store.IssueFilter{Limit: 1})

	if err != nil || len(issues) == 0 {
		t.Fatal(issues, err)
	}

	server.call("update_issue_status", map[string]any{"issue_ids": []string{issues[0].ID}, "status": "resolved"})

	// A refused change writes nothing.
	server.try("update_issue_status", map[string]any{"issue_ids": []string{issues[0].ID, "zzzzzzzz"}, "status": "ignored"})

	var page auditPage

	server.decode("list_audit", nil, &page)

	if len(page.Items) != 1 || page.Items[0].Source != store.SourceMCP || page.Items[0].Agent != "Personal token" || page.Items[0].UserName != "admin" {
		t.Errorf("MCP entries: %+v", page.Items)
	}

	rest := newRESTServer(t)
	rest.ingestFixtures()

	restIssues, err := rest.database.ListIssues(context.Background(), store.IssueFilter{Limit: 1})

	if err != nil || len(restIssues) == 0 {
		t.Fatal(restIssues, err)
	}

	rest.post("/api/v1/delete_issue", `{"issue_id":"`+restIssues[0].ID+`"}`, &struct{}{})

	entries := rest.auditEntries()

	if len(entries) != 1 || entries[0].Actor.Name != "API token" || entries[0].Actor.UserID != "" || entries[0].Source != store.SourceAPI || entries[0].Action != store.AuditDelete {
		t.Errorf("REST entries: %+v", entries)
	}

	// No endpoint deletes an entry, in any transport.
	for _, endpoint := range api.Endpoints(nil, nil) {
		if strings.Contains(endpoint.Name, "audit") && !endpoint.ReadOnly {
			t.Errorf("endpoint %s changes the audit log", endpoint.Name)
		}
	}
}

// The audit log's filters offer the values of the entries the viewer sees:
// a user only their projects' and the users they see, never an admin or
// the API token's changes of no project.
func TestAuditChoices(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)
	ann := server.webUserWith(store.RoleUser, store.OnlyProjects(1))
	bob := server.webUserWith(store.RoleUser, store.OnlyProjects(1, 2))
	cat := server.webUserWith(store.RoleUser, store.OnlyProjects(2))
	now := time.Now()
	entries := []store.AuditEntry{
		{CreatedAt: now, Actor: store.AuditActor{UserID: admin.id}, Resource: store.AuditProject, Action: store.AuditEdit, ProjectIDs: []int{1}},
		{CreatedAt: now, Actor: store.AuditActor{UserID: bob.id}, Resource: store.AuditIssue, Action: store.AuditEdit, ProjectIDs: []int{1}},
		{CreatedAt: now, Actor: store.AuditActor{UserID: cat.id}, Resource: store.AuditIssue, Action: store.AuditDelete, ProjectIDs: []int{2}},
		{CreatedAt: now, Actor: store.AuditActor{Name: "API token"}, Resource: store.AuditUser, Action: store.AuditCreate},
	}

	if err := server.database.AppendAudit(context.Background(), entries); err != nil {
		t.Fatal(err)
	}

	type choices struct {
		Resources []string `json:"resources"`
		Actions   []string `json:"actions"`
		UserIDs   []string `json:"user_ids"`
		Projects  []string `json:"projects"`
	}

	everyone := []string{admin.id, bob.id, cat.id}
	slices.Sort(everyone)

	for _, check := range []struct {
		viewer uiUser
		want   choices
	}{
		{admin, choices{
			Resources: []string{store.AuditIssue, store.AuditProject, store.AuditUser},
			Actions:   []string{store.AuditCreate, store.AuditEdit, store.AuditDelete}, UserIDs: everyone, Projects: []string{"backend", "frontend"},
		}},
		{ann, choices{
			Resources: []string{store.AuditIssue, store.AuditProject}, Actions: []string{store.AuditEdit}, UserIDs: []string{bob.id}, Projects: []string{"backend"},
		}},
	} {
		var got choices

		check.viewer.read("list_audit_choices", &got)

		if !reflect.DeepEqual(got, check.want) {
			t.Errorf("choices %+v, want %+v", got, check.want)
		}
	}
}
