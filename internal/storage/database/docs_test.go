package database

import (
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsCleanupValues: docs/configuration.md (Retention) states these numbers of the cleanup.
func TestDocsCleanupValues(t *testing.T) {
	fixtures.DocValues(t, "docs/configuration.md",
		fixtures.DocValue{Phrase: "(one came in the last {} seconds)", Value: int(defaultQuietFor.Seconds())},
		fixtures.DocValue{Phrase: "starts only once no envelope came for {} seconds", Value: int(defaultQuietFor.Seconds())},
		fixtures.DocValue{Phrase: "It deletes in batches of up to {} records", Value: cleanupChunk},
	)
}
