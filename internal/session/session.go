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

	// MaxLoginFailures attempts are allowed per client network, per email
	// and per known device within LoginWindow.
	MaxLoginFailures = 10
	LoginWindow      = 15 * time.Minute

	// Known devices get this many attempts at one email from all networks
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
// counted per network and email, and per email up to
// maxKnownDeviceFailures, so strangers' guesses cannot lock its owner out;
// any other attempt counts per network and per email, so guessing from
// many networks stays at MaxLoginFailures.
type loginLimiter struct {
	mutex        sync.Mutex
	logger       *slog.Logger
	byAddress    loginCounter
	byEmail      loginCounter
	byDevice     loginCounter // a network and an email, for known devices
	byKnownEmail loginCounter // known-device attempts at an email
	swept        time.Time
	fullWarned   time.Time
}

type loginWindowState struct {
	start time.Time
	count int
	// A network's attempts: at email alone, or also at others.
	email  string
	others bool
}

// attemptAt notes the email of a network's attempt just counted.
func (state *loginWindowState) attemptAt(email string) {
	if state.count == 1 {
		state.email = email
	} else if state.email != email {
		state.others = true
	}
}

type loginCounter map[string]*loginWindowState

// maxLimiterKeys bounds the memory a flood of made-up addresses or emails
// can take: past it a new one is refused until its window ends. Known
// devices are counted apart, so their owners still log in.
const maxLimiterKeys = 10_000

func newLoginLimiter(logger *slog.Logger) *loginLimiter {
	return &loginLimiter{logger: logger, byAddress: loginCounter{}, byEmail: loginCounter{}, byDevice: loginCounter{}, byKnownEmail: loginCounter{}}
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
	email                    string // emailKey of the typed one
	knownDevice              bool
	addressKey               string
	pairKey                  string // a network and the email, for a known device
	addressCount, emailCount int    // addressCount is the pair's for a known device
	addressStart, emailStart time.Time
}

// begin counts an attempt, or refuses it when its count is used up.
func (limiter *loginLimiter) begin(client netip.Addr, email string, knownDevice bool, now time.Time) (loginAttempt, bool) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	// Drop finished windows so the maps do not grow with every key ever seen.
	if now.Sub(limiter.swept) >= time.Minute {
		for _, counter := range []loginCounter{limiter.byAddress, limiter.byEmail, limiter.byDevice, limiter.byKnownEmail} {
			counter.sweep(now)
		}

		limiter.swept = now
	}

	network := server.ClientNetwork(client).String()
	attempt := loginAttempt{email: emailKey(email), knownDevice: knownDevice}

	// Only users' current device cookies get here, so these keys are bounded by the users.
	if knownDevice {
		attempt.pairKey = network + "\x00" + attempt.email

		if limiter.byDevice.count(attempt.pairKey, now) >= MaxLoginFailures || limiter.byKnownEmail.count(attempt.email, now) >= maxKnownDeviceFailures {
			return attempt, false
		}

		attempt.addressCount = limiter.byDevice.add(attempt.pairKey, now)
		attempt.emailCount = limiter.byKnownEmail.add(attempt.email, now)
		attempt.emailStart = limiter.byKnownEmail[attempt.email].start

		return attempt, true
	}

	attempt.addressKey = network

	if limiter.byAddress.full(attempt.addressKey) || limiter.byEmail.full(attempt.email) {
		if limiter.fullWarned.IsZero() || now.Sub(limiter.fullWarned) >= LoginWindow {
			limiter.fullWarned = now
			limiter.logger.Warn("login limit full: new addresses and emails are refused, likely a distributed guessing attack; known devices still log in",
				"keys", maxLimiterKeys, "for", LoginWindow)
		}

		return attempt, false
	}

	if limiter.byAddress.count(attempt.addressKey, now) >= MaxLoginFailures || limiter.byEmail.count(attempt.email, now) >= MaxLoginFailures {
		return attempt, false
	}

	attempt.addressCount = limiter.byAddress.add(attempt.addressKey, now)
	address := limiter.byAddress[attempt.addressKey]
	address.attemptAt(attempt.email)
	attempt.addressStart = address.start
	attempt.emailCount = limiter.byEmail.add(attempt.email, now)
	attempt.emailStart = limiter.byEmail[attempt.email].start

	return attempt, true
}

