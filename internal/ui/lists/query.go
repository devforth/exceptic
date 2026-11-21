package lists

import (
	"net/url"
	"slices"
	"strconv"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// A list URL can be edited by hand. Before anything else, a list keeps only
// the parameters it understands, with valid values; the query to the
// database, the filter fields and every link are then built from that one
// cleaned query, so the page never shows a filter that is not applied, nor
// applies one it does not show.

// EventLevels are the event levels highest first, as the forms list them.
var EventLevels = config.HighestFirst(config.Levels)

// ListParams are the parameters a list understands: each with one of a few values.
type ListParams struct {
	choices map[string][]string
}

// DeliveryStatuses are the statuses the delivery log filters by.
var DeliveryStatuses = []string{store.OutboxPending, store.OutboxFailed, store.OutboxSent}

// DeliveryListParams are those of the delivery log, which shows the connections of connectionIDs.
func DeliveryListParams(connectionIDs []string) ListParams {
	return ListParams{choices: map[string][]string{"connection": connectionIDs, "status": DeliveryStatuses}}
}

// CleanListQuery keeps the parameters of raw that params understands, with
// valid values, and the paging.
func CleanListQuery(raw url.Values, params ListParams) url.Values {
	query := url.Values{}

	for key, values := range params.choices {
		if value := raw.Get(key); value != "" && slices.Contains(values, value) {
			query.Set(key, value)
		}
	}

	if page, err := strconv.Atoi(raw.Get("page")); err == nil && page > 1 {
		query.Set("page", strconv.Itoa(page))
	}

	if perPage, err := strconv.Atoi(raw.Get("per_page")); err == nil && slices.Contains(PerPageChoices, perPage) {
		query.Set("per_page", strconv.Itoa(perPage))
	}

	return query
}
