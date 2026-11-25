package api

import (
	"cmp"
	"fmt"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// shownToAll are runs as a viewer of every project sees them.
func shownToAll(runs ...store.CleanupRun) []services.ShownRun {
	shown := make([]services.ShownRun, len(runs))

	for index, run := range runs {
		shown[index], _ = services.VisibleRun(store.Access{}, run)
	}

	return shown
}

func TestCleanupResult(t *testing.T) {
	stop := config.CleanupStopPercent
	// The data it started with: 990 bytes on disk, 2 per byte counted; 190 to free.
	started := func(run store.CleanupRun) store.CleanupRun {
		run.Status, run.Limit, run.SizeBefore, run.Compacted, run.Ratio = cmp.Or(run.Status, store.CleanupDone), 1000, 990, 990, 2

		return run
	}
	freed := func(counted int64) []store.CleanupStep { return []store.CleanupStep{{Number: 1, Freed: counted}} }

	for _, test := range []struct {
		name string
		run  store.CleanupRun
		want string
	}{
		// The last step began and stopped once enough was deleted: it did not run out.
		{"stopped in the last step", started(store.CleanupRun{Reached: store.CleanupSteps, Position: []byte{1}, Steps: freed(100)}),
			fmt.Sprintf("stopped in step %d: freed what was needed to bring the data it started with to %d%%", store.CleanupSteps, stop)},
		{"a project's run", started(store.CleanupRun{Project: 3, Reached: 6, Steps: freed(100)}),
			fmt.Sprintf("stopped in step 6: freed what was needed to bring the project's data it started with to %d%%", stop)},
		{"the steps that always run", started(store.CleanupRun{Reached: store.CleanupAlwaysSteps, Steps: freed(100)}),
			fmt.Sprintf("steps 1–%d always run; after them the data it started with was under %d%%", store.CleanupAlwaysSteps, stop)},
		{"the files started it", started(store.CleanupRun{Reached: 6, Files: 1300, Steps: freed(200)}),
			fmt.Sprintf("stopped in step 6: freed what was needed to bring the data it started with to %d%% and the files (%s after compacting, over %d%% of the limit) under the limit",
				stop, config.FormatSize(1300), config.CleanupFilesPercent)},
		// Other deletes meanwhile (a project purged, issues deleted) brought the data down: the run itself freed less.
		{"stopped once others deleted", started(store.CleanupRun{Reached: 6, Steps: freed(10)}),
			fmt.Sprintf("stopped in step 6: other deletes meanwhile freed the rest of what was needed to bring the data it started with to %d%%", stop)},
		{"the steps that always run, others deleted", started(store.CleanupRun{Reached: store.CleanupAlwaysSteps, Steps: freed(10)}),
			fmt.Sprintf("steps 1–%d always run; other deletes meanwhile freed the rest of what was needed to bring the data it started with to %d%%", store.CleanupAlwaysSteps, stop)},
		{"every step ran and freed enough", started(store.CleanupRun{Reached: store.CleanupSteps, Steps: freed(100)}), "every step ran"},
		{"every step ran short of the goal", started(store.CleanupRun{Reached: store.CleanupSteps, Steps: freed(10)}),
			fmt.Sprintf("every step ran, still above %d%%", stop)},
		{"every step ran short of the files' goal", started(store.CleanupRun{Reached: store.CleanupSteps, Files: 1300, Steps: freed(100)}),
			"every step ran, still short of what was needed"},
		{"stored before the ratio", store.CleanupRun{Status: store.CleanupDone, Reached: store.CleanupSteps}, "every step ran"},
		{"compacting was enough", store.CleanupRun{Status: store.CleanupDone}, "compacting was enough, nothing deleted"},
		{"running", store.CleanupRun{Status: store.CleanupRunning, Reached: 3}, "running, at step 3"},
		{"failed", started(store.CleanupRun{Status: store.CleanupFailed, Reached: 5, Error: "the limit changed"}), "failed in step 5"},
	} {
		if got := cleanupResult(shownToAll(test.run)[0]); got != test.want {
			t.Errorf("%s: %q, want %q", test.name, got, test.want)
		}
	}
}

// Before and after are of the data the run started with, so data arriving
// meanwhile never makes it look as if it freed less; that data is shown apart
// once it is sizable.
func TestCleanupSizesFromTheStart(t *testing.T) {
	const megabyte = 1000 * 1000

	run := func(sizeNow int64) store.CleanupRun {
		// 2 GB at the start, 400 MB freed on disk (800 MB counted at 0.5 per byte), the size measured at the end.
		return store.CleanupRun{
			Status: store.CleanupDone, Limit: 2020 * megabyte, SizeBefore: 2000 * megabyte, Compacted: 2000 * megabyte, Ratio: 0.5,
			SizeAfter: sizeNow, Reached: 6, Steps: []store.CleanupStep{{Number: 5, Freed: 300 * megabyte}, {Number: 6, Freed: 500 * megabyte}},
		}
	}
	output := cleanups(shownToAll(run(1700*megabyte), run(1605*megabyte), store.CleanupRun{Status: store.CleanupDone, SizeBefore: 900, SizeAfter: 700, Reached: 6}), func(id int) string { return fmt.Sprint(id) })
	flood, quiet, before := output.Runs[0], output.Runs[1], output.Runs[2]

	if flood.SizeBefore != config.FormatSize(2000*megabyte) || flood.SizeAfter != config.FormatSize(1600*megabyte) || flood.Freed != config.FormatSize(400*megabyte) {
		t.Errorf("during a flood: %s → %s (−%s)", flood.SizeBefore, flood.SizeAfter, flood.Freed)
	}

	if flood.Arrived != config.FormatSize(100*megabyte) || flood.SizeNow != config.FormatSize(1700*megabyte) {
		t.Errorf("arrived meanwhile %q, now %q", flood.Arrived, flood.SizeNow)
	}

	if quiet.SizeAfter != config.FormatSize(1600*megabyte) || quiet.Arrived != "" || quiet.SizeNow != "" {
		t.Errorf("a few MB arrived: %+v", quiet)
	}

	// Stored before the ratio: its sizes are the files', and one left running across the upgrade ends with the data held, so after is left out.
	if !before.BeforeFiles || before.SizeBefore != config.FormatSize(900) || before.SizeAfter != "" || before.Freed != "" || before.Arrived != "" {
		t.Errorf("stored before: %+v", before)
	}

	if flood.BeforeFiles {
		t.Error("a run with its ratio reads as the data held")
	}
}

// Steps read as ranges, durations are rounded, and a deleted project is named by its ID.
func TestCleanupsOutput(t *testing.T) {
	run := store.CleanupRun{
		Sequence: 7, Status: store.CleanupDone, Limit: 1000, Compacted: 900, Reached: 5, Duration: 12400 * time.Millisecond,
		Steps: []store.CleanupStep{
			{Number: 1, Duration: 1234567 * time.Microsecond}, {Number: 3}, {Number: 4},
			{Number: 5, Projects: map[int]*store.CleanupCounts{2: {Logs: 1}, 1: {Events: 2}}},
		},
	}
	output := cleanups(shownToAll(run), func(id int) string { return fmt.Sprintf("p%d", id) })
	got := output.Runs[0]

	if got.DeletedSteps != "1, 3–5" || got.Took != "12s" || got.Steps[0].Took != "1.235s" || got.Stop != config.FormatSize(800) || got.Needed != config.FormatSize(100) {
		t.Errorf("%+v", got)
	}

	if projects := got.Steps[3].Projects; len(projects) != 2 || projects[0].Project != "p1" || projects[1].Deleted != "1 logs" {
		t.Errorf("projects %+v", projects)
	}
}

// The compaction after a run reads as its state, and once done what the files lost and how long it took.
func TestCleanupsCompactionOutput(t *testing.T) {
	runs := []store.CleanupRun{
		{Sequence: 3, Status: store.CleanupDone, Compaction: &store.CleanupCompaction{Status: store.CompactionDone, FilesBefore: 3000, FilesAfter: 1000, Duration: 18300 * time.Millisecond}},
		// Envelopes arriving meanwhile grew the files; what it compacted away is still shown.
		{Sequence: 4, Status: store.CleanupDone, Compaction: &store.CleanupCompaction{Status: store.CompactionDone, FilesBefore: 3000, FilesAfter: 3500, Freed: 800}},
		{Sequence: 2, Status: store.CleanupDone, Compaction: &store.CleanupCompaction{Status: store.CompactionPending}},
		{Sequence: 1, Status: store.CleanupDone},
	}
	output := cleanups(shownToAll(runs...), func(id int) string { return fmt.Sprint(id) })
	done, pending, none := output.Runs[0].Compaction, output.Runs[2].Compaction, output.Runs[3].Compaction

	if done == nil || done.Status != store.CompactionDone || done.Freed != config.FormatSize(2000) || done.FilesBefore != config.FormatSize(3000) ||
		done.FilesAfter != config.FormatSize(1000) || done.Took != "18s" {
		t.Errorf("done: %+v", done)
	}

	if grew := output.Runs[1].Compaction; grew.Freed != config.FormatSize(800) || !grew.FilesGrew {
		t.Errorf("under ingest: %+v", grew)
	}

	if done.FilesGrew {
		t.Error("the files shrank")
	}

	if pending == nil || pending.Status != store.CompactionPending || pending.Freed != "" || pending.Took != "" {
		t.Errorf("pending: %+v", pending)
	}

	if none != nil {
		t.Errorf("a run without one: %+v", none)
	}
}