// succeeded clears the attempts of the known device, or of the network when
// they were all at this email (the user's own typos), and takes the right
// password off the other counts, so a user cannot clear guesses at others.
func (limiter *loginLimiter) succeeded(attempt loginAttempt) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	if attempt.knownDevice {
		delete(limiter.byDevice, attempt.pairKey)
		limiter.byKnownEmail.forgive(attempt.email, attempt.emailStart)

		return
	}

	if address := limiter.byAddress[attempt.addressKey]; address != nil && address.start.Equal(attempt.addressStart) && !address.others {
		delete(limiter.byAddress, attempt.addressKey)
	} else {
		limiter.byAddress.forgive(attempt.addressKey, attempt.addressStart)
	}

	limiter.byEmail.forgive(attempt.email, attempt.emailStart)
}

// emailKey is the email as the limit counts it: in any case (emails
// are unique in any case), capped so a long made-up one costs little memory.
func emailKey(email string) string {
	key := strings.ToLower(strings.TrimSpace(email))

	if len(key) > config.MaxEmail {
		key = key[:config.MaxEmail]
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
	Authenticate(ctx context.Context, email, password string) (store.User, bool, error)
	DeviceStamp(ctx context.Context, email string) (string, error)
	ChangeOwnPassword(ctx context.Context, viewer store.User, current, password, confirm, keepSession string) (services.Problems, bool, error)
	ChangeOwnEmail(ctx context.Context, viewer store.User, current, email string) (services.Problems, bool, string, error)
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

// ErrTooManyAttempts: the login limit refused a password change.
var ErrTooManyAttempts = errors.New(TooManyAttempts)

// LoginResult is the outcome of a login attempt.
type LoginResult int

const (
	LoginOK               LoginResult = iota // a new session was started
	LoginWrongCredentials                    // the attempt was counted
	LoginBlocked                             // the limit or a busy server refused it; the password was not checked
)

// Login checks the typed credentials of a login form and starts a new
// session when they match. Attempts are limited by loginLimiter (unknown
// emails too, so a block does not tell which exist). An error is a
// failure of the database.
func (auth *Auth) Login(w http.ResponseWriter, r *http.Request, email, password string) (LoginResult, error) {
	client := server.ClientIP(r, auth.trustedProxies)
	now := auth.now()
	attempt, allowed := auth.limiter.begin(client, email, auth.knownDevice(r, email, now), now)

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

	user, ok, err := auth.passwords.Authenticate(r.Context(), email, password)

	done()

	if err != nil {
		return LoginWrongCredentials, err
	}

	if !ok {
		auth.failed(client, attempt)

		return LoginWrongCredentials, nil
	}

	auth.limiter.succeeded(attempt)
	auth.setDeviceCookie(w, r, user.Email, now)

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
			auth.logger.Warn("too many wrong passwords from a known browser, blocked", "email", attempt.email, "client", client.String(), "attempts", MaxLoginFailures, "for", LoginWindow)
		}

		if attempt.emailCount == maxKnownDeviceFailures {
			auth.logger.Warn("too many wrong passwords from known browsers, email blocked for them, its device cookie may be copied", "email", attempt.email, "client", client.String(), "attempts", maxKnownDeviceFailures, "for", LoginWindow)
		}

		return
	}

	if attempt.addressCount == MaxLoginFailures {
		auth.logger.Warn("too many wrong passwords, address blocked", "client", client.String(), "attempts", MaxLoginFailures, "for", LoginWindow)
	}

	if attempt.emailCount == MaxLoginFailures {
		auth.logger.Warn("too many wrong passwords, email blocked for browsers that never logged in as it", "email", attempt.email, "client", client.String(), "attempts", MaxLoginFailures, "for", LoginWindow)
	}
}

// ChangePassword replaces the viewer's own password after checking the
// current one, which counts against the same limit as logins (a stolen
// session must not become a way to guess the password). The viewer's other
// sessions end; their agents stay connected, since it is the owner who
// changes it. This browser stays the known device. ErrTooManyAttempts when
// the login limit refuses the check.
func (auth *Auth) ChangePassword(w http.ResponseWriter, r *http.Request, viewer store.User, current, password, confirm string) (services.Problems, error) {
	return auth.withCurrentPassword(w, r, viewer, func() (services.Problems, bool, string, error) {
		problems, wrongCurrent, err := auth.passwords.ChangeOwnPassword(r.Context(), viewer, current, password, confirm, SessionHash(r))

		return problems, wrongCurrent, viewer.Email, err
	})
}

