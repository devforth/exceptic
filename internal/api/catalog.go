package api

import (
	"fmt"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// handlers holds what the handlers read; one per server. Data is reached
// only through the services, the same as the UI pages.
type handlers struct {
	projects config.ProjectList
	services *services.All
}

// Endpoints is the catalog. To add an endpoint, add it here with its handler
// and its transports: REST, MCP and the web app pick it up. Descriptions say when to use an endpoint; the MCP
// instructions and skills hold the workflow. Called with nil arguments it
// serves only to read the definitions.
func Endpoints(projects config.ProjectList, all *services.All) []Endpoint {
	handler := &handlers{projects: projects, services: all}

	return append([]Endpoint{
		Define(Endpoint{
			Transports: AllTransports,
			Name:       "list_issues",
			Description: "List issues (errors grouped by cause) with event count and first/last seen. " +
				"Default: unresolved, most recently seen first, every project. The way to find errors without known IDs.",
			ReadOnly: true,
			Choices:  map[string][]string{"status": statusFilters, "level": config.Levels, "sort": store.IssueSorts, "date_by": store.DateFields},
		}, handler.listIssues),
		Define(Endpoint{
			Transports: AllTransports,
			Name:       agentprompt.GetIssueTool,
			Description: "Diagnose an issue: its latest event (or event_id) as the agent prompt of notifications, with the exception chain, " +
				fmt.Sprintf("stack frames with source, message, request, runtime, tags, breadcrumbs, up to %d logs of the request and the issue history.", relatedLogsLimit),
			ReadOnly: true,
		}, handler.getIssue),
		Define(Endpoint{
			Transports:  AllTransports,
			Name:        "get_issues",
			Description: "Summaries and current statuses of 1 to 100 known issue IDs in one call, without event details.",
			ReadOnly:    true,
		}, handler.getIssues),
		Define(Endpoint{
			Transports:  AllTransports,
			Name:        "list_issue_events",
			Description: "List the stored events of an issue, newest first: time, level, environment, release and trace ID. Pass an event_id to get_issue to read one.",
			ReadOnly:    true,
		}, handler.listIssueEvents),
		Define(Endpoint{
			Transports:  AllTransports,
			Name:        "get_event",
			Description: "Like get_issue, for an event ID the SDK reported to the application (with its project).",
			ReadOnly:    true,
		}, handler.getEvent),
		Define(Endpoint{
			Transports:  AllTransports,
			Name:        "list_logs",
			Description: "Search logs, newest first. Pass trace_id (from get_issue) for the logs of one request or browser session.",
			ReadOnly:    true,
			Choices:     map[string][]string{"level": config.LogLevels},
		}, handler.listLogs),
		Define(Endpoint{
			Transports: AllTransports,
			Name:       "list_projects",
			Description: "List the monitored projects with their unresolved issue counts and DSNs (what a Sentry SDK is configured with to report to the project). " +
				"Not needed before list_issues: issue rows name their project.",
			ReadOnly: true,
		}, handler.listProjects),
		Define(Endpoint{
			Transports: AllTransports,
			Name:       agentprompt.UpdateIssueStatusTool,
			Description: fmt.Sprintf("Set the status of 1 to 100 issues in one atomic call: %s (%s), %s (%s) or %s (%s). "+
				"Only when the user asks. Returns the updated summaries.",
				store.StatusResolved, store.StatusMeanings[store.StatusResolved], store.StatusIgnored, store.StatusMeanings[store.StatusIgnored],
				store.StatusUnresolved, store.StatusMeanings[store.StatusUnresolved]),
			Idempotent: true,
			Choices:    map[string][]string{"status": store.IssueStatuses},
		}, handler.updateIssueStatus),

		Define(Endpoint{
			Transports:  AllTransports,
			Name:        "delete_issue",
			Description: "Delete an issue with its events; if the error happens again it comes back as a new issue. Admins only.",
			Destructive: true,
			Admin:       true,
		}, handler.deleteIssue),

		// The web app only: its menu (open with a temporary password, so the app
		// can send the user to choose one), the storage of the Projects page and
		// the counts and choices of the Logs page.
		Define(Endpoint{
			Name:                      "get_session",
			Description:               "The logged-in user, the app name, the unresolved issue count and the project colors stylesheet.",
			ReadOnly:                  true,
			Transports:                TransportUI,
			OpenWithTemporaryPassword: true,
		}, handler.getSession),

		Define(Endpoint{
			Name:        "count_logs",
			Description: "How many logs match the filters of list_logs, up to a limit (more says it was reached).",
			ReadOnly:    true,
			Transports:  TransportUI,
			Choices:     map[string][]string{"level": config.LogLevels},
		}, handler.countLogs),
		Define(Endpoint{
			Name:        "list_log_levels",
			Description: "The log levels some stored log has, highest first: the choices of the level filter.",
			ReadOnly:    true,
			Transports:  TransportUI,
		}, handler.listLogLevels),
		Define(Endpoint{
			Name:        "get_storage",
			Description: "How much the database holds, each project's part and when it last received, its size limit (MAX_DATABASE_SIZE) and where the cleanup stops.",
			ReadOnly:    true,
			Transports:  TransportUI,
		}, handler.getStorage),
		Define(Endpoint{
			Name:        "list_cleanups",
			Description: "The latest runs of the cleanup by size, newest first, with the steps that deleted something.",
			ReadOnly:    true,
			Transports:  TransportUI,
		}, handler.listCleanups),
		Define(Endpoint{
			Name:        "get_project_setup",
			Description: "How to connect an app to a project: the SDK of each platform with its DSN, and whether the project has received anything.",
			ReadOnly:    true,
			Transports:  TransportUI,
		}, handler.getProjectSetup),
		Define(Endpoint{
			Name:        "get_project_defaults",
			Description: "What the form of a new project starts with: the color it would show and the levels recorded by default.",
			ReadOnly:    true,
			Admin:       true,
			Transports:  TransportUI,
		}, handler.getProjectDefaults),

		// REST only: their output must not reach a model provider.
		Define(Endpoint{
			Name:        "get_event_payload",
			Description: "An event with its JSON as stored: credentials and parts that do not help a fix removed; thinned marks an old event whose details were removed.",
			ReadOnly:    true,
			Transports:  TransportREST,
		}, handler.getEventPayload),
		Define(Endpoint{
			Name:        "list_trace",
			Description: "Events (without payloads) and logs of one trace, up to 100 of each.",
			ReadOnly:    true,
			Transports:  TransportREST,
		}, handler.listTrace),
	}, handler.management()...)
}
