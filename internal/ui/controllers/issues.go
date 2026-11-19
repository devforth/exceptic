package controllers

import (
	"errors"
	"net/http"

	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/contracts"
	"github.com/devforth/exceptic/internal/ui/lists"
	"github.com/devforth/exceptic/internal/ui/presenters"
)

// IssuesPage lists issues. htmx requests (filters, page links) get only the
// "issue-list" part they replace.
func (controller *Controllers) IssuesPage(w http.ResponseWriter, r *http.Request) {
	query := lists.CleanListQuery(r.URL.Query(), lists.IssueListParams, controller.projects)
	present, err := controller.issues.Levels(r.Context())

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	data := contracts.IssuesData{
		Status: query.Get("status"),
		Levels: lists.NewLevelPicker(lists.EventLevels, present, query["level"]),
		Query:  query.Get("query"),
		Dates:  lists.ParseDateRange(query),
		Sort:   query.Get("sort"),
		Filter: lists.NewFilterLinks(r, query),
	}
	projectIDs := controller.projects.IDs(query["project"])
	data.Projects = lists.NewProjectPicker(controller.projects.Names(), query["project"])

	if data.Status == "" {
		data.Status = store.StatusUnresolved
	}

	page, perPage := lists.ReadPaging(query)
	filter := store.IssueFilter{
		ProjectIDs: projectIDs,
		Levels:     query["level"],
		Query:      data.Query,
		SeenSince:  data.Dates.Since,
		SeenUntil:  data.Dates.Until,
		Sort:       data.Sort,
	}

	if data.Status != store.StatusAll {
		filter.Status = data.Status
	}

	total, err := controller.issues.Count(r.Context(), filter)

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	// A page past the end (fewer results after a filter or a delete) shows the last one.
	data.Pager = lists.NewPager(data.Filter, page, perPage, total, lists.IssueList)
	filter.Limit, filter.Offset = perPage, (data.Pager.Page-1)*perPage

	issues, err := controller.issues.List(r.Context(), filter)

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	data.Issues = presenters.IssueRows(issues, controller.projects.Name)
	controller.renderList(w, r, "issues", lists.IssueList, data)
}

// loadIssue reads the issue from the {issueID} path value. When it fails it
// has already written the response (404 or 500) and returns ok == false.
func (controller *Controllers) loadIssue(w http.ResponseWriter, r *http.Request) (contracts.IssueRow, bool) {
	issueID, ok := controller.pathID(w, r, "issueID")

	if !ok {
		return contracts.IssueRow{}, false
	}

	stored, err := controller.issues.Get(r.Context(), issueID)

	if controller.failed(w, r, err) {
		return contracts.IssueRow{}, false
	}

	return presenters.IssueRow(stored, controller.projects.Name), true
}

// IssuePage shows an issue with one of its events (?event=<event_id>, the
// latest by default): exceptions with stack traces, breadcrumbs, context and
// the logs written during the same request (see services.Issues.RelatedLogs).
func (controller *Controllers) IssuePage(w http.ResponseWriter, r *http.Request) {
	issue, ok := controller.loadIssue(w, r)

	if !ok {
		return
	}

	data := contracts.IssueData{Issue: issue}

	var err error

	data.Events, err = controller.issues.RecentEvents(r.Context(), issue.ID)

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	data.History, err = controller.issues.History(r.Context(), issue.ID)

	if err != nil {
		controller.serverError(w, r, err)

		return
	}

	data.Event, err = controller.issues.SelectedEvent(r.Context(), issue.Issue, r.URL.Query().Get("event"))

	if err != nil && !errors.Is(err, store.ErrNotFound) {
		controller.serverError(w, r, err)

		return
	}

	if err == nil {
		data.SelectedID = data.Event.EventID
		// Stored payloads were valid when received, so a parse error here is unexpected.
		data.Parsed, err = sentry.ParseEvent(data.Event.Payload, data.Event.EventID, data.Event.ReceivedAt)

		if err != nil {
			controller.logger.Warn("stored event payload does not parse", "event_id", data.Event.EventID, "err", err)
		}

		data.RelatedLogs, data.LogsFromRequest, err = controller.issues.RelatedLogs(r.Context(), data.Event, data.Parsed)

		if err == nil {
			data.TraceLogs, err = controller.issues.TraceLogs(r.Context(), data.Event)
		}

		if err != nil {
			controller.serverError(w, r, err)

			return
		}
	}

	data.AgentPrompt = services.AgentPrompt(issue.ProjectName, issue.Issue, data.Parsed, data.RelatedLogs, data.LogsFromRequest)

	controller.views.Render(w, r, http.StatusOK, "issue", data)
}
