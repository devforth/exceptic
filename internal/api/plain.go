package api

import (
	"net/http"
	"reflect"
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
