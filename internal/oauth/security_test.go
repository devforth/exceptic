package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/storetest"

	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/store"
)

// testUserID(t, server) is the user testAuthorization creates.
func testUserID(t *testing.T, authorization *Server) string {
	t.Helper()

	user, err := authorization.Store.Superadmin(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	return user.ID
}

func testAuthorization(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	database := storetest.Open(t)

	// The user agents act for (users are created by the UI).
	if _, err := database.CreateUser(context.Background(), store.User{Username: "admin", Role: store.RoleSuperadmin, CreatedAt: time.Now()}, "x"); err != nil {
		t.Fatal(err)
	}

	authorization := &Server{Store: database, PublicURL: "https://errors.example.com", ResourcePath: "/mcp", Now: func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }}
	router := http.NewServeMux()
	authorization.Register(router, func(handler http.Handler) http.Handler { return handler })
	router.Handle("/mcp", authorization.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })))

	return authorization, router
}

func oauthRequest(handler http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://errors.example.com"+path, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:1234"

	for index := 0; index+1 < len(headers); index += 2 {
		request.Header.Add(headers[index], headers[index+1])
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}

func testClient(t *testing.T, authorization *Server) store.OAuthClient {
	t.Helper()

	client := store.OAuthClient{ID: "test-client", Name: "Test client", RedirectURIs: []string{"http://127.0.0.1:44891/callback"}, CreatedAt: authorization.now()}

	if err := authorization.Store.RegisterOAuthClient(context.Background(), client); err != nil {
		t.Fatal(err)
	}

	return client
}

const testVerifier = "01234567890123456789012345678901234567890123456789"

func testCode(t *testing.T, authorization *Server, client store.OAuthClient) url.Values {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "https://errors.example.com/oauth/authorize", nil)
	params := url.Values{"client_id": {client.ID}, "redirect_uri": {client.RedirectURIs[0]}, "response_type": {"code"}, "code_challenge_method": {"S256"}, "code_challenge": {pkceChallenge(testVerifier)}, "resource": {"https://errors.example.com/mcp"}}
	checked, err := authorization.CheckRequest(context.Background(), request, params)

	if err != nil {
		t.Fatal(err)
	}

	callback, err := authorization.Approve(context.Background(), request, checked, testUserID(t, authorization))

	if err != nil {
		t.Fatal(err)
	}

	target, err := url.Parse(callback)

	if err != nil {
		t.Fatal(err)
	}

	return url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ID}, "redirect_uri": {client.RedirectURIs[0]}, "code": {target.Query().Get("code")}, "code_verifier": {testVerifier}}
}

func exchange(t *testing.T, handler http.Handler, form url.Values, status int) map[string]any {
	t.Helper()

	response := oauthRequest(handler, http.MethodPost, TokenPath, form.Encode(), "Content-Type", "application/x-www-form-urlencoded")

	if response.Code != status {
		t.Fatalf("token endpoint: %d %s", response.Code, response.Body)
	}

	var result map[string]any

	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	return result
}

