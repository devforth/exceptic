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
	StatusIgnored:    "no notifications; for good, or until the time or number of events chosen when ignoring",
}

// IgnoreBound ends an ignore by itself: the issue is unresolved again after
// For, or once it has had Events more events. The zero value ignores for good.
type IgnoreBound struct {
	For    time.Duration
	Events int
}

// IgnoreDurations are the times an issue may be ignored for, by name.
var IgnoreDurations = []string{"1h", "24h", "7d", "30d"}

// MaxIgnoreEvents is the most events an ignore may wait for.
const MaxIgnoreEvents = 1_000_000

// ParseIgnoreFor reads a name of IgnoreDurations; "" is no time bound.
func ParseIgnoreFor(name string) (time.Duration, bool) {
	switch name {
	case "":
		return 0, true
	case "1h":
		return time.Hour, true
	case "24h":
		return 24 * time.Hour, true
	case "7d":
		return 7 * 24 * time.Hour, true
	case "30d":
		return 30 * 24 * time.Hour, true
	}

	return 0, false
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
	// Regression: the issue was resolved and happened again, or its ignore
	// ended (its time passed or this was the last event it waited for), so it is unresolved now.
	Regression bool
	// NotStored: the issue had its events of this minute, so this one only counts.
	NotStored bool
}

// ErrorFields are the top-level fields of an event's payload that describe
// the error: what an old event keeps when the cleanup thins it, and a large
// one at the last of the size cuts of ingest (with its contexts).
var ErrorFields = map[string]bool{
	"event_id": true, "timestamp": true, "level": true, "platform": true, "logger": true, "transaction": true,
	"release": true, "environment": true, "message": true, "logentry": true, "exception": true, "fingerprint": true,
	"stacktrace": true, "threads": true,
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
