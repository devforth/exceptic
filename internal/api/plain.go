package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"

	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/session"
)

// PlainRequest is a request of the web app that is no endpoint of the
// catalog, since it handles a login or a credential (session cookies, the
// login's limits, a token shown once). Its handler, given to NewUI, decodes
// In and writes Out (NoContent: 204); catalog.gen.ts declares both.
type PlainRequest struct {
	Name, Method  string
	input, output reflect.Type
}

func plainRequest[In, Out any](name, method string) PlainRequest {
	return PlainRequest{Name: name, Method: method, input: reflect.TypeFor[In](), output: reflect.TypeFor[Out]()}
}

// NoContent is the output of a plain request that answers 204.
type NoContent struct{}

// PlainRequests are every plain request; their names are no endpoint's.
var PlainRequests = []PlainRequest{
	plainRequest[LoginStateInput, LoginState]("login_state", http.MethodGet),
	plainRequest[LoginInput, LoginOutput]("login", http.MethodPost),
	plainRequest[noInput, NoContent]("logout", http.MethodPost),
	plainRequest[ChangePasswordInput, NoContent]("change_password", http.MethodPost),
	plainRequest[noInput, PersonalToken]("personal_token", http.MethodPost),
	plainRequest[AuthorizationInput, Authorization]("authorization_request", http.MethodGet),
	plainRequest[AuthorizationDecision, AuthorizationTarget]("authorization_decision", http.MethodPost),
}

// LoginStateInput: next is where to go after the login.
type LoginStateInput struct {
	Next string `json:"next,omitempty"`
}

// LoginState is what the login page shows; Redirect (a local path) when the
// browser is logged in already.
type LoginState struct {
	AppName  string `json:"app_name"`
	Disabled bool   `json:"disabled"`
	Redirect string `json:"redirect,omitempty"`
}

type LoginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Next     string `json:"next,omitempty"`
}

// LoginOutput: Next is the local path to open.
type LoginOutput struct {
	Next string `json:"next"`
}

type ChangePasswordInput struct {
	CurrentPassword string `json:"current_password"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// PersonalToken is a new personal token, shown once; SetupPrompt has an
// agent add the server with it.
type PersonalToken struct {
	Token       string `json:"token"`
	SetupPrompt string `json:"setup_prompt"`
}

// wrongLogin is the answer to a wrong password and to an unknown username alike.
const wrongLogin = "Wrong username or password."

// LoginRequests are the handlers of the login's plain requests (login_state,
// login, logout, change_password) for NewUI. Every password check goes
// through auth, so the login's limits apply to each.
func LoginRequests(logger *slog.Logger, auth *session.Auth, appName string) map[string]http.Handler {
	failed := func(w http.ResponseWriter, r *http.Request, name string, err error) {
		fail(logger, TransportUI, w, r, Endpoint{Name: name}, err)
	}

	return map[string]http.Handler{
		"login_state": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := LoginState{AppName: appName, Disabled: auth.Disabled(r.Context())}

			// Logged in already: straight on.
			if _, ok := auth.SessionUser(r); ok {
				state.Redirect = server.SafeNext(r.URL.Query().Get("next"))
			}

			server.WriteJSON(w, http.StatusOK, state)
		}),

		"login": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var input LoginInput

			if !readPlain(w, r, &input) {
				return
			}

			if auth.Disabled(r.Context()) {
				server.JSONError(w, http.StatusForbidden, "The web UI is disabled: set ADMIN_PASSWORD to enable it.")

				return
			}

			result, err := auth.Login(w, r, input.Username, input.Password)

			switch {
			case err != nil:
				failed(w, r, "login", err)
			case result == session.LoginBlocked:
				server.JSONError(w, http.StatusTooManyRequests, session.TooManyAttempts)
			case result == session.LoginWrongCredentials:
				server.JSONError(w, http.StatusUnauthorized, wrongLogin)
			default:
				server.WriteJSON(w, http.StatusOK, LoginOutput{Next: server.SafeNext(input.Next)})
			}
		}),

		// Also without a valid session: the cookie is cleared either way.
		"logout": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth.EndSession(r.Context(), w, r)
			noContent(w)
		}),

		// Open with a temporary password: it is how the user replaces it.
		"change_password": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			viewer, ok := sessionViewer(logger, auth, w, r)

			if !ok {
				return
			}

			var input ChangePasswordInput

			if !readPlain(w, r, &input) {
				return
			}

			problems, err := auth.ChangePassword(w, r, viewer, input.CurrentPassword, input.Password, input.ConfirmPassword)

			if err == nil {
				err = problemsError(problems, nil)
			}

			if err != nil {
				failed(w, r, "change_password", err)

				return
			}

			noContent(w)
		}),
	}
}

func noContent(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// readPlain decodes the JSON body of a plain request into input; it answers
// 400 itself when the body is not one.
func readPlain(w http.ResponseWriter, r *http.Request, input any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(input); err != nil {
		server.JSONError(w, http.StatusBadRequest, "the body must be a JSON object of the request's input")

		return false
	}

	return true
}

// AuthorizationInput: Query is the query of an agent's address of the
// consent page (/oauth/authorize?...), as it came.
type AuthorizationInput struct {
	Query string `json:"query"`
}

// Authorization is what the consent page shows about a valid request, and
// FormToken, which binds the decision to this session and this request.
// Refused requests answer 400 with the reason, never a redirect.
type Authorization struct {
	ClientName string `json:"client_name"`
	// ClientSite is the host that published the agent's metadata document.
	ClientSite  string `json:"client_site,omitempty"`
	RedirectURI string `json:"redirect_uri"`
	FormToken   string `json:"form_token"`
}

type AuthorizationDecision struct {
	Query     string `json:"query"`
	FormToken string `json:"form_token"`
	Allow     bool   `json:"allow"`
}

// AuthorizationTarget is where the page sends the browser: back to the
// agent, with a code or an error.
type AuthorizationTarget struct {
	RedirectURL string `json:"redirect_url"`
}
