package api

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// issueFiltersInput is what list_issues and count_issues filter on.
type issueFiltersInput struct {
	Project []string `json:"project,omitempty" jsonschema:"Project names or IDs; every project when empty."`
	Status  string   `json:"status,omitempty" jsonschema:"Issue status, unresolved by default; all for every status."`
	Level   []string `json:"level,omitempty" jsonschema:"Issue levels; every level when empty."`
	Query   string   `json:"query,omitempty" jsonschema:"Text in the issue title or location."`
	Since   string   `json:"since,omitempty" jsonschema:"RFC 3339 time, e.g. 2026-11-11T10:00:00Z: only issues with an event at or after it."`
	Until   string   `json:"until,omitempty" jsonschema:"RFC 3339 time: only issues with an event before it."`
	DateBy  string   `json:"date_by,omitempty" jsonschema:"What since/until apply to; by default any event of the issue, its first or last seen time included."`
}

type listIssuesInput struct {
	issueFiltersInput

	Sort   string `json:"sort,omitempty" jsonschema:"Order, last_seen by default: a field newest or most first, with _asc the reverse; project A to Z, project_desc the reverse."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Issues per page."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page."`
}

// statusFilters are the values of list_issues' status: "all" lists every status.
var statusFilters = append(slices.Clone(store.IssueStatuses), store.StatusAll)

