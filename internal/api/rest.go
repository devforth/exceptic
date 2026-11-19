package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/store"
)

// RESTPrefix is where the endpoints are served.
const RESTPrefix = "/api/v1/"

// maxBody caps a POST body: inputs are a few fields.
const maxBody = 1 << 16

type REST struct {
	logger         *slog.Logger
	endpoints      []Endpoint
	publicURL      string
	trustedProxies []netip.Prefix
}

// NewREST serves the catalog under /api/v1/<name> (docs/api.md);
// publicURL and trustedProxies give the server's address for DSNs.
func NewREST(logger *slog.Logger, endpoints []Endpoint, publicURL string, trustedProxies []netip.Prefix) *REST {
	return &REST{logger: logger, endpoints: endpoints, publicURL: publicURL, trustedProxies: trustedProxies}
}

// Route is the method and path of an endpoint.
func Route(endpoint Endpoint) string {
	method := http.MethodPost

	if endpoint.ReadOnly {
		method = http.MethodGet
	}

	return method + " " + RESTPrefix + endpoint.Name
}

// Register adds the routes to router, each wrapped with protect (IP
// allowlist and token).
func (handler *REST) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	for _, endpoint := range handler.endpoints {
		// Agent-only endpoints act as a user; the token is none.
		if !endpoint.AgentsOnly {
			router.Handle(Route(endpoint), protect(handler.serve(endpoint)))
		}
	}

	// Any other /api/ address: a JSON 404, not the UI's page. Less specific
	// than the ingest route /api/{projectID}/envelope/, so it does not overlap it.
	router.Handle("/api/", protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		server.JSONError(w, http.StatusNotFound, "unknown API endpoint: see /api/v1/<name> in docs/api.md")
	})))
}

// apiActor is who changes a status through the REST API: its one token.
var apiActor = store.Actor{Name: "API token", Source: store.SourceAPI}

func (handler *REST) serve(endpoint Endpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		input, err := readInput(endpoint, w, r)

		if err != nil {
			server.JSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		call := Call{ByToken: true, Actor: apiActor, Address: server.PublicURL(r, handler.publicURL, handler.trustedProxies)}
		output, err := endpoint.Run(r.Context(), call, input)

		if err != nil {
			handler.fail(w, r, endpoint, err)
			return
		}

		if text, ok := output.(Text); ok {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = io.WriteString(w, string(text)) // the client went away; nothing to do

			return
		}

		server.WriteJSON(w, http.StatusOK, output)
	})
}

func (handler *REST) fail(w http.ResponseWriter, r *http.Request, endpoint Endpoint, err error) {
	message, notFound, ok := Outcome(err)

	switch {
	case !ok:
		handler.logger.Error("api request failed", "endpoint", endpoint.Name, "path", r.URL.Path, "err", err)
		server.JSONError(w, http.StatusInternalServerError, "internal error")
	case notFound:
		server.JSONError(w, http.StatusNotFound, message)
	default:
		server.JSONError(w, http.StatusBadRequest, message)
	}
}

// readInput is the query (GET) or the JSON body (POST) as the endpoint's
// input object; the endpoint validates it against its schema.
func readInput(endpoint Endpoint, w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	if r.Method == http.MethodGet {
		return queryInput(endpoint, r)
	}

	if r.URL.RawQuery != "" {
		return nil, fmt.Errorf("pass the input of %s as a JSON body", endpoint.Name)
	}

	input := map[string]any{}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))

	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		return input, nil
	}

	if err := json.Unmarshal(body, &input); err != nil {
		return nil, fmt.Errorf("the body must be a JSON object: %w", err)
	}

	return input, nil
}

// queryInput converts query strings to the types of the input fields.
func queryInput(endpoint Endpoint, r *http.Request) (map[string]any, error) {
	input := map[string]any{}

	for name, values := range r.URL.Query() {
		fieldType, itemType, ok := endpoint.FieldType(name)

		if !ok {
			return nil, fmt.Errorf("unknown parameter %q", name)
		}

		if fieldType == "array" {
			var items []any

			for _, value := range values {
				for _, item := range strings.Split(value, ",") {
					converted, err := convert(name, itemType, strings.TrimSpace(item))

					if err != nil {
						return nil, err
					}

					items = append(items, converted)
				}
			}

			input[name] = items

			continue
		}

		if len(values) > 1 {
			return nil, fmt.Errorf("parameter %q given more than once", name)
		}

		converted, err := convert(name, fieldType, values[0])

		if err != nil {
			return nil, err
		}

		input[name] = converted
	}

	return input, nil
}

func convert(name, fieldType, value string) (any, error) {
	switch fieldType {
	case "integer":
		number, err := strconv.ParseInt(value, 10, 64)

		if err != nil {
			return nil, fmt.Errorf("%s must be an integer", name)
		}

		return number, nil
	case "boolean":
		flag, err := strconv.ParseBool(value)

		if err != nil {
			return nil, fmt.Errorf("%s must be true or false", name)
		}

		return flag, nil
	default:
		return value, nil
	}
}
