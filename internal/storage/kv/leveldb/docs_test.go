package leveldb

import (
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsJournal: docs/configuration.md (Retention) states the size of the write journal.
func TestDocsJournal(t *testing.T) {
	fixtures.DocValues(t, "docs/configuration.md",
		fixtures.DocValue{Phrase: "the write journal: up to {} MB", Value: options().WriteBuffer >> 20})
}
