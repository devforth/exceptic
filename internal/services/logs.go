package services

import (
	"context"

	"github.com/devforth/exceptic/internal/storage/store"
)

// Logs reads the logs list and deletes logs.
type Logs struct {
	store store.Store
}

func newLogs(database store.Store) *Logs {
	return &Logs{store: database}
}

// Count counts the logs that match filter (its Limit and Offset are
// ignored) up to store.LogCountLimit; more says there are more.
func (service *Logs) Count(ctx context.Context, filter store.LogFilter) (count int, more bool, err error) {
	return service.store.CountLogs(ctx, filter)
}

// Levels returns the levels at least one log has.
func (service *Logs) Levels(ctx context.Context) ([]string, error) {
	return service.store.LogLevels(ctx)
}

// List returns one page of the logs that match filter, newest first.
func (service *Logs) List(ctx context.Context, filter store.LogFilter) ([]store.LogRecord, error) {
	return service.store.ListLogs(ctx, filter)
}

// Issues gives the issue each log was written with, by log id: the logs
// list links to it.
func (service *Logs) Issues(ctx context.Context, logs []store.LogRecord) (map[int64]store.LogIssue, error) {
	return service.store.IssuesForLogs(ctx, logs)
}

// MaxActionLogs bounds the logs of one Delete: a page holds at most 100,
// the same room to spare as MaxActionIssues.
const MaxActionLogs = MaxActionIssues

// Delete deletes the chosen logs for the viewer, who must be able to
// administer (ErrForbidden); logs deleted meanwhile are skipped. It says how many went.
func (service *Logs) Delete(ctx context.Context, viewer store.User, logIDs []int64) (int, error) {
	if !viewer.CanAdminister() {
		return 0, ErrForbidden
	}

	return service.store.DeleteLogs(ctx, logIDs)
}
