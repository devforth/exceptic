package sentry

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Event is an error or message event. Only the fields the server works with are
// typed; Raw keeps the whole payload so nothing the SDK sent is lost.
// Format: https://develop.sentry.dev/sdk/data-model/event-payloads/
type Event struct {
	EventID     string    `json:"event_id"`
	Timestamp   Timestamp `json:"timestamp"`
	Level       string    `json:"level"`
	Platform    string    `json:"platform"`
	Environment string    `json:"environment"`
	Release     string    `json:"release"`
	Dist        string    `json:"dist"`
	ServerName  string    `json:"server_name"`
	Transaction string    `json:"transaction"`
	Logger      string    `json:"logger"`
	// Message is the text of the message, MessageTemplate its unformatted
	// form ("Order %s failed") when the SDK sent one; both come from
	// MessageEntry, or LogEntry when it has none.
	Message         Message                    `json:"-"`
	MessageTemplate string                     `json:"-"`
	MessageEntry    MessageEntry               `json:"message"`
	LogEntry        MessageEntry               `json:"logentry"`
	Exception       ExceptionList              `json:"exception"`
	Fingerprint     StringList                 `json:"fingerprint"`
	Tags            Tags                       `json:"tags"`
	User            *User                      `json:"user"`
	Contexts        map[string]json.RawMessage `json:"contexts"`
	Breadcrumbs     BreadcrumbList             `json:"breadcrumbs"`
	Request         json.RawMessage            `json:"request"`
	SDK             *SDK                       `json:"sdk"`

	Raw json.RawMessage `json:"-"`
}

// TruncatedField is the top-level field the server adds to an event it cut
// to its size limits: the names of the cuts, never sent by an SDK.
const TruncatedField = "_exceptic_truncated"

type Exception struct {
	Type       string      `json:"type"`
	Value      string      `json:"value"`
	Mechanism  *Mechanism  `json:"mechanism"`
	Stacktrace *Stacktrace `json:"stacktrace"`
}

type Mechanism struct {
	Type    string `json:"type"`
	Handled *bool  `json:"handled"`
	// An exception group (Python's ExceptionGroup, JS AggregateError) is sent
	// flat: each exception has an ID and names its parent's.
	IsExceptionGroup bool `json:"is_exception_group"`
	ExceptionID      *int `json:"exception_id"`
	ParentID         *int `json:"parent_id"`
}

type Stacktrace struct {
	// Frames are ordered oldest first: the frame that threw is last.
	Frames []Frame `json:"frames"`
}

type Frame struct {
	Filename    string          `json:"filename"`
	AbsPath     string          `json:"abs_path"`
	Module      string          `json:"module"`
	Package     string          `json:"package"`
	Function    string          `json:"function"`
	Lineno      int             `json:"lineno"`
	Colno       int             `json:"colno"`
	InApp       *bool           `json:"in_app"`
	ContextLine string          `json:"context_line"`
	PreContext  []string        `json:"pre_context"`
	PostContext []string        `json:"post_context"`
	Vars        json.RawMessage `json:"vars"`
}

type User struct {
	ID        FlexString `json:"id"`
	Email     string     `json:"email"`
	Username  string     `json:"username"`
	IPAddress string     `json:"ip_address"`
}

