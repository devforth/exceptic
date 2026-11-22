package ingest

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/devforth/exceptic/internal/ingest/grouping"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// StoreSink groups events into issues and maps events and logs to the records
// the pipeline stores, then reports what storing changed. It is the bridge
// between the Sentry format and the store, which knows nothing about Sentry.
type StoreSink struct {
	Logger *slog.Logger
	// Notifier is told about new and regressed issues. Nil sends nothing.
	Notifier Notifier
	// PublicURL is the UI address for links in notifications; empty means no link.
	PublicURL string
	// Recording gives the log levels stored (a project may set its own); logs
	// of other levels are dropped, events of every level are stored. The zero
	// value stores every level.
	Recording config.Recording
}

// Notifier queues alerts without blocking ingest (see package notify).
type Notifier interface {
	Notify(notification notify.Notification)
}

// eventRecord maps an event to what is stored; events of every level are.
func (sink StoreSink) eventRecord(project config.Project, event *sentry.Event, received time.Time) store.NewEvent {
	// Stored under its canonical name: filters and notifications know only those.
	event.Level = config.EventLevel(event.Level)

	trace := event.Trace()

	return store.NewEvent{
		ProjectID:    project.ID,
		EventID:      event.EventID,
		Fingerprint:  grouping.Fingerprint(event),
		Title:        event.Title(),
		Culprit:      grouping.Culprit(event),
		Level:        event.Level,
		Environment:  event.Environment,
		Release:      event.Release,
		TraceID:      trace.TraceID,
		SpanID:       trace.SpanID,
		ParentSpanID: trace.ParentSpanID,
		Timestamp:    event.Timestamp.Time,
		ReceivedAt:   received,
		Payload:      event.Raw,
	}
}

// report logs what storing an event changed and notifies of new and
// regressed issues.
func (sink StoreSink) report(project config.Project, event *sentry.Event, result store.SaveResult, receivedAt time.Time) {
	attributes := []any{"project", project.Name, "issue_id", result.IssueID, "event_id", event.EventID, "title", event.Title()}

	switch {
	case result.Duplicate:
		sink.Logger.Debug("duplicate event ignored", attributes...)
	case result.NewIssue:
		sink.Logger.Info("new issue", attributes...)
		sink.notify(notify.NewIssue, project, result.IssueID, event, receivedAt)
	case result.Regression:
		sink.Logger.Info("issue regressed", attributes...)
		sink.notify(notify.Regression, project, result.IssueID, event, receivedAt)
	default:
		sink.Logger.Debug("event stored", attributes...)
	}
}

func (sink StoreSink) notify(kind notify.Kind, project config.Project, issueID string, event *sentry.Event, receivedAt time.Time) {
	if sink.Notifier == nil {
		return
	}

	issueURL := ""

	if sink.PublicURL != "" {
		issueURL = fmt.Sprintf("%s/issues/%s", sink.PublicURL, issueID)
	}

	sink.Notifier.Notify(notify.Notification{
		Kind:         kind,
		ProjectID:    project.ID,
		Project:      project.Name,
		ProjectColor: project.Color,
		IssueID:      issueID,
		IssueURL:     issueURL,
		Title:        event.Title(),
		Culprit:      grouping.Culprit(event),
		Level:        event.Level,
		Environment:  event.Environment,
		Release:      event.Release,
		Time:         receivedAt,
		EventID:      event.EventID,
	})
}

// logRecords maps logs to what is stored, without the levels not recorded
// for the project.
func (sink StoreSink) logRecords(project config.Project, logs []sentry.Log) ([]store.NewLog, error) {
	records := make([]store.NewLog, 0, len(logs))
	levels := sink.Recording.LogLevelsFor(project.LogLevels)

	for _, record := range logs {
		level := config.LogLevel(record.Level)

		if !config.Records(levels, level) {
			continue
		}

		attributes, err := json.Marshal(record.Attributes)

		if err != nil {
			return nil, fmt.Errorf("log attributes: %w", err)
		}

		records = append(records, store.NewLog{
			ProjectID:      project.ID,
			Timestamp:      record.Timestamp.Time,
			Level:          level,
			SeverityNumber: record.SeverityNumber,
			Body:           record.Body,
			TraceID:        record.TraceID,
			SpanID:         record.SpanID,
			Attributes:     attributes,
			Fingerprint:    grouping.LogFingerprint(record.Body),
		})
	}

	return records, nil
}
