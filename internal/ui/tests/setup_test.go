package tests

import (
	"github.com/devforth/exceptic/internal/services"
)

// The slow password key derivation would take seconds per test server under
// the race detector.
func init() {
	services.PasswordIterations = 1_000
}
