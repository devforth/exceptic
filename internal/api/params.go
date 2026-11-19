package api

import (
	"cmp"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Lists return DefaultLimit rows unless limit says otherwise (0 too),
// MaxLimit at most (the schema of every limit says so, see Define).
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Page is a list with the cursor of the next page; no cursor on the last one.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// pageLimit is the page size of a limit the schema has checked.
func pageLimit(limit int) int {
	return cmp.Or(limit, DefaultLimit)
}

// Cursors are plain numbers: an offset for issues and projects (sorted by
// fields that change) and deliveries, the last ID seen for events and logs
// (newest first).
func parseCursor(cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}

	value, err := strconv.ParseInt(cursor, 10, 64)

	if err != nil || value < 0 {
		return 0, Invalid("invalid cursor: pass next_cursor of the previous page")
	}

	return value, nil
}

// parseOffset reads an offset cursor (issues, projects, deliveries).
func parseOffset(cursor string) (int, error) {
	offset, err := parseCursor(cursor)

	if err != nil || offset > store.MaxOffset {
		return 0, Invalid("invalid cursor: pass next_cursor of the previous page")
	}

	return int(offset), nil
}

// projectIDs resolves a project given by name or ID as a filter: none when empty.
func projectIDs(projects config.Projects, nameOrID string) ([]int, error) {
	if nameOrID = strings.TrimSpace(nameOrID); nameOrID == "" {
		return nil, nil
	}

	project, ok := projects.Find(nameOrID)

	if !ok {
		return nil, Invalid("unknown project %q (see list_projects)", nameOrID)
	}

	return []int{project.ID}, nil
}

// levels is a level filter (checked by the schema): none when empty.
func levels(level string) []string {
	if level == "" {
		return nil
	}

	return []string{level}
}

func parseTime(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)

	if err != nil {
		return time.Time{}, Invalid("%s must be an RFC 3339 time, e.g. 2026-11-11T10:00:00Z", name)
	}

	return parsed, nil
}
