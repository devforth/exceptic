package database

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
)

// What the cleanup measures is the data the database holds (liveSize): the
// inbox, the journal and deleted records not yet compacted do not count, and
// the files only start a run when compacting cannot bring them back.

// liveStore is a store on disk with 300 events in 30 issues and 3000 logs
// of one project, all in tables.
func liveStore(t *testing.T) *Store {
	t.Helper()

	return liveData(t, openOnDisk(t, t.TempDir()))
}

// liveData stores liveStore's data in database.
func liveData(t *testing.T, database *Store) *Store {
	t.Helper()

	seedShares(t, database, 100)
	clockAt(database, scenarioNow)

	random := rand.New(rand.NewPCG(3, 3))
	scenarioEvents(t, database, random, 1, "live", 30, 10, "error", scenarioNow.Add(-48*3600e9))
	scenarioLogs(t, database, random, 1, 2000, "info")
	scenarioLogs(t, database, random, 1, 1000, "error")
	intoTables(t, database)

	return database
}

func measuredUsage(t *testing.T, database *Store) (int64, diskUsage) {
	t.Helper()

	live, disk, err := database.liveSize()

	if err != nil {
		t.Fatal(err)
	}

	return live, disk
}

// envelopesOf queues envelopes of random bytes until the inbox holds about bytes.
func envelopesOf(t *testing.T, database *Store, bytes int64) {
	t.Helper()

	appendBacklog(t, database, int(bytes/100_000)+1)
	intoTables(t, database)
}

// Deleting issues lowers what the database holds at once; the files keep
// the deleted records until a compaction.
func TestLiveSizeDropsAtOnce(t *testing.T) {
	ctx := context.Background()
	database := liveStore(t)
	before, counted := usage(t, database), database.logicalSize()
	_, disk := measuredUsage(t, database)
	issues := database.issueIDs(func(*issueRecord) bool { return true })

	if _, err := database.DeleteIssues(ctx, issues[:len(issues)*2/3]); err != nil {
		t.Fatal(err)
	}

	after := usage(t, database)
	freed := disk.onDisk(counted - database.logicalSize())

	t.Logf("held %s → %s, files %s → %s", config.FormatSize(before.Live), config.FormatSize(after.Live), config.FormatSize(before.Files), config.FormatSize(after.Files))

	if dropped := before.Live - after.Live; math.Abs(percentOff(dropped, freed)) > 5 {
		t.Errorf("the database holds %d less after deleting %d on disk", dropped, freed)
	}

	if after.Files < before.Files {
		t.Errorf("the files shrank from %d to %d without a compaction", before.Files, after.Files)
	}

	if after.OnDisk[1] <= 0 || after.OnDisk[1] > after.Live {
		t.Errorf("the project holds %d of %d", after.OnDisk[1], after.Live)
	}
}

// A run starts when the data held reaches 99% of the limit, not at 98%.
func TestCleanupStartsAtTheStartPercent(t *testing.T) {
	ctx := context.Background()
	database := liveStore(t)
	live, _ := measuredUsage(t, database)

	if run, err := database.Cleanup(ctx, AllProjects, live*100/98); err != nil || run.Status != "" {
		t.Fatalf("at 98%%: %+v, %v", run, err)
	}

	at := live * 100 / 99
	run, err := database.Cleanup(ctx, AllProjects, at)

	if err != nil || run.Status != cleanupDone || run.SizeBefore != live || run.Files != 0 {
		t.Fatalf("at 99%%: %+v, %v", run, err)
	}

	if len(run.Steps) == 0 || float64(run.SizeAfter) > 0.82*float64(at) {
		t.Errorf("the run deleted %d steps, the database holds %s of %s after it", len(run.Steps), config.FormatSize(run.SizeAfter), config.FormatSize(at))
	}
}

// Envelopes waiting in the inbox and deleted records put the files over the
// limit, but under CleanupFilesPercent of it, with the data held under the
// start: no run.
func TestCleanupIgnoresWhatTheDataDoesNotHold(t *testing.T) {
	ctx := context.Background()
	database := liveStore(t)
	issues := database.issueIDs(func(*issueRecord) bool { return true })

	// Under config.StalePercent, so the check does not compact them first.
	if _, err := database.DeleteIssues(ctx, issues[:2]); err != nil {
		t.Fatal(err)
	}

	live, _ := measuredUsage(t, database)
	maxBytes := live * 100 / 90
	envelopesOf(t, database, maxBytes*15/100)
	live, disk := measuredUsage(t, database)

	if float64(live) >= 0.9*float64(maxBytes)*1.01 || disk.files <= maxBytes || disk.files*100 >= maxBytes*config.CleanupFilesPercent {
		t.Fatalf("held %d, files %d, limit %d: want the files between the limit and %d%% of it", live, disk.files, maxBytes, config.CleanupFilesPercent)
	}

	if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
		t.Errorf("runs: %+v", runs)
	}
}

