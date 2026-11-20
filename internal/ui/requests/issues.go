// Package requests reads what the UI's requests carry about HTTP itself:
// bulk selections, redirect targets, htmx headers and the viewer.
package requests

import (
	"fmt"
	"net/http"

	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// ReadBulkIssues reads the selected "issue" IDs of the issues list. problem
// is the reason the form is rejected (400), empty when it is valid.
func ReadBulkIssues(r *http.Request) (issueIDs []string, problem string) {
	if err := r.ParseForm(); err != nil {
		return nil, "invalid form"
	}

	if len(r.PostForm["issue"]) > services.MaxActionIssues {
		return nil, fmt.Sprintf("at most %d issues at once", services.MaxActionIssues)
	}

	issueIDs = make([]string, 0, len(r.PostForm["issue"]))

	for _, value := range r.PostForm["issue"] {
		if !store.ValidID(value) {
			return nil, "invalid issue id"
		}

		issueIDs = append(issueIDs, value)
	}

	return issueIDs, ""
}
