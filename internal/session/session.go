// Package session is the login of the web app: session cookies, the limit
// on login attempts and the form token of the OAuth consent page.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

const (
	// CookieName holds the session token; the database keeps its hash.
	CookieName = "exceptic_session"
	// deviceCookie marks a browser the user logged in from (loginLimiter).
	deviceCookie   = "exceptic_device"
	deviceLifetime = 400 * 24 * time.Hour // the longest browsers keep a cookie

	// MaxLoginFailures attempts are allowed per client network, per username
	// and per known device within LoginWindow.
	MaxLoginFailures = 10
	LoginWindow      = 15 * time.Minute

	// Known devices get this many attempts at one username from all networks
	// together, so a copied device cookie is no way around the limit.
	maxKnownDeviceFailures = 3 * MaxLoginFailures
)

func newToken() string {
	var token [32]byte

	_, _ = rand.Read(token[:])

	return hex.EncodeToString(token[:])
}

// loginLimiter counts login attempts in fixed windows. An attempt is counted
// before its password is checked, under one lock, so concurrent attempts
// cannot pass it. A browser the user logged in from (a known device) is
// counted per network and username, and per username up to
// maxKnownDeviceFailures, so strangers' guesses cannot lock its owner out;
// any other attempt counts per network and per username, so guessing from
// many networks stays at MaxLoginFailures.
type loginLimiter struct {
	mutex           sync.Mutex
	logger          *slog.Logger
	byAddress       loginCounter
	byUsername      loginCounter
	byDevice        loginCounter // a network and a username, for known devices
	byKnownUsername loginCounter // known-device attempts at a username
	swept           time.Time
	fullWarned      time.Time
}

type loginWindowState struct {
	start time.Time
	count int
}

type loginCounter map[string]*loginWindowState

// maxLimiterKeys bounds the memory a flood of made-up addresses or usernames
// can take: past it a new one is refused until its window ends. Known
// devices are counted apart, so their owners still log in.
const maxLimiterKeys = 10_000

func newLoginLimiter(logger *slog.Logger) *loginLimiter {
	return &loginLimiter{logger: logger, byAddress: loginCounter{}, byUsername: loginCounter{}, byDevice: loginCounter{}, byKnownUsername: loginCounter{}}
}

// full reports a counter with no room for key.
func (counter loginCounter) full(key string) bool {
	_, found := counter[key]

	return !found && len(counter) >= maxLimiterKeys
}

func (counter loginCounter) count(key string, now time.Time) int {
	state := counter[key]

	if state == nil || now.Sub(state.start) >= LoginWindow {
		return 0
	}

	return state.count
}

// add counts an attempt and returns the count with it.
func (counter loginCounter) add(key string, now time.Time) int {
	state := counter[key]

	if state == nil || now.Sub(state.start) >= LoginWindow {
		state = &loginWindowState{start: now}
		counter[key] = state
	}

	state.count++

	return state.count
}

// forgive takes one attempt off key's count in the window that started at
// start; a later window keeps its count.
func (counter loginCounter) forgive(key string, start time.Time) {
	if state := counter[key]; state != nil && state.start.Equal(start) && state.count > 0 {
		state.count--
	}
}

func (counter loginCounter) sweep(now time.Time) {
	for key, state := range counter {
		if now.Sub(state.start) >= LoginWindow {
			delete(counter, key)
		}
	}
}

// loginAttempt is an attempt counted by begin, with the keys it counted
// under and the counts it reached.
type loginAttempt struct {
	username                    string // usernameKey of the typed one
	knownDevice                 bool
	addressKey                  string
	pairKey                     string // a network and the username, for a known device
	addressCount, usernameCount int    // addressCount is the pair's for a known device
	usernameStart               time.Time
}

