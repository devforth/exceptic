package services

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The setup prompt carries the new token once, has the agent add the server
// at user level and never repeat the token; the page's commands share the
// server name and endpoint.
func TestPersonalTokenSetup(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	agents := newAgents(&oauth.Server{Store: database, ResourcePath: "/mcp"}, database, &config.Config{AppName: "Acme Errors"})
	address := &url.URL{Scheme: "https", Host: "errors.example.com", Path: "/monitoring"}
	id, err := database.CreateUser(ctx, store.User{Email: "dev", Role: store.RoleUser, CreatedAt: time.Now()}, "x")

	if err != nil {
		t.Fatal(err)
	}

	viewer := store.User{ID: id, Email: "dev", Role: store.RoleUser}
	setup, err := agents.Setup(ctx, viewer, address)

	if err != nil || setup.TokenSet || setup.Unencrypted || setup.ServerName != "acme-errors" ||
		setup.ClaudeCommand != "claude mcp add --transport http acme-errors https://errors.example.com/monitoring/mcp" ||
		setup.CodexCommand != "codex mcp add acme-errors --url https://errors.example.com/monitoring/mcp" ||
		!strings.Contains(setup.ProjectConfig, `"url": "https://errors.example.com/monitoring/mcp"`) {
		t.Fatalf("%+v %v", setup, err)
	}

	token, err := agents.NewPersonalToken(ctx, viewer, address)

	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`claude mcp add --transport http --scope user acme-errors https://errors.example.com/monitoring/mcp --header "Authorization: Bearer ` + token.Token + `"`,
		"[mcp_servers.acme-errors]", "Never repeat the token",
	} {
		if !strings.Contains(token.SetupPrompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, token.SetupPrompt)
		}
	}

	if setup, _ := agents.Setup(ctx, viewer, address); !setup.TokenSet {
		t.Error("the new token is not set")
	}
}
