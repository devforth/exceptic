// Package grouping decides which events are one issue: events of a project
// with the same Fingerprint, a hash of the event's Components.
package grouping

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/sentry"
)

// maxFingerprintValues: the custom fingerprint values grouping reads. SDKs
// send a few; each "{{ default }}" stands for all default components.
const maxFingerprintValues = 64

// Fingerprint returns the grouping hash of the event: 32 hex characters, the
// hash of its Components joined by newlines.
func Fingerprint(event *sentry.Event) string {
	hash := sha256.New()

	var buffer []byte

	first := true

	eachComponent(event, func(component string) {
		buffer = buffer[:0]

		if !first {
			buffer = append(buffer, '\n')
		}

		first = false

		buffer = append(buffer, component...)
		hash.Write(buffer)
	})

	return hex.EncodeToString(hash.Sum(nil)[:16])
}

// Components returns the values the fingerprint is built from.
// Exported so tests and, later, the UI can show why events were grouped.
func Components(event *sentry.Event) []string {
	var components []string

	eachComponent(event, func(component string) {
		components = append(components, component)
	})

	return components
}

// eachComponent calls each with the components in order; the default ones
// are built once, however many "{{ default }}" stand for them.
func eachComponent(event *sentry.Event, each func(string)) {
	if len(event.Fingerprint) == 0 {
		for _, component := range defaultComponents(event) {
			each(component)
		}

		return
	}

	each("custom")

	var defaults []string

	messages := &eventMessages{event: event}

	for _, value := range event.Fingerprint[:min(len(event.Fingerprint), maxFingerprintValues)] {
		if !isDefaultPlaceholder(value) {
			each(fingerprintValue(event, messages, value))

			continue
		}

		if defaults == nil {
			defaults = defaultComponents(event)
		}

		for _, component := range defaults {
			each(component)
		}
	}
}

func isDefaultPlaceholder(value string) bool {
	return strings.ReplaceAll(value, " ", "") == "{{default}}"
}

func defaultComponents(event *sentry.Event) []string {
	main := event.MainException()
	// captureMessage sends an exception without a type and with a synthetic stack
	// of the call site. Group it by the message: different messages logged from
	// the same place are different problems.
	isMessage := main == nil || (main.Type == "" && event.Message != "")

	if isMessage {
		// The template ("Login failed for %s") when the SDK sent one, so the
		// values put into it never split the issue.
		text := event.MessageTemplate

		if text == "" {
			text = string(event.Message)
		}

		if text == "" && main != nil {
			text = main.Value
		}

		return []string{"message", NormalizeMessage(text)}
	}

	components := []string{"exception"}
	javascript := isJavaScriptPlatform(event.Platform)

	if group, ok := exceptionGroupComponents(event.Exception, javascript); ok {
		return append(components, group...)
	}

	for _, exception := range event.Exception {
		components = append(components, exceptionComponents(exception, javascript)...)
	}

	return components
}

func exceptionComponents(exception sentry.Exception, javascript bool) []string {
	components := []string{"type:" + exception.Type}
	frames := groupingFrames(exception.Stacktrace)

	if len(frames) == 0 {
		// No stack: the value is all we have to tell errors of one type apart.
		return append(components, "value:"+NormalizeMessage(exception.Value))
	}

	for _, frame := range frames {
		components = append(components, "frame:"+frameKey(frame, javascript))
	}

	// Without a source line that counts where it was thrown (a browser sends
	// none, a long line does not count), the value tells apart the throws of
	// one function.
	if sourceLine(frames[len(frames)-1]) == "" {
		components = append(components, "value:"+NormalizeMessage(exception.Value))
	}

	return components
}

// groupingFrames returns the in-app frames of the synchronous part of the
// stack, or all its frames if none is in-app.
func groupingFrames(stacktrace *sentry.Stacktrace) []sentry.Frame {
	if stacktrace == nil {
		return nil
	}

	frames := synchronousFrames(stacktrace.Frames)

	var inApp []sentry.Frame

	for _, frame := range frames {
		if frame.IsInApp() {
			inApp = append(inApp, frame)
		}
	}

	if len(inApp) > 0 {
		return inApp
	}

	return frames
}

// synchronousFrames are the frames (oldest first) after the newest async
// boundary: a frame whose function names the async parent with "*".
func synchronousFrames(frames []sentry.Frame) []sentry.Frame {
	for index := len(frames) - 1; index >= 0; index-- {
		if strings.Contains(frames[index].Function, "*") {
			if index == len(frames)-1 {
				return frames
			}

			return frames[index+1:]
		}
	}

	return frames
}

// frameLocation is the module, or the file if there is no module. Both are
// normalized: SDKs build the module from the file path, so it carries the same
// changing parts (e.g. tsx watch adds "?<timestamp>" to the path on every restart).
func frameLocation(frame sentry.Frame) string {
	if frame.Module != "" {
		return normalizeFilename(frame.Module)
	}

	return normalizeFilename(frame.Filename)
}

// maxSourceLineLength: a longer source line (a minified bundle, generated
// code) changes with every build, so it does not count.
const maxSourceLineLength = 120

// sourceLine is the frame's source line as it counts for grouping: spaces
// collapsed, since whitespace-only edits (indentation) must not split a
// group, and empty when there is none or it is too long.
func sourceLine(frame sentry.Frame) string {
	line := strings.Join(strings.Fields(frame.ContextLine), " ")

	if utf8.RuneCountInString(line) > maxSourceLineLength {
		return ""
	}

	return line
}