type issueOutput struct {
	ID         string `json:"id"`
	Project    string `json:"project"`
	Title      string `json:"title"`
	Location   string `json:"location,omitempty"`
	Level      string `json:"level"`
	Status     string `json:"status"`
	EventCount int    `json:"event_count"`
	// NotStored of EventCount: over the events an issue stores a minute.
	NotStored int `json:"not_stored,omitempty"`
	// StoredEvents are the events stored now, what list_issue_events reaches.
	StoredEvents int        `json:"stored_events"`
	FirstSeen    time.Time  `json:"first_seen"`
	LastSeen     time.Time  `json:"last_seen"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
	// IgnoredUntil and IgnoreEventsLeft end an ignore: then it is unresolved again.
	IgnoredUntil     *time.Time `json:"ignored_until,omitempty"`
	IgnoreEventsLeft int        `json:"ignore_events_left,omitempty"`
	// MCPReference is the request that has a coding agent fetch the issue
	// ("Copy for agent"); the web app's only, agents know the tool.
	MCPReference string `json:"mcp_reference,omitempty"`
}

func (handler *handlers) issueOutput(call Call, issue store.Issue) issueOutput {
	output := issueOutput{
		ID: issue.ID, Project: handler.projectName(issue.ProjectID), Title: call.Text(issue.Title), Location: call.Text(issue.Culprit),
		Level: issue.Level, Status: issue.Status, EventCount: issue.EventCount, NotStored: issue.NotStored, StoredEvents: issue.Stored, FirstSeen: issue.FirstSeen, LastSeen: issue.LastSeen, ResolvedAt: issue.ResolvedAt,
		IgnoredUntil: issue.IgnoredUntil, IgnoreEventsLeft: issue.IgnoreEventsLeft(),
	}

	if call.Transport == TransportUI {
		output.MCPReference = agentprompt.MCPReference(handler.services.AppName, issue.ID)
	}

	return output
}

// projectName is empty for data of a deleted project.
func (handler *handlers) projectName(projectID int) string {
	project, _ := handler.projects.List().ByID(projectID)

	return project.Name
}

// visibleProjects are the projects the call sees: names and IDs of the
// others are unknown to it, as those of projects that do not exist.
func (handler *handlers) visibleProjects(call Call) config.Projects {
	return handler.services.Projects.Visible(call.Access())
}

func (handler *handlers) listIssues(ctx context.Context, call Call, input listIssuesInput) (Page[issueOutput], error) {
	output := Page[issueOutput]{Items: []issueOutput{}}
	filter, err := handler.issueFilter(call, input)

	if err != nil {
		return output, err
	}

	limit := filter.Limit
	filter.Limit++ // one extra row tells whether there is a next page
	issues, err := handler.services.Issues.List(ctx, call.Access(), filter)

	if err != nil {
		return output, err
	}

	issues, more := store.Page(issues, limit)

	for _, issue := range issues {
		output.Items = append(output.Items, handler.issueOutput(call, issue))
	}

	if more {
		output.NextCursor = strconv.Itoa(filter.Offset + limit)
	}

	return output, nil
}

func (handler *handlers) countIssues(ctx context.Context, call Call, input issueFiltersInput) (countOutput, error) {
	filter, err := handler.issueFilter(call, listIssuesInput{issueFiltersInput: input})

	if err != nil {
		return countOutput{}, err
	}

	count, more, err := handler.services.Issues.Count(ctx, call.Access(), filter)

	return countOutput{Count: count, More: more}, err
}

func (handler *handlers) listIssueLevels(ctx context.Context, call Call, _ noInput) (levelsOutput, error) {
	present, err := handler.services.Issues.Levels(ctx, call.Access())

	return levelsOutput{Levels: append([]string{}, config.InLevelOrder(present, config.Levels)...)}, err
}

type projectNamesOutput struct {
	// Projects are names, in the order of the projects' list.
	Projects []string `json:"projects"`
}

func (handler *handlers) listIssueProjects(ctx context.Context, call Call, _ noInput) (projectNamesOutput, error) {
	ids, err := handler.services.Issues.Projects(ctx, call.Access())

	return handler.projectNames(ids), err
}

// projectNames are the names of the projects of ids, in the projects' order.
func (handler *handlers) projectNames(ids []int) projectNamesOutput {
	output := projectNamesOutput{Projects: []string{}}

	for _, project := range handler.services.Projects.List() {
		if slices.Contains(ids, project.ID) {
			output.Projects = append(output.Projects, project.Name)
		}
	}

	return output
}

func (handler *handlers) issueFilter(call Call, input listIssuesInput) (store.IssueFilter, error) {
	filter := store.IssueFilter{Query: strings.TrimSpace(input.Query), Sort: input.Sort, DateField: input.DateBy}

	var err error

	if filter.ProjectIDs, err = handler.projectIDs(call, input.Project); err != nil {
		return filter, err
	}

	switch input.Status {
	case "":
		filter.Status = store.StatusUnresolved
	case store.StatusAll:
	default:
		filter.Status = input.Status
	}

	filter.Levels = input.Level

	if filter.SeenSince, err = parseTime("since", input.Since); err != nil {
		return filter, err
	}

	if filter.SeenUntil, err = parseTime("until", input.Until); err != nil {
		return filter, err
	}

	filter.Limit = pageLimit(input.Limit)
	filter.Offset, err = parseOffset(input.Cursor)

	return filter, err
}

type getIssueInput struct {
	IssueID string `json:"issue_id" jsonschema:"Issue ID, as list_issues returns it."`
	EventID string `json:"event_id,omitempty" jsonschema:"An event of this issue (see list_issue_events); its latest event when empty."`
}

func (handler *handlers) getIssue(ctx context.Context, call Call, input getIssueInput) (Text, error) {
	issue, err := handler.services.Issues.Get(ctx, call.Access(), input.IssueID)

	if err != nil {
		return "", err
	}

	event, err := handler.services.Issues.SelectedEvent(ctx, issue, input.EventID)

	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}

	// Asked for an event the issue does not have: say so rather than
	// describe another one.
	if err != nil && input.EventID != "" {
		return "", notFound("event %s of issue %s", input.EventID, issue.ID)
	}

	return handler.describe(ctx, call, issue, event)
}

type getEventInput struct {
	Project string `json:"project" jsonschema:"Project name or ID."`
	EventID string `json:"event_id" jsonschema:"Event ID: 32 hex characters, with or without dashes."`
}

func (handler *handlers) findEvent(ctx context.Context, call Call, input getEventInput) (store.Event, error) {
	project, err := handler.services.Projects.Find(call.Access(), input.Project)

	if err != nil {
		return store.Event{}, err
	}

	event, err := handler.services.Issues.Event(ctx, call.Access(), project.ID, input.EventID)

	if errors.Is(err, store.ErrNotFound) {
		return event, notFound("event %s of project %q", input.EventID, input.Project)
	}

	return event, err
}

func (handler *handlers) getEvent(ctx context.Context, call Call, input getEventInput) (Text, error) {
	event, err := handler.findEvent(ctx, call, input)

	if err != nil {
		return "", err
	}

	issue, err := handler.services.Issues.Get(ctx, call.Access(), event.IssueID)

	if err != nil {
		return "", err
	}

	return handler.describe(ctx, call, issue, event)
}

// getEventPayload is REST only: the payload is the event as stored.
func (handler *handlers) getEventPayload(ctx context.Context, call Call, input getEventInput) (store.Event, error) {
	return handler.findEvent(ctx, call, input)
}

// relatedLogsLimit is how many logs of its request an event's prompt holds.
const relatedLogsLimit = 20

// describe writes an issue and one of its events (none when the events are
// gone) as a full agent prompt, after a line of IDs for follow-up calls.
// The prompt is built from an allowlist and redacted (package agentprompt),
// whoever asks.
func (handler *handlers) describe(ctx context.Context, call Call, issue store.Issue, event store.Event) (Text, error) {
	var (
		parsed  *sentry.Event
		related services.RelatedLogs
		err     error
	)

	ids := []string{fmt.Sprintf("Issue ID: %s", issue.ID)}

	if event.EventID != "" {
		if parsed, err = sentry.ParseEvent(event.Payload, event.EventID, event.ReceivedAt); err != nil {
			return "", err
		}

		if related, err = handler.services.Issues.RelatedLogsLimit(ctx, call.Access(), event, parsed, relatedLogsLimit+1); err != nil {
			return "", err
		}

		ids = append(ids, "Event ID: "+event.EventID)

		if traceID := agentprompt.TraceID(event.TraceID); traceID != "" {
			ids = append(ids, "Trace ID: "+traceID)
		}
	} else {
		ids = append(ids, "No stored event: only the issue history is known.")
	}

	logs := related.Logs

	if related.Stopped {
		ids = append(ids, fmt.Sprintf("Related logs: the search stopped after %s; there may be more (list_logs with trace_id).", logSearchTime))
	}

	if len(logs) > relatedLogsLimit {
		logs = logs[len(logs)-relatedLogsLimit:] // oldest first: keep the newest
		ids = append(ids, fmt.Sprintf("Related logs: the %d most recent matches; use filtered list_logs for more.", relatedLogsLimit))
	}

	input := services.AgentInput(handler.projectName(issue.ProjectID), issue, parsed, logs, related.FromRequest)

	return Text(strings.Join(ids, "\n") + "\n\n" + agentprompt.Build(input)), nil
}

type listIssueEventsInput struct {
	IssueID string `json:"issue_id" jsonschema:"Issue ID."`
	Limit   int    `json:"limit,omitempty" jsonschema:"Events per page."`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page."`
}

