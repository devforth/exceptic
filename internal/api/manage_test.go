package api_test

import (
	"context"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// as is the server seen by an agent of a new user with role.
func (server *testServer) as(role string) (*testServer, string) {
	server.t.Helper()

	ctx := context.Background()
	id, err := server.database.CreateUser(ctx, store.User{Email: role + "-user", Role: role, CreatedAt: time.Now()}, "x")

	if err != nil {
		server.t.Fatal(err)
	}

	token, err := server.authorization.NewPersonalToken(ctx, id)

	if err != nil {
		server.t.Fatal(err)
	}

	other := *server
	other.token, other.session = token, nil

	return &other, id
}

func toolNames(t *testing.T, session *mcp.ClientSession) []string {
	t.Helper()

	var names []string

	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}

		names = append(names, tool.Name)
	}

	return names
}

// An agent is offered only what its user's role allows, and the role is
// read at every request.
func TestToolsFollowTheRole(t *testing.T) {
	server := newTestServer(t)
	admin := toolNames(t, server.connect())

	for _, name := range []string{"create_user", "delete_issue", "create_connection", "update_project", "get_account"} {
		if !slices.Contains(admin, name) {
			t.Errorf("an admin is not offered %s", name)
		}
	}

	// Never a tool: an endpoint MCP does not serve (the own password and
	// minting MCP tokens are none, in any transport).
	for _, endpoint := range api.Endpoints(nil, nil) {
		if !endpoint.Serves(api.TransportMCP) && slices.Contains(admin, endpoint.Name) {
			t.Errorf("%s offered to agents", endpoint.Name)
		}
	}

	user, userID := server.as(store.RoleUser)
	names := toolNames(t, user.connect())

	for _, name := range []string{"delete_issue", "delete_project"} {
		if slices.Contains(names, name) {
			t.Errorf("user is offered %s", name)
		}
	}

	for _, name := range []string{"get_account", "update_account_name", "list_agents", "update_issue_status", "create_user", "create_connection", "update_project"} {
		if !slices.Contains(names, name) {
			t.Errorf("user is not offered %s", name)
		}
	}

	// Calling an admin tool anyway does not run it.
	if result, err := user.connect().CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_project", Arguments: map[string]any{"project": "backend"}}); err == nil && !result.IsError {
		t.Fatal("a user's agent deleted a project")
	}

	// Made an admin: the next request offers the admin tools.
	if err := server.database.UpdateUser(context.Background(), userID, store.UserChange{Role: store.RoleAdmin}); err != nil {
		t.Fatal(err)
	}

	if names := toolNames(t, user.connect()); !slices.Contains(names, "delete_project") {
		t.Errorf("tools after promotion: %v", names)
	}
}

func TestDestructiveToolsAskForConfirmation(t *testing.T) {
	server := newTestServer(t)
	destructive := 0

	for tool, err := range server.connect().Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}

		if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			destructive++

			if !strings.Contains(tool.Description, "explicit confirmation") {
				t.Errorf("%s: %s", tool.Name, tool.Description)
			}
		}
	}

	if destructive < 5 {
		t.Fatalf("%d destructive tools", destructive)
	}

	// Changes of who may access the server or where errors go are confirmed too.
	for tool, err := range server.connect().Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}

		sensitive := slices.Contains([]string{"create_user", "update_user", "create_connection", "update_connection", "set_connection_active", "update_project", "set_project_active"}, tool.Name)

		if sensitive != strings.Contains(tool.Description, "Changes who can access") {
			t.Errorf("%s: %s", tool.Name, tool.Description)
		}
	}
}

type projectSettings struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	DSN       string   `json:"dsn"`
	Color     string   `json:"color"`
	LogLevels []string `json:"log_levels"`
}

