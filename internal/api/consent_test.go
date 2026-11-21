package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
)

const verifier = "01234567890123456789012345678901234567890123456789"

// authorizationParams registers an agent and returns the query of its
// address of the consent page.
func (server *restServer) authorizationParams() url.Values {
	server.t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(`{"client_name":"Codex <test>","redirect_uris":["http://127.0.0.1:44891/callback"]}`))
	request.Header.Set("Content-Type", "application/json")

	client := decoded[struct {
		ID string `json:"client_id"`
	}](server.t, serve(server, request), http.StatusCreated)
	challenge := sha256.Sum256([]byte(verifier))

	return url.Values{
		"client_id": {client.ID}, "redirect_uri": {"http://127.0.0.1:44891/callback"},
		"response_type": {"code"}, "state": {"opaque state & return"},
		"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"resource": {"http://example.com/mcp"},
	}
}

type authorization struct {
	ClientName  string `json:"client_name"`
	ClientSite  string `json:"client_site"`
	RedirectURI string `json:"redirect_uri"`
	FormToken   string `json:"form_token"`
}

// consent is what the consent page shows the user for params.
func (user uiUser) consent(params url.Values) *httptest.ResponseRecorder {
	return user.call(http.MethodGet, "authorization_request?"+url.Values{"query": {params.Encode()}}.Encode(), "")
}

// decide sends the user's decision as the consent page does.
func (user uiUser) decide(params url.Values, formToken string, allow bool) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"query": params.Encode(), "form_token": formToken, "allow": allow})

	return user.call(http.MethodPost, "authorization_decision", string(body))
}

// allowed returns the callback of an allowed request.
func (user uiUser) allowed(params url.Values) *url.URL {
	user.server.t.Helper()

	shown := decoded[authorization](user.server.t, user.consent(params), http.StatusOK)
	target := decoded[struct {
		RedirectURL string `json:"redirect_url"`
	}](user.server.t, user.decide(params, shown.FormToken, true), http.StatusOK)
	callback, err := url.Parse(target.RedirectURL)

	if err != nil {
		user.server.t.Fatal(err)
	}

	return callback
}

func (server *restServer) tokenRequest(form url.Values, want int) map[string]any {
	server.t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return decoded[map[string]any](server.t, serve(server, request), want)
}

func copyParams(params url.Values) url.Values {
	copied := url.Values{}

	for key, values := range params {
		copied[key] = append([]string(nil), values...)
	}

	return copied
}

// The consent page is a page of the web app: its address is the web app's,
// framed by no one, its referrer kept on this origin; the old form is gone.
func TestConsentPageIsTheWebApp(t *testing.T) {
	server := newRESTServer(t)
	params := server.authorizationParams()
	response := serve(server, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+params.Encode(), nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), fixtures.WebPage) {
		t.Fatalf("consent page: %d %s", response.Code, response.Body)
	}

	if policy := response.Header().Get("Referrer-Policy"); policy != "same-origin" {
		t.Errorf("Referrer-Policy %q", policy)
	}

	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Errorf("CSP %q", response.Header().Get("Content-Security-Policy"))
	}

	// A form posted to the page's address, as the server-rendered page took it, issues nothing.
	user := server.webUser(store.RoleAdmin, false)
	form := copyParams(params)
	form.Set("decision", "allow")
	request := httptest.NewRequest(http.MethodPost, "/oauth/authorize", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", uiOrigin)
	request.AddCookie(sessionCookie(user))

	if response := serve(server, request); response.Code < 400 || response.Header().Get("Location") != "" {
		t.Errorf("form post: %d %v", response.Code, response.Header())
	}
}

func sessionCookie(user uiUser) *http.Cookie {
	return &http.Cookie{Name: session.CookieName, Value: user.cookie}
}

