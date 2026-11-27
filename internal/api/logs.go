package api

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
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
	FromID int64  `json:"from_id,omitempty" jsonschema:"Only logs with this ID or a greater one."`
}

// searchOutput says that a log search stopped at its time limit.
type searchOutput struct {
	// Stopped: the search ran out of time; logs received before SearchedTo
	// may hold more matches.
	Stopped    bool       `json:"stopped,omitempty"`
	SearchedTo *time.Time `json:"searched_to,omitempty"`
}

func searchOf(scan store.LogScan) searchOutput {
	if !scan.Stopped {
		return searchOutput{}
	}

	return searchOutput{Stopped: true, SearchedTo: timeIfSet(scan.SearchedTo)}
}

type logsPage struct {
	Items      []logOutput `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	searchOutput
}

type logOutput struct {
	ID      int64     `json:"id"`
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

func (handler *handlers) listLogs(ctx context.Context, call Call, input listLogsInput) (logsPage, error) {
	output := logsPage{Items: []logOutput{}}
	filter, err := handler.logFilter(call, input.logFiltersInput)

	if err != nil {
		return output, err
	}

	if input.Offset > 0 && input.Cursor != "" {
		return output, Invalid("pass offset or cursor, not both")
	}

	if input.Offset < 0 || input.Offset > store.MaxOffset {
		return output, Invalid("offset out of range")
	}

	if input.FromID < 0 {
		return output, Invalid("from_id out of range")
	}

	limit := pageLimit(input.Limit)
	filter.Limit, filter.Offset, filter.FromID = limit+1, input.Offset, input.FromID

	if filter.BeforeID, err = parseCursor(input.Cursor); err != nil {
		return output, err
	}

	page, err := handler.services.Logs.List(ctx, call.Access(), filter)

	if err != nil {
		return output, err
	}

	logs, more := store.Page(page.Logs, limit)
	issues, err := handler.services.Logs.Issues(ctx, call.Access(), logs)

	if err != nil {
		return output, err
	}

	for _, record := range logs {
		item := logOutput{
			ID: record.ID, Time: record.Timestamp, Project: handler.projectName(record.ProjectID), Level: record.Level,
			Body: call.Text(record.Body), TraceID: call.TraceID(record.TraceID),
		}

		if issue, ok := issues[record.ID]; ok {
			item.Issue = &logIssueOutput{ID: issue.IssueID, Title: call.Text(issue.Title)}
		}

		output.Items = append(output.Items, item)
	}

	// A search that ran out of time goes on below where it stopped, unless it
	// may have stopped while skipping the offset: the cursor would then give
	// the rows the offset was to skip.
	switch output.searchOutput = searchOf(page.LogScan); {
	case more:
		output.NextCursor = strconv.FormatInt(logs[limit-1].ID, 10)
	case page.Stopped && page.Rest > 0 && (input.Offset == 0 || len(logs) > 0):
		output.NextCursor = strconv.FormatInt(page.Rest, 10)
	}

	return output, nil
}

type deleteLogsInput struct {
	LogIDs []int64 `json:"log_ids" jsonschema:"The logs to delete; logs deleted meanwhile are skipped."`
}

type deletedLogsOutput struct {
	Deleted int `json:"deleted"`
}

// deleteLogs deletes the chosen logs; services.Logs checks the role.
func (handler *handlers) deleteLogs(ctx context.Context, call Call, input deleteLogsInput) (deletedLogsOutput, error) {
	switch {
	case len(input.LogIDs) == 0:
		return deletedLogsOutput{}, Invalid("pass log_ids")
	case len(input.LogIDs) > services.MaxActionLogs:
		return deletedLogsOutput{}, Invalid("at most %d logs at once", services.MaxActionLogs)
	}

	deleted, err := handler.services.Logs.Delete(ctx, call.Access(), call.Viewer, input.LogIDs)
	handler.audit(ctx, call, logChanges(deleted)...)

	return deletedLogsOutput{Deleted: len(deleted)}, err
}

type countOutput struct {
	Count int `json:"count"`
	// More: the count stopped at its limit; there are more.
	More bool `json:"more,omitempty"`
}

type logCountOutput struct {
	countOutput
	searchOutput
	// FromID is list_logs' from_id for these logs: the list then searches no
	// further back than this count did.
	FromID int64 `json:"from_id,omitempty"`
}

func (handler *handlers) countLogs(ctx context.Context, call Call, input logFiltersInput) (logCountOutput, error) {
	filter, err := handler.logFilter(call, input)

	if err != nil {
		return logCountOutput{}, err
	}

	count, err := handler.services.Logs.Count(ctx, call.Access(), filter)

	return logCountOutput{countOutput: countOutput{Count: count.Count, More: count.More}, searchOutput: searchOf(count.LogScan), FromID: count.FromID}, err
}

type levelsOutput struct {
	// Levels are highest first.
	Levels []string `json:"levels"`
}

func (handler *handlers) listLogLevels(ctx context.Context, call Call, _ noInput) (levelsOutput, error) {
	present, err := handler.services.Logs.Levels(ctx, call.Access())

	return levelsOutput{Levels: append([]string{}, config.InLevelOrder(present, config.LogLevels)...)}, err
}

func (handler *handlers) listLogProjects(ctx context.Context, call Call, _ noInput) (projectNamesOutput, error) {
	ids, err := handler.services.Logs.Projects(ctx, call.Access())

	return handler.projectNames(ids), err
}

func (handler *handlers) logFilter(call Call, input logFiltersInput) (store.LogFilter, error) {
	filter := store.LogFilter{Query: strings.TrimSpace(input.Query), TraceID: strings.TrimSpace(input.TraceID), Levels: input.Level}

	var err error

	if filter.ProjectIDs, err = handler.projectIDs(call, input.Project); err != nil {
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
	searchOutput
}

// listTrace is REST only: log attributes are raw SDK data.
func (handler *handlers) listTrace(ctx context.Context, call Call, input listTraceInput) (traceOutput, error) {
	traceID := strings.TrimSpace(input.TraceID)

	if traceID == "" {
		return traceOutput{}, Invalid("trace_id is required")
	}

	output := traceOutput{TraceID: traceID}
	events, err := handler.services.Issues.Events(ctx, call.Access(), store.EventFilter{TraceID: traceID, Limit: MaxLimit})

	if err != nil {
		return output, err
	}

	logs, err := handler.services.Logs.List(ctx, call.Access(), store.LogFilter{TraceID: traceID, Limit: MaxLimit})

	if err != nil {
		return output, err
	}

	output.Events, output.Logs = append([]store.EventSummary{}, events...), append([]store.LogRecord{}, logs.Logs...)
	output.searchOutput = searchOf(logs.LogScan)

	return output, nil
}
