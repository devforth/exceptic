package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The token pass gives what the tree pass it replaced gave (reference*
// below, kept as it was, with the size limits added): the same JSON once
// decoded. Key order and string escapes may differ; the stored payload is
// always decoded again.
func TestPrepareMatchesTheTreePass(t *testing.T) {
	for index, payload := range samplePayloads(t) {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			if got, want := normalized(prepareEvent(payload)), normalized(referencePrepare(payload)); got != want {
				t.Fatalf("event of %s:\ngot  %s\nwant %s", payload, got, want)
			}

			if got, want := normalized(scrubJSON(payload)), normalized(referenceScrub(payload)); got != want {
				t.Fatalf("scrub of %s:\ngot  %s\nwant %s", payload, got, want)
			}
		})
	}

	for index, payload := range largePayloads(t) {
		if got, want := normalized(prepareEvent(payload)), normalized(referencePrepare(payload)); got != want {
			t.Errorf("large event %d differs:\ngot  %.2000s\nwant %.2000s", index, got, want)
		}
	}
}

func FuzzPrepareMatchesTheTreePass(f *testing.F) {
	for _, payload := range samplePayloads(f) {
		f.Add(payload)
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		if got, want := normalized(prepareEvent(payload)), normalized(referencePrepare(payload)); got != want {
			t.Fatalf("event of %q:\ngot  %s\nwant %s", payload, got, want)
		}

		if got, want := normalized(scrubJSON(payload)), normalized(referenceScrub(payload)); got != want {
			t.Fatalf("scrub of %q:\ngot  %s\nwant %s", payload, got, want)
		}
	})
}

