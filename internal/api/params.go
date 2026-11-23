package api

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/services"
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

// projectIDs resolves projects given by name or ID as a filter (none when
// empty), each once: an unknown one is not found, one outside the call's
// projects forbidden (services.Projects.Find).
func (handler *handlers) projectIDs(call Call, namesOrIDs []string) ([]int, error) {
	var ids []int

	for _, nameOrID := range namesOrIDs {
		project, err := handler.services.Projects.Find(call.Access(), nameOrID)

		if err != nil {
			return nil, err
		}

		if !slices.Contains(ids, project.ID) {
			ids = append(ids, project.ID)
		}
	}

	return ids, nil
}

// projectsField is projectIDs for a form's field: an unknown project is
// the field's problem (400), one outside the call's projects a refusal
// (403) at the field.
func (handler *handlers) projectsField(call Call, field string, namesOrIDs []string) ([]int, error) {
	ids, err := handler.projectIDs(call, namesOrIDs)

	var reason services.Reason

	if !errors.As(err, &reason) {
		return ids, err
	}

	problem := strings.Replace(reason.Text, "not found: project", "unknown project", 1)
	input, _ := problemsError(map[string]string{field: problem}, nil).(InputError)

	if errors.Is(reason, services.ErrHidden) {
		input.status, input.hidden = http.StatusForbidden, true
	}

	return nil, input
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