// begin counts an attempt, or refuses it when its count is used up.
func (limiter *loginLimiter) begin(client netip.Addr, username string, knownDevice bool, now time.Time) (loginAttempt, bool) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	// Drop finished windows so the maps do not grow with every key ever seen.
	if now.Sub(limiter.swept) >= time.Minute {
		for _, counter := range []loginCounter{limiter.byAddress, limiter.byUsername, limiter.byDevice, limiter.byKnownUsername} {
			counter.sweep(now)
		}

		limiter.swept = now
	}

	network := server.ClientNetwork(client).String()
	attempt := loginAttempt{username: usernameKey(username), knownDevice: knownDevice}

	// Only users' current device cookies get here, so these keys are bounded by the users.
	if knownDevice {
		attempt.pairKey = network + "\x00" + attempt.username

		if limiter.byDevice.count(attempt.pairKey, now) >= MaxLoginFailures || limiter.byKnownUsername.count(attempt.username, now) >= maxKnownDeviceFailures {
			return attempt, false
		}

		attempt.addressCount = limiter.byDevice.add(attempt.pairKey, now)
		attempt.usernameCount = limiter.byKnownUsername.add(attempt.username, now)
		attempt.usernameStart = limiter.byKnownUsername[attempt.username].start

		return attempt, true
	}

	attempt.addressKey = network

	if limiter.byAddress.full(attempt.addressKey) || limiter.byUsername.full(attempt.username) {
		if limiter.fullWarned.IsZero() || now.Sub(limiter.fullWarned) >= LoginWindow {
			limiter.fullWarned = now
			limiter.logger.Warn("login limit full: new addresses and usernames are refused, likely a distributed guessing attack; known devices still log in",
				"keys", maxLimiterKeys, "for", LoginWindow)
		}

		return attempt, false
	}

	if limiter.byAddress.count(attempt.addressKey, now) >= MaxLoginFailures || limiter.byUsername.count(attempt.username, now) >= MaxLoginFailures {
		return attempt, false
	}

	attempt.addressCount = limiter.byAddress.add(attempt.addressKey, now)
	attempt.usernameCount = limiter.byUsername.add(attempt.username, now)
	attempt.usernameStart = limiter.byUsername[attempt.username].start

	return attempt, true
}

// succeeded clears the attempts of the network (or the known device) and
// takes the right password off the other counts.
func (limiter *loginLimiter) succeeded(attempt loginAttempt) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	if attempt.knownDevice {
		delete(limiter.byDevice, attempt.pairKey)
		limiter.byKnownUsername.forgive(attempt.username, attempt.usernameStart)

		return
	}

	delete(limiter.byAddress, attempt.addressKey)
	limiter.byUsername.forgive(attempt.username, attempt.usernameStart)
}

// usernameKey is the username as the limit counts it: in any case (usernames
// are unique in any case), capped so a long made-up one costs little memory.
func usernameKey(username string) string {
	key := strings.ToLower(strings.TrimSpace(username))

	if len(key) > config.MaxUsername {
		key = key[:config.MaxUsername]
	}

	return key
}

// deviceKey signs device cookies: derived from SECRET_KEY (or secret.key),
// so they survive restarts; random without one (tests).
func deviceKey(secretKey string) []byte {
	if secretKey == "" {
		key := make([]byte, 32)
		_, _ = rand.Read(key)

		return key
	}

	sum := sha256.Sum256([]byte("login device\x00" + secretKey))

	return sum[:]
}

// Auth is the login of the panel's users: sessions and the limit on login attempts.
type Auth struct {
	logger         *slog.Logger
	sessions       *services.Sessions
	passwords      passwordChecker
	trustedProxies []netip.Prefix
	sessionLife    time.Duration // SESSION_LIFETIME
	limiter        *loginLimiter
	deviceKey      []byte
	// checks holds a place per password check running (passwordChecks);
	// knownChecks is one more that only known devices use, so their owners
	// never wait behind strangers. A check waits at most checkWait for one.
	checks      chan struct{}
	knownChecks chan struct{}
	checkWait   time.Duration
	now         func() time.Time
	// opened: a superadmin exists, so the UI is open; it never stops
	// existing, so the database is asked only until then.
	opened atomic.Bool
}

// passwordChecker checks passwords: services.Users.
type passwordChecker interface {
	Authenticate(ctx context.Context, username, password string) (store.User, bool, error)
	DeviceStamp(ctx context.Context, username string) (string, error)
	ChangeOwnPassword(ctx context.Context, viewer store.User, current, password, confirm, keepSession string) (services.Problems, bool, error)
}

