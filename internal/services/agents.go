package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Agents: the coding agents connected to the MCP server, the personal tokens
// and the consent page (package oauth does the work). Whose agents a viewer
// may see or disconnect is store.User.CanAccessAgentsOf.
type Agents struct {
	authorization *oauth.Server
	store         store.Store
	serverName    string // what agents list the server under
	ipsRestricted bool
}

func newAgents(authorization *oauth.Server, database store.Store, settings *config.Config) *Agents {
	return &Agents{authorization: authorization, store: database, serverName: config.AppSlug(settings.AppName), ipsRestricted: len(settings.AllowedIPs) > 0}
}

// Address is the server's address as agents reach it.
func (agents *Agents) Address(r *http.Request) *url.URL {
	return agents.authorization.Address(r)
}

// CheckRequest validates an authorization request for the consent page.
func (agents *Agents) CheckRequest(ctx context.Context, r *http.Request, params url.Values) (oauth.Request, error) {
	return agents.authorization.CheckRequest(ctx, r, params)
}

// Decide checks the request again, as posted back by the consent page, and
// returns where to send the browser: with a code when allowed, for the agent
// to act as the viewer.
func (agents *Agents) Decide(ctx context.Context, r *http.Request, viewer store.User, params url.Values, allow bool) (string, error) {
	request, err := agents.CheckRequest(ctx, r, params)

	if err != nil {
		return "", err
	}

	if allow {
		return agents.authorization.Approve(ctx, r, request, viewer.ID)
	}

	return agents.authorization.Deny(r, request)
}

// AgentSetup is how to connect an agent to the server at an address.
type AgentSetup struct {
	URL           string // the MCP endpoint
	ServerName    string
	ClaudeCommand string
	CodexCommand  string
	ProjectConfig string // .mcp.json of a repository
	TokenSet      bool   // the viewer has a personal token
	Unencrypted   bool   // agents would reach it over plain HTTP from another machine
	IPsRestricted bool
}

// Setup is what the MCP page shows the viewer.
func (agents *Agents) Setup(ctx context.Context, viewer store.User, address *url.URL) (AgentSetup, error) {
	endpoint := agents.authorization.ResourceAt(address)
	tokenSet, err := agents.store.HasPersonalToken(ctx, viewer.ID)

	return AgentSetup{
		URL: endpoint, ServerName: agents.serverName,
		ClaudeCommand: "claude mcp add --transport http " + agents.serverName + " " + endpoint,
		CodexCommand:  "codex mcp add " + agents.serverName + " --url " + endpoint,
		ProjectConfig: projectConfig(agents.serverName, endpoint),
		TokenSet:      tokenSet, Unencrypted: oauth.Unencrypted(address), IPsRestricted: agents.ipsRestricted,
	}, err
}

// Page lists the agents of ownerID (the viewer's own when empty): a user
// the viewer may not see the agents of is ErrForbidden, a missing one
// store.ErrNotFound.
func (agents *Agents) Page(ctx context.Context, viewer store.User, ownerID string) (AgentsPage, error) {
	page := AgentsPage{Owner: viewer, Owners: []store.User{viewer}}

	if viewer.CanAdminister() {
		users, err := agents.store.ListUsers(ctx)

		if err != nil {
			return page, err
		}

		page.Owners = slices.DeleteFunc(users, func(user store.User) bool { return !viewer.CanAccessAgentsOf(user) })

		if index := slices.IndexFunc(page.Owners, func(user store.User) bool { return user.ID == ownerID }); index >= 0 {
			page.Owner = page.Owners[index]
		}
	}

	if ownerID != "" && page.Owner.ID != ownerID {
		if !store.ValidID(ownerID) {
			return page, store.ErrNotFound
		}

		owner, err := agents.store.GetUser(ctx, ownerID)

		if err != nil {
			return page, err
		}

		return page, agentsRefusal(viewer, owner)
	}

	var err error

	page.Grants, err = agents.authorization.Grants(ctx, page.Owner.ID)

	return page, err
}

// PersonalToken is a new personal token, shown once, with a prompt that has
// a coding agent add the server with it.
type PersonalToken struct {
	Token       string
	SetupPrompt string
}

// NewPersonalToken replaces the viewer's personal token.
func (agents *Agents) NewPersonalToken(ctx context.Context, viewer store.User, address *url.URL) (PersonalToken, error) {
	token, err := agents.authorization.NewPersonalToken(ctx, viewer.ID)

	if err != nil {
		return PersonalToken{}, err
	}

	return PersonalToken{Token: token, SetupPrompt: setupPrompt(agents.serverName, agents.authorization.ResourceAt(address), token)}, nil
}

// agentsRefusal: why viewer may not see or disconnect the agents of owner.
func agentsRefusal(viewer, owner store.User) error {
	if viewer.CanManage(owner) {
		return Forbidden(TakeOverProblem)
	}

	return Forbidden("your role may not access the agents of this user")
}

// Disconnect revokes an agent the viewer may access.
func (agents *Agents) Disconnect(ctx context.Context, viewer store.User, grantID string) error {
	grant, err := agents.authorization.Grant(ctx, grantID)

	if err != nil {
		return err
	}

	owner, err := agents.store.GetUser(ctx, grant.UserID)

	if err != nil {
		return err
	}

	if !viewer.CanAccessAgentsOf(owner) {
		return agentsRefusal(viewer, owner)
	}

	return agents.authorization.Disconnect(ctx, grantID)
}

// setupPrompt asks a coding agent to add the MCP server with a personal
// token itself and to check that it works. The token is shown once, so is
// the prompt.
func setupPrompt(serverName, endpoint, token string) string {
	header := "Authorization: Bearer " + token

	return fmt.Sprintf(`Add the MCP server %[1]q to your own MCP configuration: it gives you the errors and logs of my applications.

- Transport: Streamable HTTP
- URL: %[2]s
- Header: %[3]s

Add it at user level, not in a file of a repository: the token is a secret.
- Claude Code: claude mcp add --transport http --scope user %[1]s %[2]s --header %[3]q
- Codex, in ~/.codex/config.toml:
  [mcp_servers.%[1]s]
  url = %[2]q
  http_headers = { Authorization = %[4]q }
- Another client: the same URL and header in its MCP settings.

Then check the connection: call the list_projects tool and tell me which projects you see. If you must be restarted to load the server, tell me. If it is not reachable, show me the error and do not try other URLs.
Never repeat the token in your answers, files or commits.
`, serverName, endpoint, header, "Bearer "+token)
}

// projectConfig is the .mcp.json of Claude Code, for a repository: each
// developer then signs in in their own browser.
func projectConfig(serverName, endpoint string) string {
	type server struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}

	config := map[string]map[string]server{"mcpServers": {serverName: {Type: "http", URL: endpoint}}}
	encoded, _ := json.MarshalIndent(config, "", "  ")

	return string(encoded)
}
