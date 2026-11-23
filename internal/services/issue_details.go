package services

import (
	"context"
	"errors"

	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// EventState is what the issue page can show of the event it asked for.
type EventState string

const (
	EventShown      EventState = "shown"      // read and parsed
	EventUnreadable EventState = "unreadable" // stored, but its payload does not parse
	EventMissing    EventState = "missing"    // not stored (the cleanup deleted it, or another issue's)
	EventNone       EventState = "none"       // the issue has no stored event
)

// EventStates are every EventState.
var EventStates = []EventState{EventShown, EventUnreadable, EventMissing, EventNone}

// RecentEventsShown is how many events of an issue its page lists at first.
const RecentEventsShown = 20

// IssueDetails is what the issue page shows.
type IssueDetails struct {
	Issue   store.Issue
	History []store.StatusChange
	// Events are the newest, one more than RecentEventsShown when there are more.
	Events     []store.EventSummary
	EventState EventState
	// Event is the selected one, when stored; Parsed only when it reads.
	Event           store.Event
	Parsed          *sentry.Event
	RelatedLogs     []store.LogRecord
	LogsFromRequest bool
	TraceHasLogs    bool
}

// Details reads an issue with one of its events: eventID (UUID or hex), the
// latest when empty. A missing issue is store.ErrNotFound; a missing,
// foreign or unreadable event is a state of the page, not an error.
func (issues *Issues) Details(ctx context.Context, access store.Access, issueID, eventID string) (IssueDetails, error) {
	var details IssueDetails

	if !store.ValidID(issueID) {
		return details, store.ErrNotFound
	}

	issue, err := issues.Get(ctx, access, issueID)

	if err != nil {
		return details, err
	}

	details.Issue = issue

	if details.History, err = issues.History(ctx, issue); err != nil {
		return details, err
	}

	if details.Events, err = issues.Events(ctx, access, store.EventFilter{IssueID: issue.ID, Limit: RecentEventsShown + 1}); err != nil {
		return details, err
	}

	event, err := issues.SelectedEvent(ctx, issue, eventID)

	switch {
	case errors.Is(err, store.ErrNotFound) && len(details.Events) > 0:
		details.EventState = EventMissing

		return details, nil
	case errors.Is(err, store.ErrNotFound):
		details.EventState = EventNone

		return details, nil
	case err != nil:
		return details, err
	}

	details.Event = event
	// Stored payloads were valid when received, so a parse error here is unexpected.
	details.Parsed, err = sentry.ParseEvent(event.Payload, event.EventID, event.ReceivedAt)

	if err != nil {
		issues.logger.Warn("stored event payload does not parse", "event_id", event.EventID, "err", err)

		details.Parsed, details.EventState = nil, EventUnreadable

		return details, nil
	}

	details.EventState = EventShown

	if details.RelatedLogs, details.LogsFromRequest, err = issues.RelatedLogs(ctx, access, event, details.Parsed); err != nil {
		return details, err
	}

	details.TraceHasLogs, err = issues.TraceLogs(ctx, access, event)

	return details, err
}
