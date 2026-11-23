package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// storedAccess is a user's access as stored, whoever may see it.
func (server *restServer) storedAccess(id string) store.Access {
	server.t.Helper()

	user, err := server.database.GetUser(context.Background(), id)

	if err != nil {
		server.t.Fatal(err)
	}

	return user.Access
}

func sameAccess(got, want store.Access) bool {
	return got.Limited == want.Limited && slices.Equal(got.ProjectIDs, want.ProjectIDs)
}

// A limited admin's new project joins his access at once; a deleted one
// leaves everyone's, and a user left without projects sees none.
func TestProjectAccessProjectWrites(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))
	other := server.webUserWith(store.RoleUser, store.OnlyProjects(1))
	everyone := server.webUser(store.RoleUser, false)

	var created struct {
		ID int `json:"id"`
	}

	admin.request(http.MethodPost, "create_project", `{"name":"worker"}`, &created)

	if access := server.storedAccess(admin.id); !sameAccess(access, store.OnlyProjects(1, created.ID)) {
		t.Errorf("the creator's access: %+v", access)
	}

	visible := func(user uiUser) []string {
		var projects struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}

		user.read("list_projects", &projects)

		var names []string

		for _, project := range projects.Items {
			names = append(names, project.Name)
		}

		slices.Sort(names)

		return names
	}

	if got := visible(admin); !slices.Equal(got, []string{"backend", "worker"}) {
		t.Errorf("the creator sees %v", got)
	}

	if got := visible(other); !slices.Equal(got, []string{"backend"}) {
		t.Errorf("another limited user sees %v", got)
	}

	if got := visible(everyone); !slices.Equal(got, []string{"backend", "frontend", "worker"}) {
		t.Errorf("a user with every project sees %v", got)
	}

	mixed := server.webUserWith(store.RoleUser, store.OnlyProjects(1, created.ID))
	only := server.webUserWith(store.RoleUser, store.OnlyProjects(created.ID))

	admin.request(http.MethodPost, "delete_project", `{"project":"worker"}`, &struct{}{})

	if access := server.storedAccess(mixed.id); !sameAccess(access, store.OnlyProjects(1)) {
		t.Errorf("a user with the deleted project and backend: %+v", access)
	}

	if access := server.storedAccess(only.id); !access.Limited || len(access.ProjectIDs) != 0 {
		t.Fatalf("a user with only the deleted project: %+v", access)
	}

	if got := visible(only); len(got) != 0 {
		t.Errorf("a user without projects sees %v", got)
	}

	if ids := only.issueIDs("status=all"); len(ids) != 0 {
		t.Errorf("a user without projects sees issues %v", ids)
	}
}

type accessUserRow struct {
	User struct {
		ID          string `json:"id"`
		AllProjects bool   `json:"all_projects"`
		ProjectIDs  []int  `json:"project_ids"`
	} `json:"user"`
	CanManage      bool   `json:"can_manage"`
	CanTakeOver    bool   `json:"can_take_over"`
	TakeOverReason string `json:"take_over_reason"`
	AccessEditable bool   `json:"access_editable"`
	AccessReason   string `json:"access_reason"`
}

