package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// logFiltersInput is what list_logs and count_logs filter on.
type logFiltersInput struct {
	Project []string `json:"project,omitempty" jsonschema:"Project names or IDs; every project when empty."`
	Level   []string `json:"level,omitempty" jsonschema:"Log levels; every level when empty."`
	Query   string   `json:"query,omitempty" jsonschema:"Text in the log line."`
	TraceID string   `json:"trace_id,omitempty" jsonschema:"Only logs of this trace (see get_issue)."`
	Since   string   `json:"since,omitempty" jsonschema:"RFC 3339 time: only logs at or after it."`
	Until   string   `json:"until,omitempty" jsonschema:"RFC 3339 time: only logs before it."`
}

type listLogsInput struct {
	logFiltersInput

	Limit  int    `json:"limit,omitempty" jsonschema:"Logs per page."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page."`
	Offset int    `json:"offset,omitempty" jsonschema:"Logs to skip, newest first, for numbered pages; next_cursor is faster. Not with cursor."`
}

type logOutput struct {
	Time    time.Time `json:"time"`
	Project string    `json:"project"`
	Level   string    `json:"level"`
	Body    string    `json:"body"`
	TraceID string    `json:"trace_id,omitempty"`
	// Issue is the error the log was written in the request of (its span).
	Issue *logIssueOutput `json:"issue,omitempty"`
}

type logIssueOutput struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

func (handler *handlers) listLogs(ctx context.Context, call Call, input listLogsInput) (Page[logOutput], error) {
	output := Page[logOutput]{Items: []logOutput{}}
	filter, err := handler.logFilter(input.logFiltersInput)

	if err != nil {
		return output, err
	}

	if input.Offset > 0 && input.Cursor != "" {
		return output, Invalid("pass offset or cursor, not both")
	}

	if input.Offset < 0 || input.Offset > store.MaxOffset {
		return output, Invalid("offset out of range")
	}

	limit := pageLimit(input.Limit)
	filter.Limit, filter.Offset = limit+1, input.Offset

	if filter.BeforeID, err = parseCursor(input.Cursor); err != nil {
		return output, err
	}

	logs, err := handler.services.Logs.List(ctx, filter)

	if err != nil {
		return output, err
	}

	logs, more := store.Page(logs, limit)
	issues, err := handler.services.Logs.Issues(ctx, logs)

	if err != nil {
		return output, err
	}

	for _, record := range logs {
		item := logOutput{
			Time: record.Timestamp, Project: handler.projectName(record.ProjectID), Level: record.Level,
			Body: call.Text(record.Body), TraceID: call.TraceID(record.TraceID),
		}

		if issue, ok := issues[record.ID]; ok {
			item.Issue = &logIssueOutput{ID: issue.IssueID, Title: call.Text(issue.Title)}
		}

		output.Items = append(output.Items, item)
	}

	if more {
		output.NextCursor = strconv.FormatInt(logs[limit-1].ID, 10)
	}

	return output, nil
}

type countOutput struct {
	Count int `json:"count"`
	// More: the count stopped at its limit; there are more.
	More bool `json:"more,omitempty"`
}

func (handler *handlers) countLogs(ctx context.Context, _ Call, input logFiltersInput) (countOutput, error) {
	filter, err := handler.logFilter(input)

	if err != nil {
		return countOutput{}, err
	}

	count, more, err := handler.services.Logs.Count(ctx, filter)

	return countOutput{Count: count, More: more}, err
}

type levelsOutput struct {
	// Levels are highest first.
	Levels []string `json:"levels"`
}

func (handler *handlers) listLogLevels(ctx context.Context, _ Call, _ noInput) (levelsOutput, error) {
	present, err := handler.services.Logs.Levels(ctx)

	return levelsOutput{Levels: append([]string{}, config.InLevelOrder(present, config.LogLevels)...)}, err
}

func (handler *handlers) logFilter(input logFiltersInput) (store.LogFilter, error) {
	filter := store.LogFilter{Query: strings.TrimSpace(input.Query), TraceID: strings.TrimSpace(input.TraceID), Levels: input.Level}

	var err error

	if filter.ProjectIDs, err = projectIDs(handler.projects.List(), input.Project); err != nil {
		return filter, err
	}

	if filter.Since, err = parseTime("since", input.Since); err != nil {
		return filter, err
	}

	filter.Until, err = parseTime("until", input.Until)

	return filter, err
}

type listTraceInput struct {
	TraceID string `json:"trace_id" jsonschema:"Trace ID: 32 hex characters."`
}

type traceOutput struct {
	TraceID string               `json:"trace_id"`
	Events  []store.EventSummary `json:"events"`
	Logs    []store.LogRecord    `json:"logs"`
}

// listTrace is REST only: log attributes are raw SDK data.
func (handler *handlers) listTrace(ctx context.Context, _ Call, input listTraceInput) (traceOutput, error) {
	traceID := strings.TrimSpace(input.TraceID)

	if traceID == "" {
		return traceOutput{}, Invalid("trace_id is required")
	}

	output := traceOutput{TraceID: traceID}
	events, err := handler.services.Issues.Events(ctx, store.EventFilter{TraceID: traceID, Limit: MaxLimit})

	if err != nil {
		return output, err
	}

	logs, err := handler.services.Logs.List(ctx, store.LogFilter{TraceID: traceID, Limit: MaxLimit})

	if err != nil {
		return output, err
	}

	output.Events, output.Logs = append([]store.EventSummary{}, events...), append([]store.LogRecord{}, logs...)

	return output, nil
}
