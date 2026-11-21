package presenters

import (
	"fmt"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/lists"
)

func testProjects() Projects {
	return Projects{
		Recording: config.Recording{EventLevels: []string{"error", "fatal"}},
		FromEnv:   func(projectID int) bool { return projectID == 1 },
		Color:     func(project store.Project) string { return config.DefaultProjectColor(project.ID) },
	}
}

func TestProjectForm(t *testing.T) {
	presenter := testProjects()
	none := []string{}
	view := presenter.Form(store.Project{ID: 2, Name: "api", EventLevels: &[]string{"warning"}, LogLevels: &none})

	if view.New || view.FromEnv || view.Color != config.DefaultProjectColor(2) {
		t.Errorf("view = %+v", view)
	}

	if want := lists.Options(lists.EventLevels, []string{"warning"}); fmt.Sprint(view.EventLevels) != fmt.Sprint(want) {
		t.Errorf("event levels = %v, want only warning checked, highest first", view.EventLevels)
	}

	for _, option := range view.LogLevels {
		if option.Selected {
			t.Errorf("log level %s checked, want none (an empty list records nothing)", option.Name)
		}
	}

	// A new project: the defaults checked (nil log levels record every level).
	fresh := presenter.Form(store.Project{})

	if !fresh.New || fresh.FromEnv {
		t.Errorf("new project view = %+v", fresh)
	}

	for _, option := range fresh.EventLevels {
		if want := option.Name == "error" || option.Name == "fatal"; option.Selected != want {
			t.Errorf("event level %s checked = %v for a new project, want %v", option.Name, option.Selected, want)
		}
	}

	for _, option := range fresh.LogLevels {
		if !option.Selected {
			t.Errorf("log level %s not checked for a new project, want every level", option.Name)
		}
	}
}

func TestCleanupResult(t *testing.T) {
	stopped := fmt.Sprintf("stopped in step %d: the database was at %d%%", store.CleanupSteps, config.CleanupStopPercent)

	for _, test := range []struct {
		name string
		run  store.CleanupRun
		want string
	}{
		// The last step began and stopped once enough was deleted: it did not run out.
		{"stopped in the last step", store.CleanupRun{Status: store.CleanupDone, Reached: store.CleanupSteps, Position: []byte{1}}, stopped},
		{"every step ran to the end", store.CleanupRun{Status: store.CleanupDone, Reached: store.CleanupSteps}, "every step ran"},
	} {
		if got := cleanupResult(test.run); got != test.want {
			t.Errorf("%s: %q, want %q", test.name, got, test.want)
		}
	}
}
