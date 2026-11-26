package services

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// readProjectForm applies the project form (built by the catalog from a
// request's input, under the form's field names) to project and returns the field errors.
// paletteColor is the color the project has without its own one; empty for
// a new project, which keeps the color chosen.
func readProjectForm(form url.Values, project *store.Project, recording config.Recording, paletteColor string) Problems {
	problems := Problems{}

	project.Name = strings.TrimSpace(form.Get("name"))

	if !config.ValidProjectName(project.Name) {
		problems["name"] = "Use lowercase letters, digits, '-' and '_', starting with a letter or digit, not digits only."
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

	// The log levels to record. The whole list is sent with the form, so
	// nothing checked means record nothing; the default is stored as "use the
	// default", so a project follows RECORD_LOG_LEVELS until it differs.
	project.LogLevels = chosenLevels(form["log_level"], config.LogLevels, recording.LogLevels)

	// A missing share keeps the stored one (forms without the field).
	if form.Has("disk_share") {
		share, err := strconv.Atoi(strings.TrimSpace(form.Get("disk_share")))

		if err != nil || share < store.MinDiskShare || share > store.MaxDiskShare || share%store.DiskShareStep != 0 {
			problems["disk_share"] = fmt.Sprintf("Choose %d%% to %d%% in steps of %d%%.", store.MinDiskShare, store.MaxDiskShare, store.DiskShareStep)
		} else {
			project.DiskShare = share
		}
	}

	// The allowed origins, one per value; a missing field keeps them.
	if form.Has("allowed_origin") {
		origins, problem := allowedOrigins(form["allowed_origin"])

		if problem != "" {
			problems["allowed_origin"] = problem
		} else {
			project.AllowedOrigins = origins
		}
	}

	return problems
}

// allowedOrigins reads the form's origins (empty values skipped, the same
// one once) or says what is wrong.
func allowedOrigins(values []string) ([]string, string) {
	origins := []string{}

	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}

		origin, err := config.ParseOrigin(value)

		if err != nil {
			return nil, "Not a site address: " + err.Error() + "."
		}

		if !slices.Contains(origins, origin) {
			origins = append(origins, origin)
		}
	}

	if len(origins) > config.MaxAllowedOrigins {
		return nil, fmt.Sprintf("At most %d domains.", config.MaxAllowedOrigins)
	}

	if len(origins) == 0 {
		return nil, ""
	}

	return origins, ""
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
	form := url.Values{"name": {project.Name}, "color": {project.Color}, "disk_share": {strconv.Itoa(project.Share())}}

	form["log_level"] = service.recording.RecordedLogLevels(project.LogLevels)
	form["allowed_origin"] = append([]string{}, project.AllowedOrigins...)

	return form
}
