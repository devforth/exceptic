package contracts

import "time"

// AgentsView is the MCP page: how to connect a coding agent.
type AgentsView struct {
	URL                  string // the MCP endpoint
	Client               string // the selected tab
	Tabs                 []AgentTab
	ClaudeCommand        string // `claude mcp add ...`
	CodexCommand         string // `codex mcp add ...`
	ProjectConfig        string // .mcp.json, for a team sharing the setup in a repository
	PersonalTokenSet     bool
	PersonalToken        string // just created: shown once
	PersonalTokenPreview string // the same, masked
	// SetupPrompt asks an agent to add the server with the personal token
	// just created (shown with it, once).
	SetupPrompt string
	Connections []AgentConnection
	// OwnerID is whose connections are listed: the viewer, or a user the
	// viewer manages, chosen from Owners (empty when there is no choice).
	OwnerID       string
	Owners        []AgentOwner
	IPsRestricted bool // ALLOWED_IPS limits who may connect
	Unencrypted   bool // tokens would cross the network without TLS
}

// The MCP clients the page has instructions for, a tab and a panel each.
const (
	ClaudeClient = "claude"
	CodexClient  = "codex"
	OtherClient  = "other" // any other client, and the personal token
)

// Claude, Codex and Other are the tabs of the panels.
func (view AgentsView) Claude() AgentTab { return view.tab(ClaudeClient) }
func (view AgentsView) Codex() AgentTab  { return view.tab(CodexClient) }
func (view AgentsView) Other() AgentTab  { return view.tab(OtherClient) }

func (view AgentsView) tab(client string) AgentTab {
	for _, tab := range view.Tabs {
		if tab.ID == client {
			return tab
		}
	}

	return AgentTab{ID: client}
}

// AgentTab selects the instructions for one MCP client.
type AgentTab struct {
	ID     string
	Label  string
	URL    string // the page with this tab, keeping whose apps are listed
	Active bool
}

// AgentConnection describes an authorized client without exposing its tokens.
type AgentConnection struct {
	ID         string
	Name       string
	Personal   bool // the personal token, not an OAuth app
	Connected  time.Time
	LastActive *time.Time
}

// AgentOwner is a user whose connected agents an admin may list.
type AgentOwner struct {
	ID   string
	Name string
}