// A limited admin manages every user, but sees and changes only the part
// of their access within his own; the rest stays as it is.
func TestProjectAccessUsers(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))
	fullAdmin := server.webUser(store.RoleAdmin, false)
	frontend := server.webUserWith(store.RoleUser, store.OnlyProjects(2))
	mixed := server.webUserWith(store.RoleUser, store.OnlyProjects(1, 2))
	everyone := server.webUser(store.RoleUser, false)

	var rows, fullRows struct {
		Items []accessUserRow `json:"items"`
	}

	admin.read("list_users", &rows)
	fullAdmin.read("list_users", &fullRows)

	if len(rows.Items) != len(fullRows.Items) {
		t.Errorf("list_users: %d rows, %d for an admin with every project", len(rows.Items), len(fullRows.Items))
	}

	byID := map[string]accessUserRow{}

	for _, row := range rows.Items {
		byID[row.User.ID] = row
	}

	if row := byID[frontend.id]; !row.CanManage || row.User.AllProjects || len(row.User.ProjectIDs) != 0 {
		t.Errorf("a frontend user as the backend admin sees them: %+v", row)
	}

	if row := byID[mixed.id]; !slices.Equal(row.User.ProjectIDs, []int{1}) {
		t.Errorf("a user with both projects: %+v", row)
	}

	if row := byID[everyone.id]; !row.User.AllProjects {
		t.Errorf("a user with every project: %+v", row)
	}

	update := func(user uiUser, body string) *int {
		code := admin.call(http.MethodPost, "update_user", body).Code

		return &code
	}

	if code := update(frontend, fmt.Sprintf(`{"user_id":%q,"all_projects":false,"projects":["1"]}`, frontend.id)); *code != http.StatusOK {
		t.Fatalf("giving backend: %d", *code)
	}

	if access := server.storedAccess(frontend.id); !sameAccess(access, store.OnlyProjects(1, 2)) {
		t.Errorf("after giving backend: %+v", access)
	}

	if code := update(mixed, fmt.Sprintf(`{"user_id":%q,"all_projects":false,"projects":[]}`, mixed.id)); *code != http.StatusOK {
		t.Fatalf("taking backend: %d", *code)
	}

	if access := server.storedAccess(mixed.id); !sameAccess(access, store.OnlyProjects(2)) {
		t.Errorf("after taking backend: %+v", access)
	}

	hiddenOrMissing(t, "update_user with a hidden project", admin.answer(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"projects":["2"]}`, mixed.id)), admin.answer(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"projects":["999"]}`, mixed.id)), http.StatusBadRequest)

	// A user with every project: only someone with every project changes that.
	if code := admin.call(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"all_projects":false,"projects":["1"]}`, everyone.id)).Code; code != http.StatusForbidden {
		t.Errorf("limiting a user with every project: %d", code)
	}

	// Sent back as shown, every project is no change.
	if code := admin.call(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"all_projects":true}`, everyone.id)).Code; code != http.StatusOK {
		t.Errorf("saving a user with every project unchanged: %d", code)
	}

	if access := server.storedAccess(everyone.id); !access.All() {
		t.Errorf("a user with every project after the save: %+v", access)
	}

	var row accessUserRow

	admin.read("get_user?user_id="+everyone.id, &row)

	if row.AccessEditable || row.AccessReason == "" {
		t.Errorf("get_user of a user with every project: %+v", row)
	}

	admin.read("get_user?user_id="+frontend.id, &row)

	if !row.AccessEditable || !slices.Equal(row.User.ProjectIDs, []int{1}) {
		t.Errorf("get_user of a frontend user: %+v", row)
	}

	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	admin.request(http.MethodPost, "create_user", `{"username":"limited-new","role":"user"}`, &created)

	if access := server.storedAccess(created.User.ID); !sameAccess(access, store.OnlyProjects(1)) {
		t.Errorf("a user created by the backend admin: %+v", access)
	}

	if code := admin.call(http.MethodPost, "create_user", `{"username":"limited-all","role":"user","all_projects":true}`).Code; code != http.StatusForbidden {
		t.Errorf("the backend admin giving every project: %d", code)
	}

	hiddenOrMissing(t, "create_user with a hidden project",
		admin.answer(http.MethodPost, "create_user", `{"username":"limited-hidden","role":"user","projects":["2"]}`),
		admin.answer(http.MethodPost, "create_user", `{"username":"limited-hidden","role":"user","projects":["999"]}`), http.StatusBadRequest)

	fullAdmin.request(http.MethodPost, "create_user", `{"username":"full-new","role":"user"}`, &created)

	if access := server.storedAccess(created.User.ID); !access.All() {
		t.Errorf("a user created by an admin with every project: %+v", access)
	}

	// The change applies at once to the user's session.
	fullAdmin.request(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"all_projects":false,"projects":["frontend"]}`, everyone.id), &struct{}{})

	var list accessIssues

	everyone.read("list_issues?status=all", &list)

	if got := list.projects(); !slices.Equal(got, []string{"frontend"}) {
		t.Errorf("after the change: %v", got)
	}
}

// accessConnections stores a connection per kind of project choice, each
// with a delivery in the outbox; it returns their ids by name.
func accessConnections(t *testing.T, server *notificationsServer) map[string]string {
	t.Helper()

	ctx, now := context.Background(), time.Now()
	choices := map[string]store.Connection{
		"backend":  {ProjectIDs: []int{1}, Active: true},
		"frontend": {ProjectIDs: []int{2}, Active: true},
		"both":     {ProjectIDs: []int{1, 2}, Active: true},
		"all":      {AllProjects: true, Active: true},
		"empty":    {},
	}
	ids := map[string]string{}

	for name, connection := range choices {
		destinationID, err := server.database.CreateDestination(ctx, store.Destination{Type: config.ChannelEmail, Address: name + "@example.com", Verified: true, VerifiedAt: now}, now)

		if err != nil {
			t.Fatal(err)
		}

		connection.Name, connection.DestinationID, connection.Levels = name, destinationID, []string{"error"}
		id, err := server.database.CreateConnection(ctx, connection, now)

		if err != nil {
			t.Fatal(err)
		}

		notification := `{"Kind":"new_issue","Project":"backend","Title":"Error","IssueID":"issue007"}`

		if err := server.database.EnqueueOutbox(ctx, []string{id}, []int{1}, "notification", []byte(notification), now); err != nil {
			t.Fatal(err)
		}

		ids[name] = id
	}

	return ids
}

// A limited admin sees and changes only the connections of his projects,
// chooses only among them, and sees only their deliveries.
func TestProjectAccessConnections(t *testing.T) {
	server := newNotificationsServer(t)
	ids := accessConnections(t, server)
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))

	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}

	admin.read("list_connections", &list)

	if len(list.Items) != 1 || list.Items[0].ID != ids["backend"] {
		t.Errorf("list_connections: %+v", list.Items)
	}

	missingID := store.NewID()

	for _, hidden := range []string{"frontend", "both", "all", "empty"} {
		for _, request := range []struct{ method, target, body string }{
			{http.MethodGet, "get_connection?connection_id=%s", ""},
			{http.MethodPost, "update_connection", `{"connection_id":%q,"name":"changed"}`},
			{http.MethodPost, "set_connection_active", `{"connection_id":%q,"active":false}`},
			{http.MethodPost, "send_test_message", `{"connection_id":%q}`},
			{http.MethodPost, "retry_connection", `{"connection_id":%q}`},
			{http.MethodPost, "test_chat", `{"connection_id":%q}`},
			{http.MethodPost, "delete_connection", `{"connection_id":%q}`},
		} {
			ask := func(id string) func() (int, string) {
				if request.method == http.MethodGet {
					return admin.answer(request.method, fmt.Sprintf(request.target, id), "")
				}

				return admin.answer(request.method, request.target, fmt.Sprintf(request.body, id))
			}

			hiddenOrMissing(t, request.target+" of "+hidden, ask(ids[hidden]), ask(missingID), http.StatusNotFound)
		}
	}

	// Nothing was deleted or changed.
	server.admin.read("list_connections", &list)

	if len(list.Items) != len(ids) {
		t.Errorf("connections left: %d", len(list.Items))
	}

	create := func(projects string) func() (int, string) {
		return admin.answer(http.MethodPost, "create_connection", `{"type":"email","email":"backend@example.com","projects":`+projects+`}`)
	}

	hiddenOrMissing(t, "create_connection for a hidden project", create(`["frontend"]`), create(`["nosuch"]`), http.StatusBadRequest)

	if code := admin.call(http.MethodPost, "create_connection", `{"type":"slack","webhook_url":"https://hooks.slack.com/services/T0/B0/x","all_projects":true}`).Code; code != http.StatusForbidden {
		t.Errorf("create_connection for every project: %d", code)
	}

	var defaults struct {
		AllProjects bool  `json:"all_projects"`
		ProjectIDs  []int `json:"project_ids"`
	}

	admin.read("get_connection_defaults", &defaults)

	if defaults.AllProjects || !slices.Equal(defaults.ProjectIDs, []int{1}) {
		t.Errorf("get_connection_defaults: %+v", defaults)
	}

	var deliveries struct {
		Items []struct {
			ConnectionID string `json:"connection_id"`
		} `json:"items"`
	}

	admin.read("list_deliveries", &deliveries)

	if len(deliveries.Items) != 1 || deliveries.Items[0].ConnectionID != ids["backend"] {
		t.Errorf("list_deliveries: %+v", deliveries.Items)
	}

	var count struct {
		Count int `json:"count"`
	}

	admin.read("count_deliveries", &count)

	if count.Count != 1 {
		t.Errorf("count_deliveries: %d", count.Count)
	}

	for _, name := range []string{"count_deliveries", "list_deliveries"} {
		if response := admin.call(http.MethodGet, name+"?connection_id="+ids["frontend"], ""); response.Code != http.StatusForbidden {
			t.Errorf("%s of a hidden connection: %d %s", name, response.Code, response.Body)
		}
	}

	// An address a hidden connection sends to is refused without naming it.
	response := admin.call(http.MethodPost, "start_email_verification", `{"email":"frontend@example.com"}`)

	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), ids["frontend"]) || strings.Contains(response.Body.String(), `"duplicate"`) {
		t.Errorf("start_email_verification of a hidden connection's address: %d %s", response.Code, response.Body)
	}

	var visible map[string]json.RawMessage

	response = admin.call(http.MethodPost, "start_email_verification", `{"email":"backend@example.com"}`)

	if err := json.Unmarshal(response.Body.Bytes(), &visible); err != nil || visible["duplicate"] == nil {
		t.Errorf("start_email_verification of his connection's address: %d %s", response.Code, response.Body)
	}
}

// An agent of a limited admin is held to the same rules.
func TestProjectAccessManageMCP(t *testing.T) {
	server := newTestServer(t)
	seedProjects(t, server.database)
	admin := server.asWith(store.RoleAdmin, store.OnlyProjects(1))

	var project struct {
		ID int `json:"id"`
	}

	admin.decode("create_project", map[string]any{"name": "worker"}, &project)

	var projects accessList

	admin.decode("list_projects", nil, &projects)

	if len(projects.Items) != 2 {
		t.Errorf("list_projects after create_project: %d", len(projects.Items))
	}

	ctx := context.Background()
	id, err := server.database.CreateUser(ctx, store.User{Username: "frontend-dev", Role: store.RoleUser, Access: store.OnlyProjects(2), CreatedAt: time.Now()}, "x")

	if err != nil {
		t.Fatal(err)
	}

	admin.decode("update_user", map[string]any{"user_id": id, "projects": []string{"backend"}}, &struct{}{})

	user, err := server.database.GetUser(ctx, id)

	if err != nil || !sameAccess(user.Access, store.OnlyProjects(1, 2)) {
		t.Errorf("after update_user: %+v %v", user.Access, err)
	}

	if _, isError := admin.try("update_user", map[string]any{"user_id": id, "projects": []string{"frontend"}}); !isError {
		t.Error("update_user with a hidden project was accepted")
	}

	hidden, _ := admin.try("create_user", map[string]any{"username": "agent-new", "role": "user", "projects": []string{"frontend"}})
	missing, _ := admin.try("create_user", map[string]any{"username": "agent-new", "role": "user", "projects": []string{"nosuch"}})

	if !strings.HasPrefix(hidden, "forbidden: ") || !strings.Contains(hidden, "not in your projects") || !strings.HasPrefix(missing, "invalid: ") || !strings.Contains(missing, `unknown project "nosuch"`) {
		t.Errorf("MCP create_user with a hidden project: %q, with a missing one: %q", hidden, missing)
	}

	var created struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	admin.decode("create_user", map[string]any{"username": "agent-new", "role": "user"}, &created)

	if user, err := server.database.GetUser(ctx, created.User.ID); err != nil || !sameAccess(user.Access, store.OnlyProjects(1, project.ID)) {
		t.Errorf("a user created by the agent: %+v %v", user.Access, err)
	}

	// The frontend user (now with backend too) is not his to take over.
	for _, name := range []string{"reset_user_password", "delete_user"} {
		if _, isError := admin.try(name, map[string]any{"user_id": id}); !isError {
			t.Errorf("%s of a user with a hidden project was accepted", name)
		}
	}

	if _, err := server.database.GetUser(ctx, id); err != nil {
		t.Errorf("the user after the refused delete: %v", err)
	}

	admin.decode("reset_user_password", map[string]any{"user_id": created.User.ID}, &struct{}{})
	admin.decode("delete_user", map[string]any{"user_id": created.User.ID}, &struct{}{})

	var connections accessList

	admin.decode("list_connections", nil, &connections)

	if len(connections.Items) != 0 {
		t.Errorf("list_connections: %d", len(connections.Items))
	}
}

// A limited admin may not take over a user (role user, the ones an admin
// manages) with projects he does not have: resetting their password would let
// him log in as them, and renaming or deleting them is theirs to decide who
// have them all. Within his projects he may. Their access he still changes as
// far as he sees it. (Other admins he does not manage at all: TestUIUsers.)
func TestProjectAccessTakeOver(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))
	backend := server.webUserWith(store.RoleUser, store.OnlyProjects(1))
	targets := map[string]uiUser{
		"frontend":            server.webUserWith(store.RoleUser, store.OnlyProjects(2)),
		"both":                server.webUserWith(store.RoleUser, store.OnlyProjects(1, 2)),
		"every project":       server.webUser(store.RoleUser, false),
		"backend, for a role": server.webUserWith(store.RoleUser, store.OnlyProjects(1)),
	}

	for name, target := range targets {
		var row accessUserRow

		admin.read("get_user?user_id="+target.id, &row)

		if covered := name == "backend, for a role"; row.CanTakeOver != covered || !row.CanManage || (row.TakeOverReason == "") == !covered {
			t.Errorf("get_user of %s: %+v", name, row)
		}

		if name == "backend, for a role" {
			continue
		}

		before, err := server.database.GetUser(context.Background(), target.id)

		if err != nil {
			t.Fatal(err)
		}

		for _, request := range []string{"reset_user_password", "delete_user"} {
			response := admin.call(http.MethodPost, request, fmt.Sprintf(`{"user_id":%q}`, target.id))

			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), services.TakeOverProblem) {
				t.Errorf("%s of %s: %d %s", request, name, response.Code, response.Body)
			}
		}

		for field, body := range map[string]string{
			"role": fmt.Sprintf(`{"user_id":%q,"role":%q}`, target.id, store.RoleAdmin),
			"name": fmt.Sprintf(`{"user_id":%q,"name":"Renamed"}`, target.id),
		} {
			response := admin.call(http.MethodPost, "update_user", body)

			if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"`+field+`"`) {
				t.Errorf("update_user %s of %s: %d %s", field, name, response.Code, response.Body)
			}
		}

		user, err := server.database.GetUser(context.Background(), target.id)

		if err != nil || user.Role != before.Role || user.Name != before.Name {
			t.Errorf("%s after the refusals: %+v %v", name, user, err)
		}

		if code := target.call(http.MethodGet, "get_session", "").Code; code != http.StatusOK {
			t.Errorf("the session of %s after the refused reset: %d", name, code)
		}
	}

	// The part of the access he sees still changes, name and role sent back as they are.
	both := targets["both"]

	if code := admin.call(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"name":"Web user","role":"user","projects":[]}`, both.id)).Code; code != http.StatusOK {
		t.Errorf("taking backend from a user with both: %d", code)
	}

	if access := server.storedAccess(both.id); !sameAccess(access, store.OnlyProjects(2)) {
		t.Errorf("after taking backend: %+v", access)
	}

	if code := admin.call(http.MethodPost, "reset_user_password", fmt.Sprintf(`{"user_id":%q}`, backend.id)).Code; code != http.StatusOK {
		t.Errorf("reset_user_password of a backend user: %d", code)
	}

	covered := targets["backend, for a role"]

	if code := admin.call(http.MethodPost, "update_user", fmt.Sprintf(`{"user_id":%q,"role":"user","name":"Renamed"}`, covered.id)).Code; code != http.StatusOK {
		t.Errorf("renaming a backend user: %d", code)
	}

	if code := admin.call(http.MethodPost, "delete_user", fmt.Sprintf(`{"user_id":%q}`, backend.id)).Code; code != http.StatusOK {
		t.Errorf("delete_user of a backend user: %d", code)
	}
}

// A limited admin lists and disconnects the agents only of the users he
// may take over, as resetting their password would.
func TestProjectAccessAgents(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))
	backend := server.webUserWith(store.RoleUser, store.OnlyProjects(1))
	frontend := server.webUserWith(store.RoleUser, store.OnlyProjects(2))
	fullAdmin := server.webUser(store.RoleAdmin, false)

	tokens := map[string]string{}

	for name, user := range map[string]uiUser{"backend": backend, "frontend": frontend, "full admin": fullAdmin} {
		tokens[name] = user.newPersonalToken()
		server.mcpAccess(tokens[name], http.StatusOK)
	}

	list := decoded[agentsList](t, admin.call(http.MethodGet, "list_agents", ""), http.StatusOK)

	if !list.owns(admin.id) || !list.owns(backend.id) || list.owns(frontend.id) || list.owns(fullAdmin.id) {
		t.Errorf("owners: %+v", list.Owners)
	}

	missingID := store.NewID()

	for name, user := range map[string]uiUser{"frontend": frontend, "full admin": fullAdmin} {
		grant := decoded[agentsList](t, user.call(http.MethodGet, "list_agents", ""), http.StatusOK).Agents[0].GrantID

		reason := map[string]string{"frontend": services.TakeOverProblem, "full admin": "your role may not"}[name]

		forbiddenOrMissing(t, "list_agents of "+name, admin.answer(http.MethodGet, "list_agents?user_id="+user.id, ""), admin.answer(http.MethodGet, "list_agents?user_id="+missingID, ""), reason, http.StatusNotFound)
		forbiddenOrMissing(t, "disconnect_agent of "+name, admin.answer(http.MethodPost, "disconnect_agent", `{"grant_id":"`+grant+`"}`), admin.answer(http.MethodPost, "disconnect_agent", `{"grant_id":"`+missingID+`"}`), reason, http.StatusNotFound)
		server.mcpAccess(tokens[name], http.StatusOK)
	}

	list = decoded[agentsList](t, admin.call(http.MethodGet, "list_agents?user_id="+backend.id, ""), http.StatusOK)

	if len(list.Agents) != 1 {
		t.Fatalf("the backend user's agents: %+v", list)
	}

	if code := admin.call(http.MethodPost, "disconnect_agent", `{"grant_id":"`+list.Agents[0].GrantID+`"}`).Code; code != http.StatusOK {
		t.Errorf("disconnecting the backend user's agent: %d", code)
	}

	server.mcpAccess(tokens["backend"], http.StatusUnauthorized)
}

// Only someone with every project sets a disk share, which decides how the
// projects compete for the database; a taken name of a project the admin
// does not see is refused without saying it is a project's.
func TestProjectAccessProjectSettings(t *testing.T) {
	server := newRESTServer(t)
	seedProjects(t, server.database)
	admin := server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))
	fullAdmin := server.webUser(store.RoleAdmin, false)

	// A share is refused as forbidden (403); a taken name, hidden or not,
	// as a conflict (409), so the status tells nothing of a hidden project.
	refused := func(user uiUser, name, body string, status int, field, problem string) {
		t.Helper()

		response := user.call(http.MethodPost, name, body)

		var answer struct {
			Fields map[string]string `json:"fields"`
		}

		_ = json.Unmarshal(response.Body.Bytes(), &answer)

		if response.Code != status || answer.Fields[field] != problem {
			t.Errorf("%s %s: %d %s", name, body, response.Code, response.Body)
		}
	}

	const (
		shareProblem = "Only someone with access to all projects can change the disk share."
		hiddenName   = "This name cannot be used."
		takenName    = "Another project already has this name."
	)

	refused(admin, "update_project", `{"project":"backend","disk_share":50}`, http.StatusForbidden, "disk_share", shareProblem)
	refused(admin, "create_project", `{"name":"worker","disk_share":50}`, http.StatusForbidden, "disk_share", shareProblem)

	var project struct {
		DiskShare int `json:"disk_share"`
	}

	admin.request(http.MethodPost, "update_project", `{"project":"backend","color":"#123456"}`, &project)

	if project.DiskShare != 100 {
		t.Errorf("the disk share after another change: %d", project.DiskShare)
	}

	fullAdmin.request(http.MethodPost, "update_project", `{"project":"backend","disk_share":50}`, &project)

	if project.DiskShare != 50 {
		t.Errorf("the disk share set with every project: %d", project.DiskShare)
	}

	// Sent back unchanged, as the form does, a share he may not set is no change.
	admin.request(http.MethodPost, "update_project", `{"project":"backend","disk_share":50,"color":""}`, &project)

	refused(admin, "create_project", `{"name":"frontend"}`, http.StatusConflict, "name", hiddenName)
	refused(admin, "update_project", `{"project":"backend","name":"frontend"}`, http.StatusConflict, "name", hiddenName)
	refused(admin, "create_project", `{"name":"backend"}`, http.StatusConflict, "name", takenName)
	refused(fullAdmin, "create_project", `{"name":"frontend"}`, http.StatusConflict, "name", takenName)
}

// The delivery log of a limited admin shows only messages about his
// projects: a connection narrowed to them keeps its earlier messages about
// others, and a message stored before messages had projects is shown only
// with every project.
func TestProjectAccessDeliveries(t *testing.T) {
	server := newNotificationsServer(t)
	ctx, now := context.Background(), time.Now()
	destinationID, err := server.database.CreateDestination(ctx, store.Destination{Type: config.ChannelEmail, Address: "both@example.com", Verified: true, VerifiedAt: now}, now)

	if err != nil {
		t.Fatal(err)
	}

	id, err := server.database.CreateConnection(ctx, store.Connection{Name: "both", DestinationID: destinationID, Levels: []string{"error"}, ProjectIDs: []int{1, 2}, Active: true}, now)

	if err != nil {
		t.Fatal(err)
	}

	for _, message := range []struct {
		projects []int
		title    string
	}{
		{[]int{1}, "Backend error"},
		{[]int{2}, "Secret frontend error"},
		{[]int{1, 2}, "Summary of both"},
		{nil, "Stored before projects"},
	} {
		payload := `{"Kind":"new_issue","Project":"backend","Title":"` + message.title + `","IssueID":"issue007"}`

		if err := server.database.EnqueueOutbox(ctx, []string{id}, message.projects, "notification", []byte(payload), now); err != nil {
			t.Fatal(err)
		}
	}

	server.admin.request(http.MethodPost, "update_connection", fmt.Sprintf(`{"connection_id":%q,"projects":["backend"]}`, id), &struct{}{})

	titles := func(user uiUser) []string {
		var deliveries struct {
			Items []struct {
				Title string `json:"title"`
			} `json:"items"`
		}

		user.read("list_deliveries", &deliveries)

		var count struct {
			Count int `json:"count"`
		}

		user.read("count_deliveries", &count)

		var names []string

		for _, item := range deliveries.Items {
			names = append(names, item.Title)
		}

		if count.Count != len(names) {
			t.Errorf("count_deliveries %d, listed %v", count.Count, names)
		}

		slices.Sort(names)

		return names
	}

	if got := titles(server.webUserWith(store.RoleAdmin, store.OnlyProjects(1))); !slices.Equal(got, []string{"Backend error"}) {
		t.Errorf("backend admin: %v", got)
	}

	if got := titles(server.admin); len(got) != 4 {
		t.Errorf("every project: %v", got)
	}
}
