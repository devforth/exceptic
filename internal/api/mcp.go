package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/logging"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/store"
)

// MCPPath is where agents connect.
const MCPPath = "/mcp"

type MCP struct {
	oauth    *oauth.Server
	logger   *slog.Logger
	store    store.Store
	projects config.ProjectList
	http     http.Handler
}

// roles are the roles a server is built for.
var roles = []string{store.RoleAdmin, store.RoleUser}

// NewMCP builds the MCP servers at /mcp from the catalog: the endpoints a
// user's role allows, plus fetch_skill; appName (APP_NAME) names them to
// agents. projects name the viewer's in a refusal.
func NewMCP(logger *slog.Logger, database store.Store, projects config.ProjectList, endpoints []Endpoint, authorization *oauth.Server, appName, version string) *MCP {
	handler := &MCP{oauth: authorization, logger: logger, store: database, projects: projects}
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

// agentEndpoints are the tools viewer may use: the catalog's MCP endpoints
// their role allows, plus fetch_skill.
func agentEndpoints(endpoints []Endpoint, viewer store.User) []Endpoint {
	var tools []Endpoint

	for _, endpoint := range append(endpoints, fetchSkill) {
		if endpoint.AllowedFor(Call{Transport: TransportMCP, Viewer: viewer}) {
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
// tool errors the agent reads, starting with their kind (OutcomeOf):
// "forbidden: ...", "not found: ...", anything unexpected logged and
// reported without details.
func (handler *MCP) run(endpoint Endpoint) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		call, ok := ctx.Value(callerKey{}).(Call)

		// The SDK runs tools in goroutines without a recover: a panic would stop the server.
		defer func() {
			if recovered := recover(); recovered != nil {
				result, err = handler.failed(call, endpoint, logging.PanicError(recovered))
			}
		}()

		if !ok {
			return handler.failed(call, endpoint, errors.New("mcp call without its caller"))
		}

		var input map[string]any

		if len(request.Params.Arguments) > 0 {
			if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
				return handler.failed(call, endpoint, Invalid("arguments must be a JSON object"))
			}
		}

		output, err := endpoint.Run(ctx, call, input)

		if err != nil {
			return handler.failed(call, endpoint, err)
		}

		if text, ok := output.(Text); ok {
			return textResult(string(text)), nil
		}

		encoded, err := json.Marshal(output)

		if err != nil {
			return handler.failed(call, endpoint, err)
		}

		return textResult(string(encoded)), nil
	}
}

func textResult(content string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: content}}}
}

// failed is the tool error of err. A record outside the viewer's projects
// names the projects they see, which list_projects shows them anyway, so
// the agent tells it from a deleted one.
func (handler *MCP) failed(call Call, endpoint Endpoint, err error) (*mcp.CallToolResult, error) {
	outcome := OutcomeOf(err)

	if outcome.Logged {
		handler.logger.Error("mcp tool failed", "tool", endpoint.Name, "err", err)
	}

	text := outcome.ToolText()

	if outcome.Hidden {
		var names []string

		for _, project := range call.Access().Visible(handler.projects.List()) {
			names = append(names, project.Name)
		}

		text += " (" + cmp.Or(strings.Join(names, ", "), "you have none") + ")"
	}

	result := textResult(text)
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
			handler.logger.Error("mcp request without its grant", "path", r.URL.Path)
			server.JSONError(w, http.StatusInternalServerError, "internal error")

			return
		}

		viewer, err := handler.store.GetUser(r.Context(), grant.UserID)

		switch {
		case errors.Is(err, store.ErrNotFound):
			handler.logger.Warn("mcp grant without its user", "grant", grant.ID)
			handler.oauth.RefuseToken(w, r)

			return
		case err != nil:
			handler.logger.Error("mcp grant's user not read", "grant", grant.ID, "err", err)
			server.JSONError(w, http.StatusInternalServerError, "internal error")

			return
		}

		address := server.PublicURL(r, handler.oauth.PublicURL, handler.oauth.TrustedProxies)
		call := Call{Transport: TransportMCP, Viewer: viewer, Actor: viewer.Actor(store.SourceMCP), Address: address}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), callerKey{}, call)))
	})
}
