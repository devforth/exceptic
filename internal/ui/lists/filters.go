// Package lists is what the filtered lists (issues, logs, deliveries) and
// their templates share: the cleaned query, filter links, chips, badges,
// pickers, pagination and date ranges.
package lists

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/devforth/exceptic/internal/storage/store"
)

// FilterLinks builds links that narrow the current list: clicking a project or
// level in a row keeps the other filters and starts from the first page.
type FilterLinks struct {
	path  string
	query url.Values
}

// NewFilterLinks builds the links of a list from its cleaned query (see
// CleanListQuery), never from the raw URL.
func NewFilterLinks(r *http.Request, query url.Values) FilterLinks {
	return FilterLinks{path: r.URL.Path, query: query}
}

// With returns the current list URL with key set to value, from the first page: the badge links of a list (linkBadge).
func (links FilterLinks) With(key, value string) string {
	query := links.firstPage()
	query.Set(key, value)

	return links.path + "?" + query.Encode()
}

// Current is the list URL with the current filters and page: where bulk
// actions return to.
func (links FilterLinks) Current() string {
	return links.url(links.query)
}

// firstPage is a copy of the query without the page.
func (links FilterLinks) firstPage() url.Values {
	query := maps.Clone(links.query)
	delete(query, "page")

	return query
}

func (links FilterLinks) url(query url.Values) string {
	if len(query) == 0 {
		return links.path
	}

	return links.path + "?" + query.Encode()
}

// filterField describes a list filter for the "active filters" chips.
type filterField struct {
	key, label string
	// unset is the value that means "not filtered" (e.g. the default status).
	unset string
	// names shows a value in words; values not listed are shown as is.
	names map[string]string
	// dates: the date range (from, to) as one chip.
	dates bool
	// badges: several values, shown as badges of this kind in one chip, each
	// removable on its own.
	badges string
}

var issueFilterFields = []filterField{
	{key: "project", label: "Project", badges: KindProject},
	{key: "status", label: "Status", unset: store.StatusUnresolved, badges: KindStatus},
	{key: "level", label: "Level", badges: KindLevel},
	{key: "query", label: "Search"},
	{label: "Dates", dates: true},
	{key: "sort", label: "Sort", unset: store.DefaultIssueSort, names: map[string]string{
		store.SortLastSeenAsc: "last seen, oldest first", store.SortFirstSeen: "first seen, newest first", store.SortFirstSeenAsc: "first seen, oldest first",
		store.SortEventCount: "most events", store.SortEventCountAsc: "fewest events", store.SortProject: "project A–Z", store.SortProjectDesc: "project Z–A",
	}},
}

type ActiveFilter struct {
	Label, Value string
	RemoveURL    string // the list without this filter
	// Badges: several chosen values in one chip, each with its own × (RemoveURL).
	Badges []Badge
}

type ActiveFilters struct {
	Items    []ActiveFilter
	ClearURL string // the list without any filter; the page size stays
}

// active lists the filters applied to the current list, in fields order.
func (links FilterLinks) active(fields []filterField) ActiveFilters {
	// The page size is not a filter: "Clear all" keeps it.
	result := ActiveFilters{ClearURL: links.path}

	if perPage := links.query.Get("per_page"); perPage != "" {
		result.ClearURL = links.url(url.Values{"per_page": {perPage}})
	}

	for _, field := range fields {
		if field.badges != "" {
			result.Items = append(result.Items, links.badgeChips(field)...)

			continue
		}

		if field.dates {
			result.Items = append(result.Items, links.datesChip(field)...)

			continue
		}

		value := strings.TrimSpace(links.query.Get(field.key))

		if value == "" || value == field.unset {
			continue
		}

		if name, ok := field.names[value]; ok {
			value = name
		}

		without := links.firstPage()
		without.Del(field.key)
		result.Items = append(result.Items, ActiveFilter{Label: field.label, Value: value, RemoveURL: links.url(without)})
	}

	return result
}

