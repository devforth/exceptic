package database

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// How a project's size on disk is measured (projectOnDisk): against the
// files a store holding only its data takes, and as the cleanup's deletes
// stay in the files and leave them.

// measuredBound is how far, in percent, a project's measured size on disk
// may be from the files a store holding only its data takes.
const measuredBound = 10.0

// measuredPart is the error logs from (inclusive) to (exclusive) of a project.
type measuredPart struct {
	project, from, to int
	compressible      bool
}

// measuredLetters are random letters the bodies are cut from, made once.
var measuredLetters = func() []byte {
	random := rand.New(rand.NewPCG(1, 1))
	letters := make([]byte, 1<<20)

	for at := range letters {
		letters[at] = 'a' + byte(random.IntN(26))
	}

	return letters
}()

// measuredBody is one log of a part: random letters, which snappy barely
// shrinks, or one letter repeated, which it shrinks to almost nothing.
func measuredBody(project, index int, compressible bool) string {
	prefix := fmt.Sprintf("p%d-%06d ", project, index)

	if compressible {
		return prefix + strings.Repeat("x", 2000)
	}

	at := rand.New(rand.NewPCG(uint64(project), uint64(index))).IntN(len(measuredLetters) - 2000)

	return prefix + string(measuredLetters[at:at+2000])
}

func writeMeasured(t *testing.T, database *Store, part measuredPart) {
	t.Helper()

	for start := part.from; start < part.to; start += 500 {
		logs := make([]store.NewLog, 0, 500)

		for index := start; index < min(start+500, part.to); index++ {
			logs = append(logs, store.NewLog{ProjectID: part.project, Timestamp: database.now(), Level: "error", Body: measuredBody(part.project, index, part.compressible)})
		}

		if err := database.SaveLogs(context.Background(), logs, 0); err != nil {
			t.Fatal(err)
		}
	}
}

// trueSize is what the parts take on disk alone: the files of a store
// holding only them, fully compacted, less those of the same store empty.
func trueSize(t *testing.T, parts ...measuredPart) int64 {
	t.Helper()

	alone := openOnDisk(t, t.TempDir())
	seedShares(t, alone, 100, 100, 100)
	clockAt(alone, scenarioNow)

	compactedSize := func() int64 {
		if err := alone.db.CompactRange(nil, nil); err != nil {
			t.Fatal(err)
		}

		size, err := alone.db.Size()

		if err != nil {
			t.Fatal(err)
		}

		return size
	}

	empty := compactedSize()

	for _, part := range parts {
		writeMeasured(t, alone, part)
	}

	return compactedSize() - empty
}

func percentOff(measured, truth int64) float64 {
	return float64(measured-truth) / float64(max(truth, 1)) * 100
}

// keptTail is the part of a project's logs still stored: the cleanup deletes
// error logs oldest first, so what is left is the newest of those written.
func keptTail(t *testing.T, database *Store, project, written int) measuredPart {
	t.Helper()

	kept := projectLogs(t, database, project)
	part := measuredPart{project: project, from: written - kept, to: written}

	if kept == 0 {
		return part
	}

	oldest, err := database.logs.By("project", kv.U32(uint32(project)), kv.U8(levelRank("error"))).First(database.db)

	if err != nil {
		t.Fatal(err)
	}

	if want := fmt.Sprintf("p%d-%06d ", project, part.from); !strings.HasPrefix(oldest.Record.Body, want) {
		t.Fatalf("project %d keeps %d logs from %q, want from %q", project, kept, oldest.Record.Body[:12], want)
	}

	return part
}

// compareMeasured logs each project's measured size against its true one
// and returns the largest error in percent.
func compareMeasured(t *testing.T, database *Store, stage string, truth map[int]int64) float64 {
	t.Helper()

	onDisk := usage(t, database).OnDisk
	worst := 0.0

	for project := 1; project <= len(truth); project++ {
		off := percentOff(onDisk[project], truth[project])
		worst = max(worst, math.Abs(off))
		t.Logf("%s: project %d measured %s, alone %s: %+.1f%%", stage, project, config.FormatSize(onDisk[project]), config.FormatSize(truth[project]), off)
	}

	return worst
}

