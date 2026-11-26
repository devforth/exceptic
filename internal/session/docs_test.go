package session

import (
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsLoginLimit: docs/deployment.md (Login limit) states these numbers of the limiter.
func TestDocsLoginLimit(t *testing.T) {
	fixtures.DocValues(t, "docs/deployment.md",
		fixtures.DocValue{Phrase: "counts before the password is checked, in {}-minute windows", Value: int(LoginWindow.Minutes())},
		fixtures.DocValue{Phrase: "a signed cookie for {} days", Value: int(deviceLifetime.Hours() / 24)},
		fixtures.DocValue{Phrase: "A known device gets {} attempts per network", Value: MaxLoginFailures},
		fixtures.DocValue{Phrase: "all known devices together {} per email", Value: maxKnownDeviceFailures},
		fixtures.DocValue{Phrase: "per email from all addresses, {} each", Value: MaxLoginFailures},
		fixtures.DocValue{Phrase: "guessing one password from many addresses gets {} tries", Value: MaxLoginFailures},
		fixtures.DocValue{Phrase: "no place within {} seconds", Value: int(passwordCheckWait.Seconds())},
		fixtures.DocValue{Phrase: "Past {} counted addresses or emails", Value: fixtures.Thousands(maxLimiterKeys)},
	)
}
