package config

import "testing"

func TestParseOrigin(t *testing.T) {
	for entry, want := range map[string]string{
		" https://App.example.com ": "https://app.example.com",
		"http://localhost:3500":     "http://localhost:3500",
		"http://[::1]:8080":         "http://[::1]:8080",
		"https://*.example.com":     "https://*.example.com",
	} {
		if origin, err := ParseOrigin(entry); err != nil || origin != want {
			t.Errorf("%q: %q %v, want %q", entry, origin, err, want)
		}
	}

	for _, entry := range []string{"https://app.example.com/", "app.example.com", "*", "https://app.*.com", "ftp://example.com", ""} {
		if _, err := ParseOrigin(entry); err == nil {
			t.Errorf("%q accepted", entry)
		}
	}
}

func TestOriginAllowed(t *testing.T) {
	project := Project{AllowedOrigins: []string{"https://app.example.com", "https://*.example.org", "http://*.local.test:8080"}}

	for origin, want := range map[string]bool{
		"":                             true, // a backend SDK
		"https://App.example.com":      true,
		"https://admin.example.com":    false,
		"http://app.example.com":       false,
		"https://eu.example.org":       true,
		"https://a.b.example.org":      true,
		"https://example.org":          false,
		"https://evilexample.org":      false,
		"https://eu.example.org:8443":  false,
		"http://web.local.test:8080":   true,
		"http://web.local.test":        false,
		"https://example.org.evil.com": false,
	} {
		if got := project.OriginAllowed(origin); got != want {
			t.Errorf("%q: %v, want %v", origin, got, want)
		}
	}

	if !(Project{}).OriginAllowed("https://anywhere.example") {
		t.Error("a project without origins refused a site")
	}
}