// samplePayloads are the events and logs of every fixture plus the edge cases of the rules.
func samplePayloads(t testing.TB) [][]byte {
	payloads := [][]byte{
		[]byte(`{"message":"Bearer syntheticcredential","request":{"headers":[["Authorization","syntheticsecret"]]},"extra":{"n":12345678901234567890,"password":"syntheticsecret"},"modules":{"unused":"1"},"exception":{"values":[{"stacktrace":{"frames":[{"in_app":true,"vars":{"password":"syntheticsecret"}},{"in_app":false,"vars":{"large":"unused"}}]}}]}}`),
		[]byte(`{"breadcrumbs":{"values":[` + strings.Repeat(`{"message":"step"},`, 79) + `{"message":"last"}]}}`),
		[]byte(`{"breadcrumbs":[` + strings.Repeat(`{"message":"step"},`, 60) + `{"message":"last"}],"threads":[{"stacktrace":{"frames":[{"in_app":true},{"filename":"x.js","vars":{"a":1}}]}}]}`),
		[]byte(`{"breadcrumbs":null,"sdk":{"name":"node","integrations":["a"],"packages":[{"name":"b"}],"token":"x"},"debug_meta":{"images":[]},"stacktrace":{"frames":[{"in_app":true,"filename":"/app/node_modules/x.js","context_line":"lib"},{"in_app":true,"filename":"/app/a.js","context_line":"app"}]}}`),
		[]byte(`{"a":"x\u0062earer abcdefghijkl","b":"\u00e9 Bearer abcdefghijkl","c":"https://h.example/p?\u0074oken=1","d":"plain \/ <b>&","e":"\xff\xfe broken utf8","f":"\ud800 lone"}`),
		[]byte(`{"pairs":[["Cookie","sid=1"],["cookie","a","b"],["accept","*/*"],["x"],[]],"attr":{"session.id":{"value":"s","type":"string"},"api_key":null,"secret":{"nested":1},"token":[1,2]}}`),
		[]byte(`{"dup":"Bearer abcdefghijkl","dup":"second","password":"a","password":"b"} trailing`),
		[]byte(`{"exception":{"values":"not a list"},"threads":{"values":[1,"x",null]},"stacktrace":{"frames":"none"}}`),
		[]byte(`{"items":[{"body":"request","attributes":{"http.request.header.authorization":{"value":"Bearer abcdefghijkl","type":"string"},"gen_ai.usage.input_tokens":{"value":42,"type":"integer"}}}]}`),
		[]byte(`{"request":{"data":"{\"username\":\"root\",\"password\":\"p\",\"session\":{\"id\":1},\"next\":\"{\\\"token\\\":\\\"t\\\"}\"}"},"extra":{"auth":{"value":{"user":7},"token":"t","password":"p"}},"message":"POST /login {\"password\":\"p\",\"cvv\":123} cut \"token\":\"abc"}`),
		[]byte(`{"attributes":{"body":{"value":"[[\"authorization\",\"Basic dXNlcjpwYXNz\"]]","type":"string"},"session":{"value":"s","raw":"Bearer abcdefghijkl0123"},"pairs":[["cookie",{"value":1,"x":2}]]},"pretty":"{\n \"a\": 1\n}","two":"{\"a\":1} {\"token\":2}","bad":"{\"token\":1,}","escaped":"{\"\\u0074oken\":\"t\"}"}`),
		[]byte(`{"message":"login failed {'username': 'root', 'password': 'pw1', 'token': 'abc'}","breadcrumbs":[{"message":"don't retry {'session': 'p\\'w', 'cvv': 123}"}],"logentry":{"message":"{'note': 'say \"hi', 'api_key': \"it's\"} cut {'secret': 'x"}}`),
		[]byte(`{"items":[{"body":"{'password': 'pw1'} it's 'auth': gone","attributes":{"text":{"value":"\"x\" {'token': 'abc'","type":"string"}}}]}`),
		[]byte(`{"request":{"cookies":{"theme":{"b":1,"token":"t"},"sid":"s","cookie":"a=1","tz":null,"x":{"value":1}},"headers":{"Cookie":" a=1;theme=dark; b ;;","Set-Cookie":"sid=1"}},"extra":{"cookies":[["theme","dark"],["sid","s"],"lang=en; sid=2",3,null,["a"]],"cookie":{"value":"sid=1","type":"string"},"Cookies":7}}`),
		[]byte(`{"extra":{"missing":["password","email"],"pairs":[["password","p"],[["token","t"]]],"top":["token",["token","t"]]},"breadcrumbs":[["token","t"],{"data":["secret","s"]}],"exception":[["auth","a"]],"stacktrace":{"frames":[{"in_app":true},["password","p"],{"filename":"x.js","vars":[["secret","s"]]}]}}`),
		[]byte(`{"extra":{"n":4242424242424242,"m":-4242424242424242,"f":4242424242424242.5,"pan":"4242 4242 4242 4242","pem":"-----BEGIN PRIVATE KEY-----\nMII\n-----END PRIVATE KEY-----","cmd":"curl -u root:pw1 -H 'X-Api-Key: k1'","auth":"Digest a=\"b\""},"pairs":[["x",4242424242424242]],"tags":[["password",4242424242424242]],"request":{"cookies":{"theme":4242424242424242}}}`),
		[]byte(`{"extra":{"card":{"number":"n","cvv":1,"x":{"value":1}},"billing_card":{"num":{"value":"v","type":"string"},"pan":null},"card":[{"number":1}]},"stacktrace":{"frames":[{"in_app":true,"context_line":"token = sign(key)","pre_context":["password: 'p'",{"token":"t","context_line":"pin: f(x)"}],"post_context":"x"}]},"request":{"cookies":{"theme":{"cookies":{"theme":"dark","sid":"s"}},"lang":["en"],"tz":"x"}}}`),
		[]byte(`[["token","t"],["page","2"]]`), []byte(`["token","t"]`),
		[]byte("not json"), []byte("null"), []byte(""), []byte(`["Bearer syntheticcredential"]`), []byte(`"just a string ?token=1"`), []byte("12.50"),
		[]byte(`{"a":`), []byte(`{"a":1,}`), []byte(`[1,2`),
		[]byte(`{"` + sentry.TruncatedField + `":["extra"],"message":"m"}`), []byte(`{}`),
		// Long stacks: frames cut, the detail of the middle ones dropped.
		[]byte(`{"threads":{"values":[{"stacktrace":{"frames":[` + strings.Repeat(`{"vars":{"a":1}},1,`, 130) + `{}]}}]}}`),
		// Names in any case, as the event parser reads them.
		[]byte(`{"Exception":{"Values":[{"Stacktrace":{"FRAMES":[` + strings.Repeat(`{"In_App":true,"Vars":{"a":1}},{"filename":"lib.js","Context_Line":"c"},`, 130) + `{}]}}]},` +
			`"BREADCRUMBS":{"VALUES":[` + strings.Repeat(`{"message":"step"},`, 60) + `{}]},"SDK":{"Integrations":["a"]},"Modules":{"x":"1"},"Debug_Meta":{}}`),
		[]byte(`{"exception":[{"stacktrace":{"frames":[` + strings.Repeat(`{"in_app":true,"vars":{"a":1},"context_line":"c","pre_context":["p"]},{"filename":"lib.js","vars":{"b":2}},`, 30) + `{}]}}],` +
			`"stacktrace":{"frames":[` + strings.Repeat(`{"post_context":["p"]},`, 255) + `{}]}}`),
	}

	for _, fixture := range fixtures.All(t) {
		envelope, err := sentry.ParseEnvelope(fixture.Body)

		if err != nil {
			t.Fatal(err)
		}

		for _, item := range envelope.Items {
			if item.Header.Type == sentry.ItemEvent || item.Header.Type == sentry.ItemLog {
				payloads = append(payloads, item.Payload)
			}
		}
	}

	return payloads
}