// Files over CleanupFilesPercent of the limit that compacting brings back
// start no run: the size check compacts them, and a cleanup called then
// deletes nothing.
func TestFilesCompactedBackStartNoRun(t *testing.T) {
	ctx := context.Background()

	for name, check := range map[string]func(*Store, int64) []CleanupRun{
		"size check": func(database *Store, maxBytes int64) []CleanupRun { return checkSize(t, database, maxBytes) },
		"cleanup": func(database *Store, maxBytes int64) []CleanupRun {
			run, err := database.Cleanup(ctx, AllProjects, maxBytes)

			if err != nil {
				t.Fatal(err)
			}

			if run.Status != "" && (len(run.Steps) > 0 || run.Reached != 0 || run.Files != 0) {
				return []CleanupRun{run}
			}

			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			database := liveStore(t)
			_, disk := measuredUsage(t, database)
			issues := database.issueIDs(func(*issueRecord) bool { return true })

			if _, err := database.DeleteIssues(ctx, issues); err != nil {
				t.Fatal(err)
			}

			maxBytes := disk.files * 100 / 130
			live, disk := measuredUsage(t, database)

			if float64(live) >= 0.9*float64(maxBytes) || disk.files*100 <= maxBytes*config.CleanupFilesPercent {
				t.Fatalf("held %d, files %d, limit %d", live, disk.files, maxBytes)
			}

			if runs := check(database, maxBytes); len(runs) != 0 {
				t.Errorf("runs: %+v", runs)
			}

			if _, disk := measuredUsage(t, database); disk.files > maxBytes {
				t.Errorf("the files take %d after the check, the limit is %d", disk.files, maxBytes)
			}
		})
	}
}

// Files over CleanupFilesPercent of the limit that compacting cannot bring
// back (envelopes waiting in the inbox) start a run with the data held under
// the start: it deletes until the files are about back under the limit.
func TestFilesOverTheLimitStartARun(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	maxBytes := live * 100 / 70
	envelopesOf(t, database, maxBytes*65/100)
	live, disk := measuredUsage(t, database)

	if float64(live) >= 0.99*float64(maxBytes) || disk.files*100 <= maxBytes*config.CleanupFilesPercent {
		t.Fatalf("held %d, files %d, limit %d", live, disk.files, maxBytes)
	}

	runs := checkSize(t, database, maxBytes)

	if len(runs) != 1 || runs[0].Project != AllProjects || runs[0].Status != cleanupDone || runs[0].Files*100 <= maxBytes*config.CleanupFilesPercent || len(runs[0].Steps) == 0 {
		t.Fatalf("runs: %+v", runs)
	}

	if need := runs[0].Need(config.CleanupStopPercent); need < runs[0].Files-maxBytes {
		t.Errorf("the run needed %d, the files were %d over the limit", need, runs[0].Files-maxBytes)
	}

	// About: one ratio serves logs and events, which compress differently.
	if _, disk := measuredUsage(t, database); disk.files > maxBytes*105/100 {
		t.Errorf("the files take %s after the run, the limit is %s", config.FormatSize(disk.files), config.FormatSize(maxBytes))
	}

	if history, _ := database.CleanupHistory(context.Background(), 1); len(history) != 1 || history[0].Files != runs[0].Files {
		t.Errorf("history: %+v", history)
	}
}

// Envelopes already stored keep their bytes in the inbox's files until it is
// compacted, which the size check does only when ingest is quiet. When they
// put the files over CleanupFilesPercent of the limit, the inbox is compacted
// under ingest too: they start no run that deletes the data held.
func TestFilesOverTheLimitCompactTheInboxUnderIngest(t *testing.T) {
	defer func(was time.Duration) { quietFor = was }(quietFor)

	quietFor = time.Hour
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	maxBytes := live * 100 / 70
	envelopesOf(t, database, maxBytes*65/100)
	storedEnvelopes(t, database)
	live, disk := measuredUsage(t, database)

	if float64(live) >= 0.99*float64(maxBytes) || disk.files*100 <= maxBytes*config.CleanupFilesPercent {
		t.Fatalf("held %d, files %d, limit %d", live, disk.files, maxBytes)
	}

	for _, run := range checkSize(t, database, maxBytes) {
		if len(run.Steps) > 0 || run.Files != 0 {
			t.Errorf("run %+v", run)
		}
	}

	if _, disk := measuredUsage(t, database); disk.files > maxBytes {
		t.Errorf("the files take %s after the check, the limit is %s", config.FormatSize(disk.files), config.FormatSize(maxBytes))
	}
}

// storedEnvelopes deletes every inbox entry, as the writer does once it stored them.
func storedEnvelopes(t *testing.T, database *Store) {
	t.Helper()

	entries, err := database.Pending(0, math.MaxInt, nil)

	if err != nil {
		t.Fatal(err)
	}

	batch := kv.NewBatch(database.db.Engine)

	for _, entry := range entries {
		if err := database.inbox.Delete(batch, inboxRecord{InboxEntry: entry}); err != nil {
			t.Fatal(err)
		}

		database.inboxBytes.Add(-int64(entry.Size))
	}

	if err := database.db.Write(batch, false); err != nil {
		t.Fatal(err)
	}
}

// What a run freed, in bytes counted, at the ratio it started with is what
// the data held lost: the history counts from the data a run started with.
func TestRunFreedOnDiskMatchesTheDataHeld(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || len(run.Steps) == 0 || run.Ratio <= 0 {
		t.Fatalf("run %+v, %v", run, err)
	}

	// The deletes' tombstones in the journal raise the ratio a little in so small a store.
	left, known := run.Left()

	if off := percentOff(left, run.SizeAfter); !known || math.Abs(off) > 5 {
		t.Errorf("left %d from the start, measured %d after (%.1f%% off)", left, run.SizeAfter, off)
	}
}
