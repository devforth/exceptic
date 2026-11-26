package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsAgentCommands: docs/mcp.md shows the commands the MCP page and the
// setup prompt give, for the example server.
func TestDocsAgentCommands(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(fixtures.Root(), "docs", "mcp.md"))

	if err != nil {
		t.Fatal(err)
	}

	const name, endpoint = "exceptic", "https://errors.example.com/mcp"

	document := strings.ReplaceAll(string(content), " \\\n  ", " ")
	commands := []string{claudeCommand(name, endpoint), codexCommand(name, endpoint)}

	for line := range strings.Lines(setupPrompt(name, endpoint, "<token>")) {
		line = strings.TrimSpace(line)

		if command, ok := strings.CutPrefix(line, "- Claude Code: "); ok {
			commands = append(commands, command)
		}

		if strings.HasPrefix(line, "[mcp_servers.") || strings.HasPrefix(line, "url = ") || strings.HasPrefix(line, "http_headers = ") {
			commands = append(commands, line)
		}
	}

	if len(commands) != 6 {
		t.Fatalf("found %d commands in the setup prompt", len(commands))
	}

	for _, command := range commands {
		if !strings.Contains(document, command) {
			t.Errorf("docs/mcp.md lacks %q", command)
		}
	}
}