// largePayloads need the size limits, each of the cuts in turn; too large
// for the fuzz corpus.
func largePayloads(t testing.TB) [][]byte {
	stack := func(frameCount int, frame map[string]any) map[string]any {
		frames := []any{}

		for range frameCount {
			frames = append(frames, frame)
		}

		return map[string]any{"frames": frames}
	}
	// every has something for each cut, more of it the larger size.
	every := func(size int, more map[string]any) []byte {
		breadcrumbs := []any{}

		for range 50 {
			breadcrumbs = append(breadcrumbs, map[string]any{"message": strings.Repeat("b", size<<8)})
		}

		fields := map[string]any{
			"breadcrumbs": map[string]any{"values": breadcrumbs},
			"extra":       filler(3 * size),
			"request":     map[string]any{"method": "POST", "data": filler(size), "headers": map[string]any{"a": "b"}},
			"contexts": map[string]any{
				"trace": map[string]any{"trace_id": "0123456789abcdef0123456789abcdef"}, "os": map[string]any{"name": "Linux"},
				"state": filler(size),
			},
			"exception": map[string]any{"values": []any{map[string]any{"type": "Error", "mechanism": map[string]any{"vars": "x"}, "stacktrace": stack(size, map[string]any{"function": "f", "vars": filler(1)})}}},
			"threads":   []any{map[string]any{"stacktrace": stack(size, map[string]any{"function": "g", "pre_context": []string{strings.Repeat("p", 10<<10)}, "post_context": []string{"after"}})}},
		}

		maps.Copy(fields, more)

		return longEvent(t, fields)
	}

	return [][]byte{
		longEvent(t, map[string]any{"extra": map[string]any{"note": strings.Repeat("é", 20<<10), "short": "kept"}, "message": "\xff" + strings.Repeat("\xfe", 6000)}),
		every(10, map[string]any{"tags": filler(1)}), // the oldest breadcrumbs and extra are enough
		every(1, map[string]any{"tags": filler(70)}), // every cut
		longEvent(t, map[string]any{"stacktrace": stack(70, map[string]any{"function": strings.Repeat("f", 20<<10)})}),
		// frames cut first, then the variables of the frames kept
		longEvent(t, map[string]any{"stacktrace": longStack()}),
	}
}

// normalized is valid JSON decoded and written again with sorted keys, so
// outputs that differ only in key order or escapes compare equal; anything
// else is compared as it is.
func normalized(payload []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any

	if decoder.Decode(&value) != nil {
		return "raw " + string(payload)
	}

	encoded, _ := json.Marshal(value)

	return string(encoded)
}

// referencePrepare is the tree pass prepareEvent replaced: decode
// everything, remove credentials, then drop what does not help a fix; then
// the size limits (referenceTruncate).
func referencePrepare(payload []byte) []byte {
	var earlier []string

	compacted := referencePass(payload, func(document any) any {
		document = referenceValue(document, false)

		if event, ok := document.(map[string]any); ok && event != nil && referenceEventFields(event) {
			earlier = []string{"frames"}
		}

		return document
	})

	return referenceTruncate(compacted, earlier)
}

