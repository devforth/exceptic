package api_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
)

// fakeGoogle is Google's token endpoint: it checks the PKCE verifier against
// the challenge of the sign-in it was given and answers an ID token of account.
type fakeGoogle struct {
	*httptest.Server
	mutex     sync.Mutex
	challenge string
	account   map[string]any // the ID token's claims but nonce; nonce is the sign-in's unless set
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	google := &fakeGoogle{}
	google.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		google.mutex.Lock()
		defer google.mutex.Unlock()

		verifier := sha256.Sum256([]byte(r.FormValue("code_verifier")))
		clientID, secret, basic := r.BasicAuth()

		if r.URL.Path != "/token" || !basic || clientID != "client-1" || secret != "secret-1" || r.FormValue("client_secret") != "" || r.FormValue("code") != "the-code" || base64.RawURLEncoding.EncodeToString(verifier[:]) != google.challenge {
			http.Error(w, "refused", http.StatusBadRequest)

			return
		}

		payload, _ := json.Marshal(google.account)
		token := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": token})
	}))
	t.Cleanup(google.Close)

	return google
}

func withGoogle(google *fakeGoogle) func(settings *config.Config) {
	return func(settings *config.Config) {
		settings.Notifications.PublicURL = "http://localhost"
		settings.Google = config.Google{ClientID: "client-1", ClientSecret: "secret-1", AuthURL: google.URL + "/auth", TokenURL: google.URL + "/token"}
	}
}

// signInWithGoogle goes through the sign-in as a browser does, Google
// answering with account; it returns where the server sent the browser last.
func (client *browser) signInWithGoogle(google *fakeGoogle, account map[string]any, tamper func(query url.Values)) string {
	client.server.t.Helper()

	start := client.visit("/auth/google?next=%2Flogs")
	location, err := url.Parse(start.Header().Get("Location"))

	if err != nil || start.Code != http.StatusFound || !strings.HasPrefix(location.String(), google.URL+"/auth") {
		client.server.t.Fatalf("start: %d %s", start.Code, start.Header().Get("Location"))
	}

	query := location.Query()
	claims := map[string]any{"iss": "https://accounts.google.com", "aud": "client-1", "exp": time.Now().Add(time.Hour).Unix(), "nonce": query.Get("nonce")}

	for name, value := range account {
		claims[name] = value
	}

	google.mutex.Lock()
	google.challenge, google.account = query.Get("code_challenge"), claims
	google.mutex.Unlock()

	back := url.Values{"code": {"the-code"}, "state": {query.Get("state")}}

	if tamper != nil {
		tamper(back)
	}

	return client.visit(config.GoogleRedirectPath + "?" + back.Encode()).Header().Get("Location")
}

// visit opens an address of the server (not the web app's API) with the browser's cookies.
func (client *browser) visit(target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)

	for _, cookie := range client.cookies {
		request.AddCookie(cookie)
	}

	response := serve(client.server, request)

	for _, cookie := range response.Result().Cookies() {
		if cookie.MaxAge < 0 || cookie.Value == "" {
			delete(client.cookies, cookie.Name)
		} else {
			client.cookies[cookie.Name] = cookie
		}
	}

	return response
}

// activityAnswer is get_activity's answer.
type activityAnswer struct {
	Sessions []struct {
		ID      string `json:"id"`
		Method  string `json:"method"`
		Current bool   `json:"current"`
	} `json:"sessions"`
	SignIns []struct {
		Method string `json:"method"`
		Result string `json:"result"`
	} `json:"sign_ins"`
	GoogleLinkedAt *time.Time `json:"google_linked_at"`
}

// waitFor polls until ready is true, for what a goroutine records.
func waitFor(t *testing.T, what string, ready func() bool) {
	t.Helper()

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if ready() {
			return
		}
	}

	t.Fatalf("timed out waiting for %s", what)
}

