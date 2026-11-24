package services

import (
	"context"
	"maps"

	"github.com/devforth/exceptic/internal/storage/store"
)

// Logs reads the logs list and deletes logs, within the viewer's access as Issues does.
type Logs struct {
	store store.Store
}

func newLogs(database store.Store) *Logs {
	return &Logs{store: database}
}

// Count counts the logs that match filter (its Limit and Offset are
// ignored) up to store.LogCountLimit or config.LogSearchTime.
func (service *Logs) Count(ctx context.Context, access store.Access, filter store.LogFilter) (store.LogCount, error) {
	if !access.Restrict(&filter.ProjectIDs) {
		return store.LogCount{}, nil
	}

	return service.store.CountLogs(ctx, filter)
}

// Levels returns the levels at least one log has.
func (service *Logs) Levels(ctx context.Context, access store.Access) ([]string, error) {
	var projectIDs []int

	if !access.Restrict(&projectIDs) {
		return nil, nil
	}

	return service.store.LogLevels(ctx, projectIDs)
}

// List returns one page of the logs that match filter, newest first, and
// how far its scan got.
func (service *Logs) List(ctx context.Context, access store.Access, filter store.LogFilter) (store.LogPage, error) {
	if !access.Restrict(&filter.ProjectIDs) {
		return store.LogPage{Logs: []store.LogRecord{}}, nil
	}

	return service.store.ListLogs(ctx, filter)
}

// Issues gives the issue each log was written with, by log id: the logs
// list links to it. A request's issue can be of another project.
func (service *Logs) Issues(ctx context.Context, access store.Access, logs []store.LogRecord) (map[int64]store.LogIssue, error) {
	issues, err := service.store.IssuesForLogs(ctx, logs)

	maps.DeleteFunc(issues, func(_ int64, issue store.LogIssue) bool { return !access.Sees(issue.ProjectID) })

	return issues, err
}

// MaxActionLogs bounds the logs of one Delete: a page holds at most 100,
// the same room to spare as MaxActionIssues.
const MaxActionLogs = MaxActionIssues

// Delete deletes the chosen logs for the viewer, who must be able to
// administer (ErrForbidden); logs deleted meanwhile or outside access are
// skipped. It says how many went.
func (service *Logs) Delete(ctx context.Context, access store.Access, viewer store.User, logIDs []int64) (int, error) {
	if !viewer.CanAdminister() {
		return 0, Forbidden("your role may not delete logs")
	}

	return service.store.DeleteLogs(ctx, access, logIDs)
}
