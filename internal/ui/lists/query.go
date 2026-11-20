package lists

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/storage/store"
)

// A list URL can be edited by hand. Before anything else, a list keeps only
// the parameters it understands, with valid values; the query to the
// database, the filter fields, the chips and every link are then built from
// that one cleaned query, so the page never shows a filter that is not
// applied, nor applies one it does not show.

// maxSearchLength caps the search text: comparing a megabyte of pattern with
// every line helps nobody.
const maxSearchLength = 200

// EventLevels are the levels of the issue filter, highest first.
var EventLevels = config.HighestFirst(config.Levels)

// ListParams are the parameters a list understands.
type ListParams struct {
	projects bool                // ?project= (issues, logs)
	levels   []string            // the levels of this list
	single   []string            // free-text parameters, one value each
	statuses bool                // ?status= (issues)
	choices  map[string][]string // parameters with one of a few values each
	sort     bool                // ?sort= (issues)
	dates    bool                // ?from= and ?to=
}

var (
	IssueListParams = ListParams{
		projects: true, levels: EventLevels, single: []string{"query"}, statuses: true, sort: true, dates: true,
	}
)

// DeliveryStatuses are the statuses the delivery log filters by.
var DeliveryStatuses = []string{store.OutboxPending, store.OutboxFailed, store.OutboxSent}

// DeliveryListParams are those of the delivery log, which shows the connections of connectionIDs.
func DeliveryListParams(connectionIDs []string) ListParams {
	return ListParams{choices: map[string][]string{"connection": connectionIDs, "status": DeliveryStatuses}}
}

// CleanListQuery keeps the parameters of raw that params understands, with
// valid values; projects are looked up in projectList.
func CleanListQuery(raw url.Values, params ListParams, projectList config.ProjectList) url.Values {
	query := url.Values{}

	// Several values: known ones only, each once. Projects are kept by name,
	// also when given by id, so chips and colors find them.
	for _, value := range raw["project"] {
		if !params.projects {
			break
		}

		if project, ok := projectList.List().Find(strings.TrimSpace(value)); ok && !slices.Contains(query["project"], project.Name) {
			query.Add("project", project.Name)
		}
	}

	for _, value := range raw["level"] {
		if slices.Contains(params.levels, value) && !slices.Contains(query["level"], value) {
			query.Add("level", value)
		}
	}

	for _, key := range params.single {
		if value := strings.TrimSpace(raw.Get(key)); value != "" {
			query.Set(key, textutil.TruncateRunes(value, maxSearchLength))
		}
	}

	if status := raw.Get("status"); params.statuses && slices.ContainsFunc(StatusFilter, func(choice IssueStatus) bool { return choice.Name == status }) {
		query.Set("status", status)
	}

	for key, values := range params.choices {
		if value := raw.Get(key); value != "" && slices.Contains(values, value) {
			query.Set(key, value)
		}
	}

	if sort := raw.Get("sort"); params.sort && sort != "" && store.ValidIssueSort(sort) {
		query.Set("sort", sort)
	}

	if params.dates {
		cleanDates(raw, query)
	}

	if page, err := strconv.Atoi(raw.Get("page")); err == nil && page > 1 {
		query.Set("page", strconv.Itoa(page))
	}

	if perPage, err := strconv.Atoi(raw.Get("per_page")); err == nil && slices.Contains(PerPageChoices, perPage) {
		query.Set("per_page", strconv.Itoa(perPage))
	}

	return query
}

// cleanDates keeps valid days. A typed or edited URL can put "to" before
// "from": that range matches nothing, so it is read as the reverse.
func cleanDates(raw, query url.Values) {
	from, fromErr := time.Parse(time.DateOnly, raw.Get("from"))
	to, toErr := time.Parse(time.DateOnly, raw.Get("to"))

	if fromErr == nil && toErr == nil && to.Before(from) {
		from, to = to, from
	}

	if fromErr == nil {
		query.Set("from", from.Format(time.DateOnly))
	}

	if toErr == nil {
		query.Set("to", to.Format(time.DateOnly))
	}
}
