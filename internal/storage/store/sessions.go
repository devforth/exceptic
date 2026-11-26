package store

import (
	"errors"
	"time"
)

// Ways to sign in to the web app.
const (
	SignInPassword = "password"
	SignInGoogle   = "google"
)

// Results of a sign-in attempt in the sign-in log.
const (
	SignInOK            = "ok"
	SignInWrongPassword = "wrong_password"
	// SignInBlocked: the login limit refused the attempt before its password was checked.
	SignInBlocked = "blocked"
	// SignInNotLinked: a Google account whose email is the user's but another
	// Google account is linked to them.
	SignInNotLinked = "not_linked"
	// SignInUnlinked: a Google account with the user's email after Google was
	// unlinked from them, before they signed in with the password.
	SignInUnlinked = "unlinked"
)

// SignInsKept is how many sign-ins the log keeps per user, the newest, and
// as many refused attempts apart, so refusals never push out a sign-in.
const SignInsKept = 100

// SessionActiveEvery: a session's last activity is written at most this often.
const SessionActiveEvery = time.Minute

// Refusals of LinkGoogle.
var (
	ErrGoogleTaken  = errors.New("this Google account is linked to another user")
	ErrGoogleLinked = errors.New("another Google account is linked to this user")
	ErrGoogleOff    = errors.New("google was unlinked from this user: sign in with the password first")
)

// Session is a login to the web app, as its user and those above them see it;
// the cookie and its hash never leave the store, ID is a short part of the hash.
type Session struct {
	ID           string
	Method       string // SignInPassword or SignInGoogle
	Address      string // the client's address at the login
	Browser      string // its User-Agent, cut
	CreatedAt    time.Time
	LastActiveAt time.Time
	ExpiresAt    time.Time
}

// SessionID is the ID a session is shown and ended by: the first 16 of the
// 64 hex characters of its cookie's hash, which tell nothing of the cookie.
func SessionID(tokenHash string) string { return tokenHash[:min(len(tokenHash), 16)] }

// NewSession is a login to store.
type NewSession struct {
	TokenHash, UserID    string
	Method               string
	Address, Browser     string
	CreatedAt, ExpiresAt time.Time
}

// SignIn is an attempt to sign in as a user, kept in their sign-in log.
type SignIn struct {
	UserID  string
	At      time.Time
	Method  string // SignInPassword or SignInGoogle
	Result  string // SignInOK or why not
	Address string
	Browser string
}

// MaxBrowserLength is how much of a User-Agent a session and the log keep.
const MaxBrowserLength = 300
