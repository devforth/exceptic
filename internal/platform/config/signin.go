package config

import (
	"errors"
	"strings"
)

// Google is the Google sign-in of the web app: an OAuth client of Google
// Cloud whose redirect URI is GoogleRedirectPath under PUBLIC_URL. Empty:
// no Google sign-in.
type Google struct {
	ClientID, ClientSecret string
	// AuthURL and TokenURL point the sign-in at a fake Google (tests); empty: Google's.
	AuthURL, TokenURL string
}

// GoogleRedirectPath is where Google sends the browser back after a sign-in.
const GoogleRedirectPath = "/auth/google/callback"

// Enabled: users may sign in with Google.
func (google Google) Enabled() bool { return google.ClientID != "" }

// parseGoogle reads GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET: both or none,
// and only with PUBLIC_URL, which Google's registered redirect URI is built from.
func parseGoogle(getenv func(string) string, publicURL string) (Google, error) {
	google := Google{ClientID: strings.TrimSpace(getenv("GOOGLE_CLIENT_ID")), ClientSecret: strings.TrimSpace(getenv("GOOGLE_CLIENT_SECRET"))}

	switch {
	case google.ClientID == "" && google.ClientSecret == "":
		return Google{}, nil
	case google.ClientID == "" || google.ClientSecret == "":
		return Google{}, errors.New("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET: set both or neither")
	case publicURL == "":
		return Google{}, errors.New("GOOGLE_CLIENT_ID: set PUBLIC_URL too, Google sends the browser back to " + GoogleRedirectPath + " under it")
	}

	return google, nil
}
