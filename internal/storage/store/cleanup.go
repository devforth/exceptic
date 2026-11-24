package store

import (
	"fmt"
	"strings"
	"time"
)

// The cleanup by size has CleanupSteps steps (docs/configuration.md); the
// first CleanupAlwaysSteps run in full, the rest only while needed.
const (
	CleanupSteps       = 12
	CleanupAlwaysSteps = 4
	// CleanupRunsKept is how many of the latest runs the store keeps.
	CleanupRunsKept = 100
)

// Statuses of a cleanup run.
const (
	CleanupRunning = "running"
	CleanupDone    = "done"
	CleanupFailed  = "failed"
)

// Statuses of the compaction after a run.
const (
	CompactionPending = "pending"
	CompactionDone    = "done"
	// CompactionSkipped: nothing it deleted from was left to compact, or a project's run stayed under config.StalePercent of the data.
	CompactionSkipped = "skipped"
	CompactionFailed  = "failed"
	// CompactionAttempts: a failed compaction stays pending for the next passes of its job, failed after this many.
	CompactionAttempts = 3
)

// CleanupCompaction is the compaction of what a run deleted from, a pass of
// its own on the database lane after the run: the files keep the deleted
// records until it is done.
type CleanupCompaction struct {
	Status      string        `json:"status"`
	StartedAt   time.Time     `json:"started_at,omitzero"`
	Duration    time.Duration `json:"duration,omitempty"`
	FilesBefore int64         `json:"files_before,omitempty"`
	FilesAfter  int64         `json:"files_after,omitempty"`
	// Freed: the deleted records it compacted away, on disk, or what the
	// files lost when that is more; data arriving meanwhile may keep the
	// files from shrinking. 0 in compactions stored before it.
	Freed int64 `json:"freed,omitempty"`
	// Attempts made; Error the last one's while it is retried or failed.
	Attempts int    `json:"attempts,omitempty"`
	Error    string `json:"error,omitempty"`
	// Running: an attempt started and has not recorded its end; still set
	// at the next pass, it was interrupted.
	Running bool `json:"running,omitempty"`
}

// CleanupRun is one cleanup by size: what it aims at, where it is and what
// it did. The steps are those of docs/configuration.md (Retention).
type CleanupRun struct {
	Sequence uint64 `json:"seq"`
	// Project is the one project a run cleans down to its disk share; 0: all of them.
	Project int `json:"project,omitempty"`
	// Share is that project's disk share when the run started, in percent.
	Share     int           `json:"share,omitempty"`
	Status    string        `json:"status"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
	Limit     int64         `json:"limit"`
	// The sizes of the data its scope holds, as the cleanup measures it.
	SizeBefore int64 `json:"size_before"`
	Compacted  int64 `json:"compacted"` // once compacted, before deleting (only a run the files started compacts first)
	SizeAfter  int64 `json:"size_after"`
	// Files: what the files took after compacting, when they started the run
	// (over CleanupFilesPercent of the limit with the data under its start); 0 otherwise.
	Files int64 `json:"files,omitempty"`
	Goal  int64 `json:"goal"` // of the byte counters
	// Ratio: bytes on disk per byte counted when deleting started; 0 in runs stored before it.
	Ratio    float64       `json:"ratio,omitempty"`
	Reached  int           `json:"reached"`            // the step it is at; once done, the last one that ran
	Order    string        `json:"order,omitempty"`    // the steps it started with; under others a resumed run ends
	Position []byte        `json:"position,omitempty"` // where that step goes on
	Steps    []CleanupStep `json:"steps"`
	Error    string        `json:"error,omitempty"`
	// Compaction: of a run that deleted something; nil otherwise and in runs
	// stored before it, which compacted within the run.
	Compaction *CleanupCompaction `json:"compaction,omitempty"`
}

// Need is what the run had to free once compacted, in bytes on disk: the
// data down to stopPercent of the limit and, for a run the files started,
// the files back under the limit.
func (run CleanupRun) Need(stopPercent int) int64 {
	need := run.Compacted - run.Limit*int64(stopPercent)/100

	if run.Files > 0 {
		need = max(need, run.Files-run.Limit)
	}

	return max(need, 0)
}

// Left is the data the run started with (once compacted) less what it
// freed, both on disk; known is false for a run stored before Ratio. Data
// arriving meanwhile is not in it, so it shows what the run did.
func (run CleanupRun) Left() (left int64, known bool) {
	var freed int64

	for _, step := range run.Steps {
		freed += step.Freed
	}

	return max(run.Compacted-int64(float64(freed)*run.Ratio), 0), run.Ratio > 0
}

// CleanupStep is a step that deleted something: bytes freed (before
// compression) and what went, per project.
type CleanupStep struct {
	Number   int                    `json:"number"` // as in docs/configuration.md
	Name     string                 `json:"name"`
	Freed    int64                  `json:"freed"`
	Duration time.Duration          `json:"duration"`
	Projects map[int]*CleanupCounts `json:"projects"`
}

// CleanupCounts is what a step did to one project.
type CleanupCounts struct {
	Logs    int `json:"logs,omitempty"`
	Events  int `json:"events,omitempty"`
	Thinned int `json:"thinned,omitempty"`
	Issues  int `json:"issues,omitempty"`
	// StatusChanges of missing issues; one with no events left counts as project 0.
	StatusChanges int `json:"status_changes,omitempty"`
}

func (counts *CleanupCounts) Add(other CleanupCounts) {
	counts.Logs += other.Logs
	counts.Events += other.Events
	counts.Thinned += other.Thinned
	counts.Issues += other.Issues
	counts.StatusChanges += other.StatusChanges
}

// String reads "120 logs, 3 events, 5 events thinned".
func (counts CleanupCounts) String() string {
	var parts []string

	for _, part := range []struct {
		count int
		name  string
	}{{counts.Logs, "logs"}, {counts.Events, "events"}, {counts.Thinned, "events thinned"}, {counts.Issues, "issues"}, {counts.StatusChanges, "status changes"}} {
		if part.count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", part.count, part.name))
		}
	}

	return strings.Join(parts, ", ")
}
