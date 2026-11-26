package session

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The first admin: while there is none, the login of ADMIN_EMAIL and
// ADMIN_PASSWORD starts no session but sets setupCookie, signed, for
// setupLife, which opens the setup form (SetupAdmin) and nothing else.
const (
	setupCookie = "exceptic_setup"
	setupLife   = 10 * time.Minute
	setupPath   = server.UIAPIPrefix + "setup_admin"
)

// adminSetup creates the first admin: services.Users.
type adminSetup interface {
	SetupAdmin(ctx context.Context, email, password, confirm, adminPassword string) (store.User, services.Problems, error)
}

// ErrSetupExpired: the setup form came without a valid setup cookie.
var ErrSetupExpired = services.Forbidden("The setup has expired: log in with ADMIN_EMAIL and ADMIN_PASSWORD again.")

// setupLogin reports a login with ADMIN_EMAIL and ADMIN_PASSWORD while
// there is no admin. Both are compared in constant time and the admins
// read whatever was typed, so the time tells nothing.
func (auth *Auth) setupLogin(ctx context.Context, email, password string) bool {
	if auth.adminPassword == "" {
		return false
	}

	emailMatch := sameSecret(emailKey(email), emailKey(auth.adminEmail))
	passwordMatch := sameSecret(password, auth.adminPassword)
	open, err := auth.open(ctx)

	if err != nil {
		auth.logger.Error("check for an admin", "err", err)
	}

	return emailMatch&passwordMatch == 1 && err == nil && !open
}

// sameSecret compares the hashes, so the time does not depend on the lengths.
func sameSecret(typed, wanted string) int {
	typedSum, wantedSum := sha256.Sum256([]byte(typed)), sha256.Sum256([]byte(wanted))

	return subtle.ConstantTimeCompare(typedSum[:], wantedSum[:])
}

// setupTag signs a setup cookie's expiry with the env credentials, so
// changing them ends the setups started with the old ones.
func (auth *Auth) setupTag(seconds string) string {
	credentials := sha256.Sum256([]byte(emailKey(auth.adminEmail) + "\x00" + auth.adminPassword))
	mac := hmac.New(sha256.New, auth.deviceKey)
	_, _ = mac.Write([]byte("admin setup\x00" + seconds + "\x00" + hex.EncodeToString(credentials[:])))

	return hex.EncodeToString(mac.Sum(nil))
}

func (auth *Auth) setSetupCookie(w http.ResponseWriter, r *http.Request, now time.Time) {
	expires := now.Add(setupLife)
	seconds := strconv.FormatInt(expires.Unix(), 10)
	auth.writeSetupCookie(w, r, seconds+"."+auth.setupTag(seconds), expires)
}

func (auth *Auth) clearSetupCookie(w http.ResponseWriter, r *http.Request) {
	auth.writeSetupCookie(w, r, "", time.Unix(0, 0))
}

func (auth *Auth) writeSetupCookie(w http.ResponseWriter, r *http.Request, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: setupCookie, Value: value, Path: setupPath, Expires: expires, HttpOnly: true,
		Secure: server.IsHTTPS(r, auth.trustedProxies), SameSite: http.SameSiteStrictMode,
	})
}

// validSetup reports a setup cookie of the current env credentials that has not expired.
func (auth *Auth) validSetup(r *http.Request) bool {
	cookie, err := r.Cookie(setupCookie)

	if err != nil || auth.adminPassword == "" {
		return false
	}

	seconds, tag, found := strings.Cut(cookie.Value, ".")
	expires, err := strconv.ParseInt(seconds, 10, 64)

	return found && err == nil && auth.now().Unix() < expires && hmac.Equal([]byte(tag), []byte(auth.setupTag(seconds)))
}

// SetupAdmin creates the first admin from the setup form and logs them in
// with a new session. The setup cookie is required (ErrSetupExpired) and
// cleared once it has served; services.Users.SetupAdmin refuses when an
// admin exists. Problems are the form's, at their fields.
func (auth *Auth) SetupAdmin(w http.ResponseWriter, r *http.Request, email, password, confirm string) (services.Problems, error) {
	if !auth.validSetup(r) {
		return nil, ErrSetupExpired
	}

	// Hashing the password takes a core like a check.
	done, err := auth.waitForCheck(r.Context(), false)

	if errors.Is(err, errChecksBusy) {
		return nil, ErrTooManyAttempts
	}

	if err != nil {
		return nil, err
	}

	user, problems, err := auth.setup.SetupAdmin(r.Context(), email, password, confirm, auth.adminPassword)

	done()

	if errors.Is(err, services.ErrForbidden) {
		auth.clearSetupCookie(w, r)
	}

	if err != nil || len(problems) > 0 {
		return problems, err
	}

	now := auth.now()

	auth.clearSetupCookie(w, r)
	auth.opened.Store(true)
	auth.setDeviceCookie(w, r, user.Email, now)

	return nil, auth.startSession(r.Context(), w, r, user.ID, store.SignInPassword, now)
}