func frameKey(frame sentry.Frame, javascript bool) string {
	key := frameLocation(frame)
	function := frame.Function

	if javascript || isJavaScriptFile(frame.Filename) || isJavaScriptFile(frame.AbsPath) {
		function = javaScriptFunction(function)
	}

	if !isAnonymous(function) {
		key += " " + function
	}

	line := sourceLine(frame)

	switch {
	case line != "":
		return key + " | " + line
	case isAnonymous(function):
		// No name and no source: the line number is less stable across deploys,
		// but better than merging every anonymous function in the file.
		return key + " line " + strconv.Itoa(frame.Lineno)
	default:
		return key
	}
}

func isJavaScriptPlatform(platform string) bool {
	return platform == "javascript" || platform == "node"
}

var javaScriptExtensions = []string{".js", ".mjs", ".cjs", ".jsx", ".ts", ".mts", ".cts", ".tsx", ".vue", ".svelte"}

func isJavaScriptFile(path string) bool {
	if index := strings.IndexAny(path, "?#"); index >= 0 {
		path = path[:index]
	}

	extension := path[strings.LastIndexByte(path, '.')+1:]

	for _, known := range javaScriptExtensions {
		if strings.EqualFold("."+extension, known) {
			return true
		}
	}

	return false
}

// javaScriptFunction drops what names the same JavaScript function in
// different ways: the receiver ("Object.onClick", "Module.onClick" are
// onClick) and the "at " and "new " of stack lines.
func javaScriptFunction(function string) string {
	function = strings.TrimPrefix(function, "at ")
	function = strings.TrimPrefix(function, "new ")

	if index := strings.LastIndexByte(function, '.'); index >= 0 && index < len(function)-1 {
		return function[index+1:]
	}

	return function
}

func isAnonymous(function string) bool {
	switch function {
	case "", "?", "<anonymous>", "anonymous", "<unknown>":
		return true
	}

	return false
}

var (
	// Hex content hashes: webpack [contenthash] (main.3f2a1b9c.js), Next.js and
	// Turbopack chunks (page-0123456789abcdef.js, src_app_page_1a2b3c4d._.js).
	hexHashRe = regexp.MustCompile(`[._-][0-9a-fA-F]{8,}\b`)
)

// Rollup 4 / Vite 5+ chunk hashes: 8 base64url characters after "-" or "."
// (index-BbtQ7Ui0.js, SvelteKit scheduler.BvLojk_z.js, Astro Counter.BkGx5dAS.js).
const rollupHashLength = 8

// normalizeFilename drops what changes between deploys and environments:
// the origin of browser URLs (localhost vs production domain), query strings,
// and content hashes in bundle names.
func normalizeFilename(filename string) string {
	if index := strings.Index(filename, "://"); index >= 0 {
		rest := filename[index+3:]

		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			filename = rest[slash:]
		} else {
			filename = "/"
		}
	}

	if index := strings.IndexAny(filename, "?#"); index >= 0 {
		filename = filename[:index]
	}

	return stripRollupHash(hexHashRe.ReplaceAllString(filename, ""))
}

// stripRollupHash removes the hash nearest the end that only extensions follow
// (app.Settings.BkGx5dAS.js keeps app.Settings); each extension is read once.
func stripRollupHash(filename string) string {
	end := len(filename)

	for {
		head := filename[:end]
		start := len(head) - rollupHashLength

		if start > 0 && (head[start-1] == '.' || head[start-1] == '-') && isHashText(head[start:]) && looksLikeHash(head[start:]) {
			return head[:start-1] + filename[end:]
		}

		dot := len(head)

		for dot > 0 && isAlphanumeric(head[dot-1]) {
			dot--
		}

		if dot == len(head) || dot == 0 || head[dot-1] != '.' {
			return filename
		}

		end = dot - 1
	}
}

func isAlphanumeric(character byte) bool {
	return character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

// isHashText is base64url text.
func isHashText(text string) bool {
	for index := range len(text) {
		if !isAlphanumeric(text[index]) && text[index] != '_' && text[index] != '-' {
			return false
		}
	}

	return true
}

// looksLikeHash keeps words: a hash has a digit or an uppercase letter after
// the first character, a word in a file name ("settings", "Settings") has not.
func looksLikeHash(segment string) bool {
	for index, character := range segment {
		if character >= '0' && character <= '9' || index > 0 && character >= 'A' && character <= 'Z' {
			return true
		}
	}

	return false
}

// LogFingerprint groups log lines that are the same message with different
// values ("order 17 failed" and "order 42 failed"), so a line repeated in a
// loop can be limited to its newest occurrences. Short: it is stored with
// every log and indexed.
func LogFingerprint(body string) string {
	sum := sha256.Sum256([]byte(NormalizeMessage(strings.TrimSpace(body))))

	return hex.EncodeToString(sum[:8])
}

// Culprit is a short "where did it happen" label for the issue list: the
// transaction (e.g. "GET /api/orders") if set, else the function and file of
// the frame that threw.
func Culprit(event *sentry.Event) string {
	if event.Transaction != "" {
		return event.Transaction
	}

	frame := culpritFrame(event)
	location := frameLocation(frame)

	if frame.Function == "" {
		return location
	}

	return frame.Function + " (" + location + ")"
}
