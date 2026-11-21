package api

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"slices"
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/services"
)

// get_issue_details is the web app's issue page: the event as the server
// reads it (sentry's lenient decoding), so the browser never parses SDK data.

type issueDetailsInput struct {
	IssueID string `json:"issue_id" jsonschema:"Issue ID."`
	EventID string `json:"event_id,omitempty" jsonschema:"An event of this issue (UUID or 32 hex); the latest when empty."`
}

type issueDetailsOutput struct {
	Issue issueOutput `json:"issue"`
	// AgentPrompt is the "Prompt" button's text (agentprompt: allowlisted and redacted).
	AgentPrompt string               `json:"agent_prompt"`
	History     []statusChangeOutput `json:"history"`
	Events      Page[eventOutput]    `json:"events"`
	EventState  services.EventState  `json:"event_state"`
	Event       *eventDetailsOutput  `json:"event,omitempty"`
}

type statusChangeOutput struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Name is who changed it; empty for a deleted user.
	Name string `json:"name,omitempty"`
	// ByEvent: reopened because it happened again (EventID is that event).
	ByEvent   bool      `json:"by_event,omitempty"`
	ViaMCP    bool      `json:"via_mcp,omitempty"`
	EventID   string    `json:"event_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type eventDetailsOutput struct {
	EventID string `json:"event_id"`
	// Timestamp is as stored: moved by the device's clock drift (ClockDrift, "3h0m0s").
	Timestamp  time.Time `json:"timestamp"`
	ClockDrift string    `json:"clock_drift,omitempty"`
	// Thinned: the cleanup kept only the error, its stack trace and message.
	Thinned      bool               `json:"thinned,omitempty"`
	Platform     string             `json:"platform,omitempty"`
	Environment  string             `json:"environment,omitempty"`
	Release      string             `json:"release,omitempty"`
	ServerName   string             `json:"server_name,omitempty"`
	SDK          string             `json:"sdk,omitempty"`
	TraceID      string             `json:"trace_id,omitempty"`
	TraceHasLogs bool               `json:"trace_has_logs,omitempty"`
	Message      string             `json:"message,omitempty"`
	Exceptions   []exceptionOutput  `json:"exceptions"`
	Breadcrumbs  []breadcrumbOutput `json:"breadcrumbs"`
	Request      *requestOutput     `json:"request,omitempty"`
	Tags         []nameValue        `json:"tags"`
	User         *eventUserOutput   `json:"user,omitempty"`
	Contexts     []nameValue        `json:"contexts"`
	RelatedLogs  []relatedLogOutput `json:"related_logs"`
	// LogsFromRequest: the related logs are of the event's request (by span),
	// not only of its trace within a minute.
	LogsFromRequest bool `json:"logs_from_request,omitempty"`
	// Payload is the event as stored, indented.
	Payload string `json:"payload"`
}

// exceptionOutput is one exception of the chain, outermost first.
type exceptionOutput struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
	// Handled is unknown (absent) when the SDK does not say.
	Handled *bool         `json:"handled,omitempty"`
	Frames  []frameOutput `json:"frames"`
}

// frameOutput is a stack frame, newest first.
type frameOutput struct {
	Function string `json:"function,omitempty"`
	// Location is the file, or the module without one.
	Location    string             `json:"location,omitempty"`
	Line        int                `json:"line,omitempty"`
	Column      int                `json:"column,omitempty"`
	InApp       bool               `json:"in_app,omitempty"`
	SourceLines []sourceLineOutput `json:"source_lines"`
	// Vars are the local variables, indented JSON.
	Vars string `json:"vars,omitempty"`
}

type sourceLineOutput struct {
	Number  int    `json:"number"`
	Text    string `json:"text"`
	Current bool   `json:"current,omitempty"`
}

type breadcrumbOutput struct {
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Category  string     `json:"category,omitempty"`
	Level     string     `json:"level,omitempty"`
	Message   string     `json:"message,omitempty"`
	// Data is indented JSON.
	Data string `json:"data,omitempty"`
}

type requestOutput struct {
	Method  string      `json:"method,omitempty"`
	URL     string      `json:"url,omitempty"`
	Headers []nameValue `json:"headers"`
}

// nameValue is an entry of a map, sorted by name: the browser would
// reorder number-like keys of an object. A JSON value is indented.
type nameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// eventUserOutput is the user of the monitored app the event happened to.
type eventUserOutput struct {
	ID        string `json:"id,omitempty"`
	Email     string `json:"email,omitempty"`
	Username  string `json:"username,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
}

type relatedLogOutput struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Body  string    `json:"body"`
}

