package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

// parsedItem parses an envelope of one item the way ingest does before storing it.
func parsedItem(t *testing.T, itemType string, payload []byte) ([]*sentry.Event, []sentry.Log) {
	t.Helper()

	body := append([]byte("{}\n"+`{"type":"`+itemType+`"}`+"\n"), payload...)
	envelope, err := sentry.ParseEnvelope(append(body, '\n'))

	if err != nil {
		t.Fatal(err)
	}

	events, logs, err := parseItems(slog.New(slog.DiscardHandler), config.Project{ID: 1}, envelope, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	return events, logs
}

func parsedLog(t *testing.T, record map[string]any) sentry.Log {
	t.Helper()

	record["level"] = "info"
	payload, err := json.Marshal(map[string]any{"items": []any{record}})

	if err != nil {
		t.Fatal(err)
	}

	_, logs := parsedItem(t, sentry.ItemLog, payload)

	if len(logs) != 1 {
		t.Fatalf("%d logs", len(logs))
	}

	return logs[0]
}

func TestLongLogBodyIsCut(t *testing.T) {
	stored := parsedLog(t, map[string]any{"body": strings.Repeat("a", 1<<20)})

	if want := strings.Repeat("a", config.MaxLogBodyBytes) + "…"; stored.Body != want {
		t.Fatalf("body of %d bytes, want %d", len(stored.Body), len(want))
	}
}

// A character is never cut in half: two-byte letters after one ASCII byte
// put the limit in the middle of one.
func TestLogBodyIsCutWhereACharacterStarts(t *testing.T) {
	stored := parsedLog(t, map[string]any{"body": "a" + strings.Repeat("é", 1<<19)})

	if want := "a" + strings.Repeat("é", (config.MaxLogBodyBytes-1)/2) + "…"; stored.Body != want || !utf8.ValidString(stored.Body) {
		t.Fatalf("body of %d bytes, want %d", len(stored.Body), len(want))
	}
}

// Past the limit the attributes first by name stay, so a repeated log keeps the same ones.
func TestLogAttributesAreCapped(t *testing.T) {
	attributes := map[string]any{}

	for index := range 100 {
		attributes[fmt.Sprintf("attribute.%03d", index)] = map[string]any{"value": "v", "type": "string"}
	}

	stored := parsedLog(t, map[string]any{"body": "line", "attributes": attributes})

	if len(stored.Attributes) != config.MaxLogAttributes {
		t.Fatalf("%d attributes", len(stored.Attributes))
	}

	for index := range config.MaxLogAttributes {
		if _, ok := stored.Attributes[fmt.Sprintf("attribute.%03d", index)]; !ok {
			t.Errorf("attribute.%03d dropped", index)
		}
	}
}

// The SDK's own attributes stay first: the parent span ties a log to its error.
func TestLogAttributesKeepSentryOnesFirst(t *testing.T) {
	attributes := map[string]any{}

	for index := range 100 {
		attributes[fmt.Sprintf("attribute.%03d", index)] = map[string]any{"value": "v", "type": "string"}
	}

	sentryNames := []string{"sentry.trace.parent_span_id", "sentry.sdk.name", "sentry.release"}

	for _, name := range sentryNames {
		attributes[name] = map[string]any{"value": "b1c2d3e4f5a60718", "type": "string"}
	}

	stored := parsedLog(t, map[string]any{"body": "line", "attributes": attributes})

	if len(stored.Attributes) != config.MaxLogAttributes {
		t.Fatalf("%d attributes", len(stored.Attributes))
	}

	for _, name := range sentryNames {
		if _, ok := stored.Attributes[name]; !ok {
			t.Errorf("%s dropped", name)
		}
	}

	for index := range config.MaxLogAttributes - len(sentryNames) {
		if _, ok := stored.Attributes[fmt.Sprintf("attribute.%03d", index)]; !ok {
			t.Errorf("attribute.%03d dropped", index)
		}
	}
}

func TestLongLogAttributeValueIsCut(t *testing.T) {
	stored := parsedLog(t, map[string]any{"body": "line", "attributes": map[string]any{
		"long":  map[string]any{"value": strings.Repeat("x", 100<<10), "type": "string"},
		"list":  map[string]any{"value": slices.Repeat([]int{1}, 50<<10), "type": "integer"},
		"short": map[string]any{"value": 42, "type": "integer"},
	}})

	if got, want := stored.Attributes["long"].String(), strings.Repeat("x", config.MaxLogAttributeBytes)+"…"; got != want {
		t.Errorf("long: %d bytes, want %d", len(got), len(want))
	}

	// A value of another type that long is kept as its text, cut.
	if list := stored.Attributes["list"]; list.Type != "string" || !strings.HasPrefix(list.String(), "[1,1,") || len(list.String()) != config.MaxLogAttributeBytes+len("…") {
		t.Errorf("list: %s %d bytes", list.Type, len(list.String()))
	}

	if short := stored.Attributes["short"]; short.Type != "integer" || string(short.Value) != "42" {
		t.Errorf("short: %s %s", short.Type, short.Value)
	}
}

// Every text of a log has a limit: its ids (span_id may come from an
// attribute), attribute names and types.
func TestLogIDsAndAttributeNamesAreCut(t *testing.T) {
	long := strings.Repeat("a", 1<<20)
	stored := parsedLog(t, map[string]any{"body": "line", "trace_id": long, "attributes": map[string]any{
		sentry.ParentSpanAttribute: map[string]any{"value": long, "type": "string"},
		long:                       map[string]any{"value": "first", "type": "string"},
		long + "b":                 map[string]any{"value": "second", "type": "string"},
		"typed":                    map[string]any{"value": "v", "type": long},
	}})

	if len(stored.TraceID) > maxLogIDBytes+len("…") || len(stored.SpanID) > maxLogIDBytes+len("…") {
		t.Errorf("trace_id of %d bytes, span_id of %d", len(stored.TraceID), len(stored.SpanID))
	}

	if len(stored.Attributes) != 3 {
		t.Errorf("%d attributes", len(stored.Attributes))
	}

	for name, attribute := range stored.Attributes {
		if len(name) > maxLogAttributeNameBytes+len("…") || len(attribute.Type) > maxLogAttributeTypeBytes+len("…") {
			t.Errorf("attribute name of %d bytes, type of %d", len(name), len(attribute.Type))
		}

		if strings.HasPrefix(name, "aaa") && attribute.String() != "first" {
			t.Errorf("of names cut alike the first by name stays: %s", attribute.String())
		}
	}
}

// longEvent is an event with an exception and the given fields; each string
// of filler is a little under the string limit.
func longEvent(t testing.TB, fields map[string]any) []byte {
	t.Helper()

	event := map[string]any{
		"event_id": "0123456789abcdef0123456789abcdef", "timestamp": 1731400000.5, "level": "error", "platform": "node",
		"fingerprint": []string{"{{ default }}", "orders"},
		"contexts":    map[string]any{"trace": map[string]any{"trace_id": "0123456789abcdef0123456789abcdef", "span_id": "0123456789abcdef"}},
		"exception": map[string]any{"values": []any{map[string]any{"type": "OrderError", "value": "order 890 failed", "stacktrace": map[string]any{"frames": []any{
			map[string]any{"filename": "/app/src/orders.ts", "function": "loadOrders", "lineno": 12, "colno": 5, "in_app": true, "context_line": "return db.query(sql)"},
		}}}}},
	}

	maps.Copy(event, fields)

	payload, err := json.Marshal(event)

	if err != nil {
		t.Fatal(err)
	}

	return payload
}

// filler is count strings of nearly the string limit under numbered keys.
func filler(count int) map[string]any {
	values := map[string]any{}

	for index := range count {
		values[fmt.Sprintf("field%03d", index)] = strings.Repeat("v", config.MaxEventStringBytes-100)
	}

	return values
}

// decodedEvent is a prepared event as a tree, with the fields it must keep whatever is cut.
func decodedEvent(t *testing.T, prepared []byte) map[string]any {
	t.Helper()

	var event map[string]any

	if err := json.Unmarshal(prepared, &event); err != nil {
		t.Fatal(err)
	}

	original := map[string]any{}
	_ = json.Unmarshal(longEvent(t, nil), &original)

	for _, field := range []string{"event_id", "timestamp", "level", "fingerprint", "exception"} {
		if got, want := fmt.Sprint(event[field]), fmt.Sprint(original[field]); got != want {
			t.Errorf("%s changed: %s, want %s", field, got, want)
		}
	}

	if trace := fmt.Sprint(event["contexts"]); !strings.Contains(trace, "trace_id") {
		t.Errorf("trace context lost: %s", trace)
	}

	return event
}

func truncated(event map[string]any) string {
	return fmt.Sprint(event[sentry.TruncatedField])
}

func TestLongEventStringIsCut(t *testing.T) {
	prepared := prepareEvent(longEvent(t, map[string]any{"extra": map[string]any{"note": strings.Repeat("x", 5<<20)}}))
	event := decodedEvent(t, prepared)

	if note := event["extra"].(map[string]any)["note"]; note != strings.Repeat("x", config.MaxEventStringBytes)+"…" {
		t.Errorf("note of %d bytes", len(fmt.Sprint(note)))
	}

	if got := truncated(event); got != "[strings]" {
		t.Errorf("truncated %s", got)
	}
}

// An event over the size limit loses the least useful parts first, and only
// as many as it takes: here the oldest breadcrumbs and extra are enough.
func TestLargeEventLosesTheLeastUsefulFirst(t *testing.T) {
	var breadcrumbs []any

	for range 50 {
		breadcrumbs = append(breadcrumbs, map[string]any{"message": strings.Repeat("b", 10<<10)})
	}

	prepared := prepareEvent(longEvent(t, map[string]any{
		"breadcrumbs": breadcrumbs,
		"extra":       filler(320), // 5 MB
		"request":     map[string]any{"method": "POST", "url": "https://example.com/orders", "data": strings.Repeat("d", 10<<10)},
		"tags":        map[string]any{"route": "/orders"},
	}))

	if len(prepared) > config.MaxEventBytes {
		t.Fatalf("%d bytes", len(prepared))
	}

	event := decodedEvent(t, prepared)

	if got := truncated(event); got != "[breadcrumbs extra]" {
		t.Errorf("truncated %s", got)
	}

	if kept := len(event["breadcrumbs"].([]any)); kept != config.TruncatedBreadcrumbs {
		t.Errorf("%d breadcrumbs", kept)
	}

	if _, ok := event["extra"]; ok {
		t.Error("extra kept")
	}

	if request := event["request"].(map[string]any); request["data"] == nil || event["tags"] == nil {
		t.Errorf("request body or tags dropped: %v", event)
	}
}

// When nothing else is enough, only the fields that describe the error stay.
func TestHugeEventKeepsTheError(t *testing.T) {
	prepared := prepareEvent(longEvent(t, map[string]any{
		"extra":   map[string]any{"a": 1},
		"tags":    filler(320),
		"request": map[string]any{"data": "body"},
		"user":    map[string]any{"id": "1"},
	}))

	if len(prepared) > config.MaxEventBytes {
		t.Fatalf("%d bytes", len(prepared))
	}

	event := decodedEvent(t, prepared)

	if got := truncated(event); got != "[other]" {
		t.Errorf("truncated %s", got)
	}

	for _, field := range []string{"tags", "request", "user", "extra"} {
		if _, ok := event[field]; ok {
			t.Errorf("%s kept", field)
		}
	}
}

// What the cuts keep still fits the limit: the newest errors and the members
// that fit stay, the rest goes.
func TestEveryEventFitsTheLimit(t *testing.T) {
	var exceptions []any

	for index := range 300 {
		exceptions = append(exceptions, map[string]any{"type": fmt.Sprint("Error", index), "value": strings.Repeat("v", config.MaxEventStringBytes-100)})
	}

	for name, fields := range map[string]map[string]any{
		"contexts":    {"contexts": map[string]any{"os": filler(200)}},
		"long name":   {"contexts": map[string]any{"os": map[string]any{strings.Repeat("n", 3<<20): 1, "name": "Linux"}}},
		"exceptions":  {"exception": map[string]any{"values": exceptions}},
		"fingerprint": {"fingerprint": slices.Repeat([]string{strings.Repeat("f", 1000)}, 3000)},
	} {
		t.Run(name, func(t *testing.T) {
			prepared := prepareEvent(longEvent(t, fields))

			if len(prepared) > config.MaxEventBytes {
				t.Fatalf("%d bytes", len(prepared))
			}

			var event map[string]any

			if err := json.Unmarshal(prepared, &event); err != nil {
				t.Fatal(err)
			}

			if got := truncated(event); !strings.HasSuffix(got, "size]") {
				t.Errorf("truncated %s", got)
			}

			if event["event_id"] == nil || event["level"] == nil {
				t.Errorf("small error fields lost: %.300s", prepared)
			}

			if name == "exceptions" && !strings.Contains(string(prepared), `"Error299"`) {
				t.Error("the outermost error lost")
			}
		})
	}
}

// The mark is the server's: one sent by an SDK is dropped.
func TestTruncatedFieldFromSDKIsDropped(t *testing.T) {
	prepared := prepareEvent([]byte(`{"message":"m","` + sentry.TruncatedField + `":["extra"]}`))

	if bytes.Contains(prepared, []byte(sentry.TruncatedField)) {
		t.Fatalf("kept: %s", prepared)
	}
}

// Events of normal size come out of the cuts exactly as they went in.
func TestNormalEventsAreNotCut(t *testing.T) {
	for _, fixture := range fixtures.All(t) {
		envelope, err := sentry.ParseEnvelope(fixture.Body)

		if err != nil {
			t.Fatal(err)
		}

		for _, item := range envelope.Items {
			if item.Header.Type == sentry.ItemEvent {
				if prepared, compacted := prepareEvent(item.Payload), compactJSON(item.Payload); !bytes.Equal(prepared, compacted) {
					t.Errorf("%s changed:\n%s\n%s", fixture.Name, prepared, compacted)
				}
			}
		}
	}
}

// Cutting a large event takes time in proportion to its size, even when every cut is needed.
func TestLargeEventsStayLinear(t *testing.T) {
	for name, fields := range map[string]map[string]any{
		"one string": {"extra": map[string]any{"note": strings.Repeat("x", 10<<20)}},
		"every cut":  {"tags": filler(200)}, // 3 MB: the tree pass decodes it 8 times, slow under -race
	} {
		payload := longEvent(t, fields)

		// Against the tree pass on the same input, so a slow machine slows both.
		started := time.Now()

		referencePrepare(payload)

		tree := time.Since(started)
		started = time.Now()

		prepareEvent(payload)

		if tokens := time.Since(started); tokens > 10*tree+50*time.Millisecond {
			t.Errorf("%s: %s, the tree pass %s, for %d bytes", name, tokens, tree, len(payload))
		}
	}
}

// The frame cut of the compaction is named before the size cuts.
func TestFramesCutIsNamedFirst(t *testing.T) {
	event := decodedEvent(t, prepareEvent(longEvent(t, map[string]any{"stacktrace": longStack()})))

	if got := truncated(event); got != "[frames frame_vars]" {
		t.Errorf("truncated %s", got)
	}
}

// longStack is a recursion of 10,000 frames whose frames at the ends, which
// keep their variables, take more than the size limit.
func longStack() map[string]any {
	frames := make([]any, 10_000)

	for index := range frames {
		frame := map[string]any{"function": "f", "in_app": true}

		if index < 25 || index >= len(frames)-25 {
			frame["vars"] = filler(2)
		}

		frames[index] = frame
	}

	return map[string]any{"frames": frames}
}
