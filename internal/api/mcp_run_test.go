package api

import (
	"context"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devforth/exceptic/internal/storage/store"
)

// A panicking tool answers "internal error": the SDK runs tools in
// goroutines without a recover, so it would stop the whole server.
func TestToolPanicIsRecovered(t *testing.T) {
	handler := &MCP{logger: slog.New(slog.DiscardHandler)}
	endpoint := Define(Endpoint{Name: "explode", ReadOnly: true}, func(context.Context, Call, struct{}) (struct{}, error) {
		panic("boom")
	})
	ctx := context.WithValue(context.Background(), callerKey{}, Call{Viewer: store.User{ID: "u1", Role: store.RoleUser}})

	result, err := handler.run(endpoint)(ctx, &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}})

	if err != nil || !result.IsError || result.Content[0].(*mcp.TextContent).Text != "internal error" {
		t.Fatalf("result %+v, err %v", result, err)
	}
}
