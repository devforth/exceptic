package api

import (
	"context"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

type sessionOutput struct {
	User userOutput `json:"user"`
	// CanAdminister: projects, notifications and users; the app never maps roles itself.
	CanAdminister    bool   `json:"can_administer"`
	AppName          string `json:"app_name"`
	UnresolvedIssues int    `json:"unresolved_issues"`
	// ProjectColors: the shown color of every project the user sees by name, #rrggbb.
	ProjectColors map[string]string `json:"project_colors"`
	// AllProjects: the user sees every project, future ones included.
	AllProjects bool `json:"all_projects"`
	// AssignableRoles: the roles the user may give (an admin only "user"); empty for a user.
	AssignableRoles []string `json:"assignable_roles"`
}

type storageOutput struct {
	// Used is the data the database holds, as the cleanup measures it.
	UsedBytes  int64  `json:"used_bytes"`
	Used       string `json:"used"`
	LimitBytes int64  `json:"limit_bytes,omitempty"`
	Limit      string `json:"limit,omitempty"`
	Percent    int    `json:"percent,omitempty"`
	// Files is what the files take: also the ingest queue, the write journal
	// and deleted records until their next compaction.
	FilesBytes int64  `json:"files_bytes"`
	Files      string `json:"files"`
	// The cleanup starts at CleanupStartPercent of the limit and deletes down to CleanupStopPercent.
	CleanupStartPercent int `json:"cleanup_start_percent"`
	CleanupStopPercent  int `json:"cleanup_stop_percent"`
	// Projects are each project's part of Used, in list order; Other is the
	// rest. No parts while Counting (data stored before is counted once).
	Projects   []projectStorage `json:"projects"`
	OtherBytes int64            `json:"other_bytes"`
	Other      string           `json:"other"`
	Counting   bool             `json:"counting,omitempty"`
}

type projectStorage struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Size  string `json:"size,omitempty"`
	// LastReceived: when its newest error or log came; missing when none is stored.
	LastReceived *time.Time `json:"last_received,omitempty"`
	// LimitBytes: its disk share of the size limit; 0 at 100% or without a limit.
	LimitBytes int64 `json:"limit_bytes"`
	// Dropped is what ingest dropped of it in the last 24 hours, largest first; missing when nothing.
	Dropped []droppedOutput `json:"dropped,omitempty"`
	// Oldest is what it still keeps, by the kinds the cleanup deletes.
	Oldest oldestKept `json:"oldest"`
}

// oldestKept is when a project's oldest stored record of each kind came; missing when none is stored.
type oldestKept struct {
	LogsBelowError *time.Time `json:"logs_below_error,omitempty"`
	ErrorLogs      *time.Time `json:"error_logs,omitempty"`
	Errors         *time.Time `json:"errors,omitempty"`
}

// timeIfSet is nil for the zero time.
func timeIfSet(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}

	return &at
}

// droppedOutput is a count of store.Dropped: kind events, logs or envelopes.
type droppedOutput struct {
	Kind       string `json:"kind"`
	Count      int64  `json:"count"`
	Reason     string `json:"reason"`
	ReasonText string `json:"reason_text"`
}

func droppedOutputs(dropped []store.Dropped) []droppedOutput {
	var outputs []droppedOutput

	for _, count := range dropped {
		outputs = append(outputs, droppedOutput{Kind: count.Kind, Count: count.Count, Reason: count.Reason, ReasonText: store.DropReasons[count.Reason]})
	}

	return outputs
}

func (handler *handlers) getStorage(ctx context.Context, call Call, _ noInput) (storageOutput, error) {
	storage, err := handler.services.Projects.Storage(ctx, call.Access())
	output := storageOutput{
		UsedBytes: storage.UsedBytes, Used: storage.Used, LimitBytes: storage.LimitBytes, Limit: storage.Limit,
		Percent: storage.Percent, FilesBytes: storage.FilesBytes, Files: storage.Files,
		CleanupStartPercent: storage.CleanupStartPercent, CleanupStopPercent: storage.CleanupStopPercent,
		Projects: []projectStorage{}, OtherBytes: storage.OtherBytes, Other: storage.Other, Counting: storage.Counting,
	}

	for _, project := range storage.Projects {
		output.Projects = append(output.Projects, projectStorage{
			ID: project.ID, Name: project.Name, Bytes: project.Bytes, Size: project.Size, LastReceived: project.LastReceived, LimitBytes: project.LimitBytes,
			Dropped: droppedOutputs(project.Dropped),
			Oldest: oldestKept{
				LogsBelowError: timeIfSet(project.Oldest.LogsBelowError), ErrorLogs: timeIfSet(project.Oldest.ErrorLogs), Errors: timeIfSet(project.Oldest.Errors),
			},
		})
	}

	return output, err
}

func (handler *handlers) getSession(ctx context.Context, call Call, _ noInput) (sessionOutput, error) {
	unresolved, err := handler.services.Issues.Unresolved(ctx, call.Access())

	if err != nil {
		return sessionOutput{}, err
	}

	return sessionOutput{
		User: handler.userView(call.Viewer, call.Viewer), CanAdminister: call.Viewer.CanAdminister(),
		AppName: handler.services.AppName, UnresolvedIssues: unresolved,
		ProjectColors: handler.services.Projects.ColorsByName(call.Access()), AllProjects: call.Access().All(),
		AssignableRoles: append([]string{}, call.Viewer.AssignableRoles()...),
	}, nil
}
