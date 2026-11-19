package ingest

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

// parseItems parses the events and logs of an envelope; other item types are
// dropped. Any malformed item fails the whole envelope.
func parseItems(logger *slog.Logger, project config.Project, envelope *sentry.Envelope, received time.Time) ([]*sentry.Event, []sentry.Log, error) {
	var (
		events []*sentry.Event
		logs   []sentry.Log
	)

	dropped := map[string]int{}
	drift := clockDrift(envelope.Header.SentAt, received)

	for index, item := range envelope.Items {
		switch item.Header.Type {
		case sentry.ItemEvent:
			// Credentials (cookies, Authorization, tokens) are removed before
			// anything is parsed or stored (scrub.go), and so is what
			// does not help a fix, such as library source (compact.go).
			event, err := sentry.ParseEvent(prepareEvent(item.Payload), envelope.Header.EventID, received)

			if err != nil {
				return nil, nil, fmt.Errorf("item %d: %w", index+1, err)
			}

			event.Timestamp.Time = sdkTime(event.Timestamp.Time, received, drift)

			events = append(events, event)
		case sentry.ItemLog:
			batch, err := sentry.ParseLogs(scrubJSON(item.Payload), received)

			if err != nil {
				return nil, nil, fmt.Errorf("item %d: %w", index+1, err)
			}

			for index := range batch {
				batch[index].Timestamp.Time = sdkTime(batch[index].Timestamp.Time, received, drift)
			}

			logs = append(logs, batch...)
		default:
			dropped[item.Header.Type]++
		}
	}

	if len(dropped) > 0 {
		logger.Debug("dropped unsupported items", "project", project.Name, "types", dropped)
	}

	return events, logs, nil
}

// Times outside this window around the receive time are replaced by it, as
// Sentry's Relay does: a wrong unit (milliseconds) or a broken clock, and
// years the store cannot encode.
const (
	maxTimeInPast   = 30 * 24 * time.Hour
	maxTimeInFuture = time.Minute
)

// sdkTime is the time of an event or log moved by the clock drift; received
// when the SDK sent none (the parser filled in received) or one out of range.
func sdkTime(sent, received time.Time, drift time.Duration) time.Time {
	if sent.Equal(received) {
		return received
	}

	moved := sent.Add(drift)

	if moved.Before(received.Add(-maxTimeInPast)) || moved.After(received.Add(maxTimeInFuture)) {
		return received
	}

	return moved
}

// clockDrift is how far the SDK's clock was behind the server's when it sent
// the envelope (sent_at), so the times of its events and logs can be moved by
// it; nothing under a minute, which network delays explain.
func clockDrift(sentAt string, received time.Time) time.Duration {
	sent, err := time.Parse(time.RFC3339Nano, sentAt)

	if err != nil {
		return 0
	}

	if drift := received.Sub(sent); drift.Abs() >= time.Minute {
		return drift
	}

	return 0
}
