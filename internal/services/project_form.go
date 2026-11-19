package services

import (
	"net/url"
	"slices"
	"strings"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// readProjectForm applies the project form (posted by the UI, or a tool's
// input under the same names) to project and returns the field errors. The
// name is read only when it can change (not for a project from PROJECTS).
// paletteColor is the color the project has without its own one; empty for
// a new project, which keeps the color chosen.
func readProjectForm(form url.Values, project *store.Project, nameEditable bool, recording config.Recording, paletteColor string) Problems {
	problems := Problems{}

	if nameEditable {
		project.Name = strings.TrimSpace(form.Get("name"))

		if !config.ValidProjectName(project.Name) {
			problems["name"] = "Use lowercase letters, digits, '-' and '_', starting with a letter or digit, not digits only."
		}
	}

	// The palette color is stored as "" so the project keeps following the
	// palette; any other color is stored as chosen. An empty one returns to
	// the palette, a missing one keeps the color.
	if form.Has("color") {
		color := strings.ToLower(strings.TrimSpace(form.Get("color")))

		switch {
		case color == "":
			project.Color = ""
		case !config.ValidColor(color):
			problems["color"] = "Choose a color."
		case paletteColor != "" && color == paletteColor:
			project.Color = ""
		default:
			project.Color = color
		}
	}

	// The levels to record. The whole list is sent with the form, so nothing
	// checked means record nothing; the defaults are stored as "use the
	// defaults", so a project follows RECORD_*_LEVELS until it differs.
	project.EventLevels = chosenLevels(form["event_level"], config.Levels, recording.EventLevels)
	project.LogLevels = chosenLevels(form["log_level"], config.LogLevels, recording.LogLevels)

	return problems
}

// chosenLevels keeps the known levels of the form; nil when they are the
// defaults (nil defaults: every level).
func chosenLevels(values, known, defaults []string) *[]string {
	levels := config.InLevelOrder(values, known)

	if defaults == nil {
		defaults = known
	}

	if slices.Equal(levels, config.InLevelOrder(defaults, known)) {
		return nil
	}

	return &levels
}

// FormValues is project as the form would post it unchanged: a tool that
// changes some fields starts from these, so the others keep their values.
func (service *Projects) FormValues(project store.Project) url.Values {
	form := url.Values{"name": {project.Name}, "color": {project.Color}}

	form["event_level"] = recordedLevels(project.EventLevels, service.recording.EventLevels, config.Levels)
	form["log_level"] = recordedLevels(project.LogLevels, service.recording.LogLevels, config.LogLevels)

	return form
}

// recordedLevels: the project's own levels, or the defaults (nil: all).
func recordedLevels(own *[]string, defaults, known []string) []string {
	switch {
	case own != nil:
		return append([]string{}, *own...)
	case defaults != nil:
		return append([]string{}, defaults...)
	default:
		return append([]string{}, known...)
	}
}
