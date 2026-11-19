// The IP allowlist and the API token; ingest is open, SDKs authenticate with DSN keys.

package server

import (
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ClientIP returns the address of the client that made the request.
//
// Behind a reverse proxy, RemoteAddr is the proxy. Only then the client comes
// from X-Forwarded-For: each proxy appends the address it received the request
// from, so the list is read from the right, skipping trusted proxies. The first
// untrusted address is the client. Entries to the left of it could have been
// written by the client itself, so they are never used.
func ClientIP(r *http.Request, trustedProxies []netip.Prefix) netip.Addr {
	remote := remoteAddr(r)

	if !contains(trustedProxies, remote) {
		return remote
	}

	forwarded := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")

	for index := len(forwarded) - 1; index >= 0; index-- {
		address, ok := forwardedAddr(strings.TrimSpace(forwarded[index]))

		if !ok {
			// A malformed entry: stop rather than trust anything to the left of it.
			return remote
		}

		if !contains(trustedProxies, address) {
			return address
		}
	}

	return remote
}

// forwardedAddr reads an X-Forwarded-For entry; some proxies add the port
// ("203.0.113.7:51234", "[2001:db8::1]:443") or brackets.
func forwardedAddr(entry string) (netip.Addr, bool) {
	if address, err := netip.ParseAddr(strings.Trim(entry, "[]")); err == nil {
		return comparable(address), true
	}

	if addressPort, err := netip.ParseAddrPort(entry); err == nil {
		return comparable(addressPort.Addr()), true
	}

	return netip.Addr{}, false
}

// comparable is an address as the configured networks hold it: IPv4 not
// mapped into IPv6, and no zone, which a prefix never contains.
func comparable(address netip.Addr) netip.Addr {
	return address.Unmap().WithZone("")
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)

	if err != nil {
		host = r.RemoteAddr
	}

	address, err := netip.ParseAddr(host)

	if err != nil {
		return netip.Addr{}
	}

	return comparable(address)
}

// FromTrustedProxy reports whether the request came directly from a trusted
// reverse proxy, so its X-Forwarded-* headers can be believed.
func FromTrustedProxy(r *http.Request, trustedProxies []netip.Prefix) bool {
	return contains(trustedProxies, remoteAddr(r))
}

// IsHTTPS reports whether the client talks to us over HTTPS, directly or
// through a trusted proxy that says so.
func IsHTTPS(r *http.Request, trustedProxies []netip.Prefix) bool {
	if r.TLS != nil {
		return true
	}

	return FromTrustedProxy(r, trustedProxies) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// PublicURL is where clients reach this server: publicURL (PUBLIC_URL) when
// set, since SDKs and agents may use another address than the user's
// browser, otherwise the address of this request.
func PublicURL(r *http.Request, publicURL string, trustedProxies []netip.Prefix) *url.URL {
	if public, err := url.Parse(publicURL); err == nil && public.Host != "" {
		return &url.URL{Scheme: public.Scheme, Host: public.Host, Path: strings.TrimRight(public.Path, "/")}
	}

	scheme := "http"

	if IsHTTPS(r, trustedProxies) {
		scheme = "https"
	}

	return &url.URL{Scheme: scheme, Host: r.Host}
}

// ClientNetwork is what rate limits count a client as: its IPv4 address, or
// the /64 of an IPv6 one, since one host usually holds a whole /64.
func ClientNetwork(address netip.Addr) netip.Prefix {
	address = comparable(address)

	if address.Is4() {
		return netip.PrefixFrom(address, 32)
	}

	network, _ := address.Prefix(64)

	return network
}

func contains(networks []netip.Prefix, address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}

	for _, network := range networks {
		if network.Contains(address) {
			return true
		}
	}

	return false
}

// RequireAllowedIP rejects clients outside allowedIPs with 403, logging at
// debug level the address it saw. An empty list allows everyone.
func RequireAllowedIP(logger *slog.Logger, allowedIPs, trustedProxies []netip.Prefix, next http.Handler) http.Handler {
	if len(allowedIPs) == 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client := ClientIP(r, trustedProxies); !contains(allowedIPs, client) {
			logger.Debug("refused by ALLOWED_IPS", "client", client, "peer", remoteAddr(r), "path", r.URL.Path)
			JSONError(w, http.StatusForbidden, "access from this address is not allowed")

			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequireToken accepts requests with "Authorization: Bearer <token>".
// With an empty token every request is rejected: the API stays closed until
// API_TOKEN is configured.
func RequireToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			JSONError(w, http.StatusUnauthorized, "API is disabled: set API_TOKEN")
			return
		}

		header := r.Header.Get("Authorization")

		const prefix = "Bearer "

		if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) ||
			subtle.ConstantTimeCompare([]byte(strings.TrimSpace(header[len(prefix):])), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="exceptic"`)
			JSONError(w, http.StatusUnauthorized, "invalid or missing API token")

			return
		}

		next.ServeHTTP(w, r)
	})
}
