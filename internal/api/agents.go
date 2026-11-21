package api

import "context"

type mcpSetupOutput struct {
	URL           string `json:"url"`
	ServerName    string `json:"server_name"`
	ClaudeCommand string `json:"claude_command"`
	CodexCommand  string `json:"codex_command"`
	ProjectConfig string `json:"project_config"`
	// TokenSet: the caller has a personal token (its value is never shown again).
	TokenSet bool `json:"token_set"`
	// Unencrypted: agents would reach the server over plain HTTP from another machine.
	Unencrypted   bool `json:"unencrypted"`
	IPsRestricted bool `json:"ips_restricted"`
}

func (handler *handlers) getMCPSetup(ctx context.Context, call Call, _ noInput) (mcpSetupOutput, error) {
	setup, err := handler.services.Agents.Setup(ctx, call.Viewer, call.Address)

	return mcpSetupOutput{
		URL: setup.URL, ServerName: setup.ServerName, ClaudeCommand: setup.ClaudeCommand, CodexCommand: setup.CodexCommand,
		ProjectConfig: setup.ProjectConfig, TokenSet: setup.TokenSet, Unencrypted: setup.Unencrypted, IPsRestricted: setup.IPsRestricted,
	}, err
}
