package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
)

// decoded reads a JSON answer, failing unless its status is want.
func decoded[T any](t *testing.T, response *httptest.ResponseRecorder, want int) T {
	t.Helper()

	var body T

	if response.Code != want {
		t.Fatalf("%d, want %d: %s", response.Code, want, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("%v: %s", err, response.Body)
	}

	return body
}

type mcpSetup struct {
	URL           string `json:"url"`
	ServerName    string `json:"server_name"`
	ClaudeCommand string `json:"claude_command"`
	CodexCommand  string `json:"codex_command"`
	ProjectConfig string `json:"project_config"`
	TokenSet      bool   `json:"token_set"`
	Unencrypted   bool   `json:"unencrypted"`
	IPsRestricted bool   `json:"ips_restricted"`
}

type personalToken struct {
	Token       string `json:"token"`
	SetupPrompt string `json:"setup_prompt"`
}

// newPersonalToken generates the personal token of user as the MCP page does.
func (user uiUser) newPersonalToken() string {
	user.server.t.Helper()

	return decoded[personalToken](user.server.t, user.call(http.MethodPost, "personal_token", "{}"), http.StatusOK).Token
}

// mcpAccess checks what MCP answers a bearer token.
func (server *restServer) mcpAccess(bearer string, want int) {
	server.t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+bearer)

	if response := serve(server, request); response.Code != want {
		server.t.Fatalf("MCP with the token: %d, want %d: %s", response.Code, want, response.Body)
	}
}

// The MCP page's setup: the endpoint as agents reach it, the commands, and
// warnings; never the API token or a personal token.
func TestUIMCPSetup(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)
	response := user.call(http.MethodGet, "get_mcp_setup", "")
	setup := decoded[mcpSetup](t, response, http.StatusOK)

	if setup.URL != "http://example.com/mcp" || setup.ServerName != "exceptic" || setup.ClaudeCommand != "claude mcp add --transport http exceptic http://example.com/mcp" ||
		setup.CodexCommand != "codex mcp add exceptic --url http://example.com/mcp" || !strings.Contains(setup.ProjectConfig, `"url": "http://example.com/mcp"`) {
		t.Errorf("setup %+v", setup)
	}

	if !setup.Unencrypted || setup.IPsRestricted || setup.TokenSet {
		t.Errorf("flags %+v", setup)
	}

	if strings.Contains(response.Body.String(), token) {
		t.Error("the setup shows the API token")
	}

	// On this machine plain HTTP stays local.
	request := httptest.NewRequest(http.MethodGet, "http://localhost:8000/api/ui/get_mcp_setup", nil)
	request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})

	if local := decoded[mcpSetup](t, serve(server, request), http.StatusOK); local.Unencrypted || local.URL != "http://localhost:8000/mcp" {
		t.Errorf("loopback %+v", local)
	}

	personal := user.newPersonalToken()
	response = user.call(http.MethodGet, "get_mcp_setup", "")

	if setup := decoded[mcpSetup](t, response, http.StatusOK); !setup.TokenSet || strings.Contains(response.Body.String(), personal) {
		t.Errorf("after a token: %s", response.Body)
	}

	restricted := newRESTServer(t, "192.0.2.0/24") // httptest's peer
	setup = decoded[mcpSetup](t, restricted.webUser(store.RoleUser, false).call(http.MethodGet, "get_mcp_setup", ""), http.StatusOK)

	if !setup.IPsRestricted {
		t.Error("ALLOWED_IPS is not reported")
	}
}

// The personal token is generated only by the plain request of a session
// with its own password, answered once and never cached; a new one replaces
// the old.
func TestUIPersonalToken(t *testing.T) {
	server := newRESTServer(t)

	if response := (uiUser{server: server}).call(http.MethodPost, "personal_token", "{}"); response.Code != http.StatusUnauthorized {
		t.Errorf("without a session: %d", response.Code)
	}

	temporary := server.webUser(store.RoleAdmin, true)

	if response := temporary.call(http.MethodPost, "personal_token", "{}"); response.Code != http.StatusForbidden {
		t.Errorf("with a temporary password: %d", response.Code)
	}

	user := server.webUser(store.RoleUser, false)
	request := httptest.NewRequest(http.MethodPost, "/api/ui/personal_token", strings.NewReader("{}"))
	request.AddCookie(&http.Cookie{Name: session.CookieName, Value: user.cookie})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://evil.example")

	if response := serve(server, request); response.Code != http.StatusForbidden {
		t.Errorf("from another site: %d", response.Code)
	}

	response := user.call(http.MethodPost, "personal_token", "{}")
	first := decoded[personalToken](t, response, http.StatusOK)

	if !strings.HasPrefix(first.Token, "emcp_pt_") || response.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(first.SetupPrompt, `claude mcp add --transport http --scope user exceptic http://example.com/mcp --header "Authorization: Bearer `+first.Token+`"`) {
		t.Fatalf("token: %v %s", response.Header(), response.Body)
	}

	server.mcpAccess(first.Token, http.StatusOK)

	second := user.newPersonalToken()

	if second == first.Token {
		t.Fatal("the token was not replaced")
	}

	server.mcpAccess(first.Token, http.StatusUnauthorized)
	server.mcpAccess(second, http.StatusOK)

	// No read gives it again.
	for _, name := range []string{"get_mcp_setup", "list_agents"} {
		if body := user.call(http.MethodGet, name, "").Body.String(); strings.Contains(body, second) {
			t.Errorf("%s shows the token", name)
		}
	}
}

