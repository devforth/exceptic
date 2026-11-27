package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Issues: the issue list and pages, the actions on issues and the logs
// shown with an event.
type Issues struct {
	logger *slog.Logger
	store  store.Store
}

func newIssues(logger *slog.Logger, database store.Store) *Issues {
	return &Issues{logger: logger, store: database}
}

// Every method that reads project data takes the access of the viewer
// (store.User.ProjectAccess; every project for the REST API's token) and
// shows nothing outside it: a record asked for there is ErrHidden (Hidden),
// a missing one store.ErrNotFound.

// Count counts the issues that match filter (its Limit and Offset are
// ignored), up to store.LogCountLimit; more: there are more.
func (issues *Issues) Count(ctx context.Context, access store.Access, filter store.IssueFilter) (int, bool, error) {
	if !access.Restrict(&filter.ProjectIDs) {
		return 0, false, nil
	}

	return issues.store.CountIssues(ctx, filter)
}

// Levels returns the levels at least one issue has, in no order.
func (issues *Issues) Levels(ctx context.Context, access store.Access) ([]string, error) {
	var projectIDs []int

	if !access.Restrict(&projectIDs) {
		return nil, nil
	}

	return issues.store.IssueLevels(ctx, projectIDs)
}

// Projects returns the projects within access that have at least one issue:
// the choices of the issue list's project filter.
func (issues *Issues) Projects(ctx context.Context, access store.Access) ([]int, error) {
	projectIDs, err := issues.store.IssueProjects(ctx)

	return slices.DeleteFunc(projectIDs, func(id int) bool { return !access.Sees(id) }), err
}

// Unresolved counts the unresolved issues: the counter next to Issues in the menu.
// Exact whatever the number, from the counts per project (not a list's count, which stops at its limit).
func (issues *Issues) Unresolved(ctx context.Context, access store.Access) (int, error) {
	counts, err := issues.store.UnresolvedCounts(ctx)
	total := 0

	for project, count := range counts {
		if access.Sees(project) {
			total += count
		}
	}

	return total, err
}

// List returns one page of the issues that match filter.
func (issues *Issues) List(ctx context.Context, access store.Access, filter store.IssueFilter) ([]store.Issue, error) {
	if !access.Restrict(&filter.ProjectIDs) {
		return []store.Issue{}, nil
	}

	return issues.store.ListIssues(ctx, filter)
}

// Get reads an issue; store.ErrNotFound when it does not exist, ErrHidden
// outside access.
func (issues *Issues) Get(ctx context.Context, access store.Access, issueID string) (store.Issue, error) {
	issue, err := issues.store.GetIssue(ctx, issueID)

	switch {
	case errors.Is(err, store.ErrNotFound):
		return store.Issue{}, Missing("issue", issueID)
	case err == nil && !access.Sees(issue.ProjectID):
		return store.Issue{}, Hidden("issue", issueID)
	}

	return issue, err
}

// GetMany reads the issues of ids that exist, in no order, and the ids of
// those outside access.
func (issues *Issues) GetMany(ctx context.Context, access store.Access, issueIDs []string) (visible []store.Issue, hiddenIDs []string, err error) {
	found, err := issues.store.GetIssues(ctx, issueIDs)

	for _, issue := range found {
		if access.Sees(issue.ProjectID) {
			visible = append(visible, issue)
		} else {
			hiddenIDs = append(hiddenIDs, issue.ID)
		}
	}

	slices.Sort(hiddenIDs)

	return visible, hiddenIDs, err
}

// Event reads an event of a project by the ID its SDK reported, in UUID or
// hex form; store.ErrNotFound when it does not exist, ErrHidden for a project
// outside access.
func (issues *Issues) Event(ctx context.Context, access store.Access, projectID int, eventID string) (store.Event, error) {
	if !access.Sees(projectID) {
		return store.Event{}, Hidden("project", strconv.Itoa(projectID))
	}

	return issues.store.GetEvent(ctx, projectID, sentry.NormalizeEventID(eventID))
}

// Events lists the events that match filter (an issue's or a trace's), newest first.
func (issues *Issues) Events(ctx context.Context, access store.Access, filter store.EventFilter) ([]store.EventSummary, error) {
	events, err := issues.store.ListEvents(ctx, filter)

	return slices.DeleteFunc(events, func(event store.EventSummary) bool { return !access.Sees(event.ProjectID) }), err
}