func TestManageProjects(t *testing.T) {
	server := newTestServer(t)

	var created projectSettings
	server.decode("create_project", map[string]any{"name": "worker", "log_levels": []string{"error"}}, &created)

	if created.ID == 0 || !strings.Contains(created.DSN, "/"+strconv.Itoa(created.ID)) || !slices.Equal(created.LogLevels, []string{"error"}) {
		t.Fatalf("created = %+v", created)
	}

	// Fields not given keep their values.
	var updated projectSettings
	server.decode("update_project", map[string]any{"project": "worker", "color": "#123456"}, &updated)

	if updated.Color != "#123456" || !slices.Equal(updated.LogLevels, []string{"error"}) || updated.Name != "worker" {
		t.Fatalf("updated = %+v", updated)
	}

	// An empty list records nothing; the color is kept.
	var reset projectSettings
	server.decode("update_project", map[string]any{"project": "worker", "log_levels": []string{}}, &reset)

	if reset.LogLevels == nil || len(reset.LogLevels) != 0 || reset.Color != "#123456" {
		t.Fatalf("after reset = %+v, want no levels and the color kept", reset)
	}

	if text, isError := server.try("create_project", map[string]any{"name": "Not Valid"}); !isError || !strings.Contains(text, "name:") {
		t.Fatalf("invalid name: %s", text)
	}

	if text, isError := server.try("create_project", map[string]any{"name": "backend"}); !isError || !strings.Contains(text, "Another project") {
		t.Fatalf("duplicate name: %s", text)
	}

	if text, isError := server.try("update_project", map[string]any{"project": "worker", "color": "red"}); !isError || !strings.Contains(text, "color") {
		t.Fatalf("invalid color: %s", text)
	}

	// An empty color returns to the palette.
	var palette projectSettings
	server.decode("update_project", map[string]any{"project": "worker", "color": ""}, &palette)

	if palette.Color != "" {
		t.Fatalf("color after reset = %q, want the palette", palette.Color)
	}

	server.call("delete_project", map[string]any{"project": "worker"})

	if text := server.call("list_projects", nil); strings.Contains(text, "worker") {
		t.Fatalf("deleted project listed: %s", text)
	}

	if text, isError := server.try("delete_project", map[string]any{"project": "backend"}); !isError || !strings.Contains(text, "PROJECTS") {
		t.Fatalf("deleting a project from env: %s", text)
	}
}

type userOutput struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

func TestManageUsers(t *testing.T) {
	server := newTestServer(t)

	var me userOutput
	server.decode("get_account", nil, &me)

	if me.Email != "admin" || me.Role != store.RoleAdmin {
		t.Fatalf("account = %+v", me)
	}

	var created struct {
		User              userOutput `json:"user"`
		TemporaryPassword string     `json:"temporary_password"`
	}
	server.decode("create_user", map[string]any{"email": "dev@example.com", "name": "Developer", "role": "user"}, &created)

	if created.TemporaryPassword == "" || !created.User.MustChangePassword || created.User.Role != store.RoleUser {
		t.Fatalf("created = %+v", created)
	}

	var updated userOutput
	server.decode("update_user", map[string]any{"user_id": created.User.ID, "role": "admin"}, &updated)

	if updated.Role != store.RoleAdmin || updated.Name != "Developer" || updated.Email != "dev@example.com" {
		t.Fatalf("updated = %+v", updated)
	}

	server.decode("update_user", map[string]any{"user_id": created.User.ID, "email": "developer@example.com"}, &updated)

	if updated.Email != "developer@example.com" || updated.Role != store.RoleAdmin {
		t.Fatalf("after the email change = %+v", updated)
	}

	if text, isError := server.try("update_user", map[string]any{"user_id": created.User.ID, "email": "developer"}); !isError || !strings.HasPrefix(text, "invalid: email") {
		t.Fatalf("a login that is no email: %s", text)
	}

	var other struct{}
	server.decode("create_user", map[string]any{"email": "ops@example.com", "role": "user"}, &other)

	if text, isError := server.try("update_user", map[string]any{"user_id": created.User.ID, "email": "OPS@example.com"}); !isError || !strings.HasPrefix(text, "conflict: email") {
		t.Fatalf("a taken login: %s", text)
	}

	// Nobody changes the superadmin or themselves here.
	if text, isError := server.try("update_user", map[string]any{"user_id": me.ID, "role": "user"}); !isError || !strings.HasPrefix(text, "forbidden: change your own account") {
		t.Fatalf("update self: %s", text)
	}

	if text, isError := server.try("create_user", map[string]any{"email": "boss@example.com", "role": "superadmin"}); !isError {
		t.Fatalf("second superadmin: %s", text)
	}

	var reset struct {
		TemporaryPassword string `json:"temporary_password"`
	}
	server.decode("reset_user_password", map[string]any{"user_id": created.User.ID}, &reset)

	if reset.TemporaryPassword == "" || reset.TemporaryPassword == created.TemporaryPassword {
		t.Fatalf("reset = %+v", reset)
	}

	server.call("delete_user", map[string]any{"user_id": created.User.ID})

	if text := server.call("list_users", nil); strings.Contains(text, "developer@example.com") {
		t.Fatalf("deleted user listed: %s", text)
	}

	server.decode("update_account_name", map[string]any{"name": "Root"}, &me)

	if me.Name != "Root" {
		t.Fatalf("account name = %q", me.Name)
	}
}