type eventOutput struct {
	EventID     string    `json:"event_id"`
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Environment string    `json:"environment,omitempty"`
	Release     string    `json:"release,omitempty"`
	TraceID     string    `json:"trace_id,omitempty"`
	// ReceivedAt is when the server got it; Timestamp is the device's time.
	ReceivedAt time.Time `json:"received_at"`
}

// eventsPage is a page of events read with one extra row, which tells
// whether there is a next one.
func eventsPage(call Call, events []store.EventSummary, limit int) Page[eventOutput] {
	output := Page[eventOutput]{Items: []eventOutput{}}
	events, more := store.Page(events, limit)

	for _, event := range events {
		output.Items = append(output.Items, eventOutput{
			EventID: event.EventID, Timestamp: event.Timestamp, Level: event.Level, ReceivedAt: event.ReceivedAt,
			Environment: call.Text(event.Environment), Release: call.Text(event.Release), TraceID: call.TraceID(event.TraceID),
		})
	}

	if more {
		output.NextCursor = strconv.FormatInt(events[limit-1].ID, 10)
	}

	return output
}

func (handler *handlers) listIssueEvents(ctx context.Context, call Call, input listIssueEventsInput) (Page[eventOutput], error) {
	output := Page[eventOutput]{Items: []eventOutput{}}

	if _, err := handler.services.Issues.Get(ctx, call.Access(), input.IssueID); err != nil {
		return output, err
	}

	limit := pageLimit(input.Limit)
	beforeID, err := parseCursor(input.Cursor)

	if err != nil {
		return output, err
	}

	events, err := handler.services.Issues.Events(ctx, call.Access(), store.EventFilter{IssueID: input.IssueID, BeforeID: beforeID, Limit: limit + 1})

	if err != nil {
		return output, err
	}

	return eventsPage(call, events, limit), nil
}