func TestGoogleSignIn(t *testing.T) {
	google := newFakeGoogle(t)
	server := newLoginServer(t, withGoogle(google))
	admin := server.browser().loggedIn("admin", adminPassword)
	createWebUser(t, admin, "dev@example.com", store.RoleUser)

	var state api.LoginState

	if err := json.Unmarshal(server.browser().call(http.MethodGet, "login_state", nil).Body.Bytes(), &state); err != nil || !state.Google {
		t.Fatalf("login_state: %+v %v", state, err)
	}

	verified := map[string]any{"sub": "google-1", "email": "Dev@Example.com", "email_verified": true}

	// The first sign-in links the account by its verified email.
	first := server.browser()

	if next := first.signInWithGoogle(google, verified, nil); next != "/logs" || !first.open() {
		t.Fatalf("first sign-in went to %q, open %v", next, first.open())
	}

	// Later its subject finds the user, whatever email Google says (email_verified as a string).
	later := server.browser()

	if next := later.signInWithGoogle(google, map[string]any{"sub": "google-1", "email": "renamed@example.org", "email_verified": "true"}, nil); next != "/logs" || !later.open() {
		t.Fatalf("by subject went to %q", next)
	}

	for label, test := range map[string]struct {
		account map[string]any
		tamper  func(query url.Values)
		reason  string
	}{
		"unverified":        {map[string]any{"sub": "google-2", "email": "admin@example.com", "email_verified": false}, nil, "google_unverified"},
		"nobody":            {map[string]any{"sub": "google-3", "email": "nobody@example.com", "email_verified": true}, nil, "google_no_user"},
		"another account":   {map[string]any{"sub": "google-4", "email": "dev@example.com", "email_verified": true}, nil, "google_not_linked"},
		"another state":     {verified, func(query url.Values) { query.Set("state", "forged") }, "google_failed"},
		"another nonce":     {map[string]any{"sub": "google-1", "email": "dev@example.com", "email_verified": true, "nonce": "replayed"}, nil, "google_failed"},
		"another audience":  {map[string]any{"sub": "google-1", "email": "dev@example.com", "email_verified": true, "aud": "client-2"}, nil, "google_failed"},
		"expired":           {map[string]any{"sub": "google-1", "email": "dev@example.com", "email_verified": true, "exp": time.Now().Add(-time.Minute).Unix()}, nil, "google_failed"},
		"refused by Google": {verified, func(query url.Values) { query.Set("code", "another-code") }, "google_failed"},
	} {
		client := server.browser()

		if next := client.signInWithGoogle(google, test.account, test.tamper); next != "/login?error="+test.reason || client.open() {
			t.Errorf("%s: went to %q, open %v", label, next, client.open())
		}
	}

	// A callback without the cookie of a sign-in this browser started.
	if next := server.browser().visit(config.GoogleRedirectPath + "?code=the-code&state=x").Header().Get("Location"); next != "/login?error=google_failed" {
		t.Errorf("without the cookie: %q", next)
	}

	// The log of the user: the two sign-ins and the refused other account.
	var activity activityAnswer

	if err := json.Unmarshal(first.call(http.MethodGet, "get_activity", nil).Body.Bytes(), &activity); err != nil {
		t.Fatal(err)
	}

	results := map[string]int{}

	for _, signIn := range activity.SignIns {
		results[signIn.Method+" "+signIn.Result]++
	}

	if results["google ok"] < 2 || activity.GoogleLinkedAt == nil {
		t.Errorf("log %v, linked at %v", results, activity.GoogleLinkedAt)
	}

	waitFor(t, "the refused account in the log", func() bool {
		var again activityAnswer

		_ = json.Unmarshal(first.call(http.MethodGet, "get_activity", nil).Body.Bytes(), &again)

		for _, signIn := range again.SignIns {
			if signIn.Result == store.SignInNotLinked {
				return true
			}
		}

		return false
	})
}

