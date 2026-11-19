package ingest

import (
	"fmt"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

// A device whose clock is off: the times of its events and logs are moved by
// how far sent_at is from the receive time; under a minute, or without a time
// of their own, they stay.
func TestClockDrift(t *testing.T) {
	received := time.Date(2026, 11, 19, 13, 0, 0, 0, time.UTC)
	behind := received.Add(-3 * time.Hour)
	body := func(sentAt time.Time, eventTime string) []byte {
		return []byte(fmt.Sprintf(`{"sent_at":%q}`, sentAt.Format(time.RFC3339Nano)) + "\n" +
			`{"type":"event"}` + "\n" + `{"message":"slow"` + eventTime + `}` + "\n" +
			`{"type":"log"}` + "\n" + fmt.Sprintf(`{"items":[{"timestamp":%d,"level":"info","body":"line"}]}`, behind.Add(-time.Minute).Unix()))
	}

	for _, test := range []struct {
		name            string
		sentAt          time.Time
		eventTime       string
		event, logEntry time.Time
	}{
		{"3 hours behind", behind, fmt.Sprintf(`,"timestamp":%d`, behind.Add(-time.Minute).Unix()), received.Add(-time.Minute), received.Add(-time.Minute)},
		{"within a minute", received.Add(-30 * time.Second), fmt.Sprintf(`,"timestamp":%d`, behind.Unix()), behind, behind.Add(-time.Minute)},
		{"no event time", behind, "", received, received.Add(-time.Minute)},
	} {
		envelope, err := sentry.ParseEnvelope(body(test.sentAt, test.eventTime))

		if err != nil {
			t.Fatal(err)
		}

		events, logs, err := parseItems(slog.New(slog.DiscardHandler), config.Project{ID: 1}, envelope, received)

		if err != nil || len(events) != 1 || len(logs) != 1 {
			t.Fatalf("%s: %v", test.name, err)
		}

		if !events[0].Timestamp.Equal(test.event) || !logs[0].Timestamp.Equal(test.logEntry) {
			t.Errorf("%s: event %s, log %s", test.name, events[0].Timestamp.UTC(), logs[0].Timestamp.UTC())
		}
	}
}

// A time in milliseconds instead of seconds, or from a broken clock, is
// replaced by the receive time: years past 9999 cannot even be stored.
func TestOutOfRangeTimes(t *testing.T) {
	received := time.Date(2026, 11, 19, 13, 0, 0, 0, time.UTC)

	for _, test := range []struct {
		name string
		time string
		want time.Time
	}{
		{"milliseconds", strconv.FormatInt(received.UnixMilli(), 10), received},
		{"an hour ahead", strconv.FormatInt(received.Add(time.Hour).Unix(), 10), received},
		{"40 days old", strconv.FormatInt(received.Add(-40*24*time.Hour).Unix(), 10), received},
		{"a day old", strconv.FormatInt(received.Add(-24*time.Hour).Unix(), 10), received.Add(-24 * time.Hour)},
		{"year 10000 as text", `"10000-01-01T00:00:00Z"`, received},
	} {
		body := "{}\n" + `{"type":"event"}` + "\n" + `{"message":"m","timestamp":` + test.time + "}\n" +
			`{"type":"log"}` + "\n" + `{"items":[{"timestamp":` + test.time + `,"level":"info","body":"line"}]}` + "\n"
		envelope, err := sentry.ParseEnvelope([]byte(body))

		if err != nil {
			t.Fatal(err)
		}

		events, logs, err := parseItems(slog.New(slog.DiscardHandler), config.Project{ID: 1}, envelope, received)

		if err != nil || len(events) != 1 || len(logs) != 1 {
			t.Fatalf("%s: %v", test.name, err)
		}

		if !events[0].Timestamp.Equal(test.want) || !logs[0].Timestamp.Equal(test.want) {
			t.Errorf("%s: event %s, log %s, want %s", test.name, events[0].Timestamp.UTC(), logs[0].Timestamp.UTC(), test.want)
		}
	}
}