func TestManageConnections(t *testing.T) {
	server := newTestServer(t)

	// No SECRET_KEY on this server: chats cannot be added.
	if text, isError := server.try("create_connection", map[string]any{"type": "slack", "webhook_url": "https://hooks.slack.com/services/x"}); !isError || !strings.Contains(text, "not available") {
		t.Fatalf("slack without SECRET_KEY: %s", text)
	}

	// An address must be confirmed by its owner first.
	if text, isError := server.try("create_connection", map[string]any{"type": "email", "email": "dev@example.com"}); !isError || !strings.Contains(text, "start_email_verification") {
		t.Fatalf("unconfirmed email: %s", text)
	}

	if text := server.call("list_connections", nil); !strings.Contains(text, `"items":[]`) {
		t.Fatalf("connections: %s", text)
	}
}

// A failed delivery reaches the agent's model provider redacted.
func TestSendTestMessageRedactsTheError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close() // nothing listens there: the delivery fails

	server := newTestServer(t, func(settings *config.Config) {
		settings.Notifications.SMTP = config.SMTP{Host: "127.0.0.1", Port: port, From: "monitor@example.com"}
	})

	now := time.Now()
	destination := store.Destination{Type: config.ChannelEmail, Address: "dev@example.com", Verified: true, VerifiedAt: now}

	if _, err := server.database.CreateDestination(context.Background(), destination, now); err != nil {
		t.Fatal(err)
	}

	var connection struct {
		ID string `json:"id"`
	}
	server.decode("create_connection", map[string]any{"type": "email", "email": "dev@example.com"}, &connection)

	text, isError := server.try("send_test_message", map[string]any{"connection_id": connection.ID})

	if !isError || !strings.Contains(text, "not delivered") || !strings.Contains(text, "<ip>") || strings.Contains(text, "127.0.0.1") {
		t.Fatalf("send_test_message: %s", text)
	}
}

// A channel the server cannot build is its own failure, not a refused delivery.
func TestSendTestMessageWithoutChannel(t *testing.T) {
	server := newTestServer(t) // no SMTP: no email channel
	ctx := context.Background()
	now := time.Now()
	destination := store.Destination{Type: config.ChannelEmail, Address: "dev@example.com", Verified: true, VerifiedAt: now}
	destinationID, err := server.database.CreateDestination(ctx, destination, now)

	if err != nil {
		t.Fatal(err)
	}

	connection := store.NewConnection(config.ChannelEmail)
	connection.DestinationID = destinationID
	connectionID, err := server.database.CreateConnection(ctx, connection, now)

	if err != nil {
		t.Fatal(err)
	}

	if text, isError := server.try("send_test_message", map[string]any{"connection_id": connectionID}); !isError || text != "internal error" {
		t.Fatalf("send_test_message: %s", text)
	}
}

func TestDeleteIssue(t *testing.T) {
	server := newTestServer(t)

	var issues issuePage
	server.decode("list_issues", map[string]any{"limit": 1}, &issues)

	id := issues.Items[0].ID
	server.call("delete_issue", map[string]any{"issue_id": id})

	if text, isError := server.try("get_issue", map[string]any{"issue_id": id}); !isError || text != "not found: issue "+id {
		t.Fatalf("deleted issue: %s", text)
	}
}
