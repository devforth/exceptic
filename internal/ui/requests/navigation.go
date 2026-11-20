package requests

import "net/http"

// IsHTMX reports whether htmx sent the request (a swap, not a full page load).
func IsHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// HTMXTarget is the id of the element an htmx request replaces ("" for a full page load).
func HTMXTarget(r *http.Request) string {
	if !IsHTMX(r) {
		return ""
	}

	return r.Header.Get("HX-Target")
}
