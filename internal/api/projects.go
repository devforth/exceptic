package api

import (
	"context"
	"strconv"

	"github.com/devforth/exceptic/internal/platform/config"
)

type listProjectsInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Projects per page."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page."`
}

// projectView is a project as list_projects and get_project show it.
type projectView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// DSN configures a Sentry SDK to report to the project.
	DSN string `json:"dsn,omitempty"`
	// FromEnv: set by PROJECTS, so its key comes from there and it cannot be deleted.
	FromEnv bool `json:"from_env,omitempty"`
	// LogLevels are the log levels recorded, highest first; events of every level are.
	LogLevels []string `json:"log_levels"`
}

func (handler *handlers) projectView(call Call, project config.Project) projectView {
	view := projectView{ID: project.ID, Name: project.Name, FromEnv: handler.services.Projects.FromEnv(project.ID)}
	view.LogLevels = handler.services.Projects.RecordedLogLevels(project)

	if call.Address != nil {
		view.DSN = project.DSN(call.Address)
	}

	return view
}

type projectOutput struct {
	projectView

	UnresolvedIssues int `json:"unresolved_issues"`
}

func (handler *handlers) listProjects(ctx context.Context, call Call, input listProjectsInput) (Page[projectOutput], error) {
	output := Page[projectOutput]{Items: []projectOutput{}}
	limit := pageLimit(input.Limit)
	offset, err := parseOffset(input.Cursor)

	if err != nil {
		return output, err
	}

	projects := handler.projects.List()

	if offset >= len(projects) {
		return output, nil
	}

	counts, err := handler.services.Projects.UnresolvedCounts(ctx)

	if err != nil {
		return output, err
	}

	end := min(offset+limit, len(projects))

	for _, project := range projects[offset:end] {
		output.Items = append(output.Items, projectOutput{projectView: handler.projectView(call, project), UnresolvedIssues: counts[project.ID]})
	}

	if end < len(projects) {
		output.NextCursor = strconv.Itoa(end)
	}

	return output, nil
}