// appendBacklog queues envelopes no project's data counts: count of 100 KB random bytes.
func appendBacklog(t *testing.T, database *Store, count int) int64 {
	t.Helper()

	random := rand.New(rand.NewPCG(7, 7))

	var total int64

	for range count {
		body := make([]byte, 100_000)

		for at := range body {
			body[at] = byte(random.Uint32())
		}

		entry, err := database.Append(1, database.now(), body)

		if err != nil {
			t.Fatal(err)
		}

		total += int64(entry.Size)
	}

	return total
}

// honest is a project's part of the files when they hold no deleted
// records: the files without the ingest queue's, in the proportion of its
// bytes to all counted.
func honest(t *testing.T, database *Store, project int) int64 {
	t.Helper()

	size, err := database.db.Size()

	if err != nil {
		t.Fatal(err)
	}

	inbox, err := database.db.SizeOf(database.inbox.KeyRange())

	if err != nil {
		t.Fatal(err)
	}

	return int64(float64(size-inbox) * float64(database.projectBytes(project)) / float64(database.logicalSize()))
}

// What the Projects page shows and the cleanup compares with a share is a
// project's part of the files: within measuredBound of what its data alone
// takes, with envelopes in the inbox, after a project's run left its deletes
// in the files (too few for compactStale), after a restart and after the
// compaction that frees them.
func TestDiskShareMeasuresProjectsHonestly(t *testing.T) {
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	seedShares(t, database, 100, 25)
	clockAt(database, scenarioNow)
	writeMeasured(t, database, measuredPart{project: 1, to: 3500})
	writeMeasured(t, database, measuredPart{project: 2, to: 1500})

	truth := map[int]int64{1: trueSize(t, measuredPart{project: 1, to: 3500}), 2: trueSize(t, measuredPart{project: 2, to: 1500})}
	// Still in the journal: written as they came, as the store alone would have them before its first compaction.
	compareMeasured(t, database, "in the journal", truth)

	if err := database.db.CompactRange(nil, nil); err != nil {
		t.Fatal(err)
	}

	check := func(stage string) {
		t.Helper()

		if worst := compareMeasured(t, database, stage, truth); worst > measuredBound {
			t.Errorf("%s: measured %.1f%% off, more than %.0f%%", stage, worst, measuredBound)
		}
	}

	check("compacted")

	inbox := appendBacklog(t, database, 10)
	// SizeOf reads the tables only: a backlog still in the journal counts as the projects' (an estimate, docs).
	compareMeasured(t, database, fmt.Sprintf("%s of inbox in the journal", config.FormatSize(inbox)), truth)

	if err := database.compact(database.inbox); err != nil {
		t.Fatal(err)
	}

	check(fmt.Sprintf("%s of inbox in the tables", config.FormatSize(inbox)))

	// Project 2 at 110% of its share; the database well below its limit.
	maxBytes := limitFor(usage(t, database).OnDisk[2], 25, 1.1)
	runs := checkSize(t, database, maxBytes)

	if len(runs) != 1 || runs[0].Project != 2 || runs[0].Status != cleanupDone {
		t.Fatalf("runs: %+v", runs)
	}

	run := runs[0]
	truth[2] = trueSize(t, keptTail(t, database, 2, 1500))
	t.Logf("run of project 2: limit %s, before %s, after %s (%.1f%% of the limit), alone after %s (%.1f%%)", config.FormatSize(run.Limit),
		config.FormatSize(run.SizeBefore), config.FormatSize(run.SizeAfter), float64(run.SizeAfter)*100/float64(run.Limit),
		config.FormatSize(truth[2]), float64(truth[2])*100/float64(run.Limit))

	if stale := database.staleBytes(); stale == 0 || stale*100 > config.StalePercent*database.logicalSize() {
		t.Fatalf("the run left %d stale of %d counted, want some below %d%%", stale, database.logicalSize(), config.StalePercent)
	}

	// Steps 1 to 4 have nothing to delete here, so the run stops at the first batch past 80%.
	if after := float64(run.SizeAfter) / float64(run.Limit); after < cleanupStop-0.02 || after > cleanupStop+0.01 {
		t.Errorf("the run ends measured at %.1f%% of the share's limit, want about %d%%", after*100, config.CleanupStopPercent)
	}

	if alone := float64(truth[2]) / float64(run.Limit); alone < cleanupStop-0.03 || alone > cleanupStop {
		t.Errorf("the project's data alone takes %.1f%% of the share's limit after the run, want just under %d%%", alone*100, config.CleanupStopPercent)
	}

	check("after the run, its deletes in the files")

	beforeRestart := usage(t, database).OnDisk
	stale := database.staleBytes()

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database = openOnDisk(t, dir)
	clockAt(database, scenarioNow)

	if database.staleBytes() != stale {
		t.Errorf("stale %d after a restart, was %d", database.staleBytes(), stale)
	}

	for project, size := range usage(t, database).OnDisk {
		if off := percentOff(size, beforeRestart[project]); math.Abs(off) > 1 {
			t.Errorf("project %d measured %d after a restart, %d before", project, size, beforeRestart[project])
		}
	}

	check("after a restart")

	if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
		t.Errorf("a check after the restart ran %+v", runs)
	}

	// As the run over all projects compacts at its start.
	if err := database.compact(database.logs, database.events); err != nil {
		t.Fatal(err)
	}

	if database.staleBytes() != 0 {
		t.Errorf("stale %d after compacting the logs and events", database.staleBytes())
	}

	check("after compacting the deletes")

	if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
		t.Errorf("a second check ran %+v", runs)
	}
}