// referenceTruncate applies the size limits to a prepared event as a tree:
// long strings cut, then, while the event is too large, one more of the
// cuts, each time from the whole event again. It decides on the size of its
// own encoding, so it may disagree with the token pass near the limit only.
// earlier are the cuts of the compaction, named first.
func referenceTruncate(payload []byte, earlier []string) []byte {
	decode := func() (map[string]any, bool) {
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.UseNumber()

		var event map[string]any

		return event, decoder.Decode(&event) == nil && event != nil
	}

	if _, ok := decode(); !ok {
		return payload
	}

	unchanged := func() []byte {
		if len(earlier) == 0 {
			return payload
		}

		event, _ := decode()
		event[sentry.TruncatedField] = earlier
		output, _ := referenceEncode(event)

		return output
	}

	steps := []string{"breadcrumbs", "extra", "request.data", "contexts", "frame_vars", "frame_context", "other"}

	for level := 0; level <= len(steps); level++ {
		event, _ := decode()
		names := referenceCuts(event, steps[:level])

		// The last resort is the same function, on the tree's encoding.
		if encoded, _ := referenceEncode(event); level == len(steps) && len(encoded) > config.MaxEventBytes-truncatedRoom {
			fit, _ := fitted(encoded, config.MaxEventBytes-truncatedRoom, maxFitDepth)
			decoder := json.NewDecoder(bytes.NewReader(fit))
			decoder.UseNumber()

			event = nil
			_ = decoder.Decode(&event)

			names = append(names, "size")
		}

		if len(names) == 0 {
			if len(payload) <= config.MaxEventBytes || level == len(steps) {
				return unchanged()
			}

			continue
		}

		event[sentry.TruncatedField] = slices.Concat(earlier, names)
		output, err := referenceEncode(event)

		if err != nil {
			return payload
		}

		if len(output) <= config.MaxEventBytes || level == len(steps) {
			return output
		}
	}

	return unchanged()
}

// referenceCuts cuts long strings and applies steps to event, and returns
// the names of the cuts that removed something, in their order.
func referenceCuts(event map[string]any, steps []string) []string {
	applies := func(name string) bool { return slices.Contains(steps, name) }
	done := map[string]bool{}

	if applies("other") {
		for name := range event {
			if field := sentry.FoldedName(name); !store.ErrorFields[field] && field != "contexts" {
				delete(event, name)

				done["other"] = true
			}
		}
	}

	for _, key := range referenceKeys(event, "breadcrumbs") {
		if !applies("breadcrumbs") {
			break
		}

		newest := func(list []any) []any {
			if len(list) <= config.TruncatedBreadcrumbs {
				return list
			}

			done["breadcrumbs"] = true

			return list[len(list)-config.TruncatedBreadcrumbs:]
		}

		switch breadcrumbs := event[key].(type) {
		case []any:
			event[key] = newest(breadcrumbs)
		case map[string]any:
			for _, name := range referenceKeys(breadcrumbs, "values") {
				if list, ok := breadcrumbs[name].([]any); ok {
					breadcrumbs[name] = newest(list)
				}
			}
		}
	}

	if keys := referenceKeys(event, "extra"); len(keys) > 0 && applies("extra") {
		referenceDelete(event, "extra")

		done["extra"] = true
	}

	for _, key := range referenceKeys(event, "request") {
		if request, ok := event[key].(map[string]any); ok && applies("request.data") && len(referenceKeys(request, "data")) > 0 {
			referenceDelete(request, "data")

			done["request.data"] = true
		}
	}

	for _, key := range referenceKeys(event, "contexts") {
		if contexts, ok := event[key].(map[string]any); ok && applies("contexts") {
			for name := range contexts {
				if !slices.Contains([]string{"runtime", "os", "browser", "trace"}, name) {
					delete(contexts, name)

					done["contexts"] = true
				}
			}
		}
	}

	var strip func(node any)

	strip = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			for name, step := range map[string]string{"vars": "frame_vars", "pre_context": "frame_context", "post_context": "frame_context"} {
				if keys := referenceKeys(typed, name); len(keys) > 0 && applies(step) {
					referenceDelete(typed, name)

					done[step] = true
				}
			}

			for _, child := range typed {
				strip(child)
			}
		case []any:
			for _, child := range typed {
				strip(child)
			}
		}
	}

	for _, field := range []string{"exception", "threads", "stacktrace"} {
		for _, key := range referenceKeys(event, field) {
			strip(event[key])
		}
	}

	var cut func(node any) any

	cut = func(node any) any {
		switch typed := node.(type) {
		case map[string]any:
			for name, child := range typed {
				typed[name] = cut(child)
			}
		case []any:
			for index, child := range typed {
				typed[index] = cut(child)
			}
		case string:
			if len(typed) > config.MaxEventStringBytes {
				done["strings"] = true

				return textutil.ShortenBytes(typed, config.MaxEventStringBytes)
			}
		}

		return node
	}

	cut(event)

	var names []string

	for _, name := range append([]string{"strings"}, steps...) {
		if done[name] {
			names = append(names, name)
		}
	}

	return names
}

