package api

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devforth/exceptic/internal/storage/store"
)

// A panicking tool answers "internal error": the SDK runs tools in
// goroutines without a recover, so it would stop the whole server.
func TestToolPanicIsRecovered(t *testing.T) {
	handler := &MCP{logger: slog.New(slog.DiscardHandler)}
	endpoint := Define(Endpoint{Name: "explode", ReadOnly: true, Transports: TransportMCP}, func(context.Context, Call, struct{}) (struct{}, error) {
		panic("boom")
	})
	ctx := context.WithValue(context.Background(), callerKey{}, Call{Transport: TransportMCP, Viewer: store.User{ID: "u1", Role: store.RoleUser}})

	result, err := handler.run(endpoint)(ctx, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}})

	if err != nil || !result.IsError || result.Content[0].(*mcp.TextContent).Text != "internal error" {
		t.Fatalf("result %+v, err %v", result, err)
	}
}

// Every tool that changes something tells the agent to ask the user first;
// only reading needs no confirmation.
func TestChangingToolsAskForConfirmation(t *testing.T) {
	for _, endpoint := range Endpoints(nil, nil) {
		asks := strings.Contains(endpoint.ToolDescription(), confirmAsk)

		if asks == endpoint.ReadOnly {
			t.Errorf("%s (read-only %v): asks for confirmation %v", endpoint.Name, endpoint.ReadOnly, asks)
		}
	}
}
