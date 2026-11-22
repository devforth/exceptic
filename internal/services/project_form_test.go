package services

import (
	"net/url"
	"slices"
	"testing"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

func TestProjectFormLevels(t *testing.T) {
	recording := config.Recording{LogLevels: []string{"warn", "error", "fatal"}}

	var project store.Project

	problems := readProjectForm(url.Values{
		"name": {"api"}, "log_level": {"fatal", "warn", "error"},
	}, &project, recording, "#2563eb")

	if len(problems) > 0 {
		t.Fatalf("problems = %v", problems)
	}

	if project.LogLevels != nil {
		t.Errorf("the default stored as %v, want nil (follow the default)", project.LogLevels)
	}

	// Levels the list does not know are dropped.
	readProjectForm(url.Values{"name": {"api"}, "log_level": {"info", "bogus"}}, &project, recording, "#2563eb")

	if project.LogLevels == nil || !slices.Equal(*project.LogLevels, []string{"info"}) {
		t.Errorf("log levels = %v, want [info]", project.LogLevels)
	}

	// Nothing checked records nothing.
	readProjectForm(url.Values{"name": {"api"}}, &project, recording, "#2563eb")

	if project.LogLevels == nil || len(*project.LogLevels) != 0 {
		t.Errorf("log levels = %v, want an empty list (record none)", project.LogLevels)
	}
}

func TestProjectFormValidation(t *testing.T) {
	project := store.Project{Name: "backend", Color: "#123456"}
	problems := readProjectForm(url.Values{
		"name": {"backend"}, "color": {"#2563EB"},
	}, &project, config.Recording{}, "#2563eb")

	if len(problems) > 0 {
		t.Errorf("problems = %v", problems)
	}

	// The palette color is stored as "" so the project keeps following the palette.
	if project.Color != "" {
		t.Errorf("color = %q, want empty for the palette color", project.Color)
	}

	// Digits only would read as a project ID wherever a name or ID is taken.
	for _, name := range []string{"Not Valid", "3", "-web"} {
		if problems := readProjectForm(url.Values{"name": {name}}, &project, config.Recording{}, ""); problems["name"] == "" {
			t.Errorf("invalid name %q accepted", name)
		}
	}

	for _, name := range []string{"web", "3d", "1_api", "v2-api"} {
		if problems := readProjectForm(url.Values{"name": {name}}, &project, config.Recording{}, ""); problems["name"] != "" {
			t.Errorf("valid name %q refused: %s", name, problems["name"])
		}
	}
}