// An unlink disconnects the Google sessions and keeps the account from
// linking again by email until the user signs in with the password.
func TestGoogleUnlink(t *testing.T) {
	google := newFakeGoogle(t)
	server := newLoginServer(t, withGoogle(google))
	admin := server.browser().loggedIn("admin", adminPassword)
	userID, _ := createWebUser(t, admin, "dev@example.com", store.RoleUser)
	verified := map[string]any{"sub": "google-1", "email": "dev@example.com", "email_verified": true}
	signedIn := server.browser()

	if next := signedIn.signInWithGoogle(google, verified, nil); next != "/logs" {
		t.Fatalf("sign-in went to %q", next)
	}

	if response := admin.call(http.MethodPost, "unlink_google", map[string]string{"user_id": userID}); response.Code/100 != 2 {
		t.Fatalf("unlink: %d %s", response.Code, response.Body)
	}

	if signedIn.open() {
		t.Error("the Google session outlived the unlink")
	}

	if next := server.browser().signInWithGoogle(google, verified, nil); next != "/login?error=google_unlinked" {
		t.Errorf("before a password sign-in: %q", next)
	}

	server.browser().loggedIn("dev@example.com", "user password 123")

	if next := server.browser().signInWithGoogle(google, verified, nil); next != "/logs" {
		t.Errorf("after a password sign-in: %q", next)
	}
}

func TestGoogleSignInOff(t *testing.T) {
	server := newLoginServer(t)

	if code := server.browser().visit("/auth/google").Code; code == http.StatusFound {
		t.Errorf("without GOOGLE_CLIENT_ID the sign-in starts: %d", code)
	}
}

// Sessions and the sign-in log: one's own on the account page, a user's for
// those above them; ending a session by take-over, never one's own current.
func TestSignInActivity(t *testing.T) {
	server := newLoginServer(t)
	admin := server.browser().loggedIn("admin", adminPassword)
	userID, user := createWebUser(t, admin, "dev@example.com", store.RoleUser)

	// Ten wrong passwords, then blocked ones: the log records the block once.
	guesser := server.browser()

	for range 15 {
		guesser.login("dev@example.com", "a wrong password")
	}

	activityOf := func(client *browser, target string) (activityAnswer, int) {
		response := client.call(http.MethodGet, "get_activity"+target, nil)

		var activity activityAnswer

		_ = json.Unmarshal(response.Body.Bytes(), &activity)

		return activity, response.Code
	}

	refusals := func() map[string]int {
		activity, _ := activityOf(admin, "?user_id="+userID)
		results := map[string]int{}

		for _, signIn := range activity.SignIns {
			results[signIn.Result]++
		}

		return results
	}

	waitFor(t, "the refusals in the log", func() bool {
		results := refusals()

		return results[store.SignInWrongPassword] == session.MaxLoginFailures && results[store.SignInBlocked] > 0
	})

	if results := refusals(); results[store.SignInBlocked] != 1 {
		t.Errorf("blocked attempts recorded %d times, want once", results[store.SignInBlocked])
	}

	own, code := activityOf(user, "")

	if code != http.StatusOK || len(own.Sessions) != 1 || !own.Sessions[0].Current || own.Sessions[0].Method != store.SignInPassword {
		t.Fatalf("own activity %d: %+v", code, own)
	}

	if response := user.call(http.MethodPost, "end_session", map[string]string{"session_id": own.Sessions[0].ID}); response.Code != http.StatusForbidden {
		t.Errorf("ending one's current session: %d", response.Code)
	}

	adminID := server.adminID(t)

	if _, code := activityOf(user, "?user_id="+adminID); code != http.StatusForbidden {
		t.Errorf("a user reading an admin's activity: %d", code)
	}

	if response := admin.call(http.MethodPost, "end_session", map[string]string{"user_id": userID, "session_id": own.Sessions[0].ID}); response.Code != http.StatusOK {
		t.Fatalf("the admin ending the user's session: %d %s", response.Code, response.Body)
	}

	if user.open() {
		t.Error("the ended session still opens the web app")
	}
}

// adminID is the id of the admin the server was started with.
func (server *restServer) adminID(t *testing.T) string {
	t.Helper()

	admins, err := server.database.Admins(t.Context())

	if err != nil || len(admins) == 0 {
		t.Fatal(err, admins)
	}

	return admins[0].ID
}