// Envelopes processed but not compacted away keep their bytes in the files;
// they are no project's, so a project under its share is not cleaned for them.
func TestDiskShareLeavesOutTheInbox(t *testing.T) {
	database := openOnDisk(t, t.TempDir())
	seedShares(t, database, 50)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	writeProjectLogs(t, database, 1, 1000)

	random := rand.New(rand.NewPCG(1, 2))

	var (
		envelopes []Envelope
		inbox     int64
	)

	for range 40 {
		body := make([]byte, 100_000)

		for at := range body {
			body[at] = byte(random.Uint32())
		}

		entry, err := database.Append(1, database.now(), body)

		if err != nil {
			t.Fatal(err)
		}

		envelopes = append(envelopes, Envelope{Sequence: entry.Sequence, InboxSize: entry.Size})
		inbox += int64(entry.Size)
	}

	// Into the tables, as a busy server's inbox gets before its envelopes are stored.
	if err := database.compact(database.inbox); err != nil {
		t.Fatal(err)
	}

	if _, err := database.Write(envelopes); err != nil {
		t.Fatal(err)
	}

	size, _ := database.db.Size()
	// The project's share is half way between its size with the envelopes and without.
	maxBytes := 2*size - inbox

	if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
		t.Fatalf("%s on disk, %s of it processed envelopes; runs %+v", config.FormatSize(size), config.FormatSize(inbox), runs)
	}

	if onDisk := usage(t, database).OnDisk[1]; onDisk <= 0 || onDisk > size-inbox/2 {
		t.Errorf("project 1 takes %d bytes of %d, %d of them processed envelopes", onDisk, size, inbox)
	}
}

// One ratio serves every project (docs: an estimate): a project whose data
// compresses well is measured above its files, one whose data does not
// below. The parts still add up to the files without the inbox.
func TestDiskShareMeasuresMixedCompression(t *testing.T) {
	database := openOnDisk(t, t.TempDir())
	seedShares(t, database, 100, 100)
	clockAt(database, scenarioNow)

	parts := []measuredPart{{project: 1, to: 1000, compressible: true}, {project: 2, to: 1000}}

	for _, part := range parts {
		writeMeasured(t, database, part)
	}

	appendBacklog(t, database, 10)

	if err := database.db.CompactRange(nil, nil); err != nil {
		t.Fatal(err)
	}

	truth := map[int]int64{1: trueSize(t, parts[0]), 2: trueSize(t, parts[1])}
	compareMeasured(t, database, "compressible and random", truth)

	size, _ := database.db.Size()
	inbox, _ := database.db.SizeOf(database.inbox.KeyRange())
	onDisk := usage(t, database).OnDisk

	if off := percentOff(onDisk[1]+onDisk[2], size-inbox); math.Abs(off) > 1 {
		t.Errorf("the parts add up to %d, the files without the inbox %d (%+.1f%%)", onDisk[1]+onDisk[2], size-inbox, off)
	}
}

