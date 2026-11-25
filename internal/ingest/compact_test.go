package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/ingest/grouping"
	"github.com/devforth/exceptic/internal/sentry"
)

func TestCompactDropsWhatDoesNotHelpAFix(t *testing.T) {
	var breadcrumbs []string

	for index := range 80 {
		breadcrumbs = append(breadcrumbs, fmt.Sprintf(`{"message":"step %d"}`, index))
	}

	payload := `{
	  "event_id": "0123456789abcdef0123456789abcdef", "timestamp": 1731400000.123456789,
	  "modules": {"express": "4.19.2", "lodash": "4.17.21"},
	  "debug_meta": {"images": [{"type": "sourcemap", "code_file": "app.js"}]},
	  "sdk": {"name": "sentry.javascript.node", "version": "11.11.0", "integrations": ["Http", "Console"], "packages": [{"name": "npm:@sentry/node"}]},
	  "breadcrumbs": {"values": [` + strings.Join(breadcrumbs, ",") + `]},
	  "exception": {"values": [{"type": "Error", "value": "boom", "stacktrace": {"frames": [
	    {"filename": "node_modules/express/lib/router.js", "function": "handle", "in_app": false,
	     "pre_context": ["library line before"], "context_line": "library line", "post_context": ["library line after"], "vars": {"req": "library vars"}},
	    {"filename": "/app/src/orders.ts", "function": "loadOrders", "in_app": true,
	     "pre_context": ["app line before"], "context_line": "return db.query(sql)", "post_context": ["app line after"], "vars": {"sql": "app vars"}}
	  ]}}]},
	  "request": {"url": "https://example.com/orders"}
	}`

	compacted := string(compactJSON([]byte(payload)))

	for _, gone := range []string{`"modules"`, "lodash", "4.19.2", "debug_meta", "integrations", "npm:@sentry/node", "library line", "library vars", `"step 29"`} {
		if strings.Contains(compacted, gone) {
			t.Errorf("%q must be dropped:\n%s", gone, compacted)
		}
	}

	for _, kept := range []string{
		"app line before", "return db.query(sql)", "app line after", "app vars", // the application's code
		`"name":"sentry.javascript.node"`, `"version":"11.11.0"`, // the SDK itself
		`"step 30"`, `"step 79"`, // the newest breadcrumbs
		`"function":"handle"`, // library frames stay, without their source
		"https://example.com/orders", "1731400000.123456789",
	} {
		if !strings.Contains(compacted, kept) {
			t.Errorf("%q must be kept:\n%s", kept, compacted)
		}
	}

	if !json.Valid([]byte(compacted)) {
		t.Fatal("not valid JSON")
	}
}

// Without frames of the application (some SDKs mark none), every frame keeps
// its source: it is all there is, and grouping uses it.
func TestCompactKeepsLibrarySourceWithoutAppFrames(t *testing.T) {
	payload := `{"exception": {"values": [{"type": "Error", "stacktrace": {"frames": [{"filename": "a.js", "context_line": "only source"}]}}]}}`

	if compacted := string(compactJSON([]byte(payload))); !strings.Contains(compacted, "only source") {
		t.Fatalf("source dropped: %s", compacted)
	}
}

// A dev server (Vite) serves libraries from the page's origin, so the browser
// SDK marks their frames in-app; they are still library code.
func TestCompactDropsSourceOfInstalledPackagesMarkedInApp(t *testing.T) {
	payload := `{"exception": {"values": [{"type": "Error", "stacktrace": {"frames": [
		{"filename": "http://localhost:8080/node_modules/.vite/deps/vue.js", "in_app": true, "context_line": "library source"},
		{"filename": "http://localhost:8080/src/App.vue", "in_app": true, "context_line": "app source"}]}}]}}`
	compacted := string(compactJSON([]byte(payload)))

	if strings.Contains(compacted, "library source") || !strings.Contains(compacted, "app source") {
		t.Fatalf("library source kept or app source dropped: %s", compacted)
	}
}

func TestCompactLeavesOtherInputAlone(t *testing.T) {
	for _, payload := range []string{"not json", `["a list"]`, "null"} {
		if got := string(compactJSON([]byte(payload))); got != payload {
			t.Errorf("compactJSON(%q) = %q", payload, got)
		}
	}
}

// compactJSON runs Event on a payload the way ingest does; anything that is
// not JSON is returned unchanged.
func compactJSON(payload []byte) []byte {
	var output bytes.Buffer

	if _, err := compactEvent(newTokenDecoder(payload), newTokenEncoder(&output)); err != nil {
		return payload
	}

	return bytes.TrimRight(output.Bytes(), "\n")
}

// stackEvent is an event whose exception has count frames of the
// application, oldest first, each with its source and variables.
func stackEvent(count int) []byte {
	frames := make([]map[string]any, count)

	for index := range frames {
		frames[index] = map[string]any{
			"filename": "/app/src/tree.ts", "function": fmt.Sprintf("walk%d", index), "lineno": index + 1, "in_app": true,
			"context_line": fmt.Sprintf("return walk%d(node.next)", index), "pre_context": []string{"before"}, "post_context": []string{"after"},
			"vars": map[string]any{"depth": index},
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"platform":  "node",
		"exception": map[string]any{"values": []any{map[string]any{"type": "RangeError", "value": "Maximum call stack size exceeded", "stacktrace": map[string]any{"frames": frames}}}},
		"threads":   []any{map[string]any{"stacktrace": map[string]any{"frames": frames}}},
	})

	return payload
}

