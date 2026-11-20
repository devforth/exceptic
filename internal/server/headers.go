package server

import (
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// ContentSecurityPolicy is the policy of a page: scripts and styles only from
// this server, plus scriptSources (hashes of the page's inline scripts);
// formTargets are other origins a form may send the browser to (the OAuth
// consent page returns to the agent; browsers apply form-action to redirects too).
func ContentSecurityPolicy(scriptSources []string, formTargets ...string) string {
	scripts := strings.Join(append([]string{"'self'"}, scriptSources...), " ")
	forms := strings.Join(append([]string{"'self'"}, formTargets...), " ")

	return "default-src 'self'; script-src " + scripts + "; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; form-action " +
		forms + "; base-uri 'none'"
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
