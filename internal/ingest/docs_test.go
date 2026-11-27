package ingest

import (
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

type docValue = fixtures.DocValue

// TestDocsIngestValues: what the documents say of ingest's own limits comes from here.
func TestDocsIngestValues(t *testing.T) {
	fixtures.DocValues(t, "docs/sdk.md",
		docValue{Phrase: "(more than {} breadcrumbs)", Value: maxBreadcrumbs},
		docValue{Phrase: "what each project sent in the last {} hours", Value: dropHours},
	)
	fixtures.DocValues(t, "docs/ui.md",
		docValue{Phrase: "What the project sent in the last {} hours that was not stored", Value: dropHours},
		docValue{Phrase: "(first time in {} hours)", Value: dropHours},
	)
}
