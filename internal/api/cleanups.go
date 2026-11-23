package api

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// cleanupsOutput is the history of the cleanup by size, with the texts and
// sizes as the server writes them.
type cleanupsOutput struct {
	// StopPercent: deleting stops at this percent of the limit.
	StopPercent int `json:"stop_percent"`
	// Kept: how many runs the server keeps.
	Kept int          `json:"kept"`
	Runs []cleanupRun `json:"runs"`
}

type cleanupRun struct {
	ID uint64 `json:"id"`
	// Project: the one project the run cleaned down to its disk share; missing for a run over all projects.
	Project   *cleanupProject `json:"project,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	// Status is running, done or failed.
	Status     string `json:"status"`
	Limit      string `json:"limit"`
	SizeBefore string `json:"size_before"`
	Compacted  string `json:"compacted"`
	SizeAfter  string `json:"size_after"`
	// Stop is where deleting stops; Needed what was to free after compacting.
	Stop   string `json:"stop"`
	Needed string `json:"needed"`
	// DeletedSteps are the steps that deleted something: "1, 4–7".
	DeletedSteps string        `json:"deleted_steps"`
	Result       string        `json:"result"`
	Took         string        `json:"took"`
	Error        string        `json:"error,omitempty"`
	Steps        []cleanupStep `json:"steps"`
}

type cleanupProject struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// cleanupStep is a step that deleted something, numbered as in docs/configuration.md.
type cleanupStep struct {
	Number   int              `json:"number"`
	Name     string           `json:"name"`
	Freed    string           `json:"freed"`
	Took     string           `json:"took"`
	Projects []cleanupDeleted `json:"projects"`
}

// cleanupDeleted is what a step deleted in one project: "120 logs, 3 events".
type cleanupDeleted struct {
	Project string `json:"project"`
	Deleted string `json:"deleted"`
}

func (handler *handlers) listCleanups(ctx context.Context, call Call, _ noInput) (cleanupsOutput, error) {
	runs, err := handler.services.Projects.Cleanups(ctx, call.Access())

	return cleanups(runs, handler.services.Projects.Name), err
}

// cleanups shows the runs; projectName names a project by id, a deleted one too.
func cleanups(runs []store.CleanupRun, projectName func(id int) string) cleanupsOutput {
	output := cleanupsOutput{StopPercent: config.CleanupStopPercent, Kept: store.CleanupRunsKept, Runs: []cleanupRun{}}

	for _, run := range runs {
		stop := run.Limit * config.CleanupStopPercent / 100
		view := cleanupRun{
			ID: run.Sequence, StartedAt: run.StartedAt, Status: run.Status, Error: run.Error,
			Limit: config.FormatSize(run.Limit), SizeBefore: config.FormatSize(run.SizeBefore),
			Compacted: config.FormatSize(run.Compacted), SizeAfter: config.FormatSize(run.SizeAfter),
			Stop: config.FormatSize(stop), Needed: config.FormatSize(max(run.Compacted-stop, 0)),
			DeletedSteps: stepNumbers(run.Steps), Result: cleanupResult(run), Took: tookText(run.Duration),
			Steps: []cleanupStep{},
		}

		if run.Project != 0 {
			view.Project = &cleanupProject{ID: run.Project, Name: projectName(run.Project)}
		}

		for _, step := range run.Steps {
			stepView := cleanupStep{Number: step.Number, Name: step.Name, Freed: config.FormatSize(step.Freed), Took: tookText(step.Duration), Projects: []cleanupDeleted{}}

			for _, id := range slices.Sorted(maps.Keys(step.Projects)) {
				stepView.Projects = append(stepView.Projects, cleanupDeleted{Project: projectName(id), Deleted: step.Projects[id].String()})
			}

			view.Steps = append(view.Steps, stepView)
		}

		output.Runs = append(output.Runs, view)
	}

	return output
}

// tookText rounds to milliseconds under 10 s, to seconds above.
func tookText(took time.Duration) string {
	if took < 10*time.Second {
		return took.Round(time.Millisecond).String()
	}

	return took.Round(time.Second).String()
}

// cleanupResult says why a run stopped.
func cleanupResult(run store.CleanupRun) string {
	switch {
	case run.Status == store.CleanupRunning:
		return fmt.Sprintf("running, at step %d", run.Reached)
	case run.Status == store.CleanupFailed:
		return fmt.Sprintf("failed in step %d", run.Reached)
	case run.Reached == 0:
		return "compacting was enough, nothing deleted"
	case run.Reached <= store.CleanupAlwaysSteps:
		return fmt.Sprintf("steps 1–%d always run; after them %s was under %d%%", store.CleanupAlwaysSteps, cleaned(run), config.CleanupStopPercent)
	case run.Reached < store.CleanupSteps || run.Position != nil:
		return fmt.Sprintf("stopped in step %d: %s was at %d%%", run.Reached, cleaned(run), config.CleanupStopPercent)
	default:
		return "every step ran"
	}
}

// cleaned names what a run measured against its limit.
func cleaned(run store.CleanupRun) string {
	if run.Project != 0 {
		return "the project"
	}

	return "the database"
}

// stepNumbers reads the steps that deleted something as "1, 4–7".
func stepNumbers(steps []store.CleanupStep) string {
	var parts []string

	for index := 0; index < len(steps); {
		end := index

		for end+1 < len(steps) && steps[end+1].Number == steps[end].Number+1 {
			end++
		}

		if end > index {
			parts = append(parts, fmt.Sprintf("%d–%d", steps[index].Number, steps[end].Number))
		} else {
			parts = append(parts, strconv.Itoa(steps[index].Number))
		}

		index = end + 1
	}

	return strings.Join(parts, ", ")
}
