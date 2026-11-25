package grouping

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/sentry"
)

func fixtureEvent(t *testing.T, name string) *sentry.Event {
	t.Helper()

	envelope, err := sentry.ParseEnvelope(fixtures.Envelope(t, name))

	if err != nil {
		t.Fatal(err)
	}

	event, err := sentry.ParseEvent(envelope.Items[0].Payload, envelope.Header.EventID, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	return event
}

func parse(t *testing.T, payload string) *sentry.Event {
	t.Helper()

	event, err := sentry.ParseEvent([]byte(payload), "", time.Now())

	if err != nil {
		t.Fatal(err)
	}

	return event
}

// exceptionEvent builds an event with one exception and in-app frames given as "file|function|line".
func exceptionEvent(t *testing.T, errorType, value string, frames ...string) *sentry.Event {
	t.Helper()

	var frameJSON []string

	for _, frame := range frames {
		parts := strings.Split(frame, "|")
		frameJSON = append(frameJSON, `{"filename":"`+parts[0]+`","function":"`+parts[1]+`","lineno":`+parts[2]+`,"in_app":true}`)
	}

	return parse(t, `{"exception":{"values":[{"type":"`+errorType+`","value":"`+value+`",
		"stacktrace":{"frames":[{"filename":"node_modules/lib.js","function":"lib","in_app":false},`+strings.Join(frameJSON, ",")+`]}}]}}`)
}

func TestSameProblemGroupsTogether(t *testing.T) {
	testCases := map[string][2]*sentry.Event{
		"different value text": {
			exceptionEvent(t, "Error", "Order 890 not found", "app.js|load|10"),
			exceptionEvent(t, "Error", "Order 891 not found", "app.js|load|10"),
		},
		"code moved after a deploy": {
			exceptionEvent(t, "Error", "boom", "app.js|load|10", "app.js|main|3"),
			exceptionEvent(t, "Error", "boom", "app.js|load|42", "app.js|main|7"),
		},
		"message with ids": {
			parse(t, `{"message":"User 42 failed login from 10.0.0.1, request 9ec79c33-ec99-42ab-8353-589fcb2e04dc"}`),
			parse(t, `{"message":"User 7 failed login from 10.0.0.2, request 1b2c3d4e-0000-4000-8000-000000000001"}`),
		},
		"browser origin and bundle hash": {
			exceptionEvent(t, "TypeError", "x", "http://localhost:3500/assets/main.3f2a1b9c.js?v=1|render|1"),
			exceptionEvent(t, "TypeError", "x", "https://app.example.com/assets/main.77aa00ff.js|render|1"),
		},
		"vite chunk hash": {
			exceptionEvent(t, "TypeError", "x", "https://app.example.com/assets/Button-D4x_k9-a.js|render|1"),
			exceptionEvent(t, "TypeError", "x", "https://app.example.com/assets/Button-BbtQ7Ui0.js|render|1"),
		},
		// tsx watch adds "?<timestamp>" on every restart; the Node SDK copies it into the module.
		"cache-busting query in module": {
			parse(t, `{"exception":{"values":[{"type":"Error","value":"x","stacktrace":{"frames":[
				{"module":"tsx.dist.esm:index.mjs?1794411765314","filename":"/app/index.mjs?1794411765314","function":"F","in_app":false}]}}]}}`),
			parse(t, `{"exception":{"values":[{"type":"Error","value":"x","stacktrace":{"frames":[
				{"module":"tsx.dist.esm:index.mjs?1794411844942","filename":"/app/index.mjs?1794411844942","function":"F","in_app":false}]}}]}}`),
		},
	}

	for name, events := range testCases {
		t.Run(name, func(t *testing.T) {
			if Fingerprint(events[0]) != Fingerprint(events[1]) {
				t.Fatalf("want one group:\n%q\n%q", Components(events[0]), Components(events[1]))
			}
		})
	}
}

func TestDifferentProblemsSplit(t *testing.T) {
	testCases := map[string][2]*sentry.Event{
		"different type": {
			exceptionEvent(t, "TypeError", "boom", "app.js|load|10"),
			exceptionEvent(t, "RangeError", "boom", "app.js|load|10"),
		},
		"different place": {
			exceptionEvent(t, "Error", "boom", "app.js|load|10"),
			exceptionEvent(t, "Error", "boom", "app.js|save|10"),
		},
		"different message": {
			parse(t, `{"message":"Payment provider responded slowly"}`),
			parse(t, `{"message":"Payment provider is down"}`),
		},
		"no stack, different value": {
			parse(t, `{"exception":{"values":[{"type":"Error","value":"disk full"}]}}`),
			parse(t, `{"exception":{"values":[{"type":"Error","value":"permission denied"}]}}`),
		},
		"same message, different level of the chain": {
			parse(t, `{"exception":{"values":[{"type":"SyntaxError","value":"x"},{"type":"Error","value":"load failed"}]}}`),
			parse(t, `{"exception":{"values":[{"type":"TimeoutError","value":"x"},{"type":"Error","value":"load failed"}]}}`),
		},
	}

	for name, events := range testCases {
		t.Run(name, func(t *testing.T) {
			if Fingerprint(events[0]) == Fingerprint(events[1]) {
				t.Fatalf("want two groups, both are %q", Components(events[0]))
			}
		})
	}
}

// sourceEvent is an Error thrown in function at lineno, with the source line the SDK sends.
func sourceEvent(t *testing.T, function string, lineno int, source string) *sentry.Event {
	t.Helper()

	return parse(t, fmt.Sprintf(`{"exception":{"values":[{"type":"Error","value":"x","stacktrace":{"frames":[
		{"filename":"routes.ts","function":%q,"lineno":%d,"in_app":true,"context_line":%q}]}}]}}`, function, lineno, source))
}

func TestSourceLineSeparatesSameNamedFunctions(t *testing.T) {
	// JS names `handler: () => {...}` after the property, so many functions in
	// one file are called "handler". Found in the playground: 9 different
	// errors merged into one issue.
	checkout := sourceEvent(t, "handler", 120, `throw new Error("Checkout failed");`)
	invoice := sourceEvent(t, "handler", 240, `throw new Error("Could not generate invoice PDF");`)

	if Fingerprint(checkout) == Fingerprint(invoice) {
		t.Fatal("different throws in functions with the same name must not merge")
	}

	// The same line after a deploy moved it: still the same issue.
	moved := sourceEvent(t, "handler", 135, `    throw new Error("Checkout failed");`)

	if Fingerprint(checkout) != Fingerprint(moved) {
		t.Fatalf("a moved line must stay in its group:\n%q\n%q", Components(checkout), Components(moved))
	}
}

func TestAnonymousFunctions(t *testing.T) {
	handler := func(line string) *sentry.Event {
		return parse(t, `{"exception":{"values":[{"type":"Error","value":"x","stacktrace":{"frames":[
			{"filename":"routes.ts","function":"<anonymous>","lineno":10,"in_app":true,"context_line":"`+line+`"}]}}]}}`)
	}
	// Two different Express route handlers in one file: both are "<anonymous>".
	if Fingerprint(handler("throw new Error('no orders')")) == Fingerprint(handler("throw new Error('no users')")) {
		t.Fatal("different anonymous functions must not merge")
	}
	// Indentation changes don't matter.
	if Fingerprint(handler("  throw new Error('no orders')")) != Fingerprint(handler("throw new Error('no orders')")) {
		t.Fatal("whitespace must not split a group")
	}
}

func TestStackWithoutSourceAddsValue(t *testing.T) {
	// Browser SDKs send no source lines, so a shared frame is weak evidence.
	if Fingerprint(fixtureEvent(t, "browser-event-handled")) == Fingerprint(fixtureEvent(t, "browser-event-unhandled-rejection")) {
		t.Fatal("different browser errors in one handler must not merge")
	}
	// With source lines the value is not used: the stack is enough.
	withSource := func(value string) *sentry.Event {
		return parse(t, `{"exception":{"values":[{"type":"Error","value":"`+value+`","stacktrace":{"frames":[
			{"filename":"a.ts","function":"load","in_app":true,"context_line":"throw err"}]}}]}}`)
	}

	if Fingerprint(withSource("user alice not found")) != Fingerprint(withSource("user bob not found")) {
		t.Fatal("with source lines, the value must not split groups")
	}
}

func TestCustomFingerprint(t *testing.T) {
	base := `"exception":{"values":[{"type":"Error","value":"boom","stacktrace":{"frames":[{"filename":"a.js","function":"f","in_app":true}]}}]}`
	custom := parse(t, `{"fingerprint":["payment","stripe"],`+base+`}`)
	sameCustomOtherStack := parse(t, `{"fingerprint":["payment","stripe"],"message":"other"}`)
	withDefault := parse(t, `{"fingerprint":["{{ default }}","eu"],`+base+`}`)
	plain := parse(t, `{`+base+`}`)

	if Fingerprint(custom) != Fingerprint(sameCustomOtherStack) {
		t.Fatal("a custom fingerprint must override the stack")
	}

	if got := Components(withDefault); got[len(got)-1] != "eu" || !strings.Contains(strings.Join(got, " "), "frame:a.js f") {
		t.Fatalf("{{ default }} not expanded: %q", got)
	}

	if Fingerprint(withDefault) == Fingerprint(plain) {
		t.Fatal("{{ default }} plus an extra value must differ from the default alone")
	}
	// A custom value must not collide with a default component of the same text.
	if Fingerprint(parse(t, `{"fingerprint":["message","hi"]}`)) == Fingerprint(parse(t, `{"message":"hi"}`)) {
		t.Fatal("custom and default fingerprints collide")
	}
}

func TestFixtures(t *testing.T) {
	message := fixtureEvent(t, "node-event-message")

	if got := Components(message); got[0] != "message" {
		t.Fatalf("captureMessage must group by message, got %q", got)
	}

	unhandled := fixtureEvent(t, "node-event-unhandled")
	components := strings.Join(Components(unhandled), "\n")

	if strings.Contains(components, "node_modules") {
		t.Fatalf("library frames must be skipped when there are in-app frames:\n%s", components)
	}

	if Fingerprint(fixtureEvent(t, "node-event-custom-fingerprint")) == Fingerprint(unhandled) {
		t.Fatal("custom fingerprint fixture grouped with an unrelated error")
	}

	if Culprit(unhandled) == "" || Culprit(fixtureEvent(t, "browser-event-onerror")) == "" {
		t.Fatal("empty culprit")
	}
}

func TestNormalizeMessage(t *testing.T) {
	testCases := map[string]string{
		"Order 890 not found":                          "Order <n> not found",
		"Sentry test error 2026-11-11T13:31:10.278Z":   "Sentry test error <n>-<n>-<n>T<n>:<n>:<n>.<n>Z",
		"request 9ec79c33-ec99-42ab-8353-589fcb2e04dc": "request <uuid>",
		"object 0x7f3a9c00ffee12 freed":                "object <hex> freed",
		"commit deadbeefcafe1234":                      "commit <hex>",
		"short hex abc12 stays text-ish":               "short hex abc<n> stays text-ish",
		"Cannot read properties of undefined":          "Cannot read properties of undefined",
	}

	for input, want := range testCases {
		if got := NormalizeMessage(input); got != want {
			t.Errorf("NormalizeMessage(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCulprit(t *testing.T) {
	if got := Culprit(parse(t, `{"transaction":"GET /api/orders","message":"x"}`)); got != "GET /api/orders" {
		t.Fatalf("culprit = %q", got)
	}

	event := exceptionEvent(t, "Error", "x", "src/orders.ts|loadOrders|10", "src/db.ts|query|5")

	if got := Culprit(event); got != "query (src/db.ts)" {
		t.Fatalf("culprit = %q, want the frame that threw", got)
	}
}

func TestLogFingerprint(t *testing.T) {
	same := LogFingerprint("order 17 failed for 0b7c9a3e-1d2f-4a5b-8c6d-7e8f9a0b1c2d")

	if same != LogFingerprint("order 42 failed for 9a8b7c6d-5e4f-4a3b-8c2d-1e0f9a8b7c6d") {
		t.Error("the same message with other values must share a fingerprint")
	}

	if same == LogFingerprint("payment failed") {
		t.Error("different messages must not share a fingerprint")
	}
}

// browserEvent builds a stack as a browser SDK sends it in dev: every frame
// in-app, no source; "file|function|line", oldest first.
func browserEvent(t *testing.T, value string, frames ...string) *sentry.Event {
	t.Helper()

	var frameJSON []string

	for _, frame := range frames {
		parts := strings.Split(frame, "|")
		frameJSON = append(frameJSON, `{"filename":"http://web.tracklify.localhost:8080`+parts[0]+`","function":"`+parts[1]+`","lineno":`+parts[2]+`,"in_app":true}`)
	}

	return parse(t, `{"exception":{"values":[{"type":"TypeError","value":"`+value+`",
		"stacktrace":{"frames":[`+strings.Join(frameJSON, ",")+`]}}]}}`)
}

// The same error in a Vue page: once with only Vue's scheduler under it, once
// with the async path the click took (Firefox adds it, marked with "*"), the
// library frames all marked in-app by the browser SDK. One issue.
func TestBrowserErrorGroupsByItsSynchronousAppFrames(t *testing.T) {
	const value = `can't access property \"name\", taskUnderEdit.value.lastEditor is undefined`

	alone := browserEvent(t, value,
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|flushJobs|2146",
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|flushPostFlushCbs|2102",
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|callWithErrorHandling|1960",
		"/src/views/TaskView.vue|setup/<|65")
	fromClick := browserEvent(t, value,
		"/node_modules/.vite/deps/@sentry_vue.js|sentryWrapped|12683",
		"/src/components/widgets/Task.vue|_sfc_render/_cache[6]|430",
		"/src/views/MyTasks.vue|onClick|392",
		"/src/views/MyTasks.vue|openTaskDialog|105",
		"/src/store/project.js|mutationOpenTaskPopup|390",
		"/node_modules/.vite/deps/vue-router.js|promise callback*pushWithRedirect|6677",
		"/node_modules/.vite/deps/vue-router.js|finalizeNavigation|6768",
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|promise callback*queueFlush|2051",
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|flushJobs|2146",
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|flushPostFlushCbs|2102",
		"/src/views/TaskView.vue|setup/<|65")

	if Fingerprint(alone) != Fingerprint(fromClick) {
		t.Errorf("one error, two issues:\n%v\n%v", Components(alone), Components(fromClick))
	}

	if culprit := Culprit(fromClick); culprit != "setup/< (/src/views/TaskView.vue)" {
		t.Errorf("culprit = %q", culprit)
	}

	// Another place in the app's code is another problem.
	elsewhere := browserEvent(t, value,
		"/node_modules/.vite/deps/vue.runtime.esm-bundler-DTzGrwt5.js|flushJobs|2146",
		"/src/views/TaskHistory.vue|setup/<|40")

	if Fingerprint(alone) == Fingerprint(elsewhere) {
		t.Error("errors of two components grouped together")
	}
}

func TestNormalizeFilename(t *testing.T) {
	testCases := map[string]string{
		"https://app.example.com/assets/index-BbtQ7Ui0.js?v=2": "/assets/index.js",
		"/assets/Button-D4x_k9-a.js":                           "/assets/Button.js",
		"/assets/chart--abc1234.js":                            "/assets/chart.js",
		"/assets/index-BbtQ7Ui0.css":                           "/assets/index.css",
		"/node_modules/.vite/deps/chunk-ABCD1234.js":           "/node_modules/.vite/deps/chunk.js",
		"dist.server.assets:index-BbtQ7Ui0":                    "dist.server.assets:index",
		"/assets/main.3f2a1b9c.js":                             "/assets/main.js",
		"/static/js/main.3f2a1b9c4d5e6f70.chunk.js":            "/static/js/main.chunk.js",
		"/static/js/123.4a5b6c7d.js":                           "/static/js/123.js",
		"/_next/static/chunks/pages/index-4a5b6c7d8e9f0a1b.js": "/_next/static/chunks/pages/index.js",
		"/_next/static/chunks/src_app_page_tsx_1a2b3c4d._.js":  "/_next/static/chunks/src_app_page_tsx._.js",
		"/_app/immutable/chunks/scheduler.BvLojk_z.js":         "/_app/immutable/chunks/scheduler.js",
		"/_app/immutable/nodes/2.CjEaNOLf.js":                  "/_app/immutable/nodes/2.js",
		"/_astro/Counter.BkGx5dAS.js":                          "/_astro/Counter.js",
		"/_astro/app.Settings.BkGx5dAS.js":                     "/_astro/app.Settings.js",
		// Words stay: no digit and no uppercase letter after the first character.
		// An all-lowercase Rollup hash (about 1 in 400) stays too and opens a new issue.
		"/src/components/my-component.js":   "/src/components/my-component.js",
		"/src/hooks/use-settings.js":        "/src/hooks/use-settings.js",
		"/src/pages/page-Settings.js":       "/src/pages/page-Settings.js",
		"/assets/index-abcdefgh.js":         "/assets/index-abcdefgh.js",
		"/src/api/user-profile-v2.js":       "/src/api/user-profile-v2.js",
		"/app/src/routes/settings-page.tsx": "/app/src/routes/settings-page.tsx",
		"/src/store/user.reducers.js":       "/src/store/user.reducers.js",
		"/src/app.Settings.js":              "/src/app.Settings.js",
		"dist.routes:index-Settings":        "dist.routes:index-Settings",
	}

	for filename, want := range testCases {
		if got := normalizeFilename(filename); got != want {
			t.Errorf("normalizeFilename(%q) = %q, want %q", filename, got, want)
		}
	}
}

// groupCases checks pairs that must be one issue (true) or two (false).
func groupCases(t *testing.T, testCases map[string]struct {
	same   bool
	events [2]*sentry.Event
},
) {
	t.Helper()

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			if same := Fingerprint(testCase.events[0]) == Fingerprint(testCase.events[1]); same != testCase.same {
				t.Fatalf("same = %v, want %v:\n%q\n%q", same, testCase.same, Components(testCase.events[0]), Components(testCase.events[1]))
			}
		})
	}
}

type groupCase = struct {
	same   bool
	events [2]*sentry.Event
}

func TestMessagesGroupByTemplate(t *testing.T) {
	logEntry := func(template, formatted string) *sentry.Event {
		return parse(t, fmt.Sprintf(`{"logentry":{"message":%q,"params":["x"],"formatted":%q}}`, template, formatted))
	}

	groupCases(t, map[string]groupCase{
		"one template, different words": {true, [2]*sentry.Event{
			logEntry("Login failed for %s", "Login failed for alice"),
			logEntry("Login failed for %s", "Login failed for bob"),
		}},
		"the template in the message field": {true, [2]*sentry.Event{
			parse(t, `{"message":{"message":"Cart of %s is empty","formatted":"Cart of alice is empty"}}`),
			parse(t, `{"message":{"message":"Cart of %s is empty","formatted":"Cart of bob is empty"}}`),
		}},
		"different templates": {false, [2]*sentry.Event{
			logEntry("Login failed for %s", "Login failed for alice"),
			logEntry("Logout failed for %s", "Logout failed for alice"),
		}},
		"no template: the text, normalized": {true, [2]*sentry.Event{
			parse(t, `{"message":"Order 17 failed"}`),
			parse(t, `{"logentry":{"formatted":"Order 18 failed"}}`),
		}},
		"a template without parameters is the text": {true, [2]*sentry.Event{
			parse(t, `{"message":"Payment provider is down"}`),
			parse(t, `{"logentry":{"message":"Payment provider is down"}}`),
		}},
	})
}

func TestFingerprintVariables(t *testing.T) {
	event := func(fingerprint, fields, errorType, function string) *sentry.Event {
		return parse(t, `{"fingerprint":`+fingerprint+`,`+fields+`"exception":{"values":[{"type":"`+errorType+`","value":"boom",
			"stacktrace":{"frames":[{"filename":"node_modules/lib.js","abs_path":"/app/node_modules/lib.js","module":"lib","function":"lib","in_app":false},
			{"filename":"app.js","abs_path":"/app/`+function+`.js","module":"app.`+function+`","function":"`+function+`","in_app":true}]}}]}}`)
	}

	groupCases(t, map[string]groupCase{
		"error.type splits": {false, [2]*sentry.Event{
			event(`["{{ error.type }}"]`, ``, "TypeError", "load"),
			event(`["{{ error.type }}"]`, ``, "RangeError", "load"),
		}},
		"error.type ignores the stack": {true, [2]*sentry.Event{
			event(`["{{ error.type }}"]`, ``, "TypeError", "load"),
			event(`["{{ error.type }}"]`, ``, "TypeError", "save"),
		}},
		"error.value": {false, [2]*sentry.Event{
			event(`["{{ error.value }}"]`, ``, "Error", "load"),
			parse(t, `{"fingerprint":["{{ error.value }}"],"exception":{"values":[{"type":"Error","value":"other"}]}}`),
		}},
		"tags.NAME": {false, [2]*sentry.Event{
			event(`["tenant", "{{ tags.tenant }}"]`, `"tags":{"tenant":"acme"},`, "Error", "load"),
			event(`["tenant", "{{ tags.tenant }}"]`, `"tags":{"tenant":"globex"},`, "Error", "load"),
		}},
		"inside text stays as written": {true, [2]*sentry.Event{
			event(`["tenant-{{tags.tenant}}"]`, `"tags":{"tenant":"acme"},`, "Error", "load"),
			event(`["tenant-{{tags.tenant}}"]`, `"tags":{"tenant":"globex"},`, "Error", "load"),
		}},
		"tags.environment from the field": {false, [2]*sentry.Event{
			event(`["{{ default }}", "{{ tags.environment }}"]`, `"environment":"production",`, "Error", "load"),
			event(`["{{ default }}", "{{ tags.environment }}"]`, `"environment":"staging",`, "Error", "load"),
		}},
		"stack.package": {false, [2]*sentry.Event{
			parse(t, `{"fingerprint":["{{ stack.package }}"],"exception":{"values":[{"type":"E","stacktrace":{"frames":[{"function":"f","package":"/usr/lib/libssl.so","in_app":true}]}}]}}`),
			parse(t, `{"fingerprint":["{{ stack.package }}"],"exception":{"values":[{"type":"E","stacktrace":{"frames":[{"function":"f","package":"/usr/lib/libz.so","in_app":true}]}}]}}`),
		}},
		"message as messages are grouped": {true, [2]*sentry.Event{
			parse(t, `{"fingerprint":["{{ message }}"],"message":"order 890 failed"}`),
			parse(t, `{"fingerprint":["{{ message }}"],"message":"order 891 failed"}`),
		}},
		"a value that is the message is grouped as the message": {true, [2]*sentry.Event{
			parse(t, `{"fingerprint":["order 890 failed"],"message":"order 890 failed"}`),
			parse(t, `{"fingerprint":["order 891 failed"],"message":"order 891 failed"}`),
		}},
		"transaction": {false, [2]*sentry.Event{
			event(`["{{ transaction }}"]`, `"transaction":"GET /orders",`, "Error", "load"),
			event(`["{{ transaction }}"]`, `"transaction":"GET /users",`, "Error", "load"),
		}},
		"level": {false, [2]*sentry.Event{
			event(`["{{ level }}"]`, `"level":"error",`, "Error", "load"),
			event(`["{{ level }}"]`, `"level":"fatal",`, "Error", "load"),
		}},
		"logger": {false, [2]*sentry.Event{
			event(`["{{ logger }}"]`, `"logger":"orders",`, "Error", "load"),
			event(`["{{ logger }}"]`, `"logger":"users",`, "Error", "load"),
		}},
		"message": {false, [2]*sentry.Event{
			parse(t, `{"fingerprint":["{{ message }}"],"message":"disk full"}`),
			parse(t, `{"fingerprint":["{{ message }}"],"message":"disk gone"}`),
		}},
		"stack.function of the in-app frame": {false, [2]*sentry.Event{
			event(`["{{ stack.function }}"]`, ``, "Error", "load"),
			event(`["{{ stack.function }}"]`, ``, "Error", "save"),
		}},
		"stack.abs_path": {false, [2]*sentry.Event{
			event(`["{{ stack.abs_path }}"]`, ``, "Error", "load"),
			event(`["{{ stack.abs_path }}"]`, ``, "Error", "save"),
		}},
		"stack.module": {false, [2]*sentry.Event{
			event(`["{{ stack.module }}"]`, ``, "Error", "load"),
			event(`["{{ stack.module }}"]`, ``, "Error", "save"),
		}},
		"an unknown variable stays literal": {true, [2]*sentry.Event{
			event(`["{{ nothing.here }}"]`, ``, "TypeError", "load"),
			event(`["{{ nothing.here }}"]`, ``, "RangeError", "save"),
		}},
	})

	if got := Components(event(`["{{ unknown }}"]`, ``, "Error", "load")); got[1] != "{{ unknown }}" {
		t.Errorf("unknown variable: %q", got)
	}
}

// TestFingerprintVariableValues checks each variable Sentry documents
// (docs.sentry.io, Fingerprint Rules: Variables) against what it resolves to.
func TestFingerprintVariableValues(t *testing.T) {
	const stack = `"exception":{"values":[{"type":"CauseError","value":"inner"},{"type":"TypeError","value":"boom 42",
		"stacktrace":{"frames":[{"filename":"main.js","abs_path":"/app/main.js","module":"main","function":"main","package":"/opt/app/main.so","in_app":true},
		{"filename":"lib.js","abs_path":"/app/node_modules/lib.js","module":"lib","function":"lib","package":"C:\\libs\\lib.dll","in_app":false},
		{"function":"promise callback*schedule"},
		{"filename":"src/save.js","abs_path":"/app/src/save.js","module":"save","function":"save","package":"save.so","in_app":true},
		{"filename":"node_modules/dep.js","abs_path":"/app/node_modules/dep.js","module":"dep","function":"dep","in_app":true}]}}]}`

	full := `{` + stack + `,"transaction":"GET /orders","level":"warn","logger":"orders","environment":"production",
		"release":"web@1.2","dist":"7","server_name":"web-1","user":{"email":"ann@example.com","ip_address":"10.0.0.1"},
		"tags":{"tenant":"acme","level":"tagged","release":"tagged","dist":"tagged","user":"tagged","filename":"x","function":"x","server_name":"tagged"},`
	bare := `{"exception":{"values":[{"stacktrace":{"frames":[{}]}}]},`
	beforeAsync := `{"exception":{"values":[{"stacktrace":{"frames":[{"function":"main","in_app":true},{"function":"promise callback*run"},{"function":"lib","in_app":false}]}}]},`
	tagsOnly := `{"tags":{"environment":"staging","logger":"tagged","transaction":"tagged","server_name":"tagged","sentry:release":"tagged"},`

	tests := []struct{ event, variable, want string }{
		{full, "{{ default }}", ""},
		{full, "{{ error.type }}", "TypeError"},
		{full, "{{type}}", "TypeError"},
		{full, "{{ error.value }}", "boom <n>"},
		{full, "{{ value }}", "boom <n>"},
		{full, "{{ level }}", "warning"},
		{full, "{{ logger }}", "orders"},
		{full, "{{ message }}", "boom <n>"},
		{bare, "{{ message }}", "<no-message>"},
		{`{"message":"Order 7 failed","exception":{"values":[{"type":"Error","value":"other"}]},`, "{{ message }}", "Order <n> failed"},
		{full, "{{ transaction }}", "GET /orders"},
		{full, "{{ stack.function }}", "save"},
		{full, "{{ function }}", "save"},
		{full, "{{ stack.abs_path }}", "/app/src/save.js"},
		{full, "{{ path }}", "/app/src/save.js"},
		{full, "{{ stack.filename }}", "src/save.js"},
		{full, "{{ stack.module }}", "save"},
		{full, "{{ module }}", "save"},
		{full, "{{ stack.package }}", "save.so"},
		{full, "{{ package }}", "save.so"},
		{full, "{{ tags.tenant }}", "acme"},
		{full, "{{ tags.environment }}", "production"},
		{full, "{{ tags.level }}", "warning"},
		{full, "{{ tags.logger }}", "orders"},
		{full, "{{ tags.transaction }}", "GET /orders"},
		{full, "{{ tags.server_name }}", "web-1"},
		{full, "{{ tags.sentry:release }}", "web@1.2"},
		{full, "{{ tags.sentry:dist }}", "7"},
		{full, "{{ tags.sentry:user }}", "email:ann@example.com"},
		{full, "{{ tags.release }}", "<no-value-for-tag-release>"},
		{full, "{{ tags.dist }}", "<no-value-for-tag-dist>"},
		{full, "{{ tags.user }}", "<no-value-for-tag-user>"},
		{full, "{{ tags.filename }}", "<no-value-for-tag-filename>"},
		{full, "{{ tags.function }}", "<no-value-for-tag-function>"},
		{full, "{{ tag.tenant }}", "{{ tag.tenant }}"},
		{full, "{{ stack.lineno }}", "{{ stack.lineno }}"},
		{full, "tenant-{{ tags.tenant }}", "tenant-{{ tags.tenant }}"},
		{beforeAsync, "{{ stack.function }}", "main"},
		{tagsOnly, "{{ tags.environment }}", "staging"},
		{tagsOnly, "{{ tags.logger }}", "tagged"},
		{tagsOnly, "{{ tags.transaction }}", "tagged"},
		{tagsOnly, "{{ tags.server_name }}", "tagged"},
		{tagsOnly, "{{ tags.sentry:release }}", "tagged"},
		{tagsOnly, "{{ tags.level }}", "error"},
		{tagsOnly, "{{ logger }}", "<no-logger>"},
		{bare, "{{ error.type }}", "<no-type>"},
		{bare, "{{ error.value }}", "<no-value>"},
		{bare, "{{ transaction }}", "<no-transaction>"},
		{bare, "{{ stack.function }}", "<no-function>"},
		{bare, "{{ stack.abs_path }}", "<no-abs-path>"},
		{bare, "{{ stack.filename }}", "<no-filename>"},
		{bare, "{{ stack.module }}", "<no-module>"},
		{bare, "{{ stack.package }}", "<no-package>"},
		{bare, "{{ tags.tenant }}", "<no-value-for-tag-tenant>"},
		{bare, "{{ tags.environment }}", "<no-value-for-tag-environment>"},
		{bare, "{{ tags.sentry:user }}", "<no-value-for-tag-sentry:user>"},
	}

	for _, test := range tests {
		event := parse(t, test.event+`"fingerprint":["`+test.variable+`"]}`)
		got := Components(event)

		if test.want == "" {
			if !slices.Equal(got[1:], defaultComponents(event)) {
				t.Errorf("%s: %q", test.variable, got)
			}

			continue
		}

		if len(got) != 2 || got[1] != test.want {
			t.Errorf("%s in %.30s: got %q, want %q", test.variable, test.event, got, test.want)
		}
	}
}

// frameEvent is an Error with one in-app frame, given as JSON fields.
func frameEvent(t *testing.T, platform, frame string) *sentry.Event {
	t.Helper()

	return parse(t, `{"platform":"`+platform+`","exception":{"values":[{"type":"Error","value":"x","stacktrace":{"frames":[
		{"in_app":true,`+frame+`}]}}]}}`)
}

func TestJavaScriptFunctionNames(t *testing.T) {
	groupCases(t, map[string]groupCase{
		"receiver of a method": {true, [2]*sentry.Event{
			frameEvent(t, "node", `"filename":"/app/routes.js","function":"Object.onClick","context_line":"throw err"`),
			frameEvent(t, "node", `"filename":"/app/routes.js","function":"Module.onClick","context_line":"throw err"`),
		}},
		"constructor": {true, [2]*sentry.Event{
			frameEvent(t, "javascript", `"filename":"/app/cart.js","function":"new Cart"`),
			frameEvent(t, "javascript", `"filename":"/app/cart.js","function":"Cart"`),
		}},
		"at prefix": {true, [2]*sentry.Event{
			frameEvent(t, "javascript", `"filename":"/app/cart.js","function":"at Cart.load"`),
			frameEvent(t, "javascript", `"filename":"/app/cart.js","function":"load"`),
		}},
		"a JavaScript file of another platform's event": {true, [2]*sentry.Event{
			frameEvent(t, "other", `"filename":"/assets/cart.mjs","function":"a.load"`),
			frameEvent(t, "other", `"filename":"/assets/cart.mjs","function":"b.load"`),
		}},
		"other functions still split": {false, [2]*sentry.Event{
			frameEvent(t, "node", `"filename":"/app/routes.js","function":"Object.onClick","context_line":"throw err"`),
			frameEvent(t, "node", `"filename":"/app/routes.js","function":"Object.onSubmit","context_line":"throw err"`),
		}},
		"other platforms keep the whole name": {false, [2]*sentry.Event{
			frameEvent(t, "python", `"module":"shop.cart","function":"Cart.load","context_line":"raise err"`),
			frameEvent(t, "python", `"module":"shop.cart","function":"Order.load","context_line":"raise err"`),
		}},
	})
}

func TestLongSourceLinesDoNotCount(t *testing.T) {
	minified := func(tail string) string {
		return `"filename":"/app/dist/bundle.js","function":"render","lineno":1,"context_line":"` + strings.Repeat("a", 130) + tail + `"`
	}
	short := func(line string) string {
		return `"filename":"/app/routes.js","function":"render","lineno":1,"context_line":"` + line + `"`
	}

	groupCases(t, map[string]groupCase{
		"minified bundles after a build": {true, [2]*sentry.Event{
			frameEvent(t, "node", minified("b")),
			frameEvent(t, "node", minified("c")),
		}},
		"short lines still count": {false, [2]*sentry.Event{
			frameEvent(t, "node", short("throw a")),
			frameEvent(t, "node", short("throw b")),
		}},
		// Go sends source lines too; they count on every platform.
		"other platforms": {false, [2]*sentry.Event{
			frameEvent(t, "go", `"module":"shop","function":"Load","context_line":"return errA"`),
			frameEvent(t, "go", `"module":"shop","function":"Load","context_line":"return errB"`),
		}},
	})

	// Without a source line that counts, the value tells errors apart, as in a browser.
	withValue := func(value string) *sentry.Event {
		return parse(t, `{"exception":{"values":[{"type":"Error","value":"`+value+`","stacktrace":{"frames":[{"in_app":true,`+minified("")+`}]}}]}}`)
	}

	if Fingerprint(withValue("disk full")) == Fingerprint(withValue("permission denied")) {
		t.Error("a long source line counted as the source")
	}

	// A long throw line under a caller with a short one: the caller's line is
	// the same, so the value tells the throws apart (Node snips lines over 150
	// characters, so every long Node line is such a case).
	thrownAt := func(line, value string) *sentry.Event {
		return parse(t, fmt.Sprintf(`{"platform":"node","exception":{"values":[{"type":"NotFoundError","value":%q,"stacktrace":{"frames":[
			{"filename":"/app/src/routes.ts","function":"handler","in_app":true,"context_line":"const order = await loadOrder(id);"},
			{"filename":"/app/src/orders.ts","function":"loadOrder","in_app":true,"context_line":%q}]}}]}}`, value, line))
	}
	order := thrownAt("throw new NotFoundError(`Order ${orderId} was not found for customer ${customerId}; check the id and try the request again`);", "Order 1 was not found")
	customer := thrownAt("throw new NotFoundError(`Customer ${customerId} was not found while loading order ${orderId}; check the id and try again`);", "Customer 2 was not found")

	if Fingerprint(order) == Fingerprint(customer) {
		t.Error("two long throw lines in one function merged")
	}

	if Fingerprint(order) != Fingerprint(thrownAt("throw new NotFoundError(`Order ${orderId} was not found for customer ${customerId}; check the id and try the request again`);", "Order 7 was not found")) {
		t.Error("the same long throw with another id split")
	}
}

// exceptionGroup is an exception group (Python's ExceptionGroup, AggregateError)
// of the given children, sent flat with ids as the SDKs do.
func exceptionGroup(t *testing.T, children ...string) *sentry.Event {
	t.Helper()

	values := []string{`{"type":"ExceptionGroup","value":"errors","mechanism":{"type":"chained","is_exception_group":true,"exception_id":0},
		"stacktrace":{"frames":[{"filename":"jobs.py","function":"run_all","in_app":true,"context_line":"raise ExceptionGroup(errors)"}]}}`}

	for index, child := range children {
		values = append(values, fmt.Sprintf(`{"type":%q,"value":"bad","mechanism":{"type":"chained","exception_id":%d,"parent_id":0},
			"stacktrace":{"frames":[{"filename":"jobs.py","function":"run_one","in_app":true,"context_line":"raise %s()"}]}}`, child, index+1, child))
	}

	slices.Reverse(values)

	return parse(t, `{"platform":"python","exception":{"values":[`+strings.Join(values, ",")+`]}}`)
}

func TestExceptionGroups(t *testing.T) {
	groupCases(t, map[string]groupCase{
		"three or four of the same": {true, [2]*sentry.Event{
			exceptionGroup(t, "ValueError", "ValueError", "ValueError"),
			exceptionGroup(t, "ValueError", "ValueError", "ValueError", "ValueError"),
		}},
		"the same kinds in any number": {true, [2]*sentry.Event{
			exceptionGroup(t, "ValueError", "KeyError", "ValueError"),
			exceptionGroup(t, "ValueError", "KeyError"),
		}},
		"other kinds": {false, [2]*sentry.Event{
			exceptionGroup(t, "ValueError", "KeyError"),
			exceptionGroup(t, "ValueError"),
		}},
		"ids that only loop count as a chain": {false, [2]*sentry.Event{
			parse(t, `{"exception":{"values":[{"type":"A","mechanism":{"is_exception_group":true,"exception_id":0,"parent_id":1}},
				{"type":"B","mechanism":{"exception_id":1,"parent_id":0}}]}}`),
			parse(t, `{"exception":{"values":[{"type":"C","mechanism":{"is_exception_group":true,"exception_id":0,"parent_id":1}},
				{"type":"D","mechanism":{"exception_id":1,"parent_id":0}}]}}`),
		}},
		"a group of one kind is that error": {true, [2]*sentry.Event{
			exceptionGroup(t, "ValueError", "ValueError"),
			parse(t, `{"platform":"python","exception":{"values":[{"type":"ValueError","value":"bad","mechanism":{"type":"generic"},
				"stacktrace":{"frames":[{"filename":"jobs.py","function":"run_one","in_app":true,"context_line":"raise ValueError()"}]}}]}}`),
		}},
	})
}

// allocated is the bytes run allocates.
func allocated(run func()) uint64 {
	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)
	run()
	runtime.ReadMemStats(&after)

	return after.TotalAlloc - before.TotalAlloc
}