// A project floods while the database stays below its limit, so no run
// over all projects compacts; goleveldb compacts the deleted logs on its own
// between checks (forced here, as under steady writes). The measured sizes
// stay true to the files and the flood is held to its share.
func TestDiskShareMeasuresWhileTheDatabaseCompacts(t *testing.T) {
	database := openOnDisk(t, t.TempDir())
	seedShares(t, database, 100, 10)
	clockAt(database, scenarioNow)
	writeMeasured(t, database, measuredPart{project: 1, to: 250})

	const perCycle = 500

	// The share is about one cycle's logs on disk.
	maxBytes := trueSize(t, measuredPart{project: 2, to: perCycle}) * 10
	share := store.ShareLimit(maxBytes, 10)
	truth := map[int]int64{1: trueSize(t, measuredPart{project: 1, to: 250})}
	written, worst, over := 0, 0.0, 0.0

	for cycle := 1; cycle <= 4; cycle++ {
		writeMeasured(t, database, measuredPart{project: 2, from: written, to: written + perCycle})
		written += perCycle

		runs := checkSize(t, database, maxBytes)

		// What goleveldb does on its own under writes; not the cleanup's compaction.
		if err := database.db.CompactRange(nil, nil); err != nil {
			t.Fatal(err)
		}

		truth[2] = trueSize(t, keptTail(t, database, 2, written))
		worst = max(worst, compareMeasured(t, database, fmt.Sprintf("cycle %d, %d runs, stale %s", cycle, len(runs), config.FormatSize(database.staleBytes())), truth))
		over = max(over, float64(truth[2])/float64(share))
	}

	t.Logf("project 2 alone on disk at most %.0f%% of its share's limit %s; measured up to %.1f%% off", over*100, config.FormatSize(share), worst)

	if worst > measuredBound {
		t.Errorf("measured up to %.1f%% off, more than %.0f%%", worst, measuredBound)
	}

	if over > cleanupStart*1.05 {
		t.Errorf("project 2 takes %.0f%% of its share on disk after a check", over*100)
	}
}

