package server

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// specialPurpose are networks an address chosen by a client must not reach,
// besides loopback, private and link-local ones (netip's methods): fetching
// it must not become a way into this server's own network (SSRF).
var specialPurpose = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

// PublicAddress: address is on the public internet.
func PublicAddress(address netip.Addr) bool {
	address = address.Unmap()

	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}

	for _, network := range specialPurpose {
		if network.Contains(address) {
			return false
		}
	}

	return true
}

// PublicClient sends requests to addresses a client chose (an OAuth
// client's metadata, a chat webhook): it connects only to public
// addresses, checked after DNS resolution (so a name that resolves to a
// private address, even on a second lookup, is refused), follows no
// redirects, gives up after timeout and keeps no connection open.
func PublicClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: timeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)

			if err != nil {
				return err
			}

			if parsed, err := netip.ParseAddr(host); err != nil || !PublicAddress(parsed) {
				return fmt.Errorf("%s is not a public address", host)
			}

			return nil
		},
	}

	return &http.Client{
		Timeout:       timeout,
		Transport:     &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: timeout, Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
