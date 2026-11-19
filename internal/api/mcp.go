package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/store"
)

// MCPPath is where agents connect.
const MCPPath = "/mcp"

type MCP struct {
	oauth  *oauth.Server
	logger *slog.Logger
	store  store.Store
	http   http.Handler
}

// roles are the roles a server is built for.
var roles = []string{store.RoleSuperadmin, store.RoleAdmin, store.RoleUser}

// NewMCP builds the MCP servers at /mcp from the catalog: the endpoints a
// user's role allows, plus fetch_skill; appName (APP_NAME) names them to agents.
func NewMCP(logger *slog.Logger, database store.Store, endpoints []Endpoint, authorization *oauth.Server, appName, version string) *MCP {
	handler := &MCP{oauth: authorization, logger: logger, store: database}
	servers := map[string]*mcp.Server{}

	for _, role := range roles {
		roleServer := mcp.NewServer(&mcp.Implementation{Name: config.AppSlug(appName), Title: appName, Version: version},
			&mcp.ServerOptions{Instructions: serverInstructions(appName)})

		// Any user of the role: the tools depend on the role only.
		for _, endpoint := range agentEndpoints(endpoints, store.User{ID: store.NewID(), Role: role}) {
			roleServer.AddTool(tool(endpoint), handler.run(endpoint))
		}

		servers[role] = roleServer
	}

	handler.http = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		caller, ok := r.Context().Value(callerKey{}).(Call)

		if !ok {
			return nil // the SDK answers 400; Protect has let no such request through
		}

		return servers[caller.Viewer.Role]
	}, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		// The SDK rejects requests to a localhost address with another Host
		// (DNS rebinding of a local server without auth). Behind a reverse
		// proxy on the same machine every request looks like that, and the
		// endpoint requires a token. oauth.Protect additionally validates
		// browser Origins against PUBLIC_URL/the request, including GET.
		DisableLocalhostProtection: true,
	})

	return handler
}

// agentEndpoints are the tools viewer may use: the catalog without the
// REST-only endpoints and those of other roles, plus fetch_skill.
func agentEndpoints(endpoints []Endpoint, viewer store.User) []Endpoint {
	var tools []Endpoint

	for _, endpoint := range append(endpoints, fetchSkill) {
		if endpoint.AllowedFor(Call{Viewer: viewer}) {
			tools = append(tools, endpoint)
		}
	}

	return tools
}

func tool(endpoint Endpoint) *mcp.Tool {
	annotations := &mcp.ToolAnnotations{ReadOnlyHint: endpoint.ReadOnly}

	if !endpoint.ReadOnly {
		annotations.IdempotentHint = endpoint.Idempotent
		annotations.DestructiveHint = &endpoint.Destructive
	}

	return &mcp.Tool{Name: endpoint.Name, Description: endpoint.ToolDescription(), InputSchema: endpoint.InputSchema, Annotations: annotations}
}

// run calls the endpoint for the user the agent acts for. Errors become
// tool errors the agent reads: input mistakes and missing records as they
// are, anything else logged and reported without details.
func (handler *MCP) run(endpoint Endpoint) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		// The SDK runs tools in goroutines without a recover: a panic would stop the server.
		defer func() {
			if recovered := recover(); recovered != nil {
				result, err = handler.failed(endpoint, fmt.Errorf("panic: %v\n%s", recovered, debug.Stack()))
			}
		}()

		call, ok := ctx.Value(callerKey{}).(Call)

		if !ok {
			return handler.failed(endpoint, ErrNotFound)
		}

		var input map[string]any

		if len(request.Params.Arguments) > 0 {
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return handler.failed(endpoint, Invalid("arguments must be a JSON object"))
			}
		}

		output, err := endpoint.Run(ctx, call, input)

		if err != nil {
			return handler.failed(endpoint, err)
		}

		if text, ok := output.(Text); ok {
			return textResult(string(text)), nil
		}

		encoded, err := json.Marshal(output)

		if err != nil {
			return handler.failed(endpoint, err)
		}

		return textResult(string(encoded)), nil
	}
}

func textResult(content string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: content}}}
}

func (handler *MCP) failed(endpoint Endpoint, err error) (*mcp.CallToolResult, error) {
	message, _, ok := Outcome(err)

	if !ok {
		handler.logger.Error("mcp tool failed", "tool", endpoint.Name, "err", err)

		message = "internal error"
	}

	result := textResult(message)
	result.IsError = true

	return result, nil
}

// Register adds the endpoint to router, wrapped with protect (the IP
// allowlist); it needs a token. No method in its pattern: the transport
// answers GET and DELETE itself.
func (handler *MCP) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	router.Handle(MCPPath, protect(handler.oauth.Protect(handler.withCaller(handler.http))))
}

type callerKey struct{}

// withCaller reads who the agent acts for: the grant's user as stored now
// (a changed role applies at once), and the server's address as the agent
// reached it, for DSNs. It runs after oauth.Protect has checked the token.
func (handler *MCP) withCaller(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant, ok := oauth.GrantOf(auth.TokenInfoFromContext(r.Context()))

		if !ok {
			http.Error(w, "no grant", http.StatusUnauthorized)

			return
		}

		viewer, err := handler.store.GetUser(r.Context(), grant.UserID)

		if err != nil {
			handler.logger.Warn("mcp grant without its user", "grant", grant.ID, "err", err)
			http.Error(w, "unknown user", http.StatusUnauthorized)

			return
		}

		address := server.PublicURL(r, handler.oauth.PublicURL, handler.oauth.TrustedProxies)
		call := Call{Viewer: viewer, Actor: viewer.Actor(store.SourceMCP), ForAgent: true, Address: address}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, call)))
	})
}