func (handler *handlers) getIssueDetails(ctx context.Context, call Call, input issueDetailsInput) (issueDetailsOutput, error) {
	details, err := handler.services.Issues.Details(ctx, input.IssueID, input.EventID)

	if err != nil {
		return issueDetailsOutput{}, err
	}

	output := issueDetailsOutput{
		Issue:       handler.issueOutput(call, details.Issue),
		AgentPrompt: services.AgentPrompt(handler.projectName(details.Issue.ProjectID), details.Issue, details.Parsed, details.RelatedLogs, details.LogsFromRequest),
		History:     []statusChangeOutput{},
		Events:      eventsPage(call, details.Events, services.RecentEventsShown),
		EventState:  details.EventState,
	}

	for _, change := range details.History {
		output.History = append(output.History, statusChangeOutput{
			From: change.From, To: change.To, Name: change.Name, ByEvent: change.ByEvent(), ViaMCP: change.ViaMCP(),
			EventID: change.EventID, CreatedAt: change.CreatedAt,
		})
	}

	if details.Parsed != nil {
		output.Event = eventDetails(details)
	}

	return output, nil
}

func eventDetails(details services.IssueDetails) *eventDetailsOutput {
	event, parsed := details.Event, details.Parsed
	output := &eventDetailsOutput{
		EventID: parsed.EventID, Timestamp: event.Timestamp, Thinned: event.Thinned,
		Platform: parsed.Platform, Environment: parsed.Environment, Release: parsed.Release, ServerName: parsed.ServerName,
		TraceID: parsed.Trace().TraceID, TraceHasLogs: details.TraceHasLogs, Message: string(parsed.Message),
		Exceptions: []exceptionOutput{}, Breadcrumbs: []breadcrumbOutput{}, Tags: sortedPairs(parsed.Tags, func(value string) string { return value }),
		User: eventUser(parsed.User), Contexts: sortedPairs(parsed.Contexts, func(value json.RawMessage) string { return textutil.IndentJSON(value) }),
		RelatedLogs: []relatedLogOutput{}, LogsFromRequest: details.LogsFromRequest, Payload: textutil.IndentJSON(event.Payload),
	}

	if drift := services.ClockDrift(event, parsed).Round(time.Minute); drift != 0 {
		output.ClockDrift = drift.String()
	}

	if parsed.SDK != nil {
		output.SDK = parsed.SDK.Name + " " + parsed.SDK.Version
	}

	for _, exception := range parsed.Exception.OutermostFirst() {
		item := exceptionOutput{Type: exception.Type, Value: exception.Value, Frames: []frameOutput{}}

		if exception.Mechanism != nil {
			item.Handled = exception.Mechanism.Handled
		}

		for _, frame := range exception.Stacktrace.NewestFirst() {
			item.Frames = append(item.Frames, frameDetails(frame))
		}

		output.Exceptions = append(output.Exceptions, item)
	}

	for _, crumb := range parsed.Breadcrumbs {
		item := breadcrumbOutput{Category: crumb.Category, Level: crumb.Level, Message: crumb.Message}

		if !crumb.Timestamp.IsZero() {
			item.Timestamp = &crumb.Timestamp.Time
		}

		if len(crumb.Data) > 0 {
			item.Data = textutil.IndentJSON(crumb.Data)
		}

		output.Breadcrumbs = append(output.Breadcrumbs, item)
	}

	if request := sentry.ParseRequest(parsed.Request); request != nil {
		output.Request = &requestOutput{Method: request.Method, URL: request.URL, Headers: sortedPairs(request.Headers, func(value string) string { return value })}
	}

	for _, record := range details.RelatedLogs {
		output.RelatedLogs = append(output.RelatedLogs, relatedLogOutput{Time: record.Timestamp, Level: record.Level, Body: record.Body})
	}

	return output
}

func frameDetails(frame sentry.Frame) frameOutput {
	output := frameOutput{
		Function: frame.Function, Location: cmp.Or(frame.Filename, frame.Module), Line: frame.Lineno, Column: frame.Colno,
		InApp: frame.IsInApp(), SourceLines: []sourceLineOutput{},
	}

	for _, line := range frame.SourceLines() {
		output.SourceLines = append(output.SourceLines, sourceLineOutput{Number: line.Number, Text: line.Text, Current: line.Current})
	}

	if len(frame.Vars) > 0 && string(frame.Vars) != "null" {
		output.Vars = textutil.IndentJSON(frame.Vars)
	}

	return output
}

func sortedPairs[Value any](values map[string]Value, text func(Value) string) []nameValue {
	pairs := []nameValue{}

	for _, name := range slices.Sorted(maps.Keys(values)) {
		pairs = append(pairs, nameValue{Name: name, Value: text(values[name])})
	}

	return pairs
}

// eventUser is nil without any of the fields shown.
func eventUser(user *sentry.User) *eventUserOutput {
	if user == nil {
		return nil
	}

	output := eventUserOutput{ID: string(user.ID), Email: user.Email, Username: user.Username, IPAddress: user.IPAddress}

	if output == (eventUserOutput{}) {
		return nil
	}

	return &output
}