// referenceScrub is the tree pass scrubJSON replaced.
func referenceScrub(payload []byte) []byte {
	return referencePass(payload, func(document any) any { return referenceValue(document, false) })
}

func referencePass(payload []byte, change func(any) any) []byte {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var document any

	if err := decoder.Decode(&document); err != nil {
		return payload
	}

	output, err := referenceEncode(change(document))

	if err != nil {
		return payload
	}

	return output
}

func referenceEncode(value any) ([]byte, error) {
	var output bytes.Buffer

	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimRight(output.Bytes(), "\n"), nil
}

func referenceValue(node any, inList bool) any {
	return referenceText(node, scrubText, inList)
}

// referenceText is referenceValue with its strings through text, plus the
// rules that depend on a member's name: source lines and a card's number.
func referenceText(node any, text func(string) string, inList bool) any {
	switch typed := node.(type) {
	case map[string]any:
		for key, child := range typed {
			switch {
			case isCookieKey(key):
				typed[key] = filteredCookies(child, text)
			case isSensitiveKey(key):
				typed[key] = referenceFiltered(child)
			case sourceFields[key]:
				typed[key] = referenceText(child, scrubSourceText, false)
			default:
				typed[key] = referenceText(filteredCardNumber(key, child), text, false)
			}
		}

		return typed
	case []any:
		if inList && len(typed) == 2 {
			if name, ok := typed[0].(string); ok && isSensitiveKey(name) {
				typed[1] = referenceFiltered(typed[1])

				return typed
			}
		}

		for index, child := range typed {
			typed[index] = referenceText(child, text, true)
		}

		return typed
	case string:
		return text(typed)
	case json.Number:
		if isCardNumber(typed) {
			return filteredText
		}

		return node
	default:
		return node
	}
}

func referenceFiltered(node any) any {
	if attribute, ok := node.(map[string]any); ok {
		if _, hasValue := attribute["value"]; hasValue {
			return map[string]any{"value": filteredText, "type": "string"}
		}
	}

	if node == nil {
		return nil
	}

	return filteredText
}

// referenceKeys are the keys of object that read as field: the event parser
// reads names in any case.
func referenceKeys(object map[string]any, field string) []string {
	var keys []string

	for key := range object {
		if sentry.FoldedName(key) == field {
			keys = append(keys, key)
		}
	}

	return keys
}

func referenceDelete(object map[string]any, fields ...string) {
	for _, field := range fields {
		for _, key := range referenceKeys(object, field) {
			delete(object, key)
		}
	}
}

// referenceEventFields reports whether a stack lost frames.
func referenceEventFields(event map[string]any) bool {
	referenceDelete(event, "modules", "debug_meta", sentry.TruncatedField)

	for _, key := range referenceKeys(event, "sdk") {
		if sdk, ok := event[key].(map[string]any); ok {
			referenceDelete(sdk, "integrations", "packages")
		}
	}

	for _, key := range referenceKeys(event, "breadcrumbs") {
		if event[key] = referenceNewestBreadcrumbs(event[key]); event[key] == nil {
			delete(event, key)
		}
	}

	framesDropped := false

	for _, field := range []string{"exception", "threads"} {
		for _, key := range referenceKeys(event, field) {
			for _, item := range referenceValues(event[key]) {
				if object, ok := item.(map[string]any); ok {
					for _, name := range referenceKeys(object, "stacktrace") {
						framesDropped = referenceStacktrace(object[name]) || framesDropped
					}
				}
			}
		}
	}

	for _, key := range referenceKeys(event, "stacktrace") {
		framesDropped = referenceStacktrace(event[key]) || framesDropped
	}

	return framesDropped
}

// referenceValues are the items of a list, or of every "values" list of an object.
func referenceValues(node any) []any {
	switch typed := node.(type) {
	case []any:
		return typed
	case map[string]any:
		var items []any

		for _, key := range referenceKeys(typed, "values") {
			list, _ := typed[key].([]any)
			items = append(items, list...)
		}

		return items
	}

	return nil
}

func referenceNewestBreadcrumbs(node any) any {
	trim := func(list []any) []any { return list[max(0, len(list)-50):] }

	switch typed := node.(type) {
	case []any:
		return trim(typed)
	case map[string]any:
		for _, key := range referenceKeys(typed, "values") {
			if list, ok := typed[key].([]any); ok {
				typed[key] = trim(list)
			}
		}

		return typed
	}

	return node
}

