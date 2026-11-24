package store

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrNotFound is returned when the requested record does not exist.
var ErrNotFound = errors.New("not found")

type Issue struct {
	ID         string     `json:"id"`
	ProjectID  int        `json:"project_id"`
	Title      string     `json:"title"`
	Culprit    string     `json:"culprit"`
	Level      string     `json:"level"`
	Status     string     `json:"status"`
	EventCount int        `json:"event_count"`
	FirstSeen  time.Time  `json:"first_seen"`
	LastSeen   time.Time  `json:"last_seen"`
	ResolvedAt *time.Time `json:"resolved_at"`
	// IgnoredUntil: an ignored issue is unresolved again from this time.
	IgnoredUntil *time.Time `json:"ignored_until,omitempty"`
	// IgnoredUntilCount: an ignored issue is unresolved again once EventCount reaches it.
	IgnoredUntilCount int `json:"ignored_until_count,omitempty"`
	// NotStored counts the events over the cap of one issue a minute
	// (config.EventsPerIssuePerMinute): counted in EventCount, not stored.
	NotStored int `json:"not_stored,omitempty"`
	// Stored counts the events stored now; the cleanup deletes some.
	Stored int `json:"stored"`
}

// IgnoreExpired says whether the issue's ignore time has passed at now.
func (issue Issue) IgnoreExpired(now time.Time) bool {
	return issue.Status == StatusIgnored && issue.IgnoredUntil != nil && !now.Before(*issue.IgnoredUntil)
}

// At is the issue as it reads at now: one whose ignore time has passed is
// unresolved, stored so at its next event or status change.
func (issue Issue) At(now time.Time) Issue {
	if issue.IgnoreExpired(now) {
		issue.Status, issue.IgnoredUntil, issue.IgnoredUntilCount = StatusUnresolved, nil, 0
	}

	return issue
}

// IgnoreEventsLeft is how many more events end the ignore; 0 without that bound.
func (issue Issue) IgnoreEventsLeft() int {
	if issue.Status != StatusIgnored || issue.IgnoredUntilCount == 0 {
		return 0
	}

	return max(issue.IgnoredUntilCount-issue.EventCount, 1)
}

// EventSummary is an event without its payload, for lists.
type EventSummary struct {
	ID           int64     `json:"-"`
	ProjectID    int       `json:"project_id"`
	IssueID      string    `json:"issue_id"`
	EventID      string    `json:"event_id"`
	Timestamp    time.Time `json:"timestamp"`
	ReceivedAt   time.Time `json:"received_at"`
	Level        string    `json:"level"`
	Environment  string    `json:"environment"`
	Release      string    `json:"release"`
	TraceID      string    `json:"trace_id"`
	SpanID       string    `json:"span_id"`
	ParentSpanID string    `json:"parent_span_id"`
	// Thinned: the cleanup kept only the exception, message and the basics of
	// this old event to save space.
	Thinned bool `json:"thinned,omitempty"`
}

// Event is a stored event with its payload.
type Event struct {
	EventSummary
	Payload json.RawMessage `json:"payload"`
}

type LogRecord struct {
	ID             int64           `json:"id"`
	ProjectID      int             `json:"project_id"`
	Timestamp      time.Time       `json:"timestamp"`
	Level          string          `json:"level"`
	SeverityNumber int             `json:"severity_number"`
	Body           string          `json:"body"`
	TraceID        string          `json:"trace_id"`
	SpanID         string          `json:"span_id"`
	Attributes     json.RawMessage `json:"attributes"`
}

// IssueFilter selects issues. Zero values mean "any".
type IssueFilter struct {
	ProjectIDs []int    // any of these projects
	Levels     []string // any of these levels
	Status     string
	Query      string // substring of the title or culprit
	Sort       string // one of IssueSorts; "" is SortLastSeen
	// SeenSince and SeenUntil bound the date given by DateField to
	// [SeenSince, SeenUntil). Zero means no bound.
	SeenSince time.Time
	SeenUntil time.Time
	DateField string // DateAnyEvent (default), DateFirstSeen or DateLastSeen
	Limit     int
	Offset    int // at most MaxOffset
}

// MaxOffset bounds a page offset: further than any list goes, and offset+limit cannot overflow.
const MaxOffset = 1<<31 - 1

// Which date of an issue a date range applies to.
const (
	// DateAnyEvent: the issue had at least one event in the range, its first or last seen included.
	DateAnyEvent = "event"
	// DateFirstSeen: the issue appeared in the range (new problems).
	DateFirstSeen = "first_seen"
	// DateLastSeen: the latest event of the issue is in the range.
	DateLastSeen = "last_seen"
)

// DateFields are the IssueFilter.DateField values ("" is DateAnyEvent).
var DateFields = []string{DateAnyEvent, DateFirstSeen, DateLastSeen}

// The IssueFilter.Sort values: a field, newest or most first, or with _asc
// the reverse (the project name A to Z, _desc the reverse).
const (
	SortLastSeen      = "last_seen"
	SortLastSeenAsc   = "last_seen_asc"
	SortFirstSeen     = "first_seen"
	SortFirstSeenAsc  = "first_seen_asc"
	SortEventCount    = "event_count"
	SortEventCountAsc = "event_count_asc"
	SortProject       = "project"
	SortProjectDesc   = "project_desc"
)

// IssueSorts are the IssueFilter.Sort values.
var IssueSorts = []string{
	SortProject, SortProjectDesc, SortLastSeen, SortLastSeenAsc,
	SortFirstSeen, SortFirstSeenAsc, SortEventCount, SortEventCountAsc,
}

// EventFilter selects events. Events are listed newest first; BeforeID
// continues a previous page (keyset pagination, stable while events arrive).
type EventFilter struct {
	IssueID  string
	TraceID  string
	BeforeID int64
	Limit    int
}

// LogFilter selects log records, newest first, with keyset pagination like events.
type LogFilter struct {
	ProjectIDs []int    // any of these projects
	Levels     []string // any of these levels
	Query      string   // substring of the body
	TraceID    string
	SpanIDs    []string // any of these spans (used with TraceID for one request)
	Since      time.Time
	Until      time.Time
	BeforeID   int64
	FromID     int64 // only logs with this id or a greater one (LogCount.FromID)
	Limit      int
	Offset     int // page offset (list_logs takes it or BeforeID)
}

// LogScan says whether a log scan stopped at config.LogSearchTime and how far
// it got: the logs with an id below Rest were not searched (BeforeID = Rest
// goes on), those received at or after SearchedTo all were.
type LogScan struct {
	Stopped    bool
	Rest       int64
	SearchedTo time.Time
}

// LogCount is a log count; More says it stopped at LogCountLimit. A count
// that read the logs gives FromID: a list of its matches searches no further
// back (LogFilter.FromID), the oldest it found or, stopped, its Rest.
type LogCount struct {
	Count  int
	More   bool
	FromID int64
	LogScan
}

// LogPage is what a log list found within its scan.
type LogPage struct {
	Logs []LogRecord
	LogScan
}

// LogCountLimit is where counting logs stops: an exact count of millions of
// matches reads them all, while a pager only needs to know there are many.
var LogCountLimit = 10000

// LogIssue is the issue of the error raised in the same request as a log line.
type LogIssue struct {
	IssueID   string
	ProjectID int
	Title     string
}

// Page trims rows fetched with one extra (Limit = limit+1) back to limit and
// reports whether there are more: the extra row tells that a next page exists.
func Page[T any](rows []T, limit int) (page []T, more bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}

	return rows, false
}
