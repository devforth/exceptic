package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// slowPasswords refuses every password and holds each check until released,
// so the checks overlap as on a busy server.
type slowPasswords struct {
	checks   atomic.Int32
	released chan struct{}
}

func (passwords *slowPasswords) wait() {
	passwords.checks.Add(1)

	select {
	case <-passwords.released:
	case <-time.After(time.Second):
	}
}

func (passwords *slowPasswords) Authenticate(context.Context, string, string) (store.User, bool, error) {
	passwords.wait()

	return store.User{}, false, nil
}

func (passwords *slowPasswords) DeviceStamp(context.Context, string) (string, error) { return "", nil }

func (passwords *slowPasswords) ChangeOwnPassword(context.Context, store.User, string, string, string, string) (services.Problems, bool, error) {
	passwords.wait()

	return services.Problems{"current_password": "wrong"}, true, nil
}

// Attempts sent at the same time are counted before their passwords are
// checked, so they cannot run more checks than the limit allows.
func TestConcurrentAttemptsLimited(t *testing.T) {
	const attempts = 40

	for _, change := range []bool{false, true} {
		passwords := &slowPasswords{released: make(chan struct{})}

		auth := testAuth(passwords, attempts)

		var group sync.WaitGroup

		for range attempts {
			group.Go(func() {
				request := httptest.NewRequest(http.MethodPost, "/login", nil)

				if change {
					_, _ = auth.ChangePassword(httptest.NewRecorder(), request, store.User{Username: "admin"}, "guess", "new password", "new password")
				} else {
					_, _ = auth.Login(httptest.NewRecorder(), request, "admin", "guess")
				}
			})
		}

		// Release the checks once the allowed ones and any extra had time to start.
		go func() {
			for passwords.checks.Load() < MaxLoginFailures {
				time.Sleep(time.Millisecond)
			}

			time.Sleep(50 * time.Millisecond)
			close(passwords.released)
		}()

		group.Wait()

		if checks := passwords.checks.Load(); checks != MaxLoginFailures {
			t.Errorf("change password %v: %d password checks, want %d", change, checks, MaxLoginFailures)
		}
	}
}

// testAuth is an Auth with places password checks at once.
func testAuth(passwords passwordChecker, places int) *Auth {
	logger := slog.New(slog.DiscardHandler)

	return &Auth{
		logger: logger, passwords: passwords, limiter: newLoginLimiter(logger), deviceKey: deviceKey("0123456789abcdef0123456789abcdef"),
		checks: make(chan struct{}, places), knownChecks: make(chan struct{}, 1), checkWait: passwordCheckWait, now: time.Now,
	}
}

func network(index int) netip.Addr {
	return netip.MustParseAddr(fmt.Sprintf("203.0.113.%d", index))
}

// One /48 holds 65536 /64s: guesses from them count against the username
// like any others.
func subnet(index int) netip.Addr {
	return netip.MustParseAddr(fmt.Sprintf("2001:db8:1:%x::1", index))
}