func TestOriginProtection(t *testing.T) {
	authorization, handler := testAuthorization(t)
	token, err := authorization.NewPersonalToken(context.Background(), testUserID(t, authorization))

	if err != nil {
		t.Fatal(err)
	}

	for _, origin := range []string{"https://evil.example", "null", "http://errors.example.com", "https://user@errors.example.com", "https://errors.example.com/path", "https://errors.example.com?query=1"} {
		for _, path := range []string{"/mcp", RegisterPath, TokenPath, "/.well-known/oauth-protected-resource/mcp"} {
			method := http.MethodPost

			if strings.HasPrefix(path, "/.well-known/") {
				method = http.MethodGet
			}

			response := oauthRequest(handler, method, path, "", "Origin", origin, "Authorization", "Bearer "+token)

			if response.Code != http.StatusForbidden {
				t.Errorf("%s origin %q: %d", path, origin, response.Code)
			}
		}

		response := oauthRequest(handler, http.MethodGet, "/mcp", "", "Origin", origin, "Authorization", "Bearer "+token)

		if response.Code != http.StatusForbidden {
			t.Errorf("GET origin %q: %d", origin, response.Code)
		}
	}

	for _, headers := range [][]string{
		{"Authorization", "Bearer " + token},
		{"Authorization", "bearer " + token, "Origin", "https://errors.example.com"},
		{"Authorization", "Bearer " + token, "Origin", "https://ERRORS.example.com:443"},
	} {
		if response := oauthRequest(handler, http.MethodPost, "/mcp", "", headers...); response.Code != http.StatusNoContent {
			t.Errorf("valid origin: %d %s", response.Code, response.Body)
		}
	}

	response := oauthRequest(handler, http.MethodPost, "/mcp", "", "Origin", "https://errors.example.com", "Origin", "https://evil.example", "Authorization", "Bearer "+token)

	if response.Code != http.StatusForbidden {
		t.Fatal("multiple Origins accepted")
	}

	response = oauthRequest(handler, http.MethodPost, "/mcp", "", "Sec-Fetch-Site", "cross-site", "Authorization", "Bearer "+token)

	if response.Code != http.StatusForbidden {
		t.Fatal("cross-site fetch without Origin accepted")
	}

	response = oauthRequest(handler, http.MethodPost, "/mcp", "", "Authorization", "Bearer "+token, "Authorization", "Bearer "+token)

	if response.Code != http.StatusUnauthorized {
		t.Fatal("multiple credentials accepted")
	}
}

func TestOAuthRedirectValidation(t *testing.T) {
	for _, uri := range []string{
		"https://client.example/callback", "http://localhost:3000/callback", "http://127.0.0.1:44891/callback", "http://[::1]:3000/callback",
	} {
		if err := checkRedirectURI(uri); err != nil {
			t.Errorf("valid URI %q: %v", uri, err)
		}
	}

	for _, uri := range []string{
		"http://client.example/callback", "https:/callback", "https://user:password@client.example/callback",
		"https://client.example/callback#fragment", "https://client.example/callback#", "javascript:alert(1)", "data:text/html,test", "intent://callback", "file:///tmp/callback", "vscode://callback",
		"https://client.example:99999/callback", "https://client.example/callback\\other", "https://client.example;form-action/callback",
	} {
		if err := checkRedirectURI(uri); err == nil {
			t.Errorf("unsafe URI accepted: %q", uri)
		}
	}

	if !redirectMatches("http://127.0.0.1:2222/callback", "http://127.0.0.1:1111/callback") {
		t.Fatal("loopback port variation rejected")
	}

	for _, requested := range []string{"http://127.0.0.1:2222/callback#fragment", "http://127.0.0.1:2222/other", "http://localhost:1111/callback", "http://127.0.0.1:2222/callback?other=1"} {
		if redirectMatches(requested, "http://127.0.0.1:1111/callback") {
			t.Errorf("callback mismatch accepted: %s", requested)
		}
	}
}

func TestOAuthRejectsMalformedRegistration(t *testing.T) {
	_, handler := testAuthorization(t)

	for _, body := range []string{
		`{"redirect_uris":["https://client.example/callback"]} {}`,
		`{"redirect_uris":["https://client.example/callback"]}` + strings.Repeat(" ", MaxBody),
		`{"redirect_uris":["intent://callback"]}`,
	} {
		response := oauthRequest(handler, http.MethodPost, RegisterPath, body, "Content-Type", "application/json")

		if response.Code != http.StatusBadRequest {
			t.Errorf("malformed registration: %d %s", response.Code, response.Body)
		}
	}
}