type Breadcrumb struct {
	Timestamp Timestamp       `json:"timestamp"`
	Type      string          `json:"type"`
	Category  string          `json:"category"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
}

// ParseEvent decodes an event item payload. headerEventID is the envelope's
// event_id, used when the payload has none. Missing IDs, timestamps and levels
// get defaults, so the result is always usable.
func ParseEvent(payload []byte, headerEventID string, received time.Time) (*Event, error) {
	event := &Event{}

	if !Decode(payload, event) {
		return nil, errors.New("event: not a JSON object")
	}

	event.Raw = json.RawMessage(bytes.Clone(payload))

	event.EventID = NormalizeEventID(event.EventID)

	if event.EventID == "" {
		event.EventID = NormalizeEventID(headerEventID)
	}

	if event.EventID == "" {
		event.EventID = NewEventID()
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = Timestamp{received}
	}

	if event.Level == "" {
		event.Level = "error"
	}

	entry := event.MessageEntry

	if entry.Text() == "" {
		entry = event.LogEntry
	}

	event.Message = Message(entry.Text())
	event.MessageTemplate = entry.Template

	return event, nil
}

// Title is a one-line summary: "Type: value" of the outermost exception, or the message.
func (event *Event) Title() string {
	if exception := event.MainException(); exception != nil {
		switch {
		case exception.Type != "" && exception.Value != "":
			return exception.Type + ": " + exception.Value
		case exception.Type != "":
			return exception.Type
		case exception.Value != "":
			return exception.Value
		}
	}

	if event.Message != "" {
		return string(event.Message)
	}

	return "<unlabeled event>"
}

// MainException is the outermost exception. In a cause chain the SDK sends
// the innermost cause first, so it is the last one.
func (event *Event) MainException() *Exception {
	if len(event.Exception) == 0 {
		return nil
	}

	return &event.Exception[len(event.Exception)-1]
}

// EventTrace is contexts.trace of an event: where in a trace it happened.
type EventTrace struct {
	TraceID string `json:"trace_id"`
	// SpanID is the span active when the error was captured (e.g. the route
	// handler), ParentSpanID its parent (usually the HTTP request span).
	SpanID       string `json:"span_id"`
	ParentSpanID string `json:"parent_span_id"`
}

// Trace returns contexts.trace. A browser keeps one trace for the whole page
// session, so the trace alone does not identify a request; the spans do.
func (event *Event) Trace() EventTrace {
	var trace EventTrace

	if raw, ok := event.Contexts["trace"]; ok {
		Decode(raw, &trace)
	}

	return trace
}

// Reading helpers, shared by the issue page and email notifications.

// OutermostFirst orders a cause chain for reading: the error that was thrown
// last (the outer one) first, its causes below. SDKs send innermost first.
func (list ExceptionList) OutermostFirst() []Exception {
	ordered := slices.Clone([]Exception(list))
	slices.Reverse(ordered)

	return ordered
}

// NewestFirst returns the frames with the one that threw at the top, like a
// JS/Node stack trace. SDKs send oldest first.
func (stacktrace *Stacktrace) NewestFirst() []Frame {
	if stacktrace == nil {
		return nil
	}

	frames := slices.Clone(stacktrace.Frames)
	slices.Reverse(frames)

	return frames
}

// IsInApp reports whether the frame is the application's own code, not a
// library: as the SDK marked it, except a file of an installed package. A
// browser SDK marks every frame of the page's own origin in-app, and a dev
// server (Vite) serves the libraries from there too.
func (frame Frame) IsInApp() bool {
	return frame.InApp != nil && *frame.InApp && !IsLibraryPath(frame.Filename) && !IsLibraryPath(frame.AbsPath)
}

// IsLibraryPath reports whether a frame's file belongs to an installed
// package (node_modules, also as Vite's /node_modules/.vite/deps/).
func IsLibraryPath(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")

	return strings.Contains(path, "/node_modules/") || strings.HasPrefix(path, "node_modules/")
}

// SourceLine is one line of code around a frame.
type SourceLine struct {
	Number  int
	Text    string
	Current bool // the line that threw
}

// SourceLines returns the code around the frame with line numbers, or nil
// when the SDK sent no source (browsers).
func (frame Frame) SourceLines() []SourceLine {
	if frame.ContextLine == "" {
		return nil
	}

	lines := make([]SourceLine, 0, len(frame.PreContext)+1+len(frame.PostContext))
	first := frame.Lineno - len(frame.PreContext)

	for index, text := range frame.PreContext {
		lines = append(lines, SourceLine{Number: first + index, Text: text})
	}

	lines = append(lines, SourceLine{Number: frame.Lineno, Text: frame.ContextLine, Current: true})

	for index, text := range frame.PostContext {
		lines = append(lines, SourceLine{Number: frame.Lineno + 1 + index, Text: text})
	}

	return lines
}

// NewEventID returns a random 32-character hex ID in Sentry's format.
func NewEventID() string {
	var id [16]byte

	_, _ = rand.Read(id[:])

	return hex.EncodeToString(id[:])
}

// NormalizeEventID accepts both "abcd..." and UUID form with dashes, and
// returns 32 lowercase hex characters, or "" if the value is not a valid ID.
func NormalizeEventID(value string) string {
	value = strings.ToLower(strings.ReplaceAll(value, "-", ""))

	if len(value) != 32 {
		return ""
	}

	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}

	return value
}

// The types below accept every shape the Sentry protocol allows for a field and
// keep the zero value on an unexpected one, as Decode does for every other field:
// losing one field is better than losing the error report.

// Timestamp accepts Unix seconds (float) or an RFC 3339 string; a time JSON
// cannot encode (years outside 0–9999, such as milliseconds sent as seconds)
// is left zero.
type Timestamp struct{ time.Time }

// Unix seconds of 0000-01-01 and 9999-12-31T23:59:59.
const (
	minTimestampSeconds = -62167219200
	maxTimestampSeconds = 253402300799
)

func (timestamp *Timestamp) UnmarshalJSON(data []byte) error {
	var seconds float64

	if err := json.Unmarshal(data, &seconds); err == nil {
		if seconds < minTimestampSeconds || seconds >= maxTimestampSeconds+1 {
			return nil
		}

		whole := int64(seconds)
		timestamp.Time = time.Unix(whole, int64((seconds-float64(whole))*1e9)).UTC()

		return nil
	}

	var text string

	if err := json.Unmarshal(data, &text); err == nil {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				if seconds := parsed.Unix(); seconds >= minTimestampSeconds && seconds <= maxTimestampSeconds {
					timestamp.Time = parsed.UTC()
				}

				return nil
			}
		}
	}

	return nil
}

// Message is the text of a message event.
type Message string

// MessageEntry accepts a plain string (the text) or {"formatted": ...,
// "message": ..., "params": [...]}, where "message" is the template.
type MessageEntry struct {
	Formatted string
	Template  string
}

// Text is the formatted text, or the template when the SDK sent no other.
func (entry MessageEntry) Text() string {
	if entry.Formatted != "" {
		return entry.Formatted
	}

	return entry.Template
}

func (entry *MessageEntry) UnmarshalJSON(data []byte) error {
	var text string

	if err := json.Unmarshal(data, &text); err == nil {
		*entry = MessageEntry{Formatted: text}
		return nil
	}

	var object struct {
		Formatted string `json:"formatted"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal(data, &object); err == nil {
		*entry = MessageEntry{Formatted: object.Formatted, Template: object.Message}
	}

	return nil
}

