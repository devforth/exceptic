package api

import "context"

type sessionOutput struct {
	User        userOutput `json:"user"`
	DisplayName string     `json:"display_name"`
	// CanAdminister: projects, notifications and users; the app never maps roles itself.
	CanAdminister    bool   `json:"can_administer"`
	AppName          string `json:"app_name"`
	UnresolvedIssues int    `json:"unresolved_issues"`
	// ProjectColorsURL is the stylesheet of the project colors (inline styles are blocked).
	ProjectColorsURL string `json:"project_colors_url"`
}

type storageOutput struct {
	UsedBytes  int64  `json:"used_bytes"`
	Used       string `json:"used"`
	LimitBytes int64  `json:"limit_bytes,omitempty"`
	Limit      string `json:"limit,omitempty"`
	Percent    int    `json:"percent,omitempty"`
	// CleanupStopPercent: at the limit the cleanup deletes down to this percent of it.
	CleanupStopPercent int `json:"cleanup_stop_percent"`
}

func (handler *handlers) getStorage(ctx context.Context, _ Call, _ noInput) (storageOutput, error) {
	storage, err := handler.services.Projects.Storage(ctx)

	return storageOutput{
		UsedBytes: storage.UsedBytes, Used: storage.Used, LimitBytes: storage.LimitBytes, Limit: storage.Limit,
		Percent: storage.Percent, CleanupStopPercent: storage.CleanupStopPercent,
	}, err
}

func (handler *handlers) getSession(ctx context.Context, call Call, _ noInput) (sessionOutput, error) {
	unresolved, err := handler.services.Issues.Unresolved(ctx)

	if err != nil {
		return sessionOutput{}, err
	}

	return sessionOutput{
		User: userView(call.Viewer), DisplayName: call.Viewer.DisplayName(), CanAdminister: call.Viewer.CanAdminister(),
		AppName: handler.services.AppName, UnresolvedIssues: unresolved, ProjectColorsURL: handler.services.Projects.ColorsURL(),
	}, nil
}