type agentsList struct {
	UserID string `json:"user_id"`
	Owners []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"owners"`
	Agents []struct {
		GrantID    string     `json:"grant_id"`
		Name       string     `json:"name"`
		Kind       string     `json:"kind"`
		LastUsedAt *time.Time `json:"last_used_at"`
		Connected  bool       `json:"connected"`
	} `json:"agents"`
}

func (list agentsList) owns(id string) bool {
	for _, owner := range list.Owners {
		if owner.ID == id {
			return true
		}
	}

	return false
}

// Admins choose whose agents to list among the users they manage; users
// see only their own. An unused personal token is no connected app.
func TestUIAgentsOfOtherUsers(t *testing.T) {
	server := newRESTServer(t)
	root := server.webUser(store.RoleSuperadmin, false)
	admin := server.webUser(store.RoleAdmin, false)
	user := server.webUser(store.RoleUser, false)

	unused := root.newPersonalToken()
	list := decoded[agentsList](t, root.call(http.MethodGet, "list_agents", ""), http.StatusOK)

	if list.UserID != root.id || !list.owns(root.id) || !list.owns(admin.id) || !list.owns(user.id) || len(list.Agents) != 1 || list.Agents[0].Connected {
		t.Fatalf("superadmin: %+v", list)
	}

	server.mcpAccess(unused, http.StatusOK)

	rootGrant := decoded[agentsList](t, root.call(http.MethodGet, "list_agents", ""), http.StatusOK).Agents[0]

	if !rootGrant.Connected || rootGrant.LastUsedAt == nil {
		t.Errorf("a used token: %+v", rootGrant)
	}

	userToken := user.newPersonalToken()
	server.mcpAccess(userToken, http.StatusOK)

	list = decoded[agentsList](t, admin.call(http.MethodGet, "list_agents?user_id="+user.id, ""), http.StatusOK)

	if list.UserID != user.id || list.owns(root.id) || !list.owns(admin.id) || len(list.Agents) != 1 {
		t.Fatalf("admin listing a user: %+v", list)
	}

	userGrant := list.Agents[0].GrantID

	if list := decoded[agentsList](t, user.call(http.MethodGet, "list_agents", ""), http.StatusOK); list.Owners != nil || list.UserID != user.id {
		t.Errorf("a user may choose: %+v", list)
	}

	// Anyone else's agents a role may not reach are forbidden; missing ones not found.
	for label, test := range map[string]struct {
		response *httptest.ResponseRecorder
		status   int
	}{
		"user lists the superadmin's":        {user.call(http.MethodGet, "list_agents?user_id="+root.id, ""), http.StatusForbidden},
		"admin lists the superadmin's":       {admin.call(http.MethodGet, "list_agents?user_id="+root.id, ""), http.StatusForbidden},
		"user disconnects the superadmin's":  {user.call(http.MethodPost, "disconnect_agent", `{"grant_id":"`+rootGrant.GrantID+`"}`), http.StatusForbidden},
		"admin disconnects the superadmin's": {admin.call(http.MethodPost, "disconnect_agent", `{"grant_id":"`+rootGrant.GrantID+`"}`), http.StatusForbidden},
		"user disconnects nothing":           {user.call(http.MethodPost, "disconnect_agent", `{"grant_id":"zzzzzzzz"}`), http.StatusNotFound},
		"admin lists nobody's":               {admin.call(http.MethodGet, "list_agents?user_id=zzzzzzzz", ""), http.StatusNotFound},
	} {
		if test.response.Code != test.status {
			t.Errorf("%s: %d %s", label, test.response.Code, test.response.Body)
		}
	}

	server.mcpAccess(unused, http.StatusOK)

	if response := admin.call(http.MethodPost, "disconnect_agent", `{"grant_id":"`+userGrant+`"}`); response.Code != http.StatusOK {
		t.Fatalf("admin disconnects a user's agent: %d %s", response.Code, response.Body)
	}

	server.mcpAccess(userToken, http.StatusUnauthorized)

	if response := root.call(http.MethodPost, "disconnect_agent", `{"grant_id":"`+rootGrant.GrantID+`"}`); response.Code != http.StatusOK {
		t.Fatalf("own disconnect: %d", response.Code)
	}

	server.mcpAccess(unused, http.StatusUnauthorized)
}

// The setup and the token and consent requests are the web app's: agents
// and scripts never reach them.
func TestPersonalTokenIsNoTool(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleSuperadmin, false)
	personal := user.newPersonalToken()

	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+personal)
	tools := serve(server, request).Body.String()

	for _, name := range []string{"personal_token", "get_mcp_setup", "authorization_request", "authorization_decision"} {
		if strings.Contains(tools, `"`+name+`"`) {
			t.Errorf("MCP offers %s", name)
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/get_mcp_setup", nil)
	request.Header.Set("Authorization", "Bearer "+token)

	if response := serve(server, request); response.Code != http.StatusNotFound {
		t.Errorf("REST serves get_mcp_setup: %d", response.Code)
	}
}
