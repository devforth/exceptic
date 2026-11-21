package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

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
		{"compacting was enough", store.CleanupRun{Status: store.CleanupDone}, "compacting was enough, nothing deleted"},
		{"running", store.CleanupRun{Status: store.CleanupRunning, Reached: 3}, "running, at step 3"},
	} {
		if got := cleanupResult(test.run); got != test.want {
			t.Errorf("%s: %q, want %q", test.name, got, test.want)
		}
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
	output := cleanups([]store.CleanupRun{run}, func(id int) string { return fmt.Sprintf("p%d", id) })
	got := output.Runs[0]

	if got.DeletedSteps != "1, 3–5" || got.Took != "12s" || got.Steps[0].Took != "1.235s" || got.Stop != config.FormatSize(800) || got.Needed != config.FormatSize(100) {
		t.Errorf("%+v", got)
	}

	if projects := got.Steps[3].Projects; len(projects) != 2 || projects[0].Project != "p1" || projects[1].Deleted != "1 logs" {
		t.Errorf("projects %+v", projects)
	}
}