// A username guessed at from many networks is blocked for every browser
// that never logged in as it, and stays open to one that did.
func TestUsernameLimit(t *testing.T) {
	limiter := newLoginLimiter(slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	for index := range MaxLoginFailures {
		if _, allowed := limiter.begin(subnet(index), "Admin", false, now); !allowed {
			t.Fatalf("attempt %d refused", index)
		}
	}

	if _, allowed := limiter.begin(network(50), "admin", false, now); allowed {
		t.Fatal("a new network may guess the blocked username")
	}

	if _, allowed := limiter.begin(network(50), "dev", false, now); !allowed {
		t.Fatal("another username is blocked")
	}

	// The owner's browser passes, its typos counted apart from others' guesses.
	owner, allowed := limiter.begin(subnet(0), "admin", true, now)

	if !allowed {
		t.Fatal("the owner's browser is locked out by others' guesses")
	}

	limiter.succeeded(owner)

	if _, allowed := limiter.begin(network(0), "admin", false, now.Add(LoginWindow)); !allowed {
		t.Fatal("the window did not end")
	}
}

// A known browser gets MaxLoginFailures tries per network and
// maxKnownDeviceFailures from all networks, so a copied device cookie is no
// way around the limit.
func TestKnownDeviceLimit(t *testing.T) {
	limiter := newLoginLimiter(slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	for index := range MaxLoginFailures {
		if _, allowed := limiter.begin(network(1), "admin", true, now); !allowed {
			t.Fatalf("attempt %d refused", index)
		}
	}

	if _, allowed := limiter.begin(network(1), "admin", true, now); allowed {
		t.Fatal("a known browser is not limited")
	}

	if _, allowed := limiter.begin(network(1), "admin", false, now); !allowed {
		t.Fatal("a known browser's typos count against the network")
	}

	allowed := MaxLoginFailures

	for index := 2; index < 100; index++ {
		for range MaxLoginFailures {
			if _, ok := limiter.begin(network(index), "Admin", true, now); ok {
				allowed++
			}
		}
	}

	if allowed != maxKnownDeviceFailures {
		t.Fatalf("one device cookie from many networks: %d attempts, want %d", allowed, maxKnownDeviceFailures)
	}

	if _, ok := limiter.begin(network(100), "dev", true, now); !ok {
		t.Fatal("another username's known browser is blocked")
	}
}

// Past maxLimiterKeys new addresses and usernames are refused, with one
// warning per window; counted ones and known devices go on as before.
func TestFullLimiter(t *testing.T) {
	var logged strings.Builder

	limiter := newLoginLimiter(slog.New(slog.NewTextHandler(&logged, nil)))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	limiter.swept = now

	for index := range maxLimiterKeys {
		limiter.byUsername[fmt.Sprintf("made-up-%d", index)] = &loginWindowState{start: now, count: 1}
	}

	for index := range 3 {
		if _, allowed := limiter.begin(network(index), "admin", false, now); allowed {
			t.Fatal("a new username is counted past the full counter")
		}
	}

	if _, allowed := limiter.begin(network(10), "made-up-7", false, now); !allowed {
		t.Fatal("a username already counted is refused")
	}

	if _, allowed := limiter.begin(network(11), "admin", true, now); !allowed {
		t.Fatal("a known device is refused when the counter is full")
	}

	if count := strings.Count(logged.String(), "login limit full"); count != 1 {
		t.Fatalf("warned %d times, want once:\n%s", count, logged.String())
	}

	if _, allowed := limiter.begin(network(12), "admin", false, now.Add(LoginWindow+time.Minute)); !allowed {
		t.Fatal("the counter does not empty when the window ends")
	}
}

// One IPv6 host holds a whole /64: its addresses share one count.
func TestNetworkLimit(t *testing.T) {
	limiter := newLoginLimiter(slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	for index := range MaxLoginFailures {
		limiter.begin(netip.MustParseAddr(fmt.Sprintf("2001:db8:1:2::%x", index+1)), fmt.Sprintf("user%d", index), false, now)
	}

	if _, allowed := limiter.begin(netip.MustParseAddr("2001:db8:1:2::ffff"), "other", false, now); allowed {
		t.Fatal("a new address in the same /64 is not limited")
	}

	if _, allowed := limiter.begin(netip.MustParseAddr("2001:db8:1:3::1"), "other", false, now); !allowed {
		t.Fatal("another /64 is limited")
	}
}

// stampedPasswords refuses every password and gives each user's device stamp.
type stampedPasswords struct {
	stamps map[string]string
	reads  int // DeviceStamp calls
}

func (passwords *stampedPasswords) Authenticate(context.Context, string, string) (store.User, bool, error) {
	return store.User{}, false, nil
}

func (passwords *stampedPasswords) DeviceStamp(_ context.Context, username string) (string, error) {
	passwords.reads++

	return passwords.stamps[strings.ToLower(username)], nil
}

func (passwords *stampedPasswords) ChangeOwnPassword(context.Context, store.User, string, string, string, string) (services.Problems, bool, error) {
	return services.Problems{"current_password": "wrong"}, true, nil
}

// deviceRequest is a login from a browser with the device cookie value.
func deviceRequest(value string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/login", nil)
	request.AddCookie(&http.Cookie{Name: deviceCookie, Value: value})

	return request
}

// The device cookie counts only for the user it was issued for, unchanged,
// before it expires and while the user's stamp (password, role) is the same.
func TestKnownDevice(t *testing.T) {
	passwords := &stampedPasswords{stamps: map[string]string{"admin": "stamp-1", "dev": "stamp-2"}}
	auth := testAuth(passwords, 1)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	login := httptest.NewRecorder()
	auth.setDeviceCookie(login, httptest.NewRequest(http.MethodPost, "/login", nil), "Admin", now)

	cookie := login.Result().Cookies()[0]
	known := func(value, username string, at time.Time) bool {
		return auth.knownDevice(deviceRequest(value), username, at)
	}

	if !known(cookie.Value, "admin", now) {
		t.Fatal("the browser that logged in is not known")
	}

	if known(cookie.Value, "dev", now) {
		t.Fatal("the cookie of one user counts for another")
	}

	if known(cookie.Value, "admin", now.Add(deviceLifetime)) {
		t.Fatal("an expired cookie counts")
	}

	stamp, _, _ := strings.Cut(cookie.Value, ".")
	later := strconv.FormatInt(now.Add(10*deviceLifetime).Unix(), 10)

	if known(later+cookie.Value[len(stamp):], "admin", now) {
		t.Fatal("a cookie with a changed expiry counts")
	}

	other := testAuth(passwords, 1)
	other.deviceKey = deviceKey("another key, another key, another")

	if other.knownDevice(deviceRequest(cookie.Value), "admin", now) {
		t.Fatal("a cookie signed with another key counts")
	}

	passwords.stamps["admin"] = "stamp-3"

	if known(cookie.Value, "admin", now) {
		t.Fatal("the cookie counts after the password or role changed")
	}

	delete(passwords.stamps, "admin")

	if known(cookie.Value, "admin", now) {
		t.Fatal("the cookie counts after the user was deleted")
	}
}

// A check waits for a place at most checkWait and is then refused like a
// blocked one, while a known device has a place of its own.
func TestPasswordCheckWait(t *testing.T) {
	passwords := &stampedPasswords{stamps: map[string]string{"admin": "stamp-1"}}
	auth := testAuth(passwords, 1)
	auth.checkWait = 20 * time.Millisecond

	auth.checks <- struct{}{} // a stranger's check that does not end

	login := httptest.NewRecorder()
	auth.setDeviceCookie(login, httptest.NewRequest(http.MethodPost, "/login", nil), "admin", time.Now())

	device := login.Result().Cookies()[0]

	if result, err := auth.Login(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/login", nil), "admin", "guess"); result != LoginBlocked || err != nil {
		t.Fatalf("a check with no place: %v %v, want blocked", result, err)
	}

	change := httptest.NewRequest(http.MethodPost, "/account/password", nil)

	if problems, err := auth.ChangePassword(httptest.NewRecorder(), change, store.User{Username: "admin"}, "guess", "new", "new"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("a password change with no place: %v %v", problems, err)
	}

	if result, err := auth.Login(httptest.NewRecorder(), deviceRequest(device.Value), "admin", "typo"); result != LoginWrongCredentials || err != nil {
		t.Fatalf("the owner's browser waits behind strangers: %v %v", result, err)
	}
}

// busyPasswords refuses every password and records how many checks overlap.
type busyPasswords struct {
	running, most atomic.Int32
}

func (passwords *busyPasswords) Authenticate(context.Context, string, string) (store.User, bool, error) {
	running := passwords.running.Add(1)

	defer passwords.running.Add(-1)

	for {
		most := passwords.most.Load()

		if running <= most || passwords.most.CompareAndSwap(most, running) {
			break
		}
	}

	time.Sleep(20 * time.Millisecond)

	return store.User{}, false, nil
}

func (passwords *busyPasswords) DeviceStamp(context.Context, string) (string, error) { return "", nil }

func (passwords *busyPasswords) ChangeOwnPassword(context.Context, store.User, string, string, string, string) (services.Problems, bool, error) {
	return nil, false, nil
}

// Password checks from many networks at once take only their share of cores.
func TestPasswordChecksCapped(t *testing.T) {
	const places = 2

	passwords := &busyPasswords{}
	auth := testAuth(passwords, places)

	var group sync.WaitGroup

	for index := range 8 {
		group.Go(func() {
			request := httptest.NewRequest(http.MethodPost, "/login", nil)
			request.RemoteAddr = network(index).String() + ":1234"
			_, _ = auth.Login(httptest.NewRecorder(), request, fmt.Sprintf("user%d", index), "guess")
		})
	}

	group.Wait()

	if most := passwords.most.Load(); most != places {
		t.Errorf("%d checks at once, want %d", most, places)
	}
}

// A cookie not signed for the typed username is refused before the user is
// looked up, so its timing tells nothing about which usernames exist.
func TestForgedDeviceCookieReadsNothing(t *testing.T) {
	passwords := &stampedPasswords{stamps: map[string]string{"admin": "stamp-1"}}
	auth := testAuth(passwords, 1)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	login := httptest.NewRecorder()
	auth.setDeviceCookie(login, httptest.NewRequest(http.MethodPost, "/login", nil), "admin", now)

	cookie := login.Result().Cookies()[0]
	passwords.reads = 0

	for _, attempt := range []struct{ value, username string }{
		{"99999999999.00.00", "admin"},
		{"99999999999.00", "admin"},
		{cookie.Value, "nobody"},
	} {
		if auth.knownDevice(deviceRequest(attempt.value), attempt.username, now) {
			t.Fatalf("%q counts for %s", attempt.value, attempt.username)
		}
	}

	if passwords.reads != 0 {
		t.Errorf("forged cookies read the user %d times", passwords.reads)
	}
}

// The right password clears its network's (or device's) count and takes
// itself off the username's, in the window it was counted in only.
func TestSucceededClearsCounts(t *testing.T) {
	limiter := newLoginLimiter(slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

	for _, test := range []struct {
		knownDevice        bool
		network, usernames loginCounter
		networkKey         string
	}{
		{false, limiter.byAddress, limiter.byUsername, network(1).String() + "/32"},
		{true, limiter.byDevice, limiter.byKnownUsername, network(1).String() + "/32\x00admin"},
	} {
		var last loginAttempt

		for range MaxLoginFailures - 1 {
			last, _ = limiter.begin(network(1), "admin", test.knownDevice, now)
		}

		limiter.succeeded(last)

		if count := test.network.count(test.networkKey, now); count != 0 {
			t.Errorf("known device %v: the network keeps %d attempts", test.knownDevice, count)
		}

		if count := test.usernames.count("admin", now); count != MaxLoginFailures-2 {
			t.Errorf("known device %v: the username has %d attempts, want %d", test.knownDevice, count, MaxLoginFailures-2)
		}
	}

	// An attempt that ends after its window must not lower the next one.
	early, _ := limiter.begin(network(2), "dev", false, now)
	later := now.Add(LoginWindow)
	limiter.begin(network(3), "dev", false, later)
	limiter.succeeded(early)

	if count := limiter.byUsername.count("dev", later); count != 1 {
		t.Errorf("the next window's count is %d, want 1", count)
	}
}

// The right password of one user takes only itself off a network's count
// that holds guesses at other usernames: an insider cannot clear them.
func TestSucceededKeepsOthersGuesses(t *testing.T) {
	limiter := newLoginLimiter(slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	guesses := 0

	for cycle := range 5 {
		for index := range MaxLoginFailures - 1 {
			if _, allowed := limiter.begin(network(7), fmt.Sprintf("victim%d", cycle+index), false, now); allowed {
				guesses++
			}
		}

		if own, allowed := limiter.begin(network(7), "mallory", false, now); allowed {
			limiter.succeeded(own)
		}
	}

	if guesses > MaxLoginFailures {
		t.Fatalf("one network made %d guesses in a window, want at most %d", guesses, MaxLoginFailures)
	}
}

// A full address counter refuses a new network and still counts known ones.
func TestFullAddressCounter(t *testing.T) {
	limiter := newLoginLimiter(slog.New(slog.DiscardHandler))
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	limiter.swept = now

	limiter.begin(network(1), "admin", false, now)

	for index := range maxLimiterKeys {
		limiter.byAddress[fmt.Sprintf("made-up-%d", index)] = &loginWindowState{start: now, count: 1}
	}

	if _, allowed := limiter.begin(network(2), "admin", false, now); allowed {
		t.Fatal("a new network is counted past the full counter")
	}

	if _, allowed := limiter.begin(network(1), "admin", false, now); !allowed {
		t.Fatal("a counted network is refused")
	}
}