type issueIDsInput struct {
	IssueIDs []string `json:"issue_ids" jsonschema:"1 to 100 known issue IDs. Duplicates are handled once."`
}

type issueBatchOutput struct {
	Issues          []issueOutput `json:"issues"`
	MissingIssueIDs []string      `json:"missing_issue_ids,omitempty"`
	// ForbiddenIssueIDs exist in projects outside the caller's.
	ForbiddenIssueIDs []string `json:"forbidden_issue_ids,omitempty"`
}

func (handler *handlers) batchOutput(call Call, ids []string, issues []store.Issue, hiddenIDs []string) issueBatchOutput {
	byID := make(map[string]store.Issue, len(issues))

	for _, issue := range issues {
		byID[issue.ID] = issue
	}

	output := issueBatchOutput{Issues: []issueOutput{}}

	for _, id := range ids {
		switch issue, found := byID[id]; {
		case found:
			output.Issues = append(output.Issues, handler.issueOutput(call, issue))
		case slices.Contains(hiddenIDs, id):
			output.ForbiddenIssueIDs = append(output.ForbiddenIssueIDs, id)
		default:
			output.MissingIssueIDs = append(output.MissingIssueIDs, id)
		}
	}

	return output
}

func (handler *handlers) getIssues(ctx context.Context, call Call, input issueIDsInput) (issueBatchOutput, error) {
	ids, err := store.NormalizeIssueIDs(input.IssueIDs)

	if err != nil {
		return issueBatchOutput{}, Invalid("%s", err)
	}

	issues, hiddenIDs, err := handler.services.Issues.GetMany(ctx, call.Access(), ids)

	if err != nil {
		return issueBatchOutput{}, err
	}

	return handler.batchOutput(call, ids, issues, hiddenIDs), nil
}

// ignoreBoundInput bounds an ignore; both empty ignore for good.
type ignoreBoundInput struct {
	IgnoreFor         string `json:"ignore_for,omitempty" jsonschema:"With ignore only: unresolved again after this time."`
	IgnoreUntilEvents int    `json:"ignore_until_events,omitempty" jsonschema:"With ignore only: unresolved again once it happens this many more times."`
}

func (input ignoreBoundInput) bound() (store.IgnoreBound, error) {
	duration, ok := store.ParseIgnoreFor(input.IgnoreFor)

	if !ok {
		return store.IgnoreBound{}, Invalid("unknown ignore_for %q", input.IgnoreFor)
	}

	return store.IgnoreBound{For: duration, Events: input.IgnoreUntilEvents}, nil
}

type updateIssueStatusInput struct {
	IssueIDs []string `json:"issue_ids" jsonschema:"1 to 100 issue IDs; one ID for a single issue."`
	Status   string   `json:"status" jsonschema:"The status of every issue in the call."`
	ignoreBoundInput
}