func preparedFrames(t *testing.T, prepared []byte) (exception, thread []map[string]any, truncated []any) {
	t.Helper()

	var event struct {
		Exception struct {
			Values []struct {
				Stacktrace struct {
					Frames []map[string]any `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"exception"`
		Threads []struct {
			Stacktrace struct {
				Frames []map[string]any `json:"frames"`
			} `json:"stacktrace"`
		} `json:"threads"`
		Truncated []any `json:"_exceptic_truncated"`
	}

	if err := json.Unmarshal(prepared, &event); err != nil {
		t.Fatal(err)
	}

	return event.Exception.Values[0].Stacktrace.Frames, event.Threads[0].Stacktrace.Frames, event.Truncated
}

// A stack keeps at most its 50 oldest and 200 newest frames (a recursion of
// 10,000 frames), and the cut is named.
func TestStackKeepsOldestAndNewestFrames(t *testing.T) {
	exception, thread, truncated := preparedFrames(t, prepareEvent(stackEvent(10_000)))

	for _, frames := range [][]map[string]any{exception, thread} {
		if len(frames) != 250 {
			t.Fatalf("%d frames, want 250", len(frames))
		}

		if frames[0]["function"] != "walk0" || frames[49]["function"] != "walk49" || frames[50]["function"] != "walk9800" || frames[249]["function"] != "walk9999" {
			t.Errorf("kept %v … %v, %v … %v", frames[0]["function"], frames[49]["function"], frames[50]["function"], frames[249]["function"])
		}
	}

	if fmt.Sprint(truncated) != "[frames]" {
		t.Errorf("truncated %v", truncated)
	}
}

// Over 50 frames with source, the middle ones lose their variables and the
// lines around them; every frame keeps its source line, which grouping uses.
func TestLongStackKeepsDetailAtItsEnds(t *testing.T) {
	exception, thread, truncated := preparedFrames(t, prepareEvent(stackEvent(60)))

	for _, frames := range [][]map[string]any{exception, thread} {
		if len(frames) != 60 {
			t.Fatalf("%d frames", len(frames))
		}

		for index, frame := range frames {
			detailed := index < 25 || index >= 35

			if _, hasVars := frame["vars"]; hasVars != detailed || (frame["pre_context"] != nil) != detailed || (frame["post_context"] != nil) != detailed {
				t.Errorf("frame %d: %v, detailed %v", index, frame, detailed)
			}

			if frame["context_line"] == nil {
				t.Errorf("frame %d lost its source line", index)
			}
		}
	}

	if truncated != nil {
		t.Errorf("truncated %v: no frame was dropped", truncated)
	}

	// Up to 50 frames everything stays.
	exception, _, _ = preparedFrames(t, prepareEvent(stackEvent(50)))

	for index, frame := range exception {
		if frame["vars"] == nil || frame["pre_context"] == nil {
			t.Errorf("frame %d of 50 lost detail", index)
		}
	}
}

// Library frames already lose their source; only the frames that keep it
// count for the 50, so an application frame deep in a library stack keeps it.
func TestLongStackCountsOnlyFramesWithSource(t *testing.T) {
	frames := []any{}

	for index := range 120 {
		frames = append(frames, map[string]any{"filename": "/app/node_modules/lib/x.js", "function": fmt.Sprint("lib", index), "context_line": "lib", "vars": map[string]any{"a": 1}})
	}

	frames = append(frames[:60:60], append([]any{map[string]any{"filename": "/app/src/a.ts", "in_app": true, "context_line": "app", "vars": map[string]any{"a": 1}}}, frames[60:]...)...)
	payload, _ := json.Marshal(map[string]any{"exception": []any{map[string]any{"type": "Error", "stacktrace": map[string]any{"frames": frames}}}})

	var event struct {
		Exception []struct {
			Stacktrace struct {
				Frames []map[string]any `json:"frames"`
			} `json:"stacktrace"`
		} `json:"exception"`
	}

	if err := json.Unmarshal(prepareEvent(payload), &event); err != nil {
		t.Fatal(err)
	}

	for _, frame := range event.Exception[0].Stacktrace.Frames {
		if frame["context_line"] == "app" && frame["vars"] == nil {
			t.Errorf("the application frame lost its variables: %v", frame)
		}
	}
}

// Grouping reads the frames kept: under the cap it is the same as before.
func TestFrameCapKeepsGrouping(t *testing.T) {
	for _, count := range []int{10, 60, 250} {
		payload := stackEvent(count)
		before, _ := sentry.ParseEvent(payload, "", time.Now())
		after, _ := sentry.ParseEvent(prepareEvent(payload), "", time.Now())

		if grouping.Fingerprint(before) != grouping.Fingerprint(after) {
			t.Errorf("%d frames: grouping changed", count)
		}
	}
}

// The event parser reads field names in any case, and so do the caps.
func TestCompactReadsNamesInAnyCase(t *testing.T) {
	frames := strings.Repeat(`{"function":"f","in_app":true,"vars":{"a":1}},`, 400) + `{"function":"last","in_app":true}`
	breadcrumbs := strings.Repeat(`{"message":"step"},`, 100) + `{"message":"last"}`
	payload := `{"Exception":{"VALUES":[{"type":"E","Stacktrace":{"Frames":[` + frames + `]}}]},"Breadcrumbs":{"Values":[` + breadcrumbs + `]}}`
	event, err := sentry.ParseEvent(prepareEvent([]byte(payload)), "", time.Now())

	if err != nil {
		t.Fatal(err)
	}

	if len(event.Exception) != 1 || event.Exception[0].Stacktrace == nil {
		t.Fatalf("exception not read: %+v", event.Exception)
	}

	if count := len(event.Exception[0].Stacktrace.Frames); count != oldestFrames+newestFrames {
		t.Errorf("%d frames kept", count)
	}

	if count := len(event.Breadcrumbs); count != maxBreadcrumbs {
		t.Errorf("%d breadcrumbs kept", count)
	}
}