func TestOAuthRateLimits(t *testing.T) {
	_, handler := testAuthorization(t)

	for index := 0; index < 31; index++ {
		response := oauthRequest(handler, http.MethodPost, RegisterPath, `{}`, "Content-Type", "application/json")

		if index == 30 && (response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") == "") {
			t.Fatalf("registration flood not limited: %d", response.Code)
		}
	}

	for index := 0; index < 121; index++ {
		response := oauthRequest(handler, http.MethodPost, TokenPath, "grant_type=unsupported", "Content-Type", "application/x-www-form-urlencoded")

		if index == 120 && response.Code != http.StatusTooManyRequests {
			t.Fatalf("token flood not limited: %d", response.Code)
		}
	}

	limiter := requestLimit{perMinute: 1}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	address := netip.MustParseAddr("127.0.0.1")

	if !limiter.allow(address, now) || limiter.allow(address, now) || !limiter.allow(address, now.Add(time.Minute)) {
		t.Fatal("rate window did not reset")
	}
}

// A few clients cannot lock every other agent out: one IPv6 host counts as
// its /64, and networks past the cap share one overflow count, so a flood
// from many networks is limited too without refusing the networks counted.
func TestOAuthRateLimitPerNetwork(t *testing.T) {
	limiter := requestLimit{perMinute: 120}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	agent := netip.MustParseAddr("198.51.100.1")

	for index := range 256 {
		limiter.allow(netip.MustParseAddr(fmt.Sprintf("2001:db8:1:2::%x", index+1)), now)
	}

	if limiter.allow(netip.MustParseAddr("2001:db8:1:2::ffff"), now) {
		t.Fatal("a new address in the same /64 is not limited")
	}

	if !limiter.allow(agent, now) {
		t.Fatal("other clients locked an agent out")
	}

	for index := range limitNetworks - len(limiter.counts) {
		limiter.allow(netip.MustParseAddr(fmt.Sprintf("2001:db8:2:%x::1", index)), now)
	}

	allowed := 0

	for index := range limitNetworks {
		if limiter.allow(netip.MustParseAddr(fmt.Sprintf("2001:db8:3:%x::1", index)), now) {
			allowed++
		}
	}

	if allowed != limitOverflow*limiter.perMinute {
		t.Fatalf("networks past the cap: %d requests allowed, want %d", allowed, limitOverflow*limiter.perMinute)
	}

	if !limiter.allow(agent, now) {
		t.Fatal("a flood of networks locked a counted agent out")
	}
}

// Registrations from all networks together stop at their total.
func TestOAuthRegistrationTotal(t *testing.T) {
	limiter := requestLimit{perMinute: registrationsPerMinute, total: registrationsTotal}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	allowed := 0

	for index := range 2 * registrationsTotal {
		if limiter.allow(netip.MustParseAddr(fmt.Sprintf("203.0.113.%d", index)), now) {
			allowed++
		}
	}

	if allowed != registrationsTotal {
		t.Fatalf("%d registrations allowed, want %d", allowed, registrationsTotal)
	}

	if !limiter.allow(netip.MustParseAddr("203.0.113.1"), now.Add(time.Minute)) {
		t.Fatal("the total did not reset")
	}
}

func TestPKCEAndDuplicateParameters(t *testing.T) {
	authorization, handler := testAuthorization(t)
	client := testClient(t, authorization)
	form := testCode(t, authorization, client)
	form.Add("client_id", "other-client")
	exchange(t, handler, form, http.StatusBadRequest)
	form.Set("client_id", client.ID)
	form.Set("resource", "https://other.example/mcp")
	exchange(t, handler, form, http.StatusBadRequest)
	form.Del("resource")
	exchange(t, handler, form, http.StatusOK)

	for _, verifier := range []string{"short", strings.Repeat("x", 129), strings.Repeat("x", 42) + "!"} {
		if validVerifier(verifier) {
			t.Errorf("invalid verifier accepted: %q", verifier)
		}
	}

	params := url.Values{"client_id": {client.ID}, "redirect_uri": {client.RedirectURIs[0]}, "response_type": {"code"}, "code_challenge_method": {"S256"}}

	for _, challenge := range []string{"", "short", base64.RawURLEncoding.EncodeToString(make([]byte, 31)), strings.Repeat("a", 43) + "="} {
		params.Set("code_challenge", challenge)

		if _, err := authorization.CheckRequest(context.Background(), httptest.NewRequest(http.MethodGet, "https://errors.example.com", nil), params); err == nil {
			t.Errorf("invalid challenge accepted: %q", challenge)
		}
	}
}