func NewAuth(logger *slog.Logger, settings *config.Config, users *services.Users, sessions *services.Sessions) *Auth {
	return &Auth{
		logger: logger, sessions: sessions, passwords: users, trustedProxies: settings.TrustedProxies, sessionLife: settings.SessionLifetime,
		limiter: newLoginLimiter(logger), deviceKey: deviceKey(settings.Notifications.SecretKey),
		checks: make(chan struct{}, passwordChecks()), knownChecks: make(chan struct{}, 1), checkWait: passwordCheckWait, now: time.Now,
	}
}

// passwordChecks is how many password checks run at once: each takes a core
// for a while (PBKDF2), so with two cores or more ingest keeps half of them.
func passwordChecks() int {
	return max(1, runtime.GOMAXPROCS(0)/2)
}

// passwordCheckWait is how long an attempt waits for a password check before
// it is refused like a blocked one, so a flood cannot queue it for minutes.
const passwordCheckWait = 5 * time.Second

// errChecksBusy: no place for a password check came free within checkWait.
var errChecksBusy = errors.New("password checks busy")

// waitForCheck takes a place for a password check; call done after it.
func (auth *Auth) waitForCheck(ctx context.Context, knownDevice bool) (done func(), err error) {
	var known chan struct{} // nil, never ready, for any other attempt

	if knownDevice {
		known = auth.knownChecks
	}

	timer := time.NewTimer(auth.checkWait)

	defer timer.Stop()

	select {
	case auth.checks <- struct{}{}:
		return func() { <-auth.checks }, nil
	case known <- struct{}{}:
		return func() { <-known }, nil
	case <-timer.C:
		return nil, errChecksBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Disabled: there is no superadmin yet (ADMIN_PASSWORD was never set), so
// nobody can log in.
func (auth *Auth) Disabled(ctx context.Context) bool {
	if auth.opened.Load() {
		return false
	}

	open, err := auth.sessions.Open(ctx)
	auth.opened.Store(open)

	return err == nil && !open
}

// TooManyAttempts is what a blocked address is told, at login and when
// changing the password.
var TooManyAttempts = "Too many wrong passwords. Try again in " + config.HumanDuration(LoginWindow) + "."

// LoginResult is the outcome of a login attempt.
type LoginResult int

const (
	LoginOK               LoginResult = iota // a new session was started
	LoginWrongCredentials                    // the attempt was counted
	LoginBlocked                             // the limit or a busy server refused it; the password was not checked
)

// Login checks the typed credentials of a login form and starts a new
// session when they match. Attempts are limited by loginLimiter (unknown
// usernames too, so a block does not tell which exist). An error is a
// failure of the database.
func (auth *Auth) Login(w http.ResponseWriter, r *http.Request, username, password string) (LoginResult, error) {
	client := server.ClientIP(r, auth.trustedProxies)
	now := auth.now()
	attempt, allowed := auth.limiter.begin(client, username, auth.knownDevice(r, username, now), now)

	if !allowed {
		return LoginBlocked, nil
	}

	done, err := auth.waitForCheck(r.Context(), attempt.knownDevice)

	if errors.Is(err, errChecksBusy) {
		return LoginBlocked, nil
	}

	if err != nil {
		return LoginWrongCredentials, err
	}

	user, ok, err := auth.passwords.Authenticate(r.Context(), username, password)

	done()

	if err != nil {
		return LoginWrongCredentials, err
	}

	if !ok {
		auth.failed(client, attempt)

		return LoginWrongCredentials, nil
	}

	auth.limiter.succeeded(attempt)
	auth.setDeviceCookie(w, r, user.Username, now)

	// A new session every login.
	if err := auth.startSession(r.Context(), w, r, user.ID, now); err != nil {
		return LoginWrongCredentials, err
	}

	return LoginOK, nil
}

// failed logs a wrong password that used up the last attempt. A single one
// is a typo, not an event: only a block (likely password guessing) is
// logged. The password typed is never logged, it may be one entered in the
// wrong field.
func (auth *Auth) failed(client netip.Addr, attempt loginAttempt) {
	if attempt.knownDevice {
		if attempt.addressCount == MaxLoginFailures {
			auth.logger.Warn("too many wrong passwords from a known browser, blocked", "username", attempt.username, "client", client.String(), "attempts", MaxLoginFailures, "for", LoginWindow)
		}

		if attempt.usernameCount == maxKnownDeviceFailures {
			auth.logger.Warn("too many wrong passwords from known browsers, username blocked for them, its device cookie may be copied", "username", attempt.username, "client", client.String(), "attempts", maxKnownDeviceFailures, "for", LoginWindow)
		}

		return
	}

	if attempt.addressCount == MaxLoginFailures {
		auth.logger.Warn("too many wrong passwords, address blocked", "client", client.String(), "attempts", MaxLoginFailures, "for", LoginWindow)
	}

	if attempt.usernameCount == MaxLoginFailures {
		auth.logger.Warn("too many wrong passwords, username blocked for browsers that never logged in as it", "username", attempt.username, "client", client.String(), "attempts", MaxLoginFailures, "for", LoginWindow)
	}
}

// ChangePassword replaces the viewer's own password after checking the
// current one, which counts against the same limit as logins (a stolen
// session must not become a way to guess the password). The viewer's other
// sessions end; their agents stay connected, since it is the owner who
// changes it. This browser stays the known device.
func (auth *Auth) ChangePassword(w http.ResponseWriter, r *http.Request, viewer store.User, current, password, confirm string) (services.Problems, error) {
	client := server.ClientIP(r, auth.trustedProxies)
	now := auth.now()
	attempt, allowed := auth.limiter.begin(client, viewer.Username, auth.knownDevice(r, viewer.Username, now), now)

	if !allowed {
		return services.Problems{"current_password": TooManyAttempts}, nil
	}

	done, err := auth.waitForCheck(r.Context(), attempt.knownDevice)

	if errors.Is(err, errChecksBusy) {
		return services.Problems{"current_password": TooManyAttempts}, nil
	}

	if err != nil {
		return nil, err
	}

	problems, wrongCurrent, err := auth.passwords.ChangeOwnPassword(r.Context(), viewer, current, password, confirm, SessionHash(r))

	done()

	switch {
	case wrongCurrent:
		auth.failed(client, attempt)
	case err == nil:
		auth.limiter.succeeded(attempt)

		if len(problems) == 0 {
			auth.setDeviceCookie(w, r, viewer.Username, now)
		}
	}

	return problems, err
}

// deviceToken marks a browser that logged in as username's user with stamp
// (services.Users.DeviceStamp), until expires: "expires.user.stamp", where
// user signs the username alone, so a made-up cookie is refused before the
// database is read and its timing tells nothing about which usernames exist.
func (auth *Auth) deviceToken(username, stamp string, expires time.Time) string {
	seconds := strconv.FormatInt(expires.Unix(), 10)

	return seconds + "." + auth.deviceTag("user", seconds, usernameKey(username)) + "." + auth.deviceTag("stamp", seconds, stamp)
}

func (auth *Auth) deviceTag(kind, seconds, value string) string {
	mac := hmac.New(sha256.New, auth.deviceKey)
	_, _ = mac.Write([]byte(kind + "\x00" + seconds + "\x00" + value))

	return hex.EncodeToString(mac.Sum(nil))
}

// knownDevice reports whether the browser last logged in as the user behind
// username, who has not changed password or role, nor been deleted, since.
func (auth *Auth) knownDevice(r *http.Request, username string, now time.Time) bool {
	cookie, err := r.Cookie(deviceCookie)

	if err != nil {
		return false
	}

	parts := strings.Split(cookie.Value, ".")

	if len(parts) != 3 {
		return false
	}

	expires, err := strconv.ParseInt(parts[0], 10, 64)

	if err != nil || now.Unix() >= expires || !hmac.Equal([]byte(parts[1]), []byte(auth.deviceTag("user", parts[0], usernameKey(username)))) {
		return false
	}

	stamp, err := auth.passwords.DeviceStamp(r.Context(), username)

	if err != nil {
		auth.logger.Error("check login device", "err", err)
	}

	return stamp != "" && hmac.Equal([]byte(cookie.Value), []byte(auth.deviceToken(username, stamp, time.Unix(expires, 0))))
}

// setDeviceCookie makes the browser the known device of username's user.
func (auth *Auth) setDeviceCookie(w http.ResponseWriter, r *http.Request, username string, now time.Time) {
	stamp, err := auth.passwords.DeviceStamp(r.Context(), username)

	if err != nil || stamp == "" {
		auth.logger.Error("mark login device", "err", err)

		return
	}

	expires := now.Add(deviceLifetime)

	http.SetCookie(w, &http.Cookie{
		Name: deviceCookie, Value: auth.deviceToken(username, stamp, expires), Path: "/", Expires: expires,
		HttpOnly: true, Secure: server.IsHTTPS(r, auth.trustedProxies), SameSite: http.SameSiteLaxMode,
	})
}

// startSession creates a new session and sets its cookie. A session cookie
// the browser already had (possibly planted) is ended, never reused.
func (auth *Auth) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID string, now time.Time) error {
	if cookie, err := r.Cookie(CookieName); err == nil && cookie.Value != "" {
		if err := auth.sessions.End(ctx, secret.Hash(cookie.Value)); err != nil {
			auth.logger.Error("delete previous session", "err", err)
		}
	}

	token := newToken()
	expires := now.Add(auth.sessionLife)

	if err := auth.sessions.Start(ctx, secret.Hash(token), userID, now, expires); err != nil {
		return err
	}

	auth.setSessionCookie(w, r, token, expires)

	return nil
}

// EndSession deletes the session of the request and clears its cookie.
func (auth *Auth) EndSession(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(CookieName); err == nil {
		if err := auth.sessions.End(ctx, secret.Hash(cookie.Value)); err != nil {
			auth.logger.Error("delete session", "err", err)
		}
	}

	auth.setSessionCookie(w, r, "", time.Unix(0, 0))
}

func (auth *Auth) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   server.IsHTTPS(r, auth.trustedProxies),
		// Lax: the cookie is not sent with cross-site POSTs, which blocks CSRF
		// together with the Origin check of the web app's API (server.SameOrigin).
		SameSite: http.SameSiteLaxMode,
	})
}

