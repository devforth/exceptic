package sentry

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// largeEvent is an event of the size ingest accepts at most, frameCount
// frames with source and variables; the last frame's line number is lineno.
func largeEvent(frameCount int, lineno string) []byte {
	var payload strings.Builder

	payload.WriteString(`{"release": "1.0", "exception": {"values": [{"type": "TypeError", "value": "boom", "stacktrace": {"frames": [`)

	for index := range frameCount {
		line := fmt.Sprint(index + 1)

		if index == frameCount-1 {
			line = lineno
		}

		fmt.Fprintf(&payload, `{"filename": "/app/src/module%d.js", "function": "handler%d", "lineno": %s, "colno": 7, "in_app": true,
			"context_line": "  const result = await process(order, options);", "pre_context": ["a", "b", "c", "d", "e"],
			"post_context": ["f", "g", "h", "i", "j"], "vars": {"order": {"id": %d, "items": [1, 2, 3]}, "options": "x"}},`, index, index, line, index)
	}

	payload.WriteString(`{"filename": "/app/src/last.js"}]}}]}}`)

	return []byte(payload.String())
}

func allocatedBy(t *testing.T, payload []byte) uint64 {
	t.Helper()

	var least uint64

	for range 3 {
		var before, after runtime.MemStats

		runtime.GC()
		runtime.ReadMemStats(&before)

		if _, err := ParseEvent(payload, "", time.Now()); err != nil {
			t.Fatal(err)
		}

		runtime.ReadMemStats(&after)

		if allocated := after.TotalAlloc - before.TotalAlloc; least == 0 || allocated < least {
			least = allocated
		}
	}

	return least
}

// Anyone with a DSN can send a wrong-typed field; reading around it must not
// cost much more than reading a valid event of the same size.
func TestWrongTypedFieldCostsLittleMore(t *testing.T) {
	valid := allocatedBy(t, largeEvent(2000, "30"))
	odd := allocatedBy(t, largeEvent(2000, `"30"`))

	t.Logf("valid %d KB, wrong-typed %d KB", valid>>10, odd>>10)

	if odd > valid*5/2 {
		t.Errorf("a wrong-typed field costs %d KB, a valid event %d KB", odd>>10, valid>>10)
	}
}

// A name folds to a field exactly when the decoder reads it as that field.
func TestFoldedName(t *testing.T) {
	for _, name := range []string{"items", "Items", "ITEMS", "itemſ", "iteMS", "it_ems", "ıtems", "İtems", "frames", "FRAMES", "breadcrumbs"} {
		var batch struct {
			Items []int `json:"items"`
		}

		_ = json.Unmarshal([]byte(`{"`+name+`":[1]}`), &batch)

		if read, folded := len(batch.Items) == 1, FoldedName(name) == "items"; read != folded {
			t.Errorf("%q: read as items %v, folded %q", name, read, FoldedName(name))
		}
	}
}
