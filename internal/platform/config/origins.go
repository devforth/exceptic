package config

import (
	"fmt"
	"regexp"
	"strings"
)

// MaxAllowedOrigins is how many origins a project may list.
const MaxAllowedOrigins = 50

// originRe is scheme://host[:port], the host may start with "*." for its subdomains.
var originRe = regexp.MustCompile(`^https?://(\*\.)?([a-z0-9.-]+|\[[0-9a-f:]+\])(:[0-9]{1,5})?$`)

// ParseOrigin reads an origin a project allows: https://app.example.com,
// http://localhost:3000, or https://*.example.com for its subdomains.
// Origins are compared exactly, so a path or a trailing slash is an error.
func ParseOrigin(entry string) (string, error) {
	origin := strings.ToLower(strings.TrimSpace(entry))

	if !originRe.MatchString(origin) {
		return "", fmt.Errorf("%q is not scheme://host[:port], e.g. https://app.example.com or https://*.example.com", entry)
	}

	return origin, nil
}

// OriginAllowed: a browser of origin may send to the project. A project
// without allowed origins takes any; a request without an Origin header (a
// backend SDK) is not a browser's and is not checked.
func (project Project) OriginAllowed(origin string) bool {
	if len(project.AllowedOrigins) == 0 || origin == "" {
		return true
	}

	origin = strings.ToLower(origin)

	for _, allowed := range project.AllowedOrigins {
		if origin == allowed || subdomainOf(origin, allowed) {
			return true
		}
	}

	return false
}

// subdomainOf: origin is a subdomain of the pattern scheme://*.domain[:port],
// with the same scheme and port.
func subdomainOf(origin, pattern string) bool {
	scheme, rest, found := strings.Cut(pattern, "://*.")

	if !found {
		return false
	}

	host, ok := strings.CutPrefix(origin, scheme+"://")

	return ok && strings.HasSuffix(host, "."+rest) && !strings.ContainsAny(strings.TrimSuffix(host, "."+rest), ":/")
}
