package oauth

import (
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
)

// TestDocsOAuthValues: docs/mcp.md states how long an agent's access lasts.
func TestDocsOAuthValues(t *testing.T) {
	fixtures.DocValues(t, "docs/mcp.md",
		fixtures.DocValue{Phrase: "Access lasts {} and is renewed", Value: strings.Replace(config.HumanDuration(accessTokenTTL), "1 ", "an ", 1)},
		fixtures.DocValue{Phrase: "renewed by the agent for up to {} of inactivity", Value: config.HumanDuration(refreshTokenTTL)})
}