// A deep exception group costs memory in proportion to its size, not its square.
func TestDeepExceptionGroupStaysSmall(t *testing.T) {
	values := make([]string, 2000)

	for index := range values {
		values[len(values)-1-index] = fmt.Sprintf(`{"type":"E","value":"v","mechanism":{"is_exception_group":%t,"exception_id":%d,"parent_id":%d}}`,
			index == 0, index, index-1)
	}

	event := parse(t, `{"exception":{"values":[`+strings.Join(values, ",")+`]}}`)

	if bytes := allocated(func() { Fingerprint(event) }); bytes > 32<<20 {
		t.Errorf("a group 2000 deep allocated %d MB", bytes>>20)
	}
}

// Each "{{ default }}" stands for the default components, which are not
// built again for every one of them.
func TestRepeatedDefaultStaysSmall(t *testing.T) {
	exceptions := make([]string, 2000)

	for index := range exceptions {
		exceptions[index] = fmt.Sprintf(`{"type":"E","value":"v%d"}`, index)
	}

	defaults, _ := json.Marshal(slices.Repeat([]string{"{{ default }}"}, 5000))
	event := parse(t, `{"fingerprint":`+string(defaults)+`,"exception":{"values":[`+strings.Join(exceptions, ",")+`]}}`)

	if bytes := allocated(func() { Fingerprint(event) }); bytes > 32<<20 {
		t.Errorf("5000 defaults allocated %d MB", bytes>>20)
	}
}

