package store

import (
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"
)

// Issue statuses.
const (
	StatusUnresolved = "unresolved"
	StatusResolved   = "resolved"
	StatusIgnored    = "ignored"
	// StatusAll is the filter value that lists issues of every status.
	StatusAll = "all"
)

// IssueStatuses are the statuses an issue takes.
var IssueStatuses = []string{StatusUnresolved, StatusResolved, StatusIgnored}

// StatusMeanings say what each status means, for the UI and the agents' tools.
var StatusMeanings = map[string]string{
	StatusUnresolved: "still happening or not looked at yet",
	StatusResolved:   "fixed, reopens if the error happens again",
	StatusIgnored:    "no notifications, stays ignored",
}

// NewEvent is an event ready to store: already parsed and grouped.
// The store does not know the Sentry format; the caller fills this in.
type NewEvent struct {
	ProjectID   int
	EventID     string
	Fingerprint string
	Title       string
	Culprit     string
	Level       string
	Environment string
	Release     string
	TraceID     string
	// SpanID is the span the error was captured in, ParentSpanID its parent
	// (usually the HTTP request span). Logs of the request carry one of them.
	SpanID       string
	ParentSpanID string
	Timestamp    time.Time // when it happened, by the SDK's clock
	ReceivedAt   time.Time // when the server got it
	Payload      []byte    // full event JSON
}

// An issue keeps at most this much of an event's title and culprit: titles
// come from error messages and can be huge.
const (
	MaxTitleLength   = 500
	MaxCulpritLength = 300
)

// IssueText is the title and culprit an issue keeps of event.
func (event NewEvent) IssueText() (title, culprit string) {
	return textutil.Shorten(event.Title, MaxTitleLength), textutil.Shorten(event.Culprit, MaxCulpritLength)
}

// SaveResult tells what storing an event changed; StoreSink notifies of new issues and regressions from it.
type SaveResult struct {
	IssueID string
	// Duplicate: an event with this event_id was already stored (an SDK retry); nothing changed.
	Duplicate bool
	// NewIssue: the first event of a new issue.
	NewIssue bool
	// Regression: the issue was resolved and happened again, so it is unresolved now.
	Regression bool
}

// NewLog is a log record ready to store.
type NewLog struct {
	ProjectID      int
	Timestamp      time.Time
	Level          string
	SeverityNumber int
	Body           string
	TraceID        string
	SpanID         string // the span the log was written in
	Attributes     []byte // JSON object
	// Fingerprint groups repeats of the same message (grouping.LogFingerprint).
	Fingerprint string
}