// ChangeEmail replaces the viewer's own email after checking the current
// password, under the same limit as ChangePassword; sessions and agents
// stay, and this browser becomes the known device of the new email.
func (auth *Auth) ChangeEmail(w http.ResponseWriter, r *http.Request, viewer store.User, current, email string) (services.Problems, error) {
	return auth.withCurrentPassword(w, r, viewer, func() (services.Problems, bool, string, error) {
		return auth.passwords.ChangeOwnEmail(r.Context(), viewer, current, email)
	})
}

// withCurrentPassword runs change, which checks the viewer's current
// password, as a login attempt of the viewer's email: counted by the login
// limit, a wrong password a failure. After a change it makes this browser
// the known device of the email change returns.
func (auth *Auth) withCurrentPassword(w http.ResponseWriter, r *http.Request, viewer store.User, change func() (services.Problems, bool, string, error)) (services.Problems, error) {
	client := server.ClientIP(r, auth.trustedProxies)
	now := auth.now()
	attempt, allowed := auth.limiter.begin(client, viewer.Email, auth.knownDevice(r, viewer.Email, now), now)

	if !allowed {
		return nil, ErrTooManyAttempts
	}

	done, err := auth.waitForCheck(r.Context(), attempt.knownDevice)

	if errors.Is(err, errChecksBusy) {
		return nil, ErrTooManyAttempts
	}

	if err != nil {
		return nil, err
	}

	problems, wrongCurrent, email, err := change()

	done()

	switch {
	case wrongCurrent:
		auth.failed(client, attempt)
	case err == nil:
		auth.limiter.succeeded(attempt)

		if len(problems) == 0 {
			auth.setDeviceCookie(w, r, email, now)
		}
	}

	return problems, err
}

// deviceToken marks a browser that logged in as email's user with stamp
// (services.Users.DeviceStamp), until expires: "expires.user.stamp", where
// user signs the email alone, so a made-up cookie is refused before the
// database is read and its timing tells nothing about which emails exist.
func (auth *Auth) deviceToken(email, stamp string, expires time.Time) string {
	seconds := strconv.FormatInt(expires.Unix(), 10)

	return seconds + "." + auth.deviceTag("user", seconds, emailKey(email)) + "." + auth.deviceTag("stamp", seconds, stamp)
}

func (auth *Auth) deviceTag(kind, seconds, value string) string {
	mac := hmac.New(sha256.New, auth.deviceKey)
	_, _ = mac.Write([]byte(kind + "\x00" + seconds + "\x00" + value))

	return hex.EncodeToString(mac.Sum(nil))
}

// knownDevice reports whether the browser last logged in as the user behind
// email, who has not changed password or role, nor been deleted, since.
func (auth *Auth) knownDevice(r *http.Request, email string, now time.Time) bool {
	cookie, err := r.Cookie(deviceCookie)

	if err != nil {
		return false
	}

	parts := strings.Split(cookie.Value, ".")

	if len(parts) != 3 {
		return false
	}

	expires, err := strconv.ParseInt(parts[0], 10, 64)

	if err != nil || now.Unix() >= expires || !hmac.Equal([]byte(parts[1]), []byte(auth.deviceTag("user", parts[0], emailKey(email)))) {
		return false
	}

	stamp, err := auth.passwords.DeviceStamp(r.Context(), email)

	if err != nil {
		auth.logger.Error("check login device", "err", err)
	}

	return stamp != "" && hmac.Equal([]byte(cookie.Value), []byte(auth.deviceToken(email, stamp, time.Unix(expires, 0))))
}

// setDeviceCookie makes the browser the known device of email's user.
func (auth *Auth) setDeviceCookie(w http.ResponseWriter, r *http.Request, email string, now time.Time) {
	stamp, err := auth.passwords.DeviceStamp(r.Context(), email)

	if err != nil || stamp == "" {
		auth.logger.Error("mark login device", "err", err)

		return
	}

	expires := now.Add(deviceLifetime)

	http.SetCookie(w, &http.Cookie{
		Name: deviceCookie, Value: auth.deviceToken(email, stamp, expires), Path: "/", Expires: expires,
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