// A purge compacts the logs and events, which takes out what a project's
// run deleted; a restart keeps that, and every project measures true.
func TestDiskShareMeasuresAfterAPurgeAndRestart(t *testing.T) {
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	seedShares(t, database, 100, 25, 100)
	clockAt(database, scenarioNow)

	for _, part := range []measuredPart{{project: 1, to: 500}, {project: 2, to: 1500}, {project: 3, to: 250}} {
		writeMeasured(t, database, part)
	}

	if err := database.db.CompactRange(nil, nil); err != nil {
		t.Fatal(err)
	}

	maxBytes := usage(t, database).OnDisk[2] * 100 / 25 * 2 / 3

	if runs := checkSize(t, database, maxBytes); len(runs) != 1 || runs[0].Project != 2 {
		t.Fatalf("runs: %+v", runs)
	}

	if err := database.DeleteProject(context.Background(), 3); err != nil {
		t.Fatal(err)
	}

	if err := database.PurgeProjects(context.Background()); err != nil {
		t.Fatal(err)
	}

	truth := map[int]int64{1: trueSize(t, measuredPart{project: 1, to: 500}), 2: trueSize(t, keptTail(t, database, 2, 1500))}

	if worst := compareMeasured(t, database, "after the purge", truth); worst > measuredBound {
		t.Errorf("after the purge: measured %.1f%% off", worst)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database = openOnDisk(t, dir)
	clockAt(database, scenarioNow)

	if worst := compareMeasured(t, database, fmt.Sprintf("after a restart, stale %s", config.FormatSize(database.staleBytes())), truth); worst > measuredBound {
		t.Errorf("after a restart: measured %.1f%% off; stale %d though the purge compacted the deletes", worst, database.staleBytes())
	}
}

// The database at its limit, one project over its share and one with none:
// the project's run goes first and cleans it alone; the run over all
// projects then deletes the oldest error logs, here the other project's, and
// brings the files under the stop percent of the limit.
func TestDiskShareThenAllProjects(t *testing.T) {
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	seedShares(t, database, 40, 100)
	clockAt(database, scenarioNow)
	// The project without a share first: its logs are the oldest.
	writeMeasured(t, database, measuredPart{project: 2, to: 2000})
	writeMeasured(t, database, measuredPart{project: 1, to: 1250})

	if err := database.db.CompactRange(nil, nil); err != nil {
		t.Fatal(err)
	}

	before := usage(t, database).OnDisk
	maxBytes := (before[1] + before[2]) * 85 / 100

	if float64(before[1]) < cleanupStart*float64(store.ShareLimit(maxBytes, 40)) {
		t.Fatalf("project 1 takes %d, under its share's limit", before[1])
	}

	runs := checkSize(t, database, maxBytes)

	if len(runs) != 2 || runs[0].Project != 1 || runs[1].Project != AllProjects {
		t.Fatalf("runs: %+v", runs)
	}

	for _, run := range runs {
		lost := map[int]int{}

		for _, step := range run.Steps {
			for project, counts := range step.Projects {
				lost[project] += counts.Logs
			}
		}

		t.Logf("run of %s: limit %s, %s → %s; logs lost %v", database.scope(run.Project).name(), config.FormatSize(run.Limit),
			config.FormatSize(run.SizeBefore), config.FormatSize(run.SizeAfter), lost)
	}

	onlyProject(t, runs[0], 1)
	onlyProject(t, runs[1], 2)

	size, _ := database.db.Size()
	after := usage(t, database).OnDisk
	t.Logf("files %s of %s (%.1f%%); project 1 %s of its %s, project 2 %s", config.FormatSize(size), config.FormatSize(maxBytes),
		float64(size)*100/float64(maxBytes), config.FormatSize(after[1]), config.FormatSize(store.ShareLimit(maxBytes, 40)), config.FormatSize(after[2]))

	if float64(size) > cleanupStop*float64(maxBytes)*1.02 {
		t.Errorf("the files take %.1f%% of the limit after the runs", float64(size)*100/float64(maxBytes))
	}

	if float64(after[1]) > cleanupStop*float64(store.ShareLimit(maxBytes, 40))*1.05 {
		t.Errorf("project 1 takes %d after its run, its share's limit is %d", after[1], store.ShareLimit(maxBytes, 40))
	}

	if projectLogs(t, database, 1) == 1250 || projectLogs(t, database, 2) == 2000 {
		t.Errorf("logs kept: project 1 %d, project 2 %d", projectLogs(t, database, 1), projectLogs(t, database, 2))
	}

	if database.staleBytes() != 0 {
		t.Errorf("stale %d after the run over all projects", database.staleBytes())
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database = openOnDisk(t, dir)

	if database.staleBytes() != 0 {
		t.Errorf("stale %d after a restart", database.staleBytes())
	}
}

// A project flooding steadily far below MAX_DATABASE_SIZE is cleaned at
// every check and held to its share; the records its runs deleted fill the
// files until the cleanup over all projects compacts them, which must not
// take the quiet project's data, whether ingest is quiet at that moment or not.
func TestDiskShareSteadyFlood(t *testing.T) {
	for name, busy := range map[string]bool{"ingest quiet at the checks": false, "ingest busy at the checks": true} {
		t.Run(name, func(t *testing.T) { testSteadyFlood(t, busy) })
	}
}

func testSteadyFlood(t *testing.T, busy bool) {
	database := scenarioFlood(t, t.TempDir(), 20, 5, 2000)
	maxBytes := limitFor(measured(t, database, 2), 20, 1.2)
	share := store.ShareLimit(maxBytes, 20)
	quiet := scenarioSnapshot(t, database, 1)
	random := rand.New(rand.NewPCG(43, 47))
	cleaned, all := 0, 0

	if busy {
		// An envelope came within quietFor before every check.
		defer func(was time.Duration) { quietFor = was }(quietFor)

		quietFor = time.Hour
	}

	for round := range 8 {
		scenarioLogs(t, database, random, 2, 1500, "error")

		if busy {
			database.lastAppend.Store(time.Now().UnixNano())
		}

		for _, run := range checkSize(t, database, maxBytes) {
			if run.Project == AllProjects {
				all++

				t.Logf("round %d, over all projects: %+v", round, run)

				continue
			}

			if run.Project != 2 {
				t.Fatalf("run %+v", run)
			}

			cleaned++
		}
	}

	got := measured(t, database, 2)
	counted := database.staleBytes()

	intoTables(t, database)

	want := honest(t, database, 2)

	t.Logf("%d runs of project 2, %d over all projects; project 2 measures %d, holds %d of the files once compacted, share %d; %d bytes stale, %d counted in all",
		cleaned, all, got, want, share, counted, database.logicalSize())

	if cleaned < 6 || float64(want) > 1.1*float64(share) {
		t.Errorf("%d runs; project 2 holds %d of the files, its share is %d", cleaned, want, share)
	}

	if after := scenarioSnapshot(t, database, 1); !sameContents(after, quiet) {
		t.Errorf("the quiet project holds %s, held %s", after, quiet)
	}
}

// Deleted or rewritten records of one project stay in the files until a
// compaction: they must not raise another project's measured size, so a
// project that did not grow is not cleaned when another project's issues
// are deleted or its events thinned as they turn a month old.
func TestDiskShareOtherProjectsChanges(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, database *Store){
		"issues deleted": func(t *testing.T, database *Store) {
			var ids []string

			for _, issue := range database.table.byID {
				if issue.ProjectID == 1 {
					ids = append(ids, issue.ID)
				}
			}

			if _, err := database.DeleteIssues(context.Background(), ids); err != nil {
				t.Fatal(err)
			}
		},
		// The size check thins them before it judges the shares.
		"events thinned": func(t *testing.T, database *Store) { clockAt(database, scenarioNow.Add(2*24*time.Hour)) },
	} {
		t.Run(name, func(t *testing.T) {
			database := openOnDisk(t, t.TempDir())
			seedShares(t, database, 100, 20)
			clockAt(database, scenarioNow)

			random := rand.New(rand.NewPCG(3, 5))
			scenarioEvents(t, database, random, 1, "other", 20, 20, "error", scenarioNow.Add(-29*24*time.Hour))

			for range 3 {
				scenarioLogs(t, database, random, 2, 1000, "error")
			}

			intoTables(t, database)

			before := measured(t, database, 2)
			maxBytes := limitFor(before, 20, 0.9)

			if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
				t.Fatalf("runs before the change: %+v", runs)
			}

			change(t, database)

			runs := checkSize(t, database, maxBytes)
			after := measured(t, database, 2)

			intoTables(t, database)

			// The proportion moves with the mix of logs and events (docs: an estimate), not with what the files still hold.
			want := honest(t, database, 2)
			t.Logf("project 2 measured %s before, %s after the change, %s once compacted; runs %d", config.FormatSize(before),
				config.FormatSize(after), config.FormatSize(want), len(runs))

			if len(runs) != 0 || projectLogs(t, database, 2) != 3000 {
				t.Errorf("runs %+v; project 2 keeps %d of 3000 logs", runs, projectLogs(t, database, 2))
			}

			if off := percentOff(after, want); math.Abs(off) > measuredBound {
				t.Errorf("project 2 measured %+.1f%% off its part of the compacted files after another project's change", off)
			}
		})
	}
}

