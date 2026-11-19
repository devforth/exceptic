package oauth

import (
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/devforth/exceptic/internal/server"
)

// guard protects every OAuth endpoint and MCP itself. Native agents omit
// Origin; browser requests must use this server's public origin.
func (authorization *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Add("Vary", "Origin")

		if !authorization.sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (authorization *Server) sameOrigin(r *http.Request) bool {
	origins := r.Header.Values("Origin")

	if len(origins) == 0 {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}

	if len(origins) != 1 {
		return false
	}

	origin, err := url.Parse(origins[0])

	if err != nil || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.ForceQuery || strings.Contains(origins[0], "#") {
		return false
	}

	public := server.PublicURL(r, authorization.PublicURL, authorization.TrustedProxies)

	return origin.Scheme == public.Scheme && strings.EqualFold(origin.Hostname(), public.Hostname()) && originPort(origin) == originPort(public)
}

func originPort(origin *url.URL) string {
	if port := origin.Port(); port != "" {
		return port
	}

	if origin.Scheme == "https" {
		return "443"
	}

	return "80"
}

// requestLimit bounds the requests of each client network per minute, and
// of all together when total is set. Its map is capped and reset each
// minute; past the cap new networks share one overflow count, so a flood of
// networks is still limited without locking out the networks counted.
const (
	limitNetworks = 4096
	limitOverflow = 10 // the overflow count allows this many times perMinute
)

// retryAfter is the Retry-After of a refused registration or token request, in seconds.
var retryAfter = strconv.Itoa(int(time.Minute.Seconds()))

type requestLimit struct {
	mutex     sync.Mutex
	start     time.Time
	counts    map[netip.Prefix]int
	overflow  int
	all       int
	perMinute int
	total     int // all networks together per minute; 0 for none
}

func (authorization *Server) limit(limit *requestLimit, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limit.allow(server.ClientIP(r, authorization.TrustedProxies), authorization.now()) {
			w.Header().Set("Retry-After", retryAfter)
			server.WriteJSON(w, http.StatusTooManyRequests, oauthError("temporarily_unavailable", "too many requests: try again later"))

			return
		}

		next(w, r)
	})
}

func (limit *requestLimit) allow(address netip.Addr, now time.Time) bool {
	limit.mutex.Lock()
	defer limit.mutex.Unlock()

	start := now.Truncate(time.Minute)

	if !limit.start.Equal(start) {
		limit.start, limit.overflow, limit.all = start, 0, 0
		limit.counts = make(map[netip.Prefix]int)
	}

	if limit.total > 0 && limit.all >= limit.total {
		return false
	}

	network := server.ClientNetwork(address)

	if count, counted := limit.counts[network]; counted || len(limit.counts) < limitNetworks {
		if count >= limit.perMinute {
			return false
		}

		limit.counts[network]++
	} else {
		if limit.overflow >= limitOverflow*limit.perMinute {
			return false
		}

		limit.overflow++
	}

	limit.all++

	return true
}

func uniqueParams(params url.Values) *Error {
	for name, values := range params {
		if len(values) != 1 {
			return oauthError("invalid_request", "duplicate parameter: "+name)
		}
	}

	return nil
}

// PKCE verifiers are 43 to 128 unreserved ASCII characters (RFC 7636).
const minVerifier, maxVerifier = 43, 128

func validVerifier(verifier string) bool {
	if len(verifier) < minVerifier || len(verifier) > maxVerifier {
		return false
	}

	for _, character := range verifier {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && !strings.ContainsRune("-._~", character) {
			return false
		}
	}

	return true
}

func validPort(port string) bool {
	if port == "" {
		return true
	}

	number, err := strconv.Atoi(port)

	return err == nil && number > 0 && number <= 65535
}