// badgeChips gives one chip with every chosen value; the × of a value
// removes just that one.
func (links FilterLinks) badgeChips(field filterField) []ActiveFilter {
	chip := ActiveFilter{Label: field.label}
	selected := links.query[field.key]

	// The default (e.g. unresolved issues) is not a filter.
	if field.unset != "" && len(selected) == 1 && selected[0] == field.unset {
		return nil
	}

	for _, value := range selected {
		if strings.TrimSpace(value) == "" {
			continue
		}

		without := links.firstPage()
		without[field.key] = slices.DeleteFunc(slices.Clone(selected), func(other string) bool { return other == value })

		if len(without[field.key]) == 0 {
			delete(without, field.key)
		}

		chip.Badges = append(chip.Badges, Badge{Kind: field.badges, Name: value, RemoveURL: links.url(without)})
	}

	if len(chip.Badges) == 0 {
		return nil
	}

	return []ActiveFilter{chip}
}

// IssueFilters and LogFilters are the active filters of each list, for templates.
func (links FilterLinks) IssueFilters() ActiveFilters { return links.active(issueFilterFields) }

// datesChip shows the date range as one chip ("2026-11-01 – 2026-11-10"),
// and × removes both ends.
func (links FilterLinks) datesChip(field filterField) []ActiveFilter {
	from, to := strings.TrimSpace(links.query.Get("from")), strings.TrimSpace(links.query.Get("to"))

	var value string

	switch {
	case from != "" && to != "":
		value = from + " – " + to
	case from != "":
		value = "from " + from
	case to != "":
		value = "until " + to
	default:
		return nil
	}

	without := links.firstPage()

	for _, key := range []string{"from", "to"} {
		without.Del(key)
	}

	return []ActiveFilter{{Label: field.label, Value: value, RemoveURL: links.url(without)}}
}

// SortHeader is a sortable column heading of the issue list.
type SortHeader struct {
	Label, Class string
	URL          string // the list sorted by this column, or reversed when it already is
	Direction    string // "desc" or "asc" when the list is sorted by this column
	Title        string
}

// AriaSort is the column's aria-sort value, empty when it is not sorted by.
func (header SortHeader) AriaSort() string {
	switch header.Direction {
	case "asc":
		return "ascending"
	case "desc":
		return "descending"
	}

	return ""
}

// sortColumn is a sortable column of the issue list: its ?sort= value in the
// natural direction (first: dates and counts newest or most first, the project
// A to Z) and reversed, the value of the other direction.
type sortColumn struct {
	field, reversed, first, label, class string
}

// sortColumns are the sortable columns of the issue list, in page order.
var sortColumns = []sortColumn{
	{store.SortProject, store.SortProjectDesc, "asc", "Project", ""},
	{store.SortEventCount, store.SortEventCountAsc, "desc", "Events", "number wide-only"},
	{store.SortFirstSeen, store.SortFirstSeenAsc, "desc", "First seen", "wide-only"},
	{store.SortLastSeen, store.SortLastSeenAsc, "desc", "Last seen", "wide-only"},
}

// SortHeaders are the headings of the sortable columns: one sort at a time;
// the first click sorts in the column's natural direction, the next reverses
// it. The page starts over, the other filters stay.
func (links FilterLinks) SortHeaders() []SortHeader {
	headers := make([]SortHeader, len(sortColumns))

	for index, column := range sortColumns {
		headers[index] = links.sortHeader(column)
	}

	return headers
}

func (links FilterLinks) sortHeader(column sortColumn) SortHeader {
	current := links.query.Get("sort")

	if current == "" {
		current = store.DefaultIssueSort
	}

	header := SortHeader{Label: column.label, Class: column.class, Title: "Sort by " + strings.ToLower(column.label)}
	next := column.field

	switch current {
	case column.field:
		header.Direction, next = column.first, column.reversed
	case column.reversed:
		header.Direction = map[string]string{"asc": "desc", "desc": "asc"}[column.first]
	}

	if header.Direction != "" {
		header.Title = "Reverse the order"
	}

	query := links.firstPage()

	if next == store.DefaultIssueSort {
		query.Del("sort")
	} else {
		query.Set("sort", next)
	}

	header.URL = links.url(query)

	return header
}
