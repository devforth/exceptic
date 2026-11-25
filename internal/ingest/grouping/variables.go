package grouping

import (
	"regexp"
	"strings"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

// variableRe is a fingerprint value that is one variable, as Sentry reads
// it: a variable inside other text stays as written.
var variableRe = regexp.MustCompile(`^\{\{\s*(\S+)\s*\}\}$`)

// fingerprintValue is an SDK fingerprint value with its variable
// ("{{ error.type }}", "{{ tags.tenant }}") replaced by the event's value,
// "<no-…>" when the event has none; an unknown variable stays as written.
// The variables are those of Sentry's Fingerprint Rules (docs/sdk.md).
func fingerprintValue(event *sentry.Event, messages *eventMessages, value string) string {
	match := variableRe.FindStringSubmatch(value)

	if match == nil {
		if messages.has(value) {
			return NormalizeMessage(value)
		}

		return value
	}

	name := match[1]

	if tag, ok := strings.CutPrefix(name, "tags."); ok {
		if resolved := tagValue(event, tag); resolved != "" {
			return resolved
		}

		return "<no-value-for-tag-" + tag + ">"
	}

	resolved, placeholder, ok := variable(event, name)

	switch {
	case !ok:
		return value
	case resolved == "":
		return placeholder
	case name == "message" || name == "error.value" || name == "value":
		return NormalizeMessage(resolved)
	}

	return resolved
}

func variable(event *sentry.Event, name string) (value, placeholder string, ok bool) {
	switch name {
	case "transaction":
		return event.Transaction, "<no-transaction>", true
	case "message":
		return firstNonEmpty(string(event.Message), mainException(event).Value), "<no-message>", true
	case "level":
		return eventLevel(event), "<no-level>", true
	case "logger":
		return event.Logger, "<no-logger>", true
	case "error.type", "type":
		return mainException(event).Type, "<no-type>", true
	case "error.value", "value":
		return mainException(event).Value, "<no-value>", true
	case "stack.function", "function":
		return crashFrame(event).Function, "<no-function>", true
	case "stack.abs_path", "path":
		frame := crashFrame(event)

		return firstNonEmpty(frame.AbsPath, frame.Filename), "<no-abs-path>", true
	case "stack.filename":
		frame := crashFrame(event)

		return firstNonEmpty(frame.Filename, frame.AbsPath), "<no-filename>", true
	case "stack.module", "module":
		return crashFrame(event).Module, "<no-module>", true
	case "stack.package", "package":
		return lastPathSegment(crashFrame(event).Package), "<no-package>", true
	}

	return "", "", false
}

// tagValue is a tag as Sentry has it when it groups an event: the event's
// fields set or replace some (Relay and the event manager), and release,
// dist, user, filename and function are not kept as sent.
func tagValue(event *sentry.Event, name string) string {
	sent := event.Tags[name]

	switch name {
	case "level":
		return eventLevel(event)
	case "environment":
		return firstNonEmpty(event.Environment, sent)
	case "logger":
		return firstNonEmpty(event.Logger, sent)
	case "transaction":
		return firstNonEmpty(event.Transaction, sent)
	case "server_name":
		return firstNonEmpty(event.ServerName, sent)
	case "sentry:release":
		return firstNonEmpty(event.Release, sent)
	case "sentry:dist":
		if event.Release != "" {
			return firstNonEmpty(event.Dist, sent)
		}
	case "sentry:user":
		return firstNonEmpty(userTag(event.User), sent)
	case "release", "dist", "user", "filename", "function":
		return ""
	}

	return sent
}

// userTag names the user by the first of id, username, email and IP.
func userTag(user *sentry.User) string {
	if user == nil {
		return ""
	}

	switch {
	case user.ID != "":
		return "id:" + string(user.ID)
	case user.Username != "":
		return "username:" + user.Username
	case user.Email != "":
		return "email:" + user.Email
	case user.IPAddress != "" && user.IPAddress != "{{auto}}":
		return "ip:" + user.IPAddress
	}

	return ""
}

func eventLevel(event *sentry.Event) string {
	return config.EventLevel(event.Level)
}

// eventMessages are the event's message and its errors' texts, which plain
// fingerprint values that equal one group with numbers and IDs ignored; read
// once for all the values.
type eventMessages struct {
	event *sentry.Event
	texts map[string]bool
}

func (messages *eventMessages) has(value string) bool {
	if value == "" {
		return false
	}

	if messages.texts == nil {
		messages.texts = map[string]bool{string(messages.event.Message): true}

		for index := range messages.event.Exception {
			messages.texts[messages.event.Exception[index].Value] = true
		}
	}

	return messages.texts[value]
}

func mainException(event *sentry.Event) sentry.Exception {
	if main := event.MainException(); main != nil {
		return *main
	}

	return sentry.Exception{}
}

// crashFrame is Sentry's crashing frame: the newest in-app frame of the
// outermost error, else its newest frame.
func crashFrame(event *sentry.Event) sentry.Frame {
	stacktrace := mainException(event).Stacktrace

	if stacktrace == nil || len(stacktrace.Frames) == 0 {
		return sentry.Frame{}
	}

	for index := len(stacktrace.Frames) - 1; index >= 0; index-- {
		if stacktrace.Frames[index].IsInApp() {
			return stacktrace.Frames[index]
		}
	}

	return stacktrace.Frames[len(stacktrace.Frames)-1]
}

// culpritFrame is the frame Culprit names: the newest in-app frame of the
// synchronous part of the stack, else its newest frame.
func culpritFrame(event *sentry.Event) sentry.Frame {
	frames := groupingFrames(mainException(event).Stacktrace)

	if len(frames) == 0 {
		return sentry.Frame{}
	}

	return frames[len(frames)-1]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

func lastPathSegment(path string) string {
	return path[strings.LastIndexAny(path, `/\`)+1:]
}