// The whole sign-in of an agent: the consent, the code, the tokens.
func TestOAuthAuthorizationFlow(t *testing.T) {
	server := newRESTServer(t)
	params := server.authorizationParams()

	if response := (uiUser{server: server}).consent(params); response.Code != http.StatusUnauthorized {
		t.Fatalf("without a session: %d", response.Code)
	}

	user := server.webUser(store.RoleAdmin, false)
	shown := decoded[authorization](t, user.consent(params), http.StatusOK)

	if shown.ClientName != "Codex <test>" || shown.ClientSite != "" || shown.RedirectURI != "http://127.0.0.1:44891/callback" || shown.FormToken == "" {
		t.Fatalf("shown %+v", shown)
	}

	callback := user.allowed(params)

	if callback.Host != "127.0.0.1:44891" || callback.Query().Get("state") != params.Get("state") || callback.Query().Get("iss") != "http://example.com" || callback.Query().Get("code") == "" {
		t.Fatalf("invalid callback: %s", callback)
	}

	exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {params.Get("client_id")}, "redirect_uri": {params.Get("redirect_uri")}, "code": {callback.Query().Get("code")}, "code_verifier": {verifier}, "resource": {params.Get("resource")}}
	tokens := server.tokenRequest(exchange, http.StatusOK)
	server.tokenRequest(exchange, http.StatusBadRequest)

	// The agent acts as the user who allowed it; listed until disconnected.
	list := decoded[agentsList](t, user.call(http.MethodGet, "list_agents", ""), http.StatusOK)

	if len(list.Agents) != 1 || list.Agents[0].Name != "Codex <test>" || list.Agents[0].Kind != store.GrantOAuth || !list.Agents[0].Connected || list.Agents[0].LastUsedAt != nil {
		t.Fatalf("agents %+v", list)
	}

	server.mcpAccess(tokens["access_token"].(string), http.StatusOK)

	if list := decoded[agentsList](t, user.call(http.MethodGet, "list_agents", ""), http.StatusOK); list.Agents[0].LastUsedAt == nil {
		t.Error("last use not recorded")
	}

	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {params.Get("client_id")}, "refresh_token": {tokens["refresh_token"].(string)}}
	rotated := server.tokenRequest(refresh, http.StatusOK)

	server.mcpAccess(tokens["access_token"].(string), http.StatusUnauthorized)
	server.mcpAccess(rotated["access_token"].(string), http.StatusOK)

	if response := user.call(http.MethodPost, "disconnect_agent", `{"grant_id":"`+list.Agents[0].GrantID+`"}`); response.Code != http.StatusOK {
		t.Fatalf("disconnect: %d %s", response.Code, response.Body)
	}

	server.mcpAccess(rotated["access_token"].(string), http.StatusUnauthorized)
	refresh.Set("refresh_token", rotated["refresh_token"].(string))
	server.tokenRequest(refresh, http.StatusBadRequest)
}