// Fingerprint values are compared with the errors' texts in time that does
// not grow with both counts at once.
func TestManyFingerprintValuesStayFast(t *testing.T) {
	values, _ := json.Marshal(slices.Repeat([]string{"a"}, 20000))
	exceptions := strings.Repeat(`{"type":"E","value":"b"},`, 50000)
	few := parse(t, `{"fingerprint":`+string(values)+`,"exception":{"values":[{"type":"E","value":"b"}]}}`)
	many := parse(t, `{"fingerprint":`+string(values)+`,"exception":{"values":[`+exceptions+`{"type":"E","value":"b"}]}}`)

	started := time.Now()

	Fingerprint(few)

	single := time.Since(started)
	started = time.Now()

	Fingerprint(many)

	if took := time.Since(started); took > 10*single+50*time.Millisecond {
		t.Errorf("50000 errors: %s, one error %s", took, single)
	}
}

// Stripping a bundle hash takes time in proportion to the name's length,
// however many dots it has.
func TestRollupHashStaysLinear(t *testing.T) {
	short := strings.Repeat("a.", 1000) + "js"
	long := strings.Repeat("a.", 8000) + "js"
	started := time.Now()

	normalizeFilename(short)

	single := time.Since(started)
	started = time.Now()

	normalizeFilename(long)

	if took := time.Since(started); took > 20*single+10*time.Millisecond {
		t.Errorf("8000 dots: %s, 1000 dots %s", took, single)
	}
}

