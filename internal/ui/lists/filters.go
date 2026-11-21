// Package lists is what the filtered lists of the old pages (the delivery
// log) and their templates share: the cleaned query, links, badges, options
// and pagination.
package lists

import (
	"maps"
	"net/http"
	"net/url"
)

// FilterLinks builds the links of a list (its pages) from its query.
type FilterLinks struct {
	path  string
	query url.Values
}

// NewFilterLinks builds the links of a list from its cleaned query (see
// CleanListQuery), never from the raw URL.
func NewFilterLinks(r *http.Request, query url.Values) FilterLinks {
	return FilterLinks{path: r.URL.Path, query: query}
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
