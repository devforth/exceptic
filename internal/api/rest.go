package api

import (
	"encoding/json"
	"errors"
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

// Route is the method and path of an endpoint in a transport served over
// HTTP: GET for a read-only one, POST otherwise.
func Route(transport Transport, endpoint Endpoint) string {
	method := http.MethodPost

	if endpoint.ReadOnly {
		method = http.MethodGet
	}

	prefix := server.RESTPrefix

	if transport == TransportUI {
		prefix = server.UIAPIPrefix
	}

	return method + " " + prefix + endpoint.Name
}

// Register adds the routes to router, each wrapped with protect (IP
// allowlist and token). Other /api/ addresses are the server's (server.unknownAPI).
func (handler *REST) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	register(router, handler.logger, TransportREST, handler.endpoints, protect, handler.caller)
}

// apiActor is who changes a status through the REST API: its one token.
var apiActor = store.Actor{Name: "API token", Source: store.SourceAPI}

func (handler *REST) caller(_ http.ResponseWriter, r *http.Request) (Call, bool) {
	return Call{Transport: TransportREST, Actor: apiActor, Address: server.PublicURL(r, handler.publicURL, handler.trustedProxies)}, true
}

// register adds the routes of the endpoints transport serves, one by one: a
// subtree pattern would overlap the ingest route /api/{projectID}/envelope/.
// Another method at an endpoint's path answers 405.
func register(router *http.ServeMux, logger *slog.Logger, transport Transport, endpoints []Endpoint,
	protect func(http.Handler) http.Handler, caller func(http.ResponseWriter, *http.Request) (Call, bool),
) {
	for _, endpoint := range endpoints {
		if endpoint.Serves(transport) {
			route := Route(transport, endpoint)
			method, path, _ := strings.Cut(route, " ")
			router.Handle(route, protect(serve(logger, transport, endpoint, caller)))
			router.Handle(path, protect(wrongMethod(method, caller)))
		}
	}
}

// wrongMethod answers a request with another method than method, the
// caller checked first as for any request.
func wrongMethod(method string, caller func(http.ResponseWriter, *http.Request) (Call, bool)) http.Handler {
	allow := method

	if method == http.MethodGet {
		allow = "GET, HEAD"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := caller(w, r); ok {
			w.Header().Set("Allow", allow)
			server.JSONError(w, http.StatusMethodNotAllowed, "method not allowed: use "+method)
		}
	})
}

// serve runs endpoint over HTTP for the caller of the request; caller has
// answered itself when it returns false. The role is checked before the
// input is read.
func serve(logger *slog.Logger, transport Transport, endpoint Endpoint, caller func(http.ResponseWriter, *http.Request) (Call, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call, ok := caller(w, r)

		if !ok {
			return
		}

		if err := endpoint.Refused(call); err != nil {
			fail(logger, transport, w, r, endpoint, err)

			return
		}

		input, err := readInput(endpoint, w, r)

		if err != nil {
			fail(logger, transport, w, r, endpoint, err)

			return
		}

		output, err := endpoint.Run(r.Context(), call, input)

		if err != nil {
			fail(logger, transport, w, r, endpoint, err)

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

// fail answers err as OutcomeOf sorts it (the HTTP transports' one place),
// logging an unexpected one.
func fail(logger *slog.Logger, transport Transport, w http.ResponseWriter, r *http.Request, endpoint Endpoint, err error) {
	outcome := OutcomeOf(err)

	if outcome.Logged {
		logger.Error("api request failed", "transport", transport, "endpoint", endpoint.Name, "path", r.URL.Path, "err", err)
	}

	if outcome.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(outcome.RetryAfter.Seconds())))
	}

	server.WriteJSON(w, outcome.Status, outcome.Body)
}

// readInput is the query (GET) or the JSON body (POST) as the endpoint's
// input object; the endpoint validates it against its schema.
func readInput(endpoint Endpoint, w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	if r.Method == http.MethodGet {
		return queryInput(endpoint, r)
	}

	if r.URL.RawQuery != "" {
		return nil, Invalid("pass the input of %s as a JSON body", endpoint.Name)
	}

	input := map[string]any{}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))

	if err != nil {
		return nil, bodyError(err)
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		return input, nil
	}

	if err := json.Unmarshal(body, &input); err != nil {
		return nil, Invalid("the body must be a JSON object: %v", err)
	}

	return input, nil
}

// bodyError is a body that could not be read: 413 over maxBody.
func bodyError(err error) error {
	if tooLarge := new(http.MaxBytesError); errors.As(err, &tooLarge) {
		return InputError{status: http.StatusRequestEntityTooLarge, message: fmt.Sprintf("the body is larger than %d KiB", maxBody>>10)}
	}

	return Invalid("read body: %v", err)
}

// queryInput converts query strings to the types of the input fields.
func queryInput(endpoint Endpoint, r *http.Request) (map[string]any, error) {
	input := map[string]any{}

	for name, values := range r.URL.Query() {
		fieldType, itemType, ok := endpoint.FieldType(name)

		if !ok {
			return nil, Invalid("unknown parameter %q", name)
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
			return nil, Invalid("parameter %q given more than once", name)
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
			return nil, Invalid("%s must be an integer", name)
		}

		return number, nil
	case "boolean":
		flag, err := strconv.ParseBool(value)

		if err != nil {
			return nil, Invalid("%s must be true or false", name)
		}

		return flag, nil
	default:
		return value, nil
	}
}