// SelectedEvent is the event the issue page shows: eventID (from ?event=, in
// UUID or hex form) when given, otherwise the latest one. An event of another
// issue is store.ErrNotFound, as a missing one: the page then shows no event
// rather than another issue's. The caller has read issue within access.
func (issues *Issues) SelectedEvent(ctx context.Context, issue store.Issue, eventID string) (store.Event, error) {
	if eventID == "" {
		return issues.store.LatestEvent(ctx, issue.ID)
	}

	event, err := issues.store.GetEvent(ctx, issue.ProjectID, sentry.NormalizeEventID(eventID))

	if err == nil && event.IssueID != issue.ID {
		return store.Event{}, store.ErrNotFound
	}

	return event, err
}

// MaxActionIssues is the most issues one action takes: one page of the
// largest size, with room to spare; more would only hold the database long.
const MaxActionIssues = 1000

// The actions ApplyAction runs on issues.
const (
	ActionResolve = "resolve"
	ActionIgnore  = "ignore"
	ActionReopen  = "reopen"
	ActionDelete  = "delete"
)

// IssueActions are the actions in the order the pages offer them.
var IssueActions = []string{ActionResolve, ActionIgnore, ActionReopen, ActionDelete}

// issueActions maps an action to the status it sets; delete sets none.
var issueActions = map[string]string{
	ActionResolve: store.StatusResolved,
	ActionIgnore:  store.StatusIgnored,
	ActionReopen:  store.StatusUnresolved,
	ActionDelete:  "",
}

var ErrUnknownAction = errors.New("unknown action")

// ErrBoundWithoutIgnore: an ignore bound was given with another status.
var ErrBoundWithoutIgnore = errors.New("ignore_for and ignore_until_events only go with ignoring")

// ApplyAction runs action on the issues for the viewer, who is recorded in
// the status history; deleting needs CanAdminister. Statuses change in
// batches, each all or nothing. An issue that no longer exists (deleted
// from another tab) or is outside the viewer's access is skipped, the
// others are still processed. An ignore ends by bound (the zero value: never).
func (issues *Issues) ApplyAction(ctx context.Context, viewer store.User, action string, bound store.IgnoreBound, issueIDs []string) error {
	status, ok := issueActions[action]

	if !ok {
		return ErrUnknownAction
	}

	if action != ActionIgnore && bound != (store.IgnoreBound{}) {
		return ErrBoundWithoutIgnore
	}

	if action == ActionDelete && !viewer.CanAdminister() {
		return Forbidden("your role may not delete issues")
	}

	access := viewer.ProjectAccess()

	for batch := range slices.Chunk(issueIDs, store.MaxIssueBatch) {
		var err error

		if action == ActionDelete {
			err = issues.deleteBatch(ctx, access, batch)
		} else {
			err = issues.setStatus(ctx, access, batch, status, bound, viewer.Actor(store.SourceUI))
		}

		if err != nil {
			return fmt.Errorf("%s issues: %w", action, err)
		}
	}

	return nil
}

func (issues *Issues) deleteBatch(ctx context.Context, access store.Access, issueIDs []string) error {
	visible, _, err := issues.GetMany(ctx, access, issueIDs)

	if err != nil || len(visible) == 0 {
		return err
	}

	_, err = issues.store.DeleteIssues(ctx, issueIDsOf(visible))

	return err
}

func issueIDsOf(list []store.Issue) []string {
	ids := make([]string, len(list))

	for index, issue := range list {
		ids[index] = issue.ID
	}

	return ids
}

