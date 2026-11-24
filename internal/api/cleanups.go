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
	// A run starts at StartPercent of the limit, or when the files stay over
	// FilesPercent of it after compacting; deleting stops at StopPercent.
	StartPercent int `json:"start_percent"`
	StopPercent  int `json:"stop_percent"`
	FilesPercent int `json:"files_percent"`
	// StalePercent: a project's run compacts afterwards only once deleted data is over it of the data held.
	StalePercent int `json:"stale_percent"`
	// CompactionAttempts: how often a failed compaction is tried in all.
	CompactionAttempts int `json:"compaction_attempts"`
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
	// SizeAfter is the data the run started with less what it freed, Freed
	// the difference, so data arriving meanwhile does not hide what it did.
	SizeAfter string `json:"size_after"`
	Freed     string `json:"freed"`
	// BeforeFiles: a run stored before the data held was measured apart;
	// its sizes are the files', and after and freed are left out (empty).
	BeforeFiles bool `json:"before_files,omitempty"`
	// Arrived is the data that came meanwhile and SizeNow the data held at
	// the end, both only when that is over arrivedPercent of the limit.
	Arrived string `json:"arrived,omitempty"`
	SizeNow string `json:"size_now,omitempty"`
	// Files: what the files took after compacting when they started the run; missing otherwise.
	Files string `json:"files,omitempty"`
	// Stop is where deleting stops; Needed what was to free after compacting.
	Stop   string `json:"stop"`
	Needed string `json:"needed"`
	// DeletedSteps are the steps that deleted something: "1, 4–7".
	DeletedSteps string        `json:"deleted_steps"`
	Result       string        `json:"result"`
	Took         string        `json:"took"`
	Error        string        `json:"error,omitempty"`
	Steps        []cleanupStep `json:"steps"`
	// Compaction: of what the run deleted from, a pass of its own after it;
	// missing for a run that deleted nothing or one that compacted within it.
	Compaction *cleanupCompaction `json:"compaction,omitempty"`
}

