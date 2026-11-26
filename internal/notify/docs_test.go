package notify

import (
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
)

// TestDocsNotifyValues: the documents state these numbers of the notifier.
func TestDocsNotifyValues(t *testing.T) {
	var retries []string

	for _, delay := range defaultSchedule[:3] {
		retries = append(retries, delay.String())
	}

	fixtures.DocValues(t, "docs/configuration.md",
		fixtures.DocValue{Phrase: "is retried ({}, … then hourly)", Value: strings.Join(retries, ", ")})
	fixtures.DocValues(t, "docs/ui.md",
		fixtures.DocValue{Phrase: "It sends at most {} a minute for each project", Value: defaultMaxPerWindow},
		fixtures.DocValue{Phrase: "more issues in the last {}\")", Value: config.HumanDuration(defaultWindow)})
}