// stripRollupHash strips what the two regexps it replaced stripped.
func TestRollupHashMatchesTheRegexps(t *testing.T) {
	rollupHash := regexp.MustCompile(`[.-]([A-Za-z0-9_-]{8})$`)
	extension := regexp.MustCompile(`\.[A-Za-z0-9]+$`)
	reference := func(filename string) string {
		end := len(filename)

		for {
			head := filename[:end]

			if match := rollupHash.FindStringSubmatchIndex(head); match != nil && looksLikeHash(head[match[2]:match[3]]) {
				return head[:match[0]] + filename[end:]
			}

			found := extension.FindStringIndex(head)

			if found == nil {
				return filename
			}

			end = found[0]
		}
	}

	for _, filename := range []string{
		"", ".", "..", "a.", ".js", "index-BbtQ7Ui0.js", "index-BbtQ7Ui0", "x.BbtQ7Ui0.min.js", "a-BbtQ7Ui0.js.map",
		"-BbtQ7Ui0", "aBbtQ7Ui0.js", "a_BbtQ7Ui0.js", "a.Bbt-Q7U_.js", "a.b.c", "a.é.js", "a-BbtQ7Ui0.é.js",
		"a-BbtQ7Ui0.j_s", "a-BbtQ7Ui0..js", "a--BbtQ7Ui0.js", "dist.routes:index-Settings", "a.12345678.js",
	} {
		if got, want := stripRollupHash(filename), reference(filename); got != want {
			t.Errorf("stripRollupHash(%q) = %q, want %q", filename, got, want)
		}
	}
}

// The fingerprint is the hash of the components joined by newlines, empty ones too.
func TestFingerprintHashesTheComponents(t *testing.T) {
	for _, event := range []*sentry.Event{
		parse(t, `{"fingerprint":["","{{ default }}",""],"message":"hi"}`),
		fixtureEvent(t, "node-event-message"),
	} {
		sum := sha256.Sum256([]byte(strings.Join(Components(event), "\n")))

		if got, want := Fingerprint(event), hex.EncodeToString(sum[:16]); got != want {
			t.Errorf("fingerprint %s, want %s for %q", got, want, Components(event))
		}
	}
}