func TestRefreshReplayRevokesGrant(t *testing.T) {
	authorization, handler := testAuthorization(t)
	client := testClient(t, authorization)
	tokens := exchange(t, handler, testCode(t, authorization, client), http.StatusOK)
	refresh := url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ID}, "refresh_token": {tokens["refresh_token"].(string)}}
	rotated := exchange(t, handler, refresh, http.StatusOK)
	// Wrong client and random tokens must not revoke a legitimate grant.
	refresh.Set("client_id", "other-client")
	exchange(t, handler, refresh, http.StatusBadRequest)

	if _, err := authorization.Authenticate(context.Background(), rotated["access_token"].(string)); err != nil {
		t.Fatal("wrong client revoked grant", err)
	}

	refresh.Set("client_id", client.ID)
	refresh.Set("refresh_token", "emcp_rt_made-up")
	exchange(t, handler, refresh, http.StatusBadRequest)

	if _, err := authorization.Authenticate(context.Background(), rotated["access_token"].(string)); err != nil {
		t.Fatal("made-up token revoked grant", err)
	}

	pending := testCode(t, authorization, client)

	refresh.Set("refresh_token", tokens["refresh_token"].(string))
	exchange(t, handler, refresh, http.StatusBadRequest)

	if _, err := authorization.Authenticate(context.Background(), rotated["access_token"].(string)); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("replayed refresh did not revoke access token", err)
	}

	refresh.Set("refresh_token", rotated["refresh_token"].(string))
	exchange(t, handler, refresh, http.StatusBadRequest)
	exchange(t, handler, pending, http.StatusBadRequest)
}

func TestDisconnectInvalidatesPendingCodes(t *testing.T) {
	authorization, handler := testAuthorization(t)
	client := testClient(t, authorization)
	tokens := exchange(t, handler, testCode(t, authorization, client), http.StatusOK)
	pending := testCode(t, authorization, client)
	grant, err := authorization.Authenticate(context.Background(), tokens["access_token"].(string))

	if err != nil {
		t.Fatal(err)
	}

	if err := authorization.Store.DeleteGrant(context.Background(), grant.ID); err != nil {
		t.Fatal(err)
	}

	exchange(t, handler, pending, http.StatusBadRequest)
}

func TestOAuthExpiry(t *testing.T) {
	authorization, handler := testAuthorization(t)
	client := testClient(t, authorization)
	code := testCode(t, authorization, client)
	now := authorization.now()
	authorization.Now = func() time.Time { return now.Add(codeTTL) }

	exchange(t, handler, code, http.StatusBadRequest)
	tokens := exchange(t, handler, testCode(t, authorization, client), http.StatusOK)
	issued := authorization.now()
	authorization.Now = func() time.Time { return issued.Add(accessTokenTTL) }

	if _, err := authorization.Authenticate(context.Background(), tokens["access_token"].(string)); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("expired access token accepted", err)
	}

	authorization.Now = func() time.Time { return issued.Add(refreshTokenTTL) }

	exchange(t, handler, url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ID}, "refresh_token": {tokens["refresh_token"].(string)}}, http.StatusBadRequest)
}

func TestOAuthEndpointsRespectIPAllowlist(t *testing.T) {
	authorization, _ := testAuthorization(t)
	router := http.NewServeMux()
	authorization.Register(router, func(next http.Handler) http.Handler {
		return server.RequireAllowedIP(slog.New(slog.DiscardHandler), []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, nil, next)
	})

	for _, path := range []string{RegisterPath, TokenPath, "/.well-known/oauth-protected-resource/mcp"} {
		method := http.MethodPost

		if strings.HasPrefix(path, "/.well-known/") {
			method = http.MethodGet
		}

		response := oauthRequest(router, method, path, "", "X-Forwarded-For", "10.0.0.1")

		if response.Code != http.StatusForbidden {
			t.Errorf("IP allowlist bypassed at %s: %d", path, response.Code)
		}
	}
}