// ExceptionList is {"values": [...]} or a bare list; Decode reads it leniently.
type ExceptionList []Exception

func (list *ExceptionList) UnmarshalJSON(data []byte) error {
	return unmarshalValues(data, (*[]Exception)(list))
}

// BreadcrumbList is {"values": [...]} or a bare list; Decode reads it leniently.
type BreadcrumbList []Breadcrumb

func (list *BreadcrumbList) UnmarshalJSON(data []byte) error {
	return unmarshalValues(data, (*[]Breadcrumb)(list))
}

// unmarshalValues is strict, so a wrong-typed element fails the plain decode
// and Decode reads the list once, element by element.
func unmarshalValues[T any](data []byte, target *[]T) error {
	if jsontext.Value(data).Kind() != jsontext.KindBeginObject {
		return json.Unmarshal(data, target)
	}

	var wrapped struct {
		Values []T `json:"values"`
	}

	if err := json.Unmarshal(data, &wrapped); err != nil {
		return err
	}

	*target = wrapped.Values

	return nil
}

// Tags accepts {"key": "value"} or [["key", "value"], ...]. Non-string values are stringified.
type Tags map[string]string

func (tags *Tags) UnmarshalJSON(data []byte) error {
	result := Tags{}

	var object map[string]json.RawMessage

	if err := json.Unmarshal(data, &object); err == nil {
		for key, value := range object {
			result[key] = rawToString(value)
		}

		*tags = result

		return nil
	}

	var pairs [][]json.RawMessage

	if err := json.Unmarshal(data, &pairs); err == nil {
		for _, pair := range pairs {
			if len(pair) == 2 {
				result[rawToString(pair[0])] = rawToString(pair[1])
			}
		}

		*tags = result
	}

	return nil
}

// StringList is a list of strings where numbers and booleans are stringified.
type StringList []string

func (list *StringList) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage

	if err := json.Unmarshal(data, &values); err != nil {
		return nil
	}

	result := make(StringList, 0, len(values))

	for _, value := range values {
		result = append(result, rawToString(value))
	}

	*list = result

	return nil
}

// FlexString accepts a string or a number (user.id is often numeric).
type FlexString string

func (flex *FlexString) UnmarshalJSON(data []byte) error {
	*flex = FlexString(rawToString(data))
	return nil
}

func rawToString(raw json.RawMessage) string {
	var text string

	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var number json.Number

	if err := json.Unmarshal(raw, &number); err == nil {
		return number.String()
	}

	var boolean bool

	if err := json.Unmarshal(raw, &boolean); err == nil {
		return strconv.FormatBool(boolean)
	}

	if string(raw) == "null" {
		return ""
	}

	return string(raw)
}
