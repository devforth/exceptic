package grouping

import (
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsGroupingValues: docs/sdk.md (Grouping) states these numbers of the code.
func TestDocsGroupingValues(t *testing.T) {
	fixtures.DocValues(t, "docs/sdk.md",
		fixtures.DocValue{Phrase: "A source line over {} characters", Value: maxSourceLineLength},
		fixtures.DocValue{Phrase: "At most {} values are read", Value: maxFingerprintValues},
		fixtures.DocValue{Phrase: "grouped by its outermost {} exceptions", Value: maxGroupExceptions},
	)
}