func TestConcurrentTokenExchanges(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "authorization code", true: "refresh token"}[refresh], func(t *testing.T) {
			authorization, handler := testAuthorization(t)
			client := testClient(t, authorization)
			form := testCode(t, authorization, client)

			if refresh {
				tokens := exchange(t, handler, form, http.StatusOK)
				form = url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ID}, "refresh_token": {tokens["refresh_token"].(string)}}
			}

			results := make(chan int, 2)

			for range 2 {
				go func() {
					response := oauthRequest(handler, http.MethodPost, TokenPath, form.Encode(), "Content-Type", "application/x-www-form-urlencoded")

					results <- response.Code
				}()
			}

			first, second := <-results, <-results

			if first+second != http.StatusOK+http.StatusBadRequest || first == second {
				t.Fatalf("concurrent exchange: %d, %d", first, second)
			}

			if refresh {
				grants, err := authorization.Store.ListGrants(context.Background(), testUserID(t, authorization), authorization.now())

				if err != nil || len(grants) != 0 {
					t.Fatalf("concurrent replay did not revoke grant: %v %+v", err, grants)
				}
			}
		})
	}
}

func TestOriginBehindTrustedProxy(t *testing.T) {
	authorization, _ := testAuthorization(t)
	authorization.PublicURL = ""
	authorization.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	request := httptest.NewRequest(http.MethodPost, "http://errors.example.com/mcp", nil)
	request.RemoteAddr = "10.0.0.1:1234"
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("Origin", "https://errors.example.com")

	if !authorization.sameOrigin(request) {
		t.Fatal("trusted HTTPS proxy rejected")
	}

	request.RemoteAddr = "192.0.2.1:1234"

	if authorization.sameOrigin(request) {
		t.Fatal("untrusted proxy header accepted")
	}

	authorization.PublicURL = "https://errors.example.com/monitoring"
	request.Host = "internal-proxy:8000"

	if !authorization.sameOrigin(request) {
		t.Fatal("configured public origin rejected behind proxy")
	}
}

// failingCodes is a store whose codes cannot be read.
type failingCodes struct{ store.Store }

func (failingCodes) TakeOAuthCode(context.Context, string, time.Time) (store.OAuthCode, error) {
	return store.OAuthCode{}, errors.New("disk failed")
}

// A failure of the server is logged; the agent gets only "internal error".
func TestServerErrorsAreLogged(t *testing.T) {
	authorization, handler := testAuthorization(t)
	form := testCode(t, authorization, testClient(t, authorization))

	var logs strings.Builder

	authorization.Store = failingCodes{authorization.Store}
	authorization.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	response := oauthRequest(handler, http.MethodPost, TokenPath, form.Encode(), "Content-Type", "application/x-www-form-urlencoded")

	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "disk failed") {
		t.Errorf("token endpoint: %d %s", response.Code, response.Body)
	}

	if !strings.Contains(logs.String(), "disk failed") {
		t.Errorf("the error was not logged: %q", logs.String())
	}
}

// Plain HTTP is unencrypted unless agents reach the server on this machine.
func TestUnencrypted(t *testing.T) {
	for address, want := range map[string]bool{
		"https://errors.example.com": false, "http://errors.example.com": true, "http://192.0.2.1:8000": true,
		"http://localhost:8000": false, "http://127.0.0.1:8000": false, "http://[::1]:8000": false, "http://[::ffff:127.0.0.1]": false,
	} {
		if got := Unencrypted(mustParseURL(t, address)); got != want {
			t.Errorf("%s: %v", address, got)
		}
	}

	authorization := &Server{ResourcePath: "/mcp"}

	if got := authorization.ResourceAt(mustParseURL(t, "https://errors.example.com/monitoring")); got != "https://errors.example.com/monitoring/mcp" {
		t.Errorf("resource below a path prefix: %s", got)
	}
}

func mustParseURL(t *testing.T, address string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(address)

	if err != nil {
		t.Fatal(err)
	}

	return parsed
}
