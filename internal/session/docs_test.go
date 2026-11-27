package session

import (
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsLoginLimit: docs/deployment.md (Securing it) states these numbers of the limiter.
func TestDocsLoginLimit(t *testing.T) {
	fixtures.DocValues(t, "docs/deployment.md",
		fixtures.DocValue{Phrase: "After {} wrong passwords in", Value: MaxLoginFailures},
		fixtures.DocValue{Phrase: "wrong passwords in {} minutes a login", Value: int(LoginWindow.Minutes())})
}