// updateIssueStatus changes one or many issues at once: all or none (an
// issue outside the call's projects is forbidden, a missing one not found).
// The change is recorded in the issues' history with the call's actor.
func (handler *handlers) updateIssueStatus(ctx context.Context, call Call, input updateIssueStatusInput) (issueBatchOutput, error) {
	ids, err := store.NormalizeIssueIDs(input.IssueIDs)

	if err != nil {
		return issueBatchOutput{}, Invalid("%s", err)
	}

	bound, err := input.bound()

	if err != nil {
		return issueBatchOutput{}, err
	}

	before, _, err := handler.services.Issues.GetMany(ctx, call.Access(), ids)

	if err != nil {
		return issueBatchOutput{}, err
	}

	issues, err := handler.services.Issues.SetStatus(ctx, call.Access(), ids, input.Status, bound, call.Actor)

	if errors.Is(err, services.ErrBoundWithoutIgnore) {
		return issueBatchOutput{}, Invalid("%s", err)
	}

	if err != nil {
		return issueBatchOutput{}, err
	}

	handler.audit(ctx, call, issueChanges(before, issues, false)...)

	return handler.batchOutput(call, ids, issues, nil), nil
}

type deleteIssueInput struct {
	IssueID string `json:"issue_id" jsonschema:"Issue ID."`
}

type deletedOutput struct {
	Deleted string `json:"deleted"`
}

func (handler *handlers) deleteIssue(ctx context.Context, call Call, input deleteIssueInput) (deletedOutput, error) {
	issue, err := handler.services.Issues.Get(ctx, call.Access(), input.IssueID)

	if err != nil {
		return deletedOutput{}, err
	}

	if err := handler.services.Issues.Delete(ctx, call.Access(), input.IssueID); err != nil {
		return deletedOutput{}, err
	}

	handler.audit(ctx, call, issueChanges([]store.Issue{issue}, nil, true)...)

	return deletedOutput{Deleted: input.IssueID}, nil
}

type applyIssueActionInput struct {
	Action   string   `json:"action" jsonschema:"What to do with the issues."`
	IssueIDs []string `json:"issue_ids" jsonschema:"The issues; issues deleted meanwhile are skipped."`
	ignoreBoundInput
}

type appliedOutput struct {
	Action string `json:"action"`
	// Issues is how many issues were asked for.
	Issues int `json:"issues"`
}

// applyIssueAction runs an action of the issue list or page for the
// viewer; deleting needs an admin, which services.Issues checks.
func (handler *handlers) applyIssueAction(ctx context.Context, call Call, input applyIssueActionInput) (appliedOutput, error) {
	ids := slices.Compact(slices.Sorted(slices.Values(input.IssueIDs)))

	switch {
	case len(ids) == 0:
		return appliedOutput{}, Invalid("no issues given")
	case len(ids) > services.MaxActionIssues:
		return appliedOutput{}, Invalid("at most %d issues at once", services.MaxActionIssues)
	case slices.ContainsFunc(ids, func(id string) bool { return !store.ValidID(id) }):
		return appliedOutput{}, Invalid("invalid issue id")
	}

	bound, err := input.bound()

	if err != nil {
		return appliedOutput{}, err
	}

	before, _, err := handler.services.Issues.GetMany(ctx, call.Access(), ids)

	if err != nil {
		return appliedOutput{}, err
	}

	err = handler.services.Issues.ApplyAction(ctx, call.Viewer, input.Action, bound, ids)

	// What changed, also when a later batch failed.
	if after, _, readErr := handler.services.Issues.GetMany(ctx, call.Access(), ids); readErr == nil {
		handler.audit(ctx, call, issueChanges(before, after, input.Action == services.ActionDelete)...)
	}

	if err != nil {
		switch {
		case errors.Is(err, services.ErrUnknownAction):
			return appliedOutput{}, Invalid("unknown action %q", input.Action)
		case errors.Is(err, services.ErrBoundWithoutIgnore):
			return appliedOutput{}, Invalid("%s", err)
		}

		return appliedOutput{}, err
	}

	return appliedOutput{Action: input.Action, Issues: len(ids)}, nil
}
