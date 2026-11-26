package session

import (
	"cmp"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The addresses of Google's sign-in; tests point them at a fake.
const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

// googleCookie keeps a started sign-in (state, nonce, PKCE verifier, where
// to go after), signed, for googleStateLife, on the callback's path only.
const (
	googleCookie    = "exceptic_google"
	googleStateLife = 10 * time.Minute
)

// Reasons a Google sign-in is refused, shown on the login page (?error=).
const (
	GoogleFailed     = "google_failed"     // the state, the code or the token did not check out
	GoogleUnverified = "google_unverified" // Google has not verified the account's email
	GoogleNoUser     = "google_no_user"    // no user has this account or its email
	GoogleNotLinked  = "google_not_linked" // the user of the email has another Google account linked
	GoogleUnlinked   = "google_unlinked"   // Google was unlinked from the user of the email; the password first
)

// googleAccounts finds the user of a Google account: services.Users.
type googleAccounts interface {
	GoogleSignIn(ctx context.Context, sub, email string) (store.User, string, error)
}

type googleSignIn struct {
	settings    config.Google
	redirectURI string
	authURL     string
	tokenURL    string
	client      *http.Client
	accounts    googleAccounts
}

func newGoogleSignIn(settings *config.Config, accounts googleAccounts) *googleSignIn {
	if !settings.Google.Enabled() {
		return nil
	}

	return &googleSignIn{
		settings: settings.Google, redirectURI: strings.TrimRight(settings.Notifications.PublicURL, "/") + config.GoogleRedirectPath,
		authURL: cmp.Or(settings.Google.AuthURL, googleAuthURL), tokenURL: cmp.Or(settings.Google.TokenURL, googleTokenURL), client: &http.Client{
			Timeout: 10 * time.Second,
			// The token endpoint answers itself; a redirect would carry the code elsewhere.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}, accounts: accounts,
	}
}

// Register adds the Google sign-in's addresses (none without GOOGLE_CLIENT_ID).
func (auth *Auth) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	if auth.google == nil {
		return
	}

	router.Handle("GET /auth/google", protect(http.HandlerFunc(auth.GoogleStart)))
	router.Handle("GET "+config.GoogleRedirectPath, protect(http.HandlerFunc(auth.GoogleCallback)))
}

// GoogleEnabled: users may sign in with Google (GOOGLE_CLIENT_ID).
func (auth *Auth) GoogleEnabled() bool { return auth.google != nil }

// googleState is what the cookie of a started sign-in holds.
type googleState struct {
	State    string    `json:"state"`
	Nonce    string    `json:"nonce"`
	Verifier string    `json:"verifier"`
	Next     string    `json:"next"`
	Expires  time.Time `json:"expires"`
}

// GoogleStart sends the browser to Google's sign-in page, with a state, a
// nonce and a PKCE challenge whose secrets stay in a signed cookie.
func (auth *Auth) GoogleStart(w http.ResponseWriter, r *http.Request) {
	if auth.google == nil {
		http.NotFound(w, r)

		return
	}

	state := googleState{
		State: randomText(), Nonce: randomText(), Verifier: randomText(),
		Next: server.SafeNext(r.URL.Query().Get("next")), Expires: auth.now().Add(googleStateLife),
	}
	auth.setGoogleCookie(w, r, auth.signGoogleState(state), state.Expires)

	challenge := sha256.Sum256([]byte(state.Verifier))
	query := url.Values{
		"client_id": {auth.google.settings.ClientID}, "redirect_uri": {auth.google.redirectURI},
		"response_type": {"code"}, "scope": {"openid email"}, "prompt": {"select_account"},
		"state": {state.State}, "nonce": {state.Nonce},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}

	http.Redirect(w, r, auth.google.authURL+"?"+query.Encode(), http.StatusFound)
}

// GoogleCallback finishes a sign-in Google sent the browser back from: the
// state checked against the cookie of this browser, the code exchanged with
// the PKCE verifier, the ID token's claims checked, then a session of the
// account's user; a refusal goes back to the login page with its reason.
func (auth *Auth) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if auth.google == nil {
		http.NotFound(w, r)

		return
	}

	refuse := func(reason string) {
		http.Redirect(w, r, "/login?"+url.Values{"error": {reason}}.Encode(), http.StatusFound)
	}

	state, ok := auth.readGoogleCookie(r)
	auth.setGoogleCookie(w, r, "", time.Unix(0, 0))
	query := r.URL.Query()

	if !ok || query.Get("error") != "" || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state.State)) != 1 {
		refuse(GoogleFailed)

		return
	}

	claims, err := auth.google.exchange(r.Context(), query.Get("code"), state.Verifier)

	if err != nil {
		auth.logger.Warn("Google sign-in refused", "err", err)
		refuse(GoogleFailed)

		return
	}

	now := auth.now()

	if err := claims.check(auth.google.settings.ClientID, state.Nonce, now); err != nil {
		auth.logger.Warn("Google sign-in refused", "err", err)
		refuse(GoogleFailed)

		return
	}

	if !claims.verified() {
		refuse(GoogleUnverified)

		return
	}

	user, result, err := auth.google.accounts.GoogleSignIn(r.Context(), claims.Subject, claims.Email)

	switch {
	case errors.Is(err, store.ErrNotFound):
		refuse(GoogleNoUser)

		return
	case err != nil:
		auth.logger.Error("Google sign-in", "err", err)
		refuse(GoogleFailed)

		return
	case result != store.SignInOK:
		auth.refused(r, user.Email, store.SignInGoogle, result, now)

		if result == store.SignInUnlinked {
			refuse(GoogleUnlinked)
		} else {
			refuse(GoogleNotLinked)
		}

		return
	}

	auth.setDeviceCookie(w, r, user.Email, now)

	if err := auth.startSession(r.Context(), w, r, user.ID, store.SignInGoogle, now); err != nil {
		auth.logger.Error("start a session", "err", err)
		refuse(GoogleFailed)

		return
	}

	http.Redirect(w, r, state.Next, http.StatusFound)
}