// referenceStacktrace keeps the 50 oldest and 200 newest frames, drops the
// source of library frames when there are frames of the application, and
// over 50 frames with source the detail of those not among the 25 at each
// end; it reports whether frames were dropped.
func referenceStacktrace(node any) bool {
	stacktrace, ok := node.(map[string]any)

	if !ok {
		return false
	}

	dropped := false

	for _, key := range referenceKeys(stacktrace, "frames") {
		frames, isList := stacktrace[key].([]any)

		if !isList {
			continue
		}

		if len(frames) > 250 {
			dropped = true
			frames = slices.Concat(frames[:50], frames[len(frames)-200:])
			stacktrace[key] = frames
		}

		referenceFrames(frames)
	}

	return dropped
}

func referenceFrames(frames []any) {
	hasAppFrames := false

	for _, item := range frames {
		if frame, ok := item.(map[string]any); ok && referenceIsAppFrame(frame) {
			hasAppFrames = true

			break
		}
	}

	var withSource []map[string]any

	for _, item := range frames {
		frame, ok := item.(map[string]any)

		switch {
		case !ok:
			if !hasAppFrames {
				withSource = append(withSource, nil)
			}
		case !hasAppFrames || referenceIsAppFrame(frame):
			withSource = append(withSource, frame)
		default:
			referenceDelete(frame, "pre_context", "context_line", "post_context", "vars")
		}
	}

	if len(withSource) > 50 {
		for _, frame := range withSource[25 : len(withSource)-25] {
			referenceDelete(frame, "pre_context", "post_context", "vars")
		}
	}
}

func referenceIsAppFrame(frame map[string]any) bool {
	field := func(name string) any {
		for _, key := range referenceKeys(frame, name) {
			return frame[key]
		}

		return nil
	}

	filename, _ := field("filename").(string)
	absPath, _ := field("abs_path").(string)

	return field("in_app") == true && !sentry.IsLibraryPath(filename) && !sentry.IsLibraryPath(absPath)
}

// A JSON document in a string is read as one once: documents nested in
// strings, each level as large as the payload, cost what one document does.
func TestNestedDocumentsInTextStayLinear(t *testing.T) {
	filler := strings.Repeat("x", 1<<20)
	nested := `{"a":"` + filler + `"}`

	for range 16 {
		encoded, _ := json.Marshal(nested)
		nested = `{"a":` + string(encoded) + `}`
	}

	flat, _ := json.Marshal(`{"a":"` + strings.Repeat("x", len(nested)) + `"}`)
	nestedPayload, _ := json.Marshal(nested)

	started := time.Now()

	prepareEvent([]byte(`{"message":` + string(flat) + `}`))

	single := time.Since(started)
	started = time.Now()

	prepareEvent([]byte(`{"message":` + string(nestedPayload) + `}`))

	if took := time.Since(started); took > 4*single+50*time.Millisecond {
		t.Errorf("16 nested documents: %s, one document of the same size %s", took, single)
	}
}

var prepared []byte

func BenchmarkPrepareEvent(b *testing.B) {
	for _, name := range []string{"browser-event-handled", "node-event-handled-context", "node-logs"} {
		envelope, err := sentry.ParseEnvelope(fixtures.Envelope(b, name))

		if err != nil {
			b.Fatal(err)
		}

		payload := envelope.Items[0].Payload

		for _, method := range []struct {
			name string
			run  func([]byte) []byte
		}{{"tokens", prepareEvent}, {"tree", referencePrepare}} {
			b.Run(fmt.Sprintf("%s/%s", name, method.name), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))

				for b.Loop() {
					prepared = method.run(payload)
				}
			})
		}
	}
}