// The files' bytes per byte counted rise as data that compresses worse
// comes, while goleveldb compacts the deletes on its own (forced here): a
// project stays measured true to the files and held to its share.
func TestDiskShareMeasuresARisingRatio(t *testing.T) {
	database := openOnDisk(t, t.TempDir())
	seedShares(t, database, 100, 5)
	clockAt(database, scenarioNow)
	writeMeasured(t, database, measuredPart{project: 1, to: 2000, compressible: true})
	writeMeasured(t, database, measuredPart{project: 2, to: 2000, compressible: true})
	intoTables(t, database)

	maxBytes := limitFor(measured(t, database, 2), 5, 1.5)
	share := store.ShareLimit(maxBytes, 5)

	if runs := checkSize(t, database, maxBytes); len(runs) != 1 || runs[0].Project != 2 {
		t.Fatalf("runs: %+v", runs)
	}

	written, worst := 2000, 0.0

	for round := range 10 {
		writeMeasured(t, database, measuredPart{project: 2, from: written, to: written + 100})
		written += 100

		// What goleveldb does on its own under writes; not the cleanup's compaction.
		intoTables(t, database)

		runs := checkSize(t, database, maxBytes)
		got, want := measured(t, database, 2), honest(t, database, 2)
		off := percentOff(got, want)
		worst = max(worst, math.Abs(off))
		t.Logf("round %d: %d runs, measured %s, files %s (%.0f%% of the share), %+.1f%%", round, len(runs), config.FormatSize(got),
			config.FormatSize(want), float64(want)*100/float64(share), off)
	}

	intoTables(t, database)

	if want := honest(t, database, 2); float64(want) > cleanupStart*float64(share)*1.05 {
		t.Errorf("project 2 holds %s of the files, %.0f%% of its share", config.FormatSize(want), float64(want)*100/float64(share))
	}

	if worst > measuredBound {
		t.Errorf("measured up to %.1f%% off the files", worst)
	}
}
