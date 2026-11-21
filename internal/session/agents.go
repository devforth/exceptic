package session

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// AgentRequests are the plain requests of the web app (api.PlainRequests)
// about coding agents: the personal token, shown once, and the consent page,
// whose form token is bound to the session cookie.
func AgentRequests(auth *Auth, agents *services.Agents) map[string]http.Handler {
	return map[string]http.Handler{
		"personal_token":         http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { auth.personalToken(w, r, agents) }),
		"authorization_request":  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { auth.authorizationRequest(w, r, agents) }),
		"authorization_decision": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { auth.authorizationDecision(w, r, agents) }),
	}
}

// actingUser is the user of the session: 401 without one; 404 while the
// password is temporary (only the account page is open, as in the catalog).
func (auth *Auth) actingUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	viewer, err := auth.User(r)

	switch {
	case errors.Is(err, store.ErrNotFound):
		server.JSONError(w, http.StatusUnauthorized, "login required")
	case err != nil:
		auth.internalError(w, r, err)
	case viewer.MustChangePassword:
		server.JSONError(w, http.StatusNotFound, api.ErrNotFound.Error())
	default:
		return viewer, true
	}

	return store.User{}, false
}

func (auth *Auth) internalError(w http.ResponseWriter, r *http.Request, err error) {
	auth.logger.Error("web app request failed", "path", r.URL.Path, "err", err)
	server.JSONError(w, http.StatusInternalServerError, "internal error")
}

// personalToken replaces the viewer's personal token. The answer is the only
// place the token appears; it is never logged.
func (auth *Auth) personalToken(w http.ResponseWriter, r *http.Request, agents *services.Agents) {
	viewer, ok := auth.actingUser(w, r)

	if !ok {
		return
	}

	token, err := agents.NewPersonalToken(r.Context(), viewer, agents.Address(r))

	if err != nil {
		auth.internalError(w, r, err)

		return
	}

	server.WriteJSON(w, http.StatusOK, api.PersonalToken{Token: token.Token, SetupPrompt: token.SetupPrompt})
}

// authorizationRequest validates an agent's request for the consent page.
func (auth *Auth) authorizationRequest(w http.ResponseWriter, r *http.Request, agents *services.Agents) {
	if _, ok := auth.actingUser(w, r); !ok {
		return
	}

	params, ok := authorizationParams(w, r.URL.Query().Get("query"))

	if !ok {
		return
	}

	request, err := agents.CheckRequest(r.Context(), r, params)

	if err != nil {
		auth.authorizationError(w, r, err)

		return
	}

	server.WriteJSON(w, http.StatusOK, api.Authorization{
		ClientName: request.ClientName, ClientSite: request.ClientSite(), RedirectURI: request.RedirectURI,
		FormToken: auth.FormToken(r, consentAction(params)),
	})
}

// authorizationDecision allows or denies the request the page showed: the
// form token proves this session saw exactly these parameters. The request
// is checked again before a code is issued.
func (auth *Auth) authorizationDecision(w http.ResponseWriter, r *http.Request, agents *services.Agents) {
	viewer, ok := auth.actingUser(w, r)

	if !ok {
		return
	}

	var input api.AuthorizationDecision

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, oauth.MaxBody))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		server.JSONError(w, http.StatusBadRequest, "invalid authorization decision")

		return
	}

	params, ok := authorizationParams(w, input.Query)

	if !ok {
		return
	}

	if !auth.ValidFormToken(r, consentAction(params), input.FormToken) {
		server.JSONError(w, http.StatusForbidden, "invalid authorization form token")

		return
	}

	target, err := agents.Decide(r.Context(), r, viewer, params, input.Allow)

	if err != nil {
		auth.authorizationError(w, r, err)

		return
	}

	server.WriteJSON(w, http.StatusOK, api.AuthorizationTarget{RedirectURL: target})
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
func (auth *Auth) authorizationError(w http.ResponseWriter, r *http.Request, err error) {
	var oauthErr *oauth.Error

	if errors.As(err, &oauthErr) {
		server.JSONError(w, http.StatusBadRequest, oauthErr.Description)

		return
	}

	auth.internalError(w, r, err)
}