// setStatus changes the issues of a batch that still exist. The batch
// update is all or nothing, so one deleted meanwhile means another try
// without it.
func (issues *Issues) setStatus(ctx context.Context, access store.Access, issueIDs []string, status string, bound store.IgnoreBound, actor store.Actor) error {
	for {
		existing, _, err := issues.GetMany(ctx, access, issueIDs)

		if err != nil || len(existing) == 0 {
			return err
		}

		_, err = issues.writeStatus(ctx, issueIDsOf(existing), status, bound, actor)

		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
}

// SetStatus sets the status of every issue of ids, recorded with actor, and
// returns them: all or nothing. Issues outside access are ErrHidden, checked
// before missing ones (store.ErrNotFound, naming them); projects never
// change, so the check before the write holds. An ignore ends by bound.
func (issues *Issues) SetStatus(ctx context.Context, access store.Access, issueIDs []string, status string, bound store.IgnoreBound, actor store.Actor) ([]store.Issue, error) {
	if status != store.StatusIgnored && bound != (store.IgnoreBound{}) {
		return nil, ErrBoundWithoutIgnore
	}

	visible, hiddenIDs, err := issues.GetMany(ctx, access, issueIDs)

	switch {
	case err != nil:
		return nil, err
	case len(hiddenIDs) > 0:
		return nil, Hidden("issue", hiddenIDs...)
	}

	found := issueIDsOf(visible)
	missing := slices.DeleteFunc(slices.Clone(issueIDs), func(id string) bool { return slices.Contains(found, id) })

	if len(missing) > 0 {
		slices.Sort(missing)

		return nil, Missing("issue", slices.Compact(missing)...)
	}

	return issues.writeStatus(ctx, issueIDs, status, bound, actor)
}

func (issues *Issues) writeStatus(ctx context.Context, issueIDs []string, status string, bound store.IgnoreBound, actor store.Actor) ([]store.Issue, error) {
	if status == store.StatusIgnored {
		return issues.store.IgnoreIssues(ctx, issueIDs, bound, actor)
	}

	return issues.store.SetIssuesStatus(ctx, issueIDs, status, actor)
}

// Delete deletes an issue with its events; store.ErrNotFound when it does
// not exist. It checks no role: the Admin flag of the tool does.
func (issues *Issues) Delete(ctx context.Context, access store.Access, issueID string) error {
	if _, err := issues.Get(ctx, access, issueID); err != nil {
		return err
	}

	deleted, err := issues.store.DeleteIssues(ctx, []string{issueID})

	if err == nil && deleted == 0 {
		err = store.ErrNotFound
	}

	return err
}

// History lists the status changes of an issue (read through an access),
// newest first.
func (issues *Issues) History(ctx context.Context, issue store.Issue) ([]store.StatusChange, error) {
	return issues.store.IssueStatusHistory(ctx, issue.ID)
}

// TraceLogs reports whether the event's trace has logs: its trace ID links
// to them only then. A search that ran out of time links too: the logs page
// says how far it got.
func (issues *Issues) TraceLogs(ctx context.Context, access store.Access, event store.Event) (bool, error) {
	filter := store.LogFilter{TraceID: event.TraceID, Limit: 1}

	if event.TraceID == "" || !access.Restrict(&filter.ProjectIDs) {
		return false, nil
	}

	page, err := issues.store.ListLogs(ctx, filter)

	return len(page.Logs) > 0 || page.Stopped, err
}

// ClockDrift is how far the device's clock was off: the stored time of an
// event is its payload's moved by it, unless ingest replaced a time that
// was missing or out of range by the receive time.
func ClockDrift(event store.Event, parsed *sentry.Event) time.Duration {
	if parsed == nil || parsed.Timestamp.IsZero() || event.Timestamp.Equal(event.ReceivedAt) {
		return 0
	}

	return event.Timestamp.Sub(parsed.Timestamp.Time)
}

// relatedLogsWindow bounds the fallback by time: logs of the same trace this
// close to the error. A browser trace lasts as long as the tab is open.
const relatedLogsWindow = time.Minute

// maxRelatedLogs is how many related logs the issue page shows, and the
// most RelatedLogsLimit gives.
const maxRelatedLogs = 100

// RelatedLogs returns the logs written around an event, oldest first, and
// whether they are exactly the logs of its request.
//
// A log written during the request carries the span of the route handler (the
// event's span) or of the HTTP request (the event's parent span). Without span
// matches (SDKs that send no spans), it falls back to logs of the same trace
// within relatedLogsWindow of the error, by the server's time or the device's
// (parsed): logs sent without sent_at keep the device's.
func (issues *Issues) RelatedLogs(ctx context.Context, access store.Access, event store.Event, parsed *sentry.Event) (RelatedLogs, error) {
	return issues.RelatedLogsLimit(ctx, access, event, parsed, maxRelatedLogs)
}

// RelatedLogs are the logs written around an event, oldest first: FromRequest
// when they are of its request (by span), Stopped when the search ran out of
// time (config.LogSearchTime), so there may be more.
type RelatedLogs struct {
	Logs        []store.LogRecord
	FromRequest bool
	Stopped     bool
}

// RelatedLogsLimit is RelatedLogs with at most limit logs, the newest.
func (issues *Issues) RelatedLogsLimit(ctx context.Context, access store.Access, event store.Event, parsed *sentry.Event, limit int) (RelatedLogs, error) {
	limit = min(max(limit, 1), maxRelatedLogs)

	var projectIDs []int

	if event.TraceID == "" || !access.Restrict(&projectIDs) {
		return RelatedLogs{}, nil
	}

	var (
		logs        []store.LogRecord
		fromRequest bool
		err         error
	)

	var spans []string

	for _, span := range []string{event.SpanID, event.ParentSpanID} {
		if span != "" {
			spans = append(spans, span)
		}
	}

	// A span search that ran out of time is not followed by the trace's: the
	// page would wait for both.
	stopped := false

	if len(spans) > 0 {
		var page store.LogPage

		page, err = issues.store.ListLogs(ctx, store.LogFilter{ProjectIDs: projectIDs, TraceID: event.TraceID, SpanIDs: spans, Limit: limit})
		logs, stopped = page.Logs, page.Stopped
		fromRequest = len(logs) > 0
	}

	if err == nil && len(logs) == 0 && !stopped {
		logs, stopped, err = issues.logsAround(ctx, projectIDs, event, parsed, limit)
	}

	// Oldest first: read what happened in order.
	slices.Reverse(logs)

	return RelatedLogs{Logs: logs, FromRequest: fromRequest, Stopped: stopped}, err
}

// logsAround are the newest logs of the event's trace, at most limit,
// within relatedLogsWindow of its time by the server's clock or by the
// device's: one window around each, not the time between them.
func (issues *Issues) logsAround(ctx context.Context, projectIDs []int, event store.Event, parsed *sentry.Event, limit int) ([]store.LogRecord, bool, error) {
	times := []time.Time{event.Timestamp}

	if drift := ClockDrift(event, parsed); drift != 0 {
		times = append(times, event.Timestamp.Add(-drift))
	}

	slices.SortFunc(times, time.Time.Compare)

	// One search from the earliest to the latest: dates bound a search by when the logs
	// arrived too, and a device's log arrives now with its own time. Of those, only the
	// logs within a window of either time are kept, not the ones between.
	page, err := issues.store.ListLogs(ctx, store.LogFilter{
		ProjectIDs: projectIDs, TraceID: event.TraceID,
		Since: times[0].Add(-relatedLogsWindow), Until: times[len(times)-1].Add(relatedLogsWindow),
		Limit: limit * len(times) * relatedLogsSpread,
	})

	if err != nil {
		return nil, false, err
	}

	logs := slices.DeleteFunc(page.Logs, func(record store.LogRecord) bool {
		return !slices.ContainsFunc(times, func(at time.Time) bool {
			return record.Timestamp.Sub(at).Abs() <= relatedLogsWindow
		})
	})

	return logs[:min(len(logs), limit)], page.Stopped, nil
}

// relatedLogsSpread: a search over a device's clock drift reads this many times the
// logs it shows, so the logs between the two windows do not take their place.
const relatedLogsSpread = 10

// AgentPrompt is the text of the "Prompt" button: what a coding agent needs
// to explain the error, without what identifies servers or people.
func AgentPrompt(project string, issue store.Issue, event *sentry.Event, relatedLogs []store.LogRecord, fromRequest bool) string {
	return agentprompt.Build(AgentInput(project, issue, event, relatedLogs, fromRequest))
}

// AgentInput is what an agent may learn about an issue: the prompt and the
// MCP server describe the same allowlisted fields.
func AgentInput(project string, issue store.Issue, event *sentry.Event, relatedLogs []store.LogRecord, fromRequest bool) agentprompt.Input {
	logs := make([]agentprompt.Log, 0, len(relatedLogs))

	for _, record := range relatedLogs {
		logs = append(logs, agentprompt.Log{Time: record.Timestamp, Level: record.Level, Body: record.Body})
	}

	return agentprompt.Input{
		Project: project, Title: issue.Title, Level: issue.Level, Culprit: issue.Culprit, Event: event,
		Issue: &agentprompt.Issue{Status: issue.Status, Events: issue.EventCount, FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen},
		Logs:  logs, LogsFromRequest: fromRequest,
	}
}