// googleClaims are the ID token's claims a sign-in reads.
type googleClaims struct {
	Issuer   string          `json:"iss"`
	Audience string          `json:"aud"`
	Expires  int64           `json:"exp"`
	Nonce    string          `json:"nonce"`
	Subject  string          `json:"sub"`
	Email    string          `json:"email"`
	Verified json.RawMessage `json:"email_verified"`
}

// check: the token is Google's, for this client, not expired, and of this sign-in.
func (claims googleClaims) check(clientID, nonce string, now time.Time) error {
	switch {
	case claims.Issuer != "https://accounts.google.com" && claims.Issuer != "accounts.google.com":
		return fmt.Errorf("issuer %q", claims.Issuer)
	case claims.Audience != clientID:
		return errors.New("the token is for another client")
	case now.Unix() >= claims.Expires:
		return errors.New("the token has expired")
	case subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1:
		return errors.New("the nonce does not match")
	case claims.Subject == "" || claims.Email == "":
		return errors.New("no subject or email")
	}

	return nil
}

// verified: Google has verified the email (sent as true or "true").
func (claims googleClaims) verified() bool {
	value := strings.Trim(string(claims.Verified), `"`)

	return value == "true"
}

// exchange trades the code for the ID token at Google's token endpoint. The
// token comes straight from Google over TLS, so its claims are read without
// checking its signature (OpenID Connect Core 3.1.3.7), and checked by check.
func (google *googleSignIn) exchange(ctx context.Context, code, verifier string) (googleClaims, error) {
	form := url.Values{
		"code": {code}, "redirect_uri": {google.redirectURI}, "grant_type": {"authorization_code"}, "code_verifier": {verifier},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, google.tokenURL, strings.NewReader(form.Encode()))

	if err != nil {
		return googleClaims{}, err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// client_secret_basic, as RFC 6749 2.3.1 recommends: both form-encoded first.
	request.SetBasicAuth(url.QueryEscape(google.settings.ClientID), url.QueryEscape(google.settings.ClientSecret))
	response, err := google.client.Do(request)

	if err != nil {
		return googleClaims{}, fmt.Errorf("token request: %w", err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))

	if err != nil {
		return googleClaims{}, err
	}

	if response.StatusCode != http.StatusOK {
		return googleClaims{}, fmt.Errorf("token request answered %d", response.StatusCode)
	}

	var answer struct {
		IDToken string `json:"id_token"`
	}

	if err := json.Unmarshal(body, &answer); err != nil {
		return googleClaims{}, err
	}

	parts := strings.Split(answer.IDToken, ".")

	if len(parts) != 3 {
		return googleClaims{}, errors.New("no ID token")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])

	if err != nil {
		return googleClaims{}, err
	}

	var claims googleClaims

	return claims, json.Unmarshal(payload, &claims)
}

func (auth *Auth) signGoogleState(state googleState) string {
	payload, _ := json.Marshal(state) //nolint:errchkjson // plain strings and a time
	encoded := base64.RawURLEncoding.EncodeToString(payload)

	return encoded + "." + auth.googleTag(encoded)
}

func (auth *Auth) readGoogleCookie(r *http.Request) (googleState, bool) {
	cookie, err := r.Cookie(googleCookie)

	if err != nil {
		return googleState{}, false
	}

	encoded, tag, found := strings.Cut(cookie.Value, ".")

	if !found || !hmac.Equal([]byte(tag), []byte(auth.googleTag(encoded))) {
		return googleState{}, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)

	var state googleState

	if err != nil || json.Unmarshal(payload, &state) != nil || !auth.now().Before(state.Expires) {
		return googleState{}, false
	}

	return state, true
}

func (auth *Auth) googleTag(encoded string) string {
	mac := hmac.New(sha256.New, auth.deviceKey)
	mac.Write([]byte("google state\x00" + encoded))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (auth *Auth) setGoogleCookie(w http.ResponseWriter, r *http.Request, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: googleCookie, Value: value, Path: "/auth/google", Expires: expires, HttpOnly: true,
		Secure: server.IsHTTPS(r, auth.trustedProxies),
		// Lax: Google sends the browser back with a top-level GET, which carries it.
		SameSite: http.SameSiteLaxMode,
	})
}

// randomText is 32 random bytes, base64url: a state, a nonce, a PKCE verifier.
func randomText() string {
	value := make([]byte, 32)
	_, _ = rand.Read(value)

	return base64.RawURLEncoding.EncodeToString(value)
}
