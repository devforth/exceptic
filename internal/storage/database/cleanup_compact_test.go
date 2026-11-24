package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// A run ends once it deleted enough; compacting what it deleted from is a
// pass of its own on the database lane (CompactCleanups), recorded on the run.

// compactCleanups runs the compaction pass of the database lane once.
func compactCleanups(t *testing.T, database *Store) {
	t.Helper()

	if err := database.CompactCleanups(context.Background(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
}

func filesSize(t *testing.T, database *Store) int64 {
	t.Helper()

	_, disk := measuredUsage(t, database)

	return disk.files
}

// deletedIssues deletes 2 of 30 issues: stale records under config.StalePercent.
func deletedIssues(t *testing.T, database *Store) {
	t.Helper()

	issues := database.issueIDs(func(*issueRecord) bool { return true })

	if _, err := database.DeleteIssues(context.Background(), issues[:2]); err != nil {
		t.Fatal(err)
	}
}

// A run over all projects ends without compacting: the data held drops at
// once, the files keep what it deleted, and the run waits for its
// compaction, which then shrinks the files and is recorded on the run.
func TestCleanupRunEndsBeforeItsCompaction(t *testing.T) {
	ctx := context.Background()
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	filesBefore := filesSize(t, database)

	select {
	case <-database.compactionWake:
		t.Fatal("woken before any run")
	default:
	}

	run, err := database.Cleanup(ctx, AllProjects, live*100/99)

	if err != nil || run.Status != cleanupDone || len(run.Steps) == 0 || run.SizeAfter >= run.SizeBefore {
		t.Fatalf("run %+v, %v", run, err)
	}

	if run.Compaction == nil || run.Compaction.Status != store.CompactionPending {
		t.Fatalf("the run's compaction: %+v", run.Compaction)
	}

	if files := filesSize(t, database); files < filesBefore || database.staleBytes() == 0 {
		t.Fatalf("the run compacted: files %d → %d, stale %d", filesBefore, files, database.staleBytes())
	}

	select {
	case <-database.compactionWake:
	default:
		t.Fatal("the compaction pass was not woken")
	}

	compactCleanups(t, database)

	stored := storedRun(t, database, run.Sequence)
	compaction := stored.Compaction

	if compaction == nil || compaction.Status != store.CompactionDone || compaction.FilesAfter >= compaction.FilesBefore || compaction.StartedAt.IsZero() {
		t.Fatalf("the compaction: %+v", compaction)
	}

	// The run saved since: a few bytes in the journal.
	if files := filesSize(t, database); files > compaction.FilesAfter+4096 || database.staleBytes() != 0 {
		t.Errorf("files %d, recorded %d; stale %d", files, compaction.FilesAfter, database.staleBytes())
	}

	if stored.Status != run.Status || stored.SizeAfter != run.SizeAfter || len(stored.Steps) != len(run.Steps) {
		t.Errorf("the run changed: %+v, was %+v", stored, run)
	}

	// Done once: the next pass leaves it.
	compactCleanups(t, database)

	if again := storedRun(t, database, run.Sequence).Compaction; *again != *compaction {
		t.Errorf("compacted again: %+v, was %+v", again, compaction)
	}
}

// A restart between a run and its compaction keeps the compaction due: the
// pass at the next start does it.
func TestCleanupCompactionAfterARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	seedShares(t, database, 100)
	clockAt(database, scenarioNow)

	random := rand.New(rand.NewPCG(3, 3))
	scenarioEvents(t, database, random, 1, "live", 30, 10, "error", scenarioNow.Add(-48*3600e9))
	scenarioLogs(t, database, random, 1, 3000, "info")
	intoTables(t, database)

	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(ctx, AllProjects, live*100/99)

	if err != nil || len(run.Steps) == 0 || run.Compaction == nil || run.Compaction.Status != store.CompactionPending {
		t.Fatalf("run %+v, %v", run, err)
	}

	database = reopen(t, database, dir)
	filesBefore := filesSize(t, database)
	compactCleanups(t, database)

	compaction := storedRun(t, database, run.Sequence).Compaction

	if compaction == nil || compaction.Status != store.CompactionDone || filesSize(t, database) >= filesBefore {
		t.Fatalf("after the restart: %+v, files %d → %d", compaction, filesBefore, filesSize(t, database))
	}
}

// While a backup runs, the compaction waits (its snapshot keeps the deleted
// records in the files); it comes once the backup is over.
func TestCleanupCompactionWaitsForABackup(t *testing.T) {
	ctx := context.Background()
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(ctx, AllProjects, live*100/99)

	if err != nil || run.Compaction == nil {
		t.Fatalf("run %+v, %v", run, err)
	}

	database.backups.Add(1)
	compactCleanups(t, database)
	database.backups.Add(-1)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionPending {
		t.Fatalf("compacted during a backup: %+v", compaction)
	}

	compactCleanups(t, database)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionDone {
		t.Fatalf("after the backup: %+v", compaction)
	}
}

// A project's run compacts afterwards only once the stale records are over
// config.StalePercent of the data (compactStale): a small one is recorded
// as not needed.
func TestDiskShareCompactionFollowsStalePercent(t *testing.T) {
	database := scenarioFlood(t, t.TempDir(), 20, 5, 3000)
	maxBytes := limitFor(measured(t, database, 2), 20, 1.2)
	runs := checkSizeOnly(t, database, maxBytes)

	if len(runs) != 1 || runs[0].Project != 2 || runs[0].Compaction == nil || runs[0].Compaction.Status != store.CompactionPending {
		t.Fatalf("runs %+v", runs)
	}

	stale := database.staleBytes()
	compactCleanups(t, database)
	compaction := storedRun(t, database, runs[0].Sequence).Compaction

	switch over := stale*100 > config.StalePercent*database.logicalSize(); {
	case over && (compaction.Status != store.CompactionDone || database.staleBytes() != 0):
		t.Errorf("stale %d over %d%%: %+v, stale after %d", stale, config.StalePercent, compaction, database.staleBytes())
	case !over && (compaction.Status != store.CompactionSkipped || database.staleBytes() != stale):
		t.Errorf("stale %d under %d%%: %+v, stale after %d", stale, config.StalePercent, compaction, database.staleBytes())
	}
}

// A size check while a run waits for its compaction leaves the run's
// deleted records to it, however much they are: the compaction then frees
// them and records it on the run.
func TestSizeCheckLeavesTheRunsCompaction(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil || run.Compaction.Status != store.CompactionPending {
		t.Fatalf("run %+v, %v", run, err)
	}

	stale := database.staleBytes()

	if stale*100 <= config.StalePercent*database.logicalSize() {
		t.Fatalf("stale %d under %d%% of %d: compactStale would leave it anyway", stale, config.StalePercent, database.logicalSize())
	}

	if runs := checkSizeOnly(t, database, live*10); len(runs) != 0 || database.staleBytes() != stale {
		t.Fatalf("the size check: runs %+v, stale %d → %d", runs, stale, database.staleBytes())
	}

	compactCleanups(t, database)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionDone || compaction.FilesAfter >= compaction.FilesBefore {
		t.Errorf("the compaction: %+v", compaction)
	}
}

// A project's run and then the run over all projects: the compaction over
// all projects comes first and frees both runs' deleted records, so the
// project's has nothing left to do.
func TestCompactionOverAllProjectsFirst(t *testing.T) {
	ctx := context.Background()
	database := scenarioFlood(t, t.TempDir(), 20, 5, 3000)
	projectRun, err := database.Cleanup(ctx, 2, measured(t, database, 2)*100/120)

	if err != nil || projectRun.Compaction == nil {
		t.Fatalf("the project's run %+v, %v", projectRun, err)
	}

	live, _ := measuredUsage(t, database)
	allRun, err := database.Cleanup(ctx, AllProjects, live*100/120)

	if err != nil || allRun.Compaction == nil {
		t.Fatalf("the run over all projects %+v, %v", allRun, err)
	}

	compactCleanups(t, database)

	if compaction := storedRun(t, database, allRun.Sequence).Compaction; compaction.Status != store.CompactionDone || compaction.FilesAfter >= compaction.FilesBefore {
		t.Errorf("over all projects: %+v", compaction)
	}

	if compaction := storedRun(t, database, projectRun.Sequence).Compaction; compaction.Status != store.CompactionSkipped || database.staleBytes() != 0 {
		t.Errorf("the project's: %+v, stale %d", compaction, database.staleBytes())
	}
}

// A run over all projects whose deleted records were compacted meanwhile
// (goleveldb on its own, another run's compaction) records it as not needed.
func TestCompactionWithNothingLeftIsSkipped(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil || issuesDeleted(run) {
		t.Fatalf("run %+v, %v", run, err)
	}

	if _, err := database.compact(true, database.staleData()...); err != nil {
		t.Fatal(err)
	}

	compactCleanups(t, database)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionSkipped {
		t.Errorf("the compaction: %+v", compaction)
	}
}

// At CleanupStartPercent with the files under CleanupFilesPercent the run
// compacts nothing first: compacting moves the data held by a percent or two
// (TestCompactingBeforeAtScale), less than a run frees.
func TestCleanupDoesNotCompactBeforeDeleting(t *testing.T) {
	database := liveStore(t)
	deletedIssues(t, database)

	stale := database.staleBytes()
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Status != cleanupDone || run.Compacted != run.SizeBefore || run.Files != 0 || len(run.Steps) == 0 {
		t.Fatalf("run %+v, %v", run, err)
	}

	if database.staleBytes() <= stale {
		t.Errorf("stale %d before the run, %d after: compacted before deleting", stale, database.staleBytes())
	}
}

// A run that deleted nothing has no compaction to wait for.
func TestCleanupWithoutDeletesHasNoCompaction(t *testing.T) {
	database := liveStore(t)
	issues := database.issueIDs(func(*issueRecord) bool { return true })

	if _, err := database.DeleteIssues(context.Background(), issues); err != nil {
		t.Fatal(err)
	}

	runs := checkSize(t, database, filesSize(t, database)*100/130)

	for _, run := range runs {
		if run.Compaction != nil {
			t.Errorf("run %+v", run)
		}
	}
}

// The database lane runs the compaction pass at start (a run a restart
// left uncompacted), when a run ends, and every CleanupInterval.
func TestLaneCompactsAfterRuns(t *testing.T) {
	database := openTest(t)

	for _, job := range database.Lane(1<<30, slog.New(slog.DiscardHandler)).Jobs {
		if job.Name == "cleanup compaction" {
			if !job.AtStart || job.Wake == nil || job.Every != config.CleanupInterval {
				t.Errorf("job %+v", job)
			}

			return
		}
	}

	t.Fatal("no cleanup compaction on the database lane")
}

// reopen closes the store and opens it again: the journal goes into a table.
func reopen(t *testing.T, database *Store, dir string) *Store {
	t.Helper()

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database = openOnDisk(t, dir)
	clockAt(database, scenarioNow)

	return database
}

// What compacting before a run (the deleted and replaced records, the inbox,
// the issues and status history) would change in the data held (liveSize),
// which decides at CleanupStartPercent whether a run starts, on about 250 MB
// written as ingest writes it, goleveldb compacting on its own. Measured only
// on request: COMPACT_BEFORE_DIR=$PWD/data/compact-before go test
// ./internal/storage/database -run TestCompactingBeforeAtScale -v
// (2026-11-24: -1.6% after ingest alone, the issues' old versions; -1.3% with
// 5.5% stale; -2.8% with 28% stale, which compactStale never leaves at a check.)
func TestCompactingBeforeAtScale(t *testing.T) {
	dir := os.Getenv("COMPACT_BEFORE_DIR")

	if dir == "" {
		t.Skip("set COMPACT_BEFORE_DIR")
	}

	dir = filepath.Join(dir, fmt.Sprint(time.Now().UnixNano()))

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	database := openOnDisk(t, dir)
	seedShares(t, database, 100)
	clockAt(database, scenarioNow)

	random := rand.New(rand.NewPCG(9, 9))
	written := 0

	write := func(batches int) {
		for range batches {
			events := make([]store.NewEvent, 0, 100)
			logs := make([]store.NewLog, 0, 200)

			for range 100 {
				written++
				received := scenarioNow.Add(-72 * 3600e9).Add(time.Duration(written) * time.Second)
				payload := fmt.Sprintf(`{"exception":{"values":[{"type":"Error","value":%q}]},"extra":{"note":%q}}`, scenarioText(random, 200), scenarioText(random, 2800))
				events = append(events, store.NewEvent{
					ProjectID: 1, EventID: fmt.Sprintf("scale-%d", written), Fingerprint: fmt.Sprintf("scale-%d", random.IntN(500)),
					Title: "scale", Level: "error", Timestamp: received, ReceivedAt: received, Payload: []byte(payload),
				})
			}

			for range 200 {
				logs = append(logs, store.NewLog{ProjectID: 1, Timestamp: database.now(), Level: "info", Body: scenarioText(random, 300)})
			}

			if _, err := database.Write([]Envelope{{Events: events, Logs: logs}}); err != nil {
				t.Fatal(err)
			}
		}
	}

	measure := func(stage string) {
		live, disk := measuredUsage(t, database)
		stale := 100 * float64(database.staleBytes()) / float64(database.logicalSize())
		started := time.Now()

		if _, err := database.compact(true, append(database.staleData(), database.inbox, database.issues, database.status)...); err != nil {
			t.Fatal(err)
		}

		took := time.Since(started)
		compacted, compactedDisk := measuredUsage(t, database)
		t.Logf("%s: held %s, compacted %s (%+.2f%%) in %s; files %s → %s; stale %.1f%% of counted",
			stage, config.FormatSize(live), config.FormatSize(compacted), percentOff(compacted, live), took.Round(time.Millisecond),
			config.FormatSize(disk.files), config.FormatSize(compactedDisk.files), stale)
	}

	write(500)
	measure("ingest only")
	write(200)

	issues := database.issueIDs(func(*issueRecord) bool { return true })

	if _, err := database.DeleteIssues(context.Background(), issues[:len(issues)/15]); err != nil {
		t.Fatal(err)
	}

	measure("ingest, then issues deleted under StalePercent")
	write(200)

	if _, err := database.DeleteIssues(context.Background(), issues[len(issues)/15:len(issues)/3]); err != nil {
		t.Fatal(err)
	}

	measure("ingest, then issues deleted over StalePercent")
}

// failingCompactions makes the next failures compactions fail.
func failingCompactions(database *Store, failures int) {
	database.compactFault = func() error {
		if failures == 0 {
			return nil
		}

		failures--

		return errors.New("disk full")
	}
}

// A compaction that failed is tried again on the next passes; one that
// succeeds then is done.
func TestFailedCompactionIsRetried(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil {
		t.Fatalf("run %+v, %v", run, err)
	}

	failingCompactions(database, store.CompactionAttempts-1)

	for attempt := 1; attempt < store.CompactionAttempts; attempt++ {
		if err := database.CompactCleanups(context.Background(), slog.New(slog.DiscardHandler)); err == nil {
			t.Fatalf("attempt %d did not fail", attempt)
		}

		if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionPending ||
			compaction.Attempts != attempt || compaction.Error != "disk full" {
			t.Fatalf("after attempt %d: %+v", attempt, compaction)
		}
	}

	compactCleanups(t, database)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionDone ||
		compaction.Attempts != store.CompactionAttempts || compaction.Error != "" {
		t.Errorf("the last attempt: %+v", compaction)
	}
}

// After store.CompactionAttempts failures it stays failed with the last error.
func TestCompactionFailsAfterItsAttempts(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil {
		t.Fatalf("run %+v, %v", run, err)
	}

	failingCompactions(database, store.CompactionAttempts+1)

	for range store.CompactionAttempts {
		if err := database.CompactCleanups(context.Background(), slog.New(slog.DiscardHandler)); err == nil {
			t.Fatal("a compaction did not fail")
		}
	}

	compactCleanups(t, database)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionFailed ||
		compaction.Attempts != store.CompactionAttempts || compaction.Error != "disk full" {
		t.Errorf("after %d attempts: %+v", store.CompactionAttempts, compaction)
	}
}

// An attempt that never returns (a panic, a killed process) still counts:
// after store.CompactionAttempts of them the compaction is failed, not tried forever.
func TestInterruptedCompactionCountsItsAttempts(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil {
		t.Fatalf("run %+v, %v", run, err)
	}

	calls := 0
	database.compactFault = func() error {
		calls++

		panic("compaction crashed")
	}

	for attempt := 1; attempt <= store.CompactionAttempts+1; attempt++ {
		func() {
			defer func() { _ = recover() }()

			_ = database.CompactCleanups(context.Background(), slog.New(slog.DiscardHandler))
		}()
	}

	compaction := storedRun(t, database, run.Sequence).Compaction

	if calls != store.CompactionAttempts || compaction.Status != store.CompactionFailed ||
		compaction.Attempts != store.CompactionAttempts || compaction.Error != errCompactionInterrupted {
		t.Errorf("%d calls: %+v", calls, compaction)
	}
}

// Envelopes arriving during the compaction keep the files from shrinking;
// what it freed is still recorded, from the deleted records it compacted.
func TestCompactionFreedUnderIngest(t *testing.T) {
	database := liveStore(t)
	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil {
		t.Fatalf("run %+v, %v", run, err)
	}

	files := filesSize(t, database)
	database.compactFault = func() error {
		envelopesOf(t, database, files)

		return nil
	}

	compactCleanups(t, database)

	compaction := storedRun(t, database, run.Sequence).Compaction

	if compaction.Status != store.CompactionDone || compaction.FilesAfter < compaction.FilesBefore || compaction.Freed <= 0 {
		t.Errorf("%+v", compaction)
	}
}