// cleanupCompaction is the compaction after a run; the sizes and Took once it ran.
type cleanupCompaction struct {
	// Status is pending (retried with Error after a failure), done, skipped (nothing left to compact, or a project's deletes under the stale share) or failed.
	Status      string `json:"status"`
	Attempts    int    `json:"attempts,omitempty"`
	FilesBefore string `json:"files_before,omitempty"`
	FilesAfter  string `json:"files_after,omitempty"`
	// Freed is what it compacted away, or what the files lost when that is more.
	Freed string `json:"freed,omitempty"`
	// FilesGrew: data arriving meanwhile left the files no smaller than before.
	FilesGrew bool   `json:"files_grew,omitempty"`
	Took      string `json:"took,omitempty"`
	Error     string `json:"error,omitempty"`
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
	output := cleanupsOutput{
		StartPercent: config.CleanupStartPercent, StopPercent: config.CleanupStopPercent, FilesPercent: config.CleanupFilesPercent,
		StalePercent: config.StalePercent, CompactionAttempts: store.CompactionAttempts, Kept: store.CleanupRunsKept, Runs: []cleanupRun{},
	}

	for _, run := range runs {
		stop := run.Limit * config.CleanupStopPercent / 100
		view := cleanupRun{
			ID: run.Sequence, StartedAt: run.StartedAt, Status: run.Status, Error: run.Error,
			Limit: config.FormatSize(run.Limit), SizeBefore: config.FormatSize(run.SizeBefore),
			Compacted: config.FormatSize(run.Compacted),
			Stop:      config.FormatSize(stop), Needed: config.FormatSize(run.Need(config.CleanupStopPercent)),
			DeletedSteps: stepNumbers(run.Steps), Result: cleanupResult(run), Took: tookText(run.Duration),
			Steps: []cleanupStep{},
		}

		view.SizeAfter, view.Freed, view.Arrived, view.SizeNow = cleanupSizes(run)
		view.BeforeFiles = run.Ratio == 0 && run.Reached > 0

		if run.Files > 0 {
			view.Files = config.FormatSize(run.Files)
		}

		if compaction := run.Compaction; compaction != nil {
			view.Compaction = &cleanupCompaction{Status: compaction.Status, Attempts: compaction.Attempts, Error: compaction.Error}

			if !compaction.StartedAt.IsZero() || compaction.FilesBefore > 0 {
				view.Compaction.FilesBefore, view.Compaction.FilesAfter = config.FormatSize(compaction.FilesBefore), config.FormatSize(compaction.FilesAfter)
				view.Compaction.Freed = config.FormatSize(max(compaction.Freed, compaction.FilesBefore-compaction.FilesAfter, 0))
				view.Compaction.FilesGrew = compaction.FilesAfter >= compaction.FilesBefore
				view.Compaction.Took = tookText(compaction.Duration)
			}
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

// arrivedPercent of the limit: data arriving during a run is shown from it on.
const arrivedPercent = 1

// cleanupSizes reads a run from the data it started with: after, what it
// freed, and the data that arrived meanwhile with the data held at the end
// once sizable. A run stored before Ratio measured the files, so it has none.
func cleanupSizes(run store.CleanupRun) (after, freed, arrived, now string) {
	left, known := run.Left()

	if !known {
		if run.Reached == 0 {
			return config.FormatSize(run.SizeAfter), config.FormatSize(max(run.SizeBefore-run.SizeAfter, 0)), "", ""
		}

		return "", "", "", ""
	}

	after, freed = config.FormatSize(left), config.FormatSize(max(run.SizeBefore-left, 0))

	if extra := run.SizeAfter - left; run.SizeAfter > 0 && extra*100 >= run.Limit*arrivedPercent {
		arrived, now = config.FormatSize(extra), config.FormatSize(run.SizeAfter)
	}

	return after, freed, arrived, now
}

// tookText rounds to milliseconds under 10 s, to seconds above.
func tookText(took time.Duration) string {
	if took < 10*time.Second {
		return took.Round(time.Millisecond).String()
	}

	return took.Round(time.Second).String()
}

// cleanupResult says why a run stopped, of the data it started with: data
// arriving meanwhile is no part of what it set out to free.
func cleanupResult(run store.CleanupRun) string {
	stop := config.CleanupStopPercent
	data := "the data it started with"

	if run.Project != 0 {
		data = "the project's data it started with"
	}

	goal := fmt.Sprintf("to bring %s to %d%%", data, stop)

	if run.Files > 0 {
		goal += fmt.Sprintf(" and the files (%s after compacting, over %d%% of the limit) under the limit", config.FormatSize(run.Files), config.CleanupFilesPercent)
	}

	switch short := shortOf(run); {
	case run.Status == store.CleanupRunning:
		return fmt.Sprintf("running, at step %d", run.Reached)
	case run.Status == store.CleanupFailed:
		return fmt.Sprintf("failed in step %d", run.Reached)
	case run.Reached == 0:
		return "compacting was enough, nothing deleted"
	case short && run.Reached <= store.CleanupAlwaysSteps:
		return fmt.Sprintf("steps 1–%d always run; %s %s", store.CleanupAlwaysSteps, othersFreed, goal)
	case short && (run.Reached < store.CleanupSteps || run.Position != nil):
		return fmt.Sprintf("stopped in step %d: %s %s", run.Reached, othersFreed, goal)
	case run.Reached <= store.CleanupAlwaysSteps && run.Files > 0:
		return fmt.Sprintf("steps 1–%d always run; they freed what was needed %s", store.CleanupAlwaysSteps, goal)
	case run.Reached <= store.CleanupAlwaysSteps:
		return fmt.Sprintf("steps 1–%d always run; after them %s was under %d%%", store.CleanupAlwaysSteps, data, stop)
	case run.Reached < store.CleanupSteps || run.Position != nil:
		return fmt.Sprintf("stopped in step %d: freed what was needed %s", run.Reached, goal)
	case short && run.Files > 0:
		return "every step ran, still short of what was needed"
	case short:
		return fmt.Sprintf("every step ran, still above %d%%", stop)
	default:
		return "every step ran"
	}
}

// othersFreed: deletes beside a run brought the data down before it freed what it needed.
const othersFreed = "other deletes meanwhile freed the rest of what was needed"

// shortOf says the run freed less than it needed: it ran out of steps, or
// deletes beside it (a purged project, deleted issues or logs) brought the
// data down and it stopped early. The slack covers the goal's rounding.
func shortOf(run store.CleanupRun) bool {
	left, known := run.Left()
	need := run.Need(config.CleanupStopPercent)

	return known && run.Compacted-left < need-need/1000-int64(run.Ratio)-2
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