// A decision counts only with the form token this session got for exactly
// this request, from the app's own pages, by a user with their own password.
func TestConsentRejectsForgedDecisions(t *testing.T) {
	server := newRESTServer(t)
	params := server.authorizationParams()
	user := server.webUser(store.RoleUser, false)
	other := server.webUser(store.RoleUser, false)
	shown := decoded[authorization](t, user.consent(params), http.StatusOK)
	tampered := copyParams(params)
	tampered.Set("redirect_uri", "http://127.0.0.1:44891/other")

	extra := copyParams(params)
	extra.Set("scope", "everything")

	body, _ := json.Marshal(map[string]any{"query": params.Encode(), "form_token": shown.FormToken, "allow": true})
	crossSite := httptest.NewRequest(http.MethodPost, "/api/ui/authorization_decision", strings.NewReader(string(body)))
	crossSite.Header.Set("Content-Type", "application/json")
	crossSite.Header.Set("Origin", "http://evil.example")
	crossSite.AddCookie(sessionCookie(user))

	form := httptest.NewRequest(http.MethodPost, "/api/ui/authorization_decision", strings.NewReader(string(body)))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	form.Header.Set("Origin", uiOrigin)
	form.AddCookie(sessionCookie(user))

	for label, test := range map[string]struct {
		response *httptest.ResponseRecorder
		status   int
	}{
		"no token":          {user.decide(params, "", true), http.StatusForbidden},
		"made-up token":     {user.decide(params, strings.Repeat("0", 64), true), http.StatusForbidden},
		"tampered callback": {user.decide(tampered, shown.FormToken, true), http.StatusForbidden},
		"added parameter":   {user.decide(extra, shown.FormToken, true), http.StatusForbidden},
		"another session":   {other.decide(params, shown.FormToken, true), http.StatusForbidden},
		"no session":        {uiUser{server: server}.decide(params, shown.FormToken, true), http.StatusUnauthorized},
		"another site":      {serve(server, crossSite), http.StatusForbidden},
		"a form, not JSON":  {serve(server, form), http.StatusUnsupportedMediaType},
		"unknown field":     {user.call(http.MethodPost, "authorization_decision", `{"query":"","form_token":"","allow":true,"redirect":"x"}`), http.StatusBadRequest},
	} {
		if test.response.Code != test.status || strings.Contains(test.response.Body.String(), "code=") {
			t.Errorf("%s: %d %s", label, test.response.Code, test.response.Body)
		}
	}

	// A temporary password allows nothing, whatever the token.
	temporary := server.webUser(store.RoleAdmin, true)

	for label, response := range map[string]*httptest.ResponseRecorder{
		"page":     temporary.consent(params),
		"decision": temporary.decide(params, shown.FormToken, true),
	} {
		if response.Code != http.StatusNotFound {
			t.Errorf("temporary password, %s: %d %s", label, response.Code, response.Body)
		}
	}

	// Denied: back to the agent with the error and its state, no code.
	target := decoded[struct {
		RedirectURL string `json:"redirect_url"`
	}](t, user.decide(params, shown.FormToken, false), http.StatusOK)
	callback, err := url.Parse(target.RedirectURL)

	if err != nil || callback.Query().Get("error") != "access_denied" || callback.Query().Get("state") != params.Get("state") || callback.Query().Has("code") {
		t.Fatalf("deny: %s %v", target.RedirectURL, err)
	}
}

// A request that does not match the agent is refused with its reason, and
// the browser is never sent to its redirect_uri.
func TestConsentRejectsInvalidRequests(t *testing.T) {
	server := newRESTServer(t)
	params := server.authorizationParams()
	user := server.webUser(store.RoleUser, false)
	shown := decoded[authorization](t, user.consent(params), http.StatusOK)

	for _, test := range []struct{ name, key, value string }{
		{"unregistered client", "client_id", "missing"},
		{"unregistered callback", "redirect_uri", "http://127.0.0.1:44891/other"},
		{"fragment callback", "redirect_uri", "http://127.0.0.1:44891/callback#fragment"},
		{"missing PKCE", "code_challenge", ""},
		{"plain PKCE", "code_challenge_method", "plain"},
		{"wrong response type", "response_type", "token"},
		{"wrong resource", "resource", "https://other.example/mcp"},
	} {
		changed := copyParams(params)
		changed.Set(test.key, test.value)

		if response := user.consent(changed); response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "form_token") {
			t.Errorf("%s: %d %s", test.name, response.Code, response.Body)
		}
	}

	duplicated := copyParams(params)
	duplicated.Add("redirect_uri", "http://127.0.0.1:44891/callback")

	if response := user.consent(duplicated); response.Code != http.StatusBadRequest {
		t.Errorf("duplicate parameter: %d", response.Code)
	}

	// The token of a valid request opens no other.
	if response := user.decide(duplicated, shown.FormToken, true); response.Code != http.StatusForbidden {
		t.Errorf("decision on another request: %d", response.Code)
	}
}

func TestOAuthRejectsWrongPKCE(t *testing.T) {
	server := newRESTServer(t)
	params := server.authorizationParams()
	callback := server.webUser(store.RoleUser, false).allowed(params)

	result := server.tokenRequest(url.Values{"grant_type": {"authorization_code"}, "client_id": {params.Get("client_id")}, "redirect_uri": {params.Get("redirect_uri")}, "code": {callback.Query().Get("code")}, "code_verifier": {"wrong"}}, http.StatusBadRequest)

	if result["error"] != "invalid_grant" {
		t.Fatalf("wrong PKCE: %v", result)
	}
}
