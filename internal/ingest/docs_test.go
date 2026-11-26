package ingest

import (
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

type docValue = fixtures.DocValue

// TestDocsIngestValues: what the documents say of ingest's own limits comes from here.
func TestDocsIngestValues(t *testing.T) {
	cookies := "`" + strings.Join(harmlessCookieNames[:len(harmlessCookieNames)-1], "`, `") + "` and `" + harmlessCookieNames[len(harmlessCookieNames)-1] + "`"

	fixtures.DocValues(t, "docs/configuration.md",
		docValue{Phrase: "its name {} bytes and type", Value: maxLogAttributeNameBytes},
		docValue{Phrase: "bytes and type {}; the trace", Value: maxLogAttributeTypeBytes},
		docValue{Phrase: "the trace and span IDs {} bytes", Value: maxLogIDBytes},
		docValue{Phrase: "**Stack traces:** at most {} frames each", Value: oldestFrames + newestFrames},
		docValue{Phrase: "each, the {} oldest and the", Value: oldestFrames},
		docValue{Phrase: "oldest and the {} newest (a recursion", Value: newestFrames},
		docValue{Phrase: "recursions of any depth over {} frames are one issue", Value: oldestFrames + newestFrames},
		docValue{Phrase: "A stack with more than {} frames that keep their source", Value: detailedFrames},
		docValue{Phrase: "only for the {} oldest and", Value: detailedFrames / 2},
		docValue{Phrase: "oldest and {} newest of them", Value: detailedFrames / 2},
		docValue{Phrase: "envelopes arriving during it get `429` with `Retry-After: {}`", Value: errCompacting.retryAfter},
	)
	fixtures.DocValues(t, "docs/sdk.md",
		docValue{Phrase: "cookies one by one (all but {}), also", Value: cookies},
		docValue{Phrase: "more than {} breadcrumbs)", Value: maxBreadcrumbs},
		docValue{Phrase: "A time more than {} days before the server got it", Value: int(maxTimeInPast.Hours() / 24)},
		docValue{Phrase: "when too many bodies are being read, {} while a compaction", Value: errCompacting.retryAfter},
		docValue{Phrase: "retries later: {} seconds when its ingest queue is full", Value: inboxFullRetry},
		docValue{Phrase: "queue is full, {} when too many bodies", Value: errBusy.retryAfter},
		docValue{Phrase: "what it sent in the last {} hours (by the hour)", Value: dropHours},
	)
	fixtures.DocValues(t, "docs/ui.md",
		docValue{Phrase: "What the project sent in the last {} hours that was not stored", Value: dropHours},
		docValue{Phrase: "dropped for the first time in {} hours", Value: dropHours},
	)
}
