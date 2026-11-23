package sentry

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
)

func parseFixture(t *testing.T, name string) *Envelope {
	t.Helper()
	envelope, err := ParseEnvelope(fixtures.Envelope(t, name))

	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	return envelope
}

func firstEvent(t *testing.T, name string) *Event {
	t.Helper()
	envelope := parseFixture(t, name)

	if len(envelope.Items) != 1 || envelope.Items[0].Header.Type != ItemEvent {
		t.Fatalf("%s: want one event item, got %+v", name, envelope.Items)
	}

	event, err := ParseEvent(envelope.Items[0].Payload, envelope.Header.EventID, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	return event
}

func TestAllFixturesParse(t *testing.T) {
	for _, fixture := range fixtures.All(t) {
		t.Run(fixture.Name, func(t *testing.T) {
			envelope := parseFixture(t, fixture.Name)

			if len(envelope.Items) == 0 {
				t.Fatal("no items")
			}

			for _, item := range envelope.Items {
				switch item.Header.Type {
				case ItemEvent:
					if _, err := ParseEvent(item.Payload, envelope.Header.EventID, time.Now()); err != nil {
						t.Fatal(err)
					}
				case ItemLog:
					if _, err := ParseLogs(item.Payload, time.Now()); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestNodeUnhandledEvent(t *testing.T) {
	event := firstEvent(t, "node-event-unhandled")

	if len(event.EventID) != 32 {
		t.Fatalf("event_id = %q", event.EventID)
	}

	if event.Platform != "node" || event.Level != "error" {
		t.Fatalf("platform=%q level=%q", event.Platform, event.Level)
	}

	main := event.MainException()

	if main == nil || main.Mechanism == nil || main.Mechanism.Handled == nil || *main.Mechanism.Handled {
		t.Fatalf("want an unhandled exception, got %+v", main)
	}

	if main.Stacktrace == nil || len(main.Stacktrace.Frames) == 0 {
		t.Fatal("no stack frames")
	}

	if event.Timestamp.Year() < 2026 {
		t.Fatalf("timestamp = %v", event.Timestamp)
	}

	if event.Trace().TraceID == "" || event.Trace().SpanID == "" {
		t.Fatal("no trace_id in contexts")
	}
}

func TestCauseChainTitleUsesOutermostError(t *testing.T) {
	event := firstEvent(t, "node-event-cause-chain")

	if len(event.Exception) < 2 {
		t.Fatalf("want a cause chain, got %d exceptions", len(event.Exception))
	}

	if event.Title() == event.Exception[0].Type+": "+event.Exception[0].Value {
		t.Fatalf("title %q is the innermost cause, want the outermost error", event.Title())
	}
}

func TestMessageEvent(t *testing.T) {
	event := firstEvent(t, "node-event-message")

	if event.Message != "Payment provider responded slowly" || event.Level != "warning" {
		t.Fatalf("message=%q level=%q", event.Message, event.Level)
	}
	// captureMessage adds an exception without a type: the title is the message text.
	if event.Title() != "Payment provider responded slowly" {
		t.Fatalf("title = %q", event.Title())
	}
}

func TestHandledEventContext(t *testing.T) {
	event := firstEvent(t, "node-event-handled-context")

	if event.Tags["feature"] != "checkout" {
		t.Fatalf("tags = %v", event.Tags)
	}

	if event.User == nil || event.User.ID != "42" || event.User.Email != "john@example.com" {
		t.Fatalf("user = %+v", event.User)
	}

	if len(event.Breadcrumbs) == 0 {
		t.Fatal("no breadcrumbs")
	}
}

func TestCustomFingerprint(t *testing.T) {
	event := firstEvent(t, "node-event-custom-fingerprint")

	if len(event.Fingerprint) == 0 {
		t.Fatal("fingerprint not parsed")
	}
}

func TestBrowserEvent(t *testing.T) {
	event := firstEvent(t, "browser-event-onerror")

	if event.Platform != "javascript" {
		t.Fatalf("platform = %q", event.Platform)
	}

	if !strings.HasPrefix(event.Title(), "ReferenceError: ") {
		t.Fatalf("title = %q", event.Title())
	}
}

func TestLogs(t *testing.T) {
	envelope := parseFixture(t, "node-logs")
	logs, err := ParseLogs(envelope.Items[0].Payload, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	// The item header of node-logs says item_count 11.
	if len(logs) != 11 {
		t.Fatalf("got %d logs, want 11", len(logs))
	}

	for _, record := range logs {
		// SpanID comes from the sentry.trace.parent_span_id attribute in SDK 11.
		if record.Body == "" || record.Level == "" || record.Timestamp.IsZero() || record.SpanID == "" {
			t.Fatalf("incomplete log: %+v", record)
		}
	}

	origins := map[string]bool{}

	for _, record := range logs {
		origins[record.Attributes["sentry.origin"].String()] = true
	}

	if !origins["auto.log.pino"] {
		t.Fatalf("no pino logs among origins %v", origins)
	}
}

func TestEnvelopeWithLengthAndTrailingNewline(t *testing.T) {
	// The first payload contains a newline, so only "length" can delimit it.
	body := "{\"event_id\":\"9ec79c33ec9942ab8353589fcb2e04dc\"}\n" +
		"{\"type\":\"attachment\",\"length\":7}\nab\ncdef\n" +
		"{\"type\":\"event\"}\n{\"message\":\"hi\"}\n"
	envelope, err := ParseEnvelope([]byte(body))

	if err != nil {
		t.Fatal(err)
	}

	if len(envelope.Items) != 2 {
		t.Fatalf("got %d items, want 2", len(envelope.Items))
	}

	if string(envelope.Items[0].Payload) != "ab\ncdef" {
		t.Fatalf("attachment payload = %q", envelope.Items[0].Payload)
	}

	event, err := ParseEvent(envelope.Items[1].Payload, envelope.Header.EventID, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	if event.EventID != "9ec79c33ec9942ab8353589fcb2e04dc" || event.Message != "hi" {
		t.Fatalf("event_id=%q message=%q", event.EventID, event.Message)
	}
}

func TestEnvelopeErrors(t *testing.T) {
	testCases := map[string]string{
		"empty":             "",
		"bad header":        "not json\n",
		"bad item header":   "{}\nnot json\n{}",
		"length too large":  "{}\n{\"type\":\"event\",\"length\":100}\n{}",
		"negative length":   "{}\n{\"type\":\"event\",\"length\":-1}\n{}",
		"empty item header": "{}\n\n{\"type\":\"event\"}\n{}",
		"too many items":    "{}\n" + strings.Repeat("{\"type\":\"event\"}\n{}\n", MaxItems+1),
	}

	for name, body := range testCases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEnvelope([]byte(body)); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestLenientFieldShapes(t *testing.T) {
	// Shapes allowed by the protocol but not sent by the JS SDK.
	payload := `{
		"event_id": "9EC79C33-EC99-42AB-8353-589FCB2E04DC",
		"timestamp": "2026-11-10T16:27:00.5Z",
		"logentry": {"formatted": "formatted text", "message": "template %s"},
		"exception": [{"type": "ValueError", "value": "bad"}],
		"breadcrumbs": {"values": [{"message": "step"}]},
		"tags": [["release_stage", "beta"], ["attempt", 3]],
		"fingerprint": ["{{ default }}", 42],
		"user": {"id": 7},
		"level": "fatal"
	}`
	event, err := ParseEvent([]byte(payload), "", time.Now())

	if err != nil {
		t.Fatal(err)
	}

	checks := map[string][2]string{
		"event_id":    {event.EventID, "9ec79c33ec9942ab8353589fcb2e04dc"},
		"timestamp":   {event.Timestamp.Format(time.RFC3339Nano), "2026-11-10T16:27:00.5Z"},
		"message":     {string(event.Message), "formatted text"},
		"title":       {event.Title(), "ValueError: bad"},
		"breadcrumb":  {event.Breadcrumbs[0].Message, "step"},
		"tag string":  {event.Tags["release_stage"], "beta"},
		"tag number":  {event.Tags["attempt"], "3"},
		"fingerprint": {strings.Join(event.Fingerprint, ","), "{{ default }},42"},
		"user id":     {string(event.User.ID), "7"},
	}

	for name, check := range checks {
		if check[0] != check[1] {
			t.Errorf("%s = %q, want %q", name, check[0], check[1])
		}
	}
}

func TestWrongTypedFieldsKeepTheEvent(t *testing.T) {
	// Sentry.setUser({id: 7, username: 42}) and similar: Relay reads them as text.
	payload := `{
		"release": 5,
		"environment": true,
		"server_name": 3.5,
		"transaction": {"name": "checkout"},
		"user": {"id": 7, "username": 42, "email": false, "ip_address": 1},
		"request": {"method": 5},
		"contexts": [],
		"sdk": "sentry.javascript",
		"breadcrumbs": [{"message": 5, "level": 1}, "not a breadcrumb"]
	}`
	event, err := ParseEvent([]byte(payload), "", time.Now())

	if err != nil {
		t.Fatal(err)
	}

	checks := map[string][2]string{
		"release":     {event.Release, "5"},
		"environment": {event.Environment, "true"},
		"server_name": {event.ServerName, "3.5"},
		"transaction": {event.Transaction, ""},
		"user id":     {string(event.User.ID), "7"},
		"username":    {event.User.Username, "42"},
		"email":       {event.User.Email, "false"},
		"ip_address":  {event.User.IPAddress, "1"},
		"request":     {string(event.Request), `{"method": 5}`},
		"breadcrumbs": {strconv.Itoa(len(event.Breadcrumbs)), "1"},
		"breadcrumb":  {event.Breadcrumbs[0].Message + " " + event.Breadcrumbs[0].Level, "5 1"},
		"sdk":         {strconv.FormatBool(event.SDK == nil), "true"},
		"contexts":    {strconv.FormatBool(event.Contexts == nil), "true"},
	}

	for name, check := range checks {
		if check[0] != check[1] {
			t.Errorf("%s = %q, want %q", name, check[0], check[1])
		}
	}
}

func TestWrongTypedExceptionFieldsKeepTheChain(t *testing.T) {
	// One odd field must not empty the chain, or the event loses its title and grouping.
	payload := `{"exception": {"values": [
		7,
		{"type": "TypeError", "value": "boom", "mechanism": {"handled": "no"}, "stacktrace": {"frames": [
			"not a frame",
			{"filename": "/app/a.js", "function": "outer", "lineno": "12", "colno": 4.0, "in_app": true, "pre_context": [1, "b"]},
			{"filename": "/app/b.js", "function": 9, "lineno": 30, "in_app": "true"}
		]}}
	]}}`
	event, err := ParseEvent([]byte(payload), "", time.Now())

	if err != nil {
		t.Fatal(err)
	}

	if event.Title() != "TypeError: boom" || len(event.Exception) != 1 {
		t.Fatalf("title = %q, exceptions = %+v", event.Title(), event.Exception)
	}

	frames := event.Exception[0].Stacktrace.Frames

	if len(frames) != 2 {
		t.Fatalf("frames = %+v", frames)
	}

	checks := map[string][2]string{
		"lineno":        {strconv.Itoa(frames[0].Lineno), "12"},
		"colno":         {strconv.Itoa(frames[0].Colno), "4"},
		"in_app":        {strconv.FormatBool(frames[0].IsInApp()), "true"},
		"pre_context":   {strings.Join(frames[0].PreContext, ","), "1,b"},
		"function":      {frames[1].Function, "9"},
		"in_app string": {strconv.FormatBool(frames[1].InApp == nil), "true"},
		"handled":       {strconv.FormatBool(event.Exception[0].Mechanism.Handled == nil), "true"},
	}

	for name, check := range checks {
		if check[0] != check[1] {
			t.Errorf("%s = %q, want %q", name, check[0], check[1])
		}
	}
}

func TestEventMustBeAnObject(t *testing.T) {
	for _, payload := range []string{`[1]`, `"event"`, `5`, `null`, ` null `} {
		if _, err := ParseEvent([]byte(payload), "", time.Now()); err == nil {
			t.Errorf("%s: expected an error", payload)
		}
	}
}

func TestWrongTypedLogFields(t *testing.T) {
	payload := `{"items": [
		{"body": 5, "level": "warn", "severity_number": "13", "trace_id": 7, "attributes": {"count": {"type": "integer", "value": 3}, "odd": 1}},
		"not a log",
		{"body": "second", "timestamp": "yesterday"}
	]}`
	received := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	logs, err := ParseLogs([]byte(payload), received)

	if err != nil {
		t.Fatal(err)
	}

	if len(logs) != 2 {
		t.Fatalf("logs = %+v", logs)
	}

	first := logs[0]

	if first.Body != "5" || first.SeverityNumber != 13 || first.TraceID != "7" || len(first.Attributes) != 1 || first.Attributes["count"].String() != "3" {
		t.Fatalf("first = %+v", first)
	}

	if logs[1].Body != "second" || !logs[1].Timestamp.Equal(received) || logs[1].Level != "info" {
		t.Fatalf("second = %+v", logs[1])
	}

	for _, batch := range []string{`[]`, `null`} {
		if _, err := ParseLogs([]byte(batch), received); err == nil {
			t.Errorf("%s: expected an error for a batch that is not an object", batch)
		}
	}
}

func TestEventDefaults(t *testing.T) {
	received := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	// A wrong-shaped field (tags as a number) is dropped, not fatal.
	event, err := ParseEvent([]byte(`{"tags": 5}`), "not-an-id", received)

	if err != nil {
		t.Fatal(err)
	}

	if len(event.EventID) != 32 || !event.Timestamp.Equal(received) || event.Level != "error" {
		t.Fatalf("event_id=%q timestamp=%v level=%q", event.EventID, event.Timestamp, event.Level)
	}

	if event.Title() != "<unlabeled event>" {
		t.Fatalf("title = %q", event.Title())
	}
}

func TestInvalidEventJSON(t *testing.T) {
	if _, err := ParseEvent([]byte(`{"message": `), "", time.Now()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFrameIsInApp(t *testing.T) {
	marked, unmarked := true, false
	cases := []struct {
		frame Frame
		want  bool
	}{
		{Frame{Filename: "/app/src/server.js", InApp: &marked}, true},
		{Frame{Filename: "/app/src/server.js", InApp: &unmarked}, false},
		{Frame{Filename: "/app/src/server.js"}, false},
		{Frame{Filename: "http://localhost:8080/node_modules/.vite/deps/vue.js", InApp: &marked}, false},
		{Frame{Filename: "node_modules/express/lib/router.js", InApp: &marked}, false},
		{Frame{Filename: "app:///main.js", AbsPath: `C:\app\node_modules\pkg\index.js`, InApp: &marked}, false},
	}

	for _, test := range cases {
		if got := test.frame.IsInApp(); got != test.want {
			t.Errorf("IsInApp(%q, %q) = %v, want %v", test.frame.Filename, test.frame.AbsPath, got, test.want)
		}
	}
}

// Times JSON cannot encode are left zero, so no output fails on them.
func TestTimestampOutOfRange(t *testing.T) {
	for input, want := range map[string]time.Time{
		`1731400000.5`:                time.Unix(1731400000, 5e8).UTC(),
		`"2026-11-10T16:27:00Z"`:      time.Date(2026, 11, 10, 16, 27, 0, 0, time.UTC),
		`1731400000000`:               {},
		`-1e18`:                       {},
		`1e300`:                       {},
		`"10000-01-01T00:00:00Z"`:     {},
		`"0000-01-01T00:00:00+01:00"`: {},
	} {
		var timestamp Timestamp

		if err := json.Unmarshal([]byte(input), &timestamp); err != nil || !timestamp.Equal(want) {
			t.Errorf("%s: %v %v, want %v", input, timestamp, err, want)
		}

		if _, err := json.Marshal(timestamp.Time); err != nil {
			t.Errorf("%s: %v", input, err)
		}
	}
}
