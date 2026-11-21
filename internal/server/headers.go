package server

import (
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ContentSecurityPolicy is the policy of a page: scripts and styles only from
// this server, plus scriptSources (hashes of the page's inline scripts).
func ContentSecurityPolicy(scriptSources []string) string {
	scripts := strings.Join(append([]string{"'self'"}, scriptSources...), " ")

	return "default-src 'self'; script-src " + scripts + "; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; form-action 'self'; base-uri 'none'"
}

// SecurityHeaders: the policy, no MIME sniffing, no framing by other sites
// (clickjacking), and referrers only to this server. same-origin, not
// no-referrer: with no-referrer browsers send "Origin: null" on same-origin
// POSTs, which SameOrigin rejects.
func SecurityHeaders(header http.Header, policy string) {
	header.Set("Content-Security-Policy", policy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("X-Frame-Options", "DENY")
}

// SameOrigin: the request carries an Origin of this server's host, and of
// https when the request came over HTTPS (a page served over plain HTTP on
// the same host, a network attacker's, must not act on the HTTPS site). A
// page of another site cannot forge Origin. oauth compares with PUBLIC_URL
// instead, since agents may reach the server at another address than the browser.
func SameOrigin(r *http.Request, trustedProxies []netip.Prefix) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))

	return err == nil && origin.Host != "" && strings.EqualFold(origin.Host, r.Host) &&
		(!IsHTTPS(r, trustedProxies) || strings.EqualFold(origin.Scheme, "https"))
}

// SafeNext keeps a redirect target from the request (the post-login ?next=,
// the "return" of bulk actions) on this site: only a local path is accepted,
// so a crafted link cannot send the user elsewhere. Browsers drop tabs and
// newlines from a URL and read a backslash as "/" ("/<tab>/evil.com" opens
// evil.com), so control characters, spaces and backslashes are refused, and
// the rest must parse as a path without a scheme or host.
func SafeNext(next string) string {
	const fallback = "/issues"

	if strings.ContainsFunc(next, func(char rune) bool { return char <= ' ' || char == 0x7f || char == '\\' }) {
		return fallback
	}

	target, err := url.Parse(next)

	if err != nil || target.Scheme != "" || target.Host != "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return fallback
	}

	return next
}
