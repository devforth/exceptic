package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
)

// AgentRequests are the handlers of the plain requests about coding agents
// for NewUI: the personal token, shown once, and the consent page, whose
// form token is bound to the session cookie.
func AgentRequests(logger *slog.Logger, auth *session.Auth, agents *services.Agents) map[string]http.Handler {
	handlers := agentRequests{logger: logger, auth: auth, agents: agents}

	return map[string]http.Handler{
		"personal_token":         http.HandlerFunc(handlers.personalToken),
		"authorization_request":  http.HandlerFunc(handlers.authorizationRequest),
		"authorization_decision": http.HandlerFunc(handlers.authorizationDecision),
	}
}

type agentRequests struct {
	logger *slog.Logger
	auth   *session.Auth
	agents *services.Agents
}

// actingUser is the user of the session: 401 without one; 404 while the
// password is temporary (only the account page is open, as in the catalog).
func (handlers agentRequests) actingUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	viewer, ok := sessionViewer(handlers.logger, handlers.auth, w, r)

	if ok && viewer.MustChangePassword {
		server.JSONError(w, http.StatusNotFound, ErrNotFound.Error())

		return store.User{}, false
	}

	return viewer, ok
}

func (handlers agentRequests) failed(w http.ResponseWriter, r *http.Request, name string, err error) {
	fail(handlers.logger, TransportUI, w, r, Endpoint{Name: name}, err)
}

// personalToken replaces the viewer's personal token. The answer is the only
// place the token appears; it is never logged.
func (handlers agentRequests) personalToken(w http.ResponseWriter, r *http.Request) {
	viewer, ok := handlers.actingUser(w, r)

	if !ok {
		return
	}

	token, err := handlers.agents.NewPersonalToken(r.Context(), viewer, handlers.agents.Address(r))

	if err != nil {
		handlers.failed(w, r, "personal_token", err)

		return
	}

	server.WriteJSON(w, http.StatusOK, PersonalToken{Token: token.Token, SetupPrompt: token.SetupPrompt})
}

// authorizationRequest validates an agent's request for the consent page.
func (handlers agentRequests) authorizationRequest(w http.ResponseWriter, r *http.Request) {
	if _, ok := handlers.actingUser(w, r); !ok {
		return
	}

	params, ok := authorizationParams(w, r.URL.Query().Get("query"))

	if !ok {
		return
	}

	request, err := handlers.agents.CheckRequest(r.Context(), r, params)

	if err != nil {
		handlers.authorizationError(w, r, "authorization_request", err)

		return
	}

	server.WriteJSON(w, http.StatusOK, Authorization{
		ClientName: request.ClientName, ClientSite: request.ClientSite(), RedirectURI: request.RedirectURI,
		FormToken: handlers.auth.FormToken(r, consentAction(params)),
	})
}

// authorizationDecision allows or denies the request the page showed: the
// form token proves this session saw exactly these parameters. The request
// is checked again before a code is issued.
func (handlers agentRequests) authorizationDecision(w http.ResponseWriter, r *http.Request) {
	viewer, ok := handlers.actingUser(w, r)

	if !ok {
		return
	}

	var input AuthorizationDecision

	if !readPlain(w, r, &input) {
		return
	}

	params, ok := authorizationParams(w, input.Query)

	if !ok {
		return
	}

	if !handlers.auth.ValidFormToken(r, consentAction(params), input.FormToken) {
		server.JSONError(w, http.StatusForbidden, "invalid authorization form token")

		return
	}

	target, err := handlers.agents.Decide(r.Context(), r, viewer, params, input.Allow)

	if err != nil {
		handlers.authorizationError(w, r, "authorization_decision", err)

		return
	}

	server.WriteJSON(w, http.StatusOK, AuthorizationTarget{RedirectURL: target})
}

// authorizationParams reads the query of the consent page's address,
// duplicates kept, so the checks of oauth see it as the agent sent it.
func authorizationParams(w http.ResponseWriter, query string) (url.Values, bool) {
	params, err := url.ParseQuery(query)

	if err != nil || len(query) > oauth.MaxBody {
		server.JSONError(w, http.StatusBadRequest, "invalid authorization request")

		return nil, false
	}

	return params, true
}

// consentAction is what a consent form token is bound to: the exact request.
func consentAction(params url.Values) string {
	return oauth.AuthorizePath + "?" + params.Encode()
}

// authorizationError shows a refused request on the page (400 with its
// reason); the browser is never sent to an unchecked redirect_uri.
func (handlers agentRequests) authorizationError(w http.ResponseWriter, r *http.Request, name string, err error) {
	var oauthErr *oauth.Error

	if errors.As(err, &oauthErr) {
		server.JSONError(w, http.StatusBadRequest, oauthErr.Description)

		return
	}

	handlers.failed(w, r, name, err)
}