// Quotes in free text cost time in proportion to the size: an apostrophe
// opens no string, so it must not send the scan to the end of the text.
func TestQuotesInTextStayLinear(t *testing.T) {
	const size = 1 << 20

	baseline := strings.Repeat(`"a": 1, `, size/8)

	started := time.Now()

	scrubFreeText(baseline)

	linear := time.Since(started)

	for name, text := range map[string]string{
		"apostrophes":       strings.Repeat(`it's `, size/5),
		"open string":       `"` + strings.Repeat(`\"'`, size/3),
		"apostrophe values": strings.Repeat(`'a': '\'`, size/8),
		"collections":       strings.Repeat(`"token": [1, {"a": "]"}], `, size/26),
		"open collections":  strings.Repeat(`"token": [`, size/10),
		"backslashes":       `"token": [` + strings.Repeat(`\`, size),
		"escaped quotes":    `"token": [` + strings.Repeat(`\\\"a`, size/5),
		"service paths":     strings.Repeat("/services/T1/B1/a bot1:a webhooks/1/a ", size/38),
		"digits and colons": strings.Repeat("1:", size/2),
	} {
		started := time.Now()

		scrubFreeText(text)

		if took := time.Since(started); took > 10*linear+50*time.Millisecond {
			t.Errorf("%s: %s, %d bytes of pairs %s", name, took, len(baseline), linear)
		}
	}
}

// Multipart form data is read line by line once.
func TestMultipartStaysLinear(t *testing.T) {
	const size = 1 << 20

	baseline := strings.Repeat(`"a": 1, `, size/8)

	started := time.Now()

	scrubQuotedPairs(baseline)

	linear := time.Since(started)

	for name, text := range map[string]string{
		"parts":        strings.Repeat("--X\nContent-Disposition: form-data; name=\"password\"\n\na\n", size/52),
		"names":        strings.Repeat("Content-Disposition: form-data; name=password; name=a\n", size/54),
		"open value":   "Content-Disposition: form-data; name=password\n\n" + strings.Repeat("a\n", size/2),
		"long headers": "Content-Disposition: form-data; name=password\n" + strings.Repeat("form-data; name=a\n", size/18),
	} {
		started := time.Now()

		scrubMultipart(text)

		if took := time.Since(started); took > 10*linear+50*time.Millisecond {
			t.Errorf("%s: %s, %d bytes of pairs %s", name, took, len(baseline), linear)
		}
	}
}

// Cookie headers and keys are read in time proportional to their size.
func TestCookiesAndKeysStayLinear(t *testing.T) {
	const size = 1 << 20

	started := time.Now()

	scrubFreeText(strings.Repeat(`"a": 1, `, size/8))

	linear := time.Since(started)

	for name, run := range map[string]func(){
		"cookies":      func() { filteredCookieText(strings.Repeat("a=1; ", size/5)) },
		"empty values": func() { filteredCookieText(strings.Repeat("a ;", size/3)) },
		"key":          func() { isSensitiveKey(strings.Repeat("aB1_", size/4)) },
		"secret words": func() { isSensitiveKey(strings.Repeat("token_", size/6)) },
		"unicode key":  func() { isSensitiveKey(strings.Repeat("\u017fecret_", size/8)) },
	} {
		started := time.Now()

		run()

		if took := time.Since(started); took > 10*linear+50*time.Millisecond {
			t.Errorf("%s: %s, %d bytes of pairs %s", name, took, size, linear)
		}
	}
}

// Deep nesting costs time in proportion to the size: an array led by a
// secret's name must not be read again at every level (that grew with the
// square of the depth, a way to stall ingest with one envelope).
func TestDeepNestingStaysLinear(t *testing.T) {
	for name, payload := range map[string]string{
		"pairs":   strings.Repeat(`["token",`, 9000) + "1" + strings.Repeat(`,2]`, 9000),
		"objects": strings.Repeat(`{"session":{"a":`, 4500) + "1" + strings.Repeat(`}}`, 4500),
		"lists":   `{"breadcrumbs":[` + strings.Repeat(`[`, 9000) + strings.Repeat(`]`, 9000) + `]}`,
		"text":    `{"message":"` + strings.Repeat(`{\"session\":{\"a\":`, 4500) + "1" + strings.Repeat(`}}`, 4500) + `"}`,
		"cookies": `{"extra":` + strings.Repeat(`{"cookies":{"theme":`, 4500) + `"x"` + strings.Repeat(`}}`, 4500) + `}`,
	} {
		// Against the tree pass on the same input, so a slow machine slows both.
		started := time.Now()

		referencePrepare([]byte(payload))

		tree := time.Since(started)
		started = time.Now()

		prepareEvent([]byte(payload))

		if tokens := time.Since(started); tokens > 10*tree+50*time.Millisecond {
			t.Errorf("%s: %s, the tree pass %s, for %d bytes", name, tokens, tree, len(payload))
		}
	}
}

// Pairs in free text cost time in proportion to the size: each separator
// looks back a bounded distance, and the scan goes on after each value.
func TestKeyValuesInTextStayLinear(t *testing.T) {
	const size = 1 << 20

	baseline := strings.Repeat(`"a": 1, `, size/8)

	started := time.Now()

	scrubFreeText(baseline)

	linear := time.Since(started)

	for name, text := range map[string]string{
		"plain pairs":     strings.Repeat("a=a ", size/4),
		"empty secrets":   strings.Repeat("password= ", size/10),
		"chained secrets": strings.Repeat("password=", size/9),
		"colons":          strings.Repeat("token:", size/6),
		"field colons":    strings.Repeat(", token: ", size/9),
		"long key":        strings.Repeat("a", size) + "=1",
		"spaces":          strings.Repeat(" ", size) + "=1",
		"quoted keys":     strings.Repeat(`'a'=`, size/4),
		"brackets":        strings.Repeat(`[]=`, size/3),
		"open brackets":   strings.Repeat(`a]=`, size/3),
		"escaped values":  strings.Repeat(`password=\"`, size/11),
		"escaped pairs":   strings.Repeat(`\\\"password\\\":\\\"a\\\", `, size/32),
		"open escapes":    strings.Repeat(`\"password\":\\\"`, size/18),
		"escaped keys":    strings.Repeat(`\\\\\\\"`, size/8) + ":1",
		"collections":     strings.Repeat(`"token"=>[`, size/10),
		"open quotes":     `password='` + strings.Repeat(`\'`, size/2),
		"header lines":    strings.Repeat("Authorization: Token t\n", size/23),
		// A header line in quotes ends at its quote, and a refused value
		// must not send every pair after it to the end of the text.
		"quoted headers":  strings.Repeat(`'Authorization: ab' `, size/20),
		"double quoted":   strings.Repeat(`"Authorization: ab" `, size/20),
		"filtered bearer": strings.Repeat(`'Authorization: Bearer abcdefghijklmnopq' `, size/42),
		"quoted cookies":  strings.Repeat(`'Cookie: a' `, size/12),
		"cookie words":    strings.Repeat(`, cookies: a `, size/13),
		"prose after":     strings.Repeat(`, token: a1 b `, size/14),
		"spaced pairs":    strings.Repeat(`password = a1 b `, size/16),
		"markers":         strings.Repeat(`password=[Filtered]`, size/19),
	} {
		started := time.Now()

		scrubKeyValues(text, false) // scrubFreeText adds the regexp of links, whose cost is not this scan's

		if took := time.Since(started); took > 10*linear+50*time.Millisecond {
			t.Errorf("%s: %s, %d bytes of pairs %s", name, took, len(baseline), linear)
		}
	}
}

// The scans for schemes, command lines, card numbers, private keys, path
// parameters and URL passwords cost time in proportion to the size; against
// the regexp of schemes on words, the slowest linear scan.
func TestCredentialScansStayLinear(t *testing.T) {
	const size = 1 << 17

	baseline := strings.Repeat("Bearer a ", size/9)

	started := time.Now()

	scrubFreeText(baseline)

	linear := time.Since(started)

	for name, text := range map[string]string{
		"schemes":         strings.Repeat("Bearer ", size/7),
		"pairs":           strings.Repeat(`"a": 1, `, size/8),
		"quoted tokens":   strings.Repeat(`Token "a `, size/9),
		"digest":          "Digest " + strings.Repeat(`a="b", `, size/8),
		"basic users":     strings.Repeat("Basic a:", size/8),
		"command lines":   strings.Repeat("-u a:b ", size/7),
		"card groups":     strings.Repeat("4242 ", size/5),
		"digits":          strings.Repeat("4", size),
		"certificates":    strings.Repeat("-----BEGIN CERTIFICATE-----", size/27),
		"open labels":     strings.Repeat("-----BEGIN ", size/11),
		"private keys":    strings.Repeat("-----BEGIN PRIVATE KEY-----a-----END PRIVATE KEY-----", size/53),
		"path parameters": "/a" + strings.Repeat(";b=c", size/4),
		"userinfo":        "a://u:" + strings.Repeat("@a/", size/3),
		"schemes in URLs": strings.Repeat("a://u:p@", size/8),
	} {
		started := time.Now()

		scrubFreeText(text)

		if took := time.Since(started); took > 10*linear+50*time.Millisecond {
			t.Errorf("%s: %s, %d bytes of words after Bearer %s", name, took, len(baseline), linear)
		}
	}
}