// User returns the user of the request's session: store.ErrNotFound without
// a valid one (no cookie, an unknown or ended session, a deleted user), any
// other error when the session could not be checked.
func (auth *Auth) User(r *http.Request) (store.User, error) {
	hash := SessionHash(r)

	if hash == "" {
		return store.User{}, store.ErrNotFound
	}

	return auth.sessions.User(r.Context(), hash, auth.now())
}

// SessionUser returns the user of the request's session; false without a
// valid session.
func (auth *Auth) SessionUser(r *http.Request) (store.User, bool) {
	user, err := auth.User(r)

	if err != nil && !errors.Is(err, store.ErrNotFound) {
		auth.logger.Error("check session", "err", err)
	}

	return user, err == nil
}

// SessionHash identifies the request's session as stored ("" without one).
func SessionHash(r *http.Request) string {
	cookie, err := r.Cookie(CookieName)

	if err != nil || cookie.Value == "" {
		return ""
	}

	return secret.Hash(cookie.Value)
}

// FormToken binds an authorization form to this session and request.
// The session cookie is random and HttpOnly, so another site cannot forge it.
func (auth *Auth) FormToken(r *http.Request, value string) string {
	cookie, err := r.Cookie(CookieName)

	if err != nil || cookie.Value == "" {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(cookie.Value))
	_, _ = mac.Write([]byte(value))

	return hex.EncodeToString(mac.Sum(nil))
}

// ValidFormToken checks a session-bound token without exposing its value.
func (auth *Auth) ValidFormToken(r *http.Request, action, token string) bool {
	expected := auth.FormToken(r, action)

	return token != "" && expected != "" && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}
