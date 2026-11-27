package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The runs of the disk share: when a check starts them, what they delete,
// and how they go on or end. Most run on a store on disk (goleveldb) with
// records of realistic sizes: events of a few KB, logs of a few hundred bytes.

// seedShares stores projects 1 to len(shares) with those disk shares.
func seedShares(t *testing.T, database *Store, shares ...int) {
	t.Helper()

	for index, share := range shares {
		project := store.Project{ID: index + 1, Name: fmt.Sprintf("project-%d", index+1), PublicKey: fmt.Sprintf("key-%d", index+1)}

		if err := database.AddProjects(context.Background(), database.now(), project); err != nil {
			t.Fatal(err)
		}

		project.DiskShare = share

		if err := database.UpdateProject(context.Background(), project); err != nil {
			t.Fatal(err)
		}
	}
}

// openOnDisk opens a store in dir: only files hold deleted records until a compaction.
func openOnDisk(t *testing.T, dir string) *Store {
	t.Helper()

	database, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = database.Close() })

	return database
}

// intoTables compacts everything written, as a database much larger than its
// journal holds nearly all of its data in tables.
func intoTables(t *testing.T, database *Store) {
	t.Helper()

	if err := database.db.CompactRange(nil, nil); err != nil {
		t.Fatal(err)
	}
}

// checkSize runs the size check of the database lane once, then the
// compaction it wakes (as the lane does), and returns the runs as they ran.
func checkSize(t *testing.T, database *Store, maxBytes int64) []CleanupRun {
	t.Helper()

	runs := checkSizeOnly(t, database, maxBytes)
	compactCleanups(t, database)

	return runs
}

// checkSizeOnly runs the size check of the database lane once, without the compaction after it.
func checkSizeOnly(t *testing.T, database *Store, maxBytes int64) []CleanupRun {
	t.Helper()

	ctx := context.Background()
	before, _ := database.CleanupHistory(ctx, store.CleanupRunsKept)

	if err := database.CheckSize(ctx, maxBytes, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}

	runs, _ := database.CleanupHistory(ctx, store.CleanupRunsKept)
	runs = runs[:len(runs)-len(before)]

	// Oldest first: the order they ran in.
	for left, right := 0, len(runs)-1; left < right; left, right = left+1, right-1 {
		runs[left], runs[right] = runs[right], runs[left]
	}

	checkCollections(t, database)

	return runs
}

// onlyProject fails unless every step of the run deleted records of project alone.
func onlyProject(t *testing.T, run CleanupRun, project int) {
	t.Helper()

	for _, step := range run.Steps {
		for deleted := range step.Projects {
			if deleted != project {
				t.Errorf("run of project %d, step %d deleted in project %d", run.Project, step.Number, deleted)
			}
		}
	}
}

// freedBy is what a run freed so far, in bytes counted.
func freedBy(run CleanupRun) int64 {
	var freed int64

	for _, step := range run.Steps {
		freed += step.Freed
	}

	return freed
}

// writeProjectLogs writes count error logs of one project, each the same size.
func writeProjectLogs(t *testing.T, database *Store, project, count int) {
	t.Helper()

	logs := make([]store.NewLog, 0, count)

	for index := range count {
		logs = append(logs, store.NewLog{ProjectID: project, Timestamp: database.now(), Level: "error", Body: fmt.Sprintf("project %d line %06d %s", project, index, strings.Repeat("x", 200))})
	}

	if err := database.SaveLogs(context.Background(), logs, 0); err != nil {
		t.Fatal(err)
	}
}

func projectLogs(t *testing.T, database *Store, project int) int {
	t.Helper()

	return countLogs(t, database, store.LogFilter{ProjectIDs: []int{project}})
}

var scenarioNow = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

// scenarioText is length bytes of words in lowercase letters, which compress about as log text does.
func scenarioText(random *rand.Rand, length int) string {
	var text strings.Builder

	for text.Len() < length {
		for range 2 + random.IntN(8) {
			text.WriteByte('a' + byte(random.IntN(26)))
		}

		text.WriteByte(' ')
	}

	return text.String()[:length]
}

// scenarioEvents stores perIssue events of each of issues issues of a
// project, at the given level, received one second apart from at; each
// event's payload is about 3 KB.
func scenarioEvents(t *testing.T, database *Store, random *rand.Rand, project int, fingerprint string, issues, perIssue int, level string, at time.Time) {
	t.Helper()

	events := make([]store.NewEvent, 0, issues*perIssue)

	for index := range issues * perIssue {
		received := at.Add(time.Duration(index) * time.Second)
		payload := fmt.Sprintf(`{"exception":{"values":[{"type":"Error","value":%q}]},"extra":{"note":%q}}`, scenarioText(random, 200), scenarioText(random, 2800))
		events = append(events, store.NewEvent{
			ProjectID: project, EventID: fmt.Sprintf("%d-%s-%d-%d", project, fingerprint, at.Unix(), index), Fingerprint: fmt.Sprintf("%s-%d", fingerprint, index%issues),
			Title: fingerprint, Level: level, Timestamp: received, ReceivedAt: received, Payload: []byte(payload),
		})
	}

	if _, err := database.Write([]Envelope{{Events: events}}); err != nil {
		t.Fatal(err)
	}
}

// scenarioLogs stores count logs of a project at a level, each about 300 bytes.
func scenarioLogs(t *testing.T, database *Store, random *rand.Rand, project, count int, level string) {
	t.Helper()

	logs := make([]store.NewLog, 0, count)

	for range count {
		logs = append(logs, store.NewLog{ProjectID: project, Timestamp: database.now(), Level: level, Body: scenarioText(random, 300)})
	}

	if err := database.SaveLogs(context.Background(), logs, 0); err != nil {
		t.Fatal(err)
	}
}

// scenarioContents is every record of a project: logs and events by sequence, issues by ID with their stored events.
type scenarioContents struct {
	logs, events []uint64
	issues       []string
}

func scenarioSnapshot(t *testing.T, database *Store, project int) scenarioContents {
	t.Helper()

	var contents scenarioContents

	err := database.logs.By("project", kv.U32(uint32(project))).Each(database.db, func(entry kv.Entry[logRecord]) (bool, error) {
		contents.logs = append(contents.logs, entry.Record.Sequence)

		return true, nil
	})

	if err == nil {
		err = database.events.By("project", kv.U32(uint32(project))).Head().Each(database.db, func(entry kv.Entry[eventRecord]) (bool, error) {
			contents.events = append(contents.events, entry.Record.Sequence)

			return true, nil
		})
	}

	if err != nil {
		t.Fatal(err)
	}

	issues, err := database.issues.Primary().All(database.db)

	if err != nil {
		t.Fatal(err)
	}

	for _, issue := range issues {
		if issue.ProjectID == project {
			contents.issues = append(contents.issues, fmt.Sprintf("%s:%d", issue.ID, issue.Stored))
		}
	}

	slices.Sort(contents.logs)
	slices.Sort(contents.events)
	slices.Sort(contents.issues)

	return contents
}

func (contents scenarioContents) String() string {
	return fmt.Sprintf("%d logs, %d events, %d issues", len(contents.logs), len(contents.events), len(contents.issues))
}

func sameContents(first, second scenarioContents) bool {
	return slices.Equal(first.logs, second.logs) && slices.Equal(first.events, second.events) && slices.Equal(first.issues, second.issues)
}

// levelLogs are the sequences of a project's logs of a level, oldest first.
func levelLogs(t *testing.T, database *Store, project int, level string) []uint64 {
	t.Helper()

	var sequences []uint64

	err := database.logs.By("project", kv.U32(uint32(project)), kv.U8(levelRank(level))).Each(database.db, func(entry kv.Entry[logRecord]) (bool, error) {
		sequences = append(sequences, entry.Record.Sequence)

		return true, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	return sequences
}

// projectIssue is the issue of a project's fingerprint.
func projectIssue(database *Store, project int, fingerprint string) *issueRecord {
	issue, err := database.issues.GetBy(database.db, "fingerprint", kv.K(kv.U32(uint32(project)), kv.Text(fingerprint)))

	if err != nil {
		return nil
	}

	return &issue
}

// measured is a project's size as the cleanup compares it with its share (and the Projects page shows it).
func measured(t *testing.T, database *Store, project int) int64 {
	t.Helper()

	onDisk, _, err := database.scope(project).size()

	if err != nil {
		t.Fatal(err)
	}

	return onDisk
}

// limitFor is the MAX_DATABASE_SIZE at which a project measured at onDisk
// holds times its share of percent: 1.5 is half as much again as it may hold.
func limitFor(onDisk int64, share int, times float64) int64 {
	return int64(float64(onDisk) / times * 100 / float64(share))
}

// scenarioFlood is a store on disk with a quiet project 1 (share 100) and
// project 2 (share), which floods: floodEvents error events of each of 20
// issues and floodLogs error logs, most of the data.
func scenarioFlood(t *testing.T, dir string, share, floodEvents, floodLogs int) *Store {
	t.Helper()

	database := openOnDisk(t, dir)
	seedShares(t, database, 100, share)
	clockAt(database, scenarioNow)

	random := rand.New(rand.NewPCG(7, 11))
	scenarioEvents(t, database, random, 1, "quiet", 8, 5, "error", scenarioNow.Add(-2*time.Hour))
	scenarioLogs(t, database, random, 1, 100, "info")
	scenarioLogs(t, database, random, 1, 200, "error")

	if floodEvents > 0 {
		scenarioEvents(t, database, random, 2, "flood", 20, floodEvents, "error", scenarioNow.Add(-time.Hour))
	}

	for range floodLogs / 1000 {
		scenarioLogs(t, database, random, 2, 1000, "error")
	}

	intoTables(t, database)

	return database
}

// scenarioProjects is a store on disk with projects of these shares, each
// with logs of its own: count error logs of about 300 bytes.
func scenarioProjects(t *testing.T, shares []int, counts []int) *Store {
	t.Helper()

	database := openOnDisk(t, t.TempDir())
	seedShares(t, database, shares...)
	clockAt(database, scenarioNow)

	random := rand.New(rand.NewPCG(13, 17))

	for index, count := range counts {
		for written := 0; written < count; written += 1000 {
			scenarioLogs(t, database, random, index+1, min(1000, count-written), "error")
		}
	}

	intoTables(t, database)

	return database
}

// setShare sets a project's disk share.
func setShare(t *testing.T, database *Store, project, share int) {
	t.Helper()

	stored, err := database.GetProject(context.Background(), project)

	if err != nil {
		t.Fatal(err)
	}

	stored.DiskShare = share

	if err := database.UpdateProject(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
}

// stopOnceDeleted reports the time limit of the size check passed once the
// running run deleted something, as the lane's timeout does in the middle of a run.
type stopOnceDeleted struct {
	context.Context

	database *Store
}

func (ctx stopOnceDeleted) Err() error {
	if running, err := ctx.database.runningCleanup(); err == nil && len(running.Steps) > 0 {
		return context.DeadlineExceeded
	}

	return nil
}

// stopProjectRun starts the run of project with a pass that its time limit
// stops after the first batch that deleted something, and returns the run left running.
func stopProjectRun(t *testing.T, database *Store, project int, maxBytes int64) CleanupRun {
	t.Helper()

	_, err := database.cleanupPass(stopOnceDeleted{context.Background(), database}, maxBytes)

	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}

	running, err := database.runningCleanup()

	if err != nil || running.Status != cleanupRunning || running.Project != project || len(running.Steps) == 0 {
		t.Fatalf("the stopped run: %+v, %v", running, err)
	}

	return running
}

// storedRun is the run of a sequence as the history holds it.
func storedRun(t *testing.T, database *Store, sequence uint64) CleanupRun {
	t.Helper()

	history, err := database.CleanupHistory(context.Background(), store.CleanupRunsKept)

	if err != nil {
		t.Fatal(err)
	}

	for _, run := range history {
		if run.Sequence == sequence {
			return run
		}
	}

	t.Fatalf("no run %d in the history", sequence)

	return CleanupRun{}
}

// A project that floods over its share is cut back to 80% of it by itself,
// even far below MAX_DATABASE_SIZE; the other project keeps every record,
// the ingest queue keeps its envelopes, and the next check finds nothing to do.
func TestDiskShareFloodCutBack(t *testing.T) {
	for name, test := range map[string]struct {
		share, floodEvents, floodLogs int
		times                         float64 // the flood's size in its share
	}{
		"events and logs, half again its share": {20, 25, 3000, 1.5},
		"logs only, three times its share":      {20, 0, 6000, 3},
		"logs only, ten times its share":        {5, 0, 10000, 10},
	} {
		t.Run(name, func(t *testing.T) { testFloodCutBack(t, test.share, test.floodEvents, test.floodLogs, test.times) })
	}
}

func testFloodCutBack(t *testing.T, percent, floodEvents, floodLogs int, times float64) {
	database := scenarioFlood(t, t.TempDir(), percent, floodEvents, floodLogs)

	for index := range 5 {
		if _, err := database.Append(1, scenarioNow, []byte(fmt.Sprintf(`{"envelope":%d}`, index))); err != nil {
			t.Fatal(err)
		}
	}

	quiet := scenarioSnapshot(t, database, 1)
	flood := measured(t, database, 2)
	maxBytes := limitFor(flood, percent, times)
	share := store.ShareLimit(maxBytes, percent)

	if size, _ := database.db.Size(); size >= maxBytes {
		t.Fatalf("the database (%d) should stay under its limit (%d)", size, maxBytes)
	}

	// Measured as the run left it, before the compaction after it.
	runs := checkSizeOnly(t, database, maxBytes)

	if len(runs) != 1 {
		t.Fatalf("runs: %+v", runs)
	}

	run := runs[0]

	if run.Project != 2 || run.Status != cleanupDone || run.Share != percent || run.Limit != share || run.SizeBefore != run.Compacted {
		t.Fatalf("run: %+v", run)
	}

	if run.SizeBefore < flood*99/100 || run.SizeBefore > flood*101/100 {
		t.Errorf("the run started at %d, the project measured %d", run.SizeBefore, flood)
	}

	onlyProject(t, run, 2)

	if after := scenarioSnapshot(t, database, 1); !sameContents(after, quiet) {
		t.Errorf("the quiet project holds %s, held %s", after, quiet)
	}

	after := measured(t, database, 2)

	t.Logf("share %d, project 2 measured %d before (%.3f of its share), %d after (%.3f), %d logs and %d events left; steps %+v", share, flood,
		float64(flood)/float64(share), after, float64(after)/float64(share), len(scenarioSnapshot(t, database, 2).logs), len(scenarioSnapshot(t, database, 2).events), run.Steps)

	// Down to about 80% of its share: a batch stops once it freed enough, and one proportion serves
	// every project, which moves as the run changes the mix of logs and events (docs: an estimate).
	if float64(after) > 0.9*float64(share) || float64(after) < 0.7*float64(share) {
		t.Errorf("project 2 measures %.3f of its share after the run, want about 0.8", float64(after)/float64(share))
	}

	if shown := usage(t, database).OnDisk[2]; shown != after || run.SizeAfter < after*99/100 || run.SizeAfter > after*101/100 {
		t.Errorf("the Projects page shows %d, the run recorded %d, the cleanup measures %d", shown, run.SizeAfter, after)
	}

	if pending, err := database.Pending(0, 100, nil); err != nil || len(pending) != 5 {
		t.Errorf("the ingest queue holds %d envelopes, want 5 (%v)", len(pending), err)
	}

	compactCleanups(t, database)

	if again := checkSize(t, database, maxBytes); len(again) != 0 {
		t.Errorf("the next check runs again: %+v; project 2 measures %d of %d", again, measured(t, database, 2), share)
	}
}

// A project's run takes the steps of docs/configuration.md (Retention) in
// their order: steps 1 to 4 in full, then 5 to 7 until the project is at 80%
// of its share, where it stops in the middle of step 8 with the oldest error
// logs gone and everything after kept; the other project keeps all of it.
func TestDiskShareStepOrder(t *testing.T) {
	ctx := context.Background()
	database := openOnDisk(t, t.TempDir())
	seedShares(t, database, 100, 20)

	random := rand.New(rand.NewPCG(3, 5))
	old, recent := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	// grown is what project 2 grew by while write ran.
	grown := func(write func()) int64 {
		before := database.projectBytes(2)

		write()

		return database.projectBytes(2) - before
	}

	var oldLogs, oldData, resolved, debugLogs, warnings, errorLogs, fresh int64

	clockAt(database, old)

	for project := 1; project <= 2; project++ {
		logs := grown(func() {
			scenarioLogs(t, database, random, project, 50, "info")
			scenarioLogs(t, database, random, project, 50, "error")
		})
		data := grown(func() { scenarioEvents(t, database, random, project, "old", 1, 30, "error", old) })
		oldLogs, oldData = oldLogs+logs, oldData+logs+data
	}

	// The old events lose their details as they turn a month old, before any run.
	clockAt(database, scenarioNow)

	thinned := grown(func() {
		if _, err := database.ReduceAging(ctx); err != nil {
			t.Fatal(err)
		}
	})
	oldEvents := oldData + thinned - oldLogs

	clockAt(database, recent)

	for project := 1; project <= 2; project++ {
		resolved += grown(func() { scenarioEvents(t, database, random, project, "resolved", 1, 6, "error", recent) })

		if _, err := database.SetIssuesStatus(ctx, []string{projectIssue(database, project, "resolved-0").ID}, store.StatusResolved, store.Actor{}); err != nil {
			t.Fatal(err)
		}

		debugLogs += grown(func() { scenarioLogs(t, database, random, project, 200, "debug") })
		warnings += grown(func() {
			scenarioEvents(t, database, random, project, "warning", 1, 10, "warning", recent.Add(time.Hour))
		})
		errorLogs += grown(func() { scenarioLogs(t, database, random, project, 400, "error") })

		fresh += grown(func() { scenarioEvents(t, database, random, project, "fresh", 1, 30, "error", recent.Add(2*time.Hour)) })
	}

	clockAt(database, scenarioNow)

	// The goal falls half way through the error logs of step 8.
	goal := database.projectBytes(2) - (oldLogs + oldEvents/3 + resolved*5/6 + debugLogs + warnings*9/10 + fresh/3 + errorLogs/2)

	// As the check finds it: the thinned events' old versions compacted away.
	if err := database.compactStale(); err != nil {
		t.Fatal(err)
	}

	disk, err := database.diskUsage()

	if err != nil {
		t.Fatal(err)
	}

	maxBytes := int64(float64(goal) * disk.ratio / 0.8 * 100 / 20)
	quiet := scenarioSnapshot(t, database, 1)
	errorsBefore := levelLogs(t, database, 2, "error")

	if size, _ := database.db.Size(); size >= maxBytes {
		t.Fatalf("the database (%d) should stay under its limit (%d)", size, maxBytes)
	}

	runs := checkSize(t, database, maxBytes)

	if len(runs) != 1 || runs[0].Project != 2 || runs[0].Status != cleanupDone || runs[0].Reached != 8 {
		t.Fatalf("runs: %+v", runs)
	}

	onlyProject(t, runs[0], 2)

	var numbers []int

	deleted := map[int]CleanupCounts{}

	for _, step := range runs[0].Steps {
		numbers = append(numbers, step.Number)
		deleted[step.Number] = *step.Projects[2]
	}

	want := map[int]CleanupCounts{2: {Logs: 100}, 3: {Events: 10}, 4: {Events: 5}, 5: {Logs: 200}, 6: {Events: 9}, 7: {Events: 10}}

	if !slices.Equal(numbers, []int{2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("steps %v, want 2 to 8: %+v", numbers, runs[0].Steps)
	}

	for number, counts := range want {
		if deleted[number] != counts {
			t.Errorf("step %d deleted %+v, want %+v", number, deleted[number], counts)
		}
	}

	// The goal is half way through its 400 error logs.
	if logs := deleted[8].Logs; logs < 100 || logs > 300 {
		t.Errorf("step 8 deleted %d of 400 error logs, want about 200", logs)
	}

	errorsAfter := levelLogs(t, database, 2, "error")

	if !slices.Equal(errorsAfter, errorsBefore[len(errorsBefore)-len(errorsAfter):]) {
		t.Errorf("the error logs kept are not the newest %d of %d", len(errorsAfter), len(errorsBefore))
	}

	for fingerprint, stored := range map[string]int{"old-0": 20, "resolved-0": 1, "warning-0": 1, "fresh-0": 20} {
		if issue := projectIssue(database, 2, fingerprint); issue == nil || issue.Stored != stored {
			t.Errorf("issue %s keeps %+v events, want %d", fingerprint, issue, stored)
		}
	}

	if left := len(levelLogs(t, database, 2, "debug")) + len(levelLogs(t, database, 2, "info")); left != 0 {
		t.Errorf("project 2 keeps %d logs below error", left)
	}

	if after := scenarioSnapshot(t, database, 1); !sameContents(after, quiet) {
		t.Errorf("project 1 holds %s, held %s", after, quiet)
	}
}

// Every step of a project's run reads and deletes that project's records
// alone: run to the end, it empties the project and leaves the other as it was.
func TestDiskShareStepsKeepOtherProjects(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	seedShares(t, database, 5, 100)

	write := func(at time.Time) {
		clockAt(database, at)

		for project := 1; project <= 2; project++ {
			for index, level := range []string{"info", "warning", "error", "fatal"} {
				for count := range 25 {
					_, err := database.SaveEvent(ctx, store.NewEvent{
						ProjectID: project, EventID: fmt.Sprintf("%d-%s-%d-%d", project, level, at.Unix(), count), Fingerprint: fmt.Sprintf("issue-%d", index%2),
						Title: level, Level: level, Timestamp: at, ReceivedAt: at, Payload: []byte(`{"exception":{}}`),
					})

					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}

		writeLogs(t, database, "debug", 40, "debug")
		writeLogs(t, database, "error", 40, "error")
	}

	write(time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC))
	write(time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC))
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))

	other := func() string {
		issues, _, _ := database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{2}})

		return fmt.Sprintf("%d bytes, %d issues, %d logs", database.projectBytes(2), issues, projectLogs(t, database, 2))
	}
	before := other()
	run := CleanupRun{Status: cleanupRunning, Project: 1, Reached: 1}

	if err := database.advance(ctx, &run); err != nil {
		t.Fatal(err)
	}

	if after := other(); after != before || database.projectBytes(1) != 0 {
		t.Fatalf("project 2: %s, was %s; project 1 keeps %d bytes", after, before, database.projectBytes(1))
	}

	onlyProject(t, run, 1)
}

// Each project over its share is cleaned in the same check, the smallest
// share first, one after another, each against its own share alone: shares
// are ceilings and may add up to more than 100%. Only then, and only while
// the database is still at its limit, the cleanup over all projects runs.
func TestDiskShareSeveralInOneCheck(t *testing.T) {
	for name, test := range map[string]struct {
		shares     []int
		limit      float64 // of the size of the database
		projects   []int   // the runs, in order; 0 is all projects
		quietKeeps bool    // project 3, at 100%, keeps all of its data
	}{
		"shares under 100%":           {[]int{40, 30, 100}, 1.1, []int{2, 1}, true},
		"shares adding up over 100%":  {[]int{60, 60, 100}, 0.7, []int{1, 2, 0}, false},
		"the same share, by their ID": {[]int{35, 35, 100}, 1.2, []int{1, 2}, true},
	} {
		t.Run(name, func(t *testing.T) {
			database := scenarioProjects(t, test.shares, []int{3000, 3000, 300})
			size, _ := database.db.Size()
			maxBytes := int64(float64(size) * test.limit)
			quiet := scenarioSnapshot(t, database, 3)
			runs := checkSize(t, database, maxBytes)

			var projects []int

			for _, run := range runs {
				projects = append(projects, run.Project)

				if run.Status != cleanupDone {
					t.Errorf("run of project %d: %+v", run.Project, run)
				}

				if run.Project != AllProjects {
					onlyProject(t, run, run.Project)

					if run.Limit != store.ShareLimit(maxBytes, test.shares[run.Project-1]) {
						t.Errorf("run of project %d has limit %d, want its share of %d", run.Project, run.Limit, maxBytes)
					}
				}
			}

			if !slices.Equal(projects, test.projects) {
				t.Fatalf("runs of projects %v, want %v", projects, test.projects)
			}

			for project := 1; project <= 2; project++ {
				share := store.ShareLimit(maxBytes, test.shares[project-1])

				if onDisk := measured(t, database, project); float64(onDisk) > 0.9*float64(share) {
					t.Errorf("project %d measures %d after the check, its share is %d", project, onDisk, share)
				}
			}

			if after := scenarioSnapshot(t, database, 3); test.quietKeeps && !sameContents(after, quiet) {
				t.Errorf("project 3 holds %s, held %s", after, quiet)
			}

			if size, _ := database.db.Size(); size >= maxBytes {
				t.Errorf("the database holds %d after the check, its limit is %d", size, maxBytes)
			}
		})
	}
}

// A project at 100% has no limit of its own, and with MAX_DATABASE_SIZE off
// no project is cleaned: a project's run left running then ends, deleting nothing more.
func TestDiskShareWithoutALimit(t *testing.T) {
	t.Run("share 100", func(t *testing.T) {
		database := scenarioFlood(t, t.TempDir(), 100, 5, 2000)
		size, _ := database.db.Size()

		if runs := checkSize(t, database, size*3/2); len(runs) != 0 {
			t.Fatalf("runs: %+v", runs)
		}
	})

	t.Run("limit off", func(t *testing.T) {
		database := scenarioFlood(t, t.TempDir(), 5, 5, 3000)
		maxBytes := limitFor(measured(t, database, 2), 5, 3)

		if runs := checkSize(t, database, 0); len(runs) != 0 {
			t.Fatalf("runs: %+v", runs)
		}

		stopProjectRun(t, database, 2, maxBytes)

		flood := scenarioSnapshot(t, database, 2)

		if runs := checkSize(t, database, 0); len(runs) != 0 {
			t.Fatalf("runs: %+v", runs)
		}

		history, _ := database.CleanupHistory(context.Background(), 10)

		if len(history) != 1 || history[0].Status != cleanupFailed || history[0].Error != "stopped: MAX_DATABASE_SIZE turned off" {
			t.Fatalf("history: %+v", history)
		}

		if after := scenarioSnapshot(t, database, 2); !sameContents(after, flood) {
			t.Errorf("project 2 holds %s, held %s when its run stopped", after, flood)
		}
	})
}

// While a backup runs, no project's run starts or goes on (its snapshot
// keeps deleted records on disk); one stopped before goes on after it.
func TestDiskShareWaitsForABackup(t *testing.T) {
	database := scenarioFlood(t, t.TempDir(), 20, 5, 3000)
	maxBytes := limitFor(measured(t, database, 2), 20, 3)

	database.backups.Add(1)

	if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
		t.Fatalf("runs during a backup: %+v", runs)
	}

	database.backups.Add(-1)
	stopped := stopProjectRun(t, database, 2, maxBytes)
	database.backups.Add(1)

	runs := checkSize(t, database, maxBytes)
	database.backups.Add(-1)

	if run := storedRun(t, database, stopped.Sequence); len(runs) != 0 || run.Status != cleanupRunning || freedBy(run) != freedBy(stopped) {
		t.Fatalf("the run stopped: %+v; during a backup it is %+v, runs %+v", stopped, run, runs)
	}

	checkSize(t, database, maxBytes)

	if run := storedRun(t, database, stopped.Sequence); run.Status != cleanupDone || freedBy(run) <= freedBy(stopped) {
		t.Fatalf("the run stopped: %+v; after the backup it is %+v", stopped, run)
	}
}

// Cleanup starts no run beside one still running: the size check goes on only with the newest.
func TestCleanupRefusesBesideARunningRun(t *testing.T) {
	database := openTest(t)
	writeLogs(t, database, "info", 100, "line")

	running := CleanupRun{Status: cleanupRunning, Reached: 1, Limit: 1}

	if err := database.saveCleanup(&running); err != nil {
		t.Fatal(err)
	}

	if run, err := database.Cleanup(context.Background(), AllProjects, 1); err == nil || run.Status != "" {
		t.Fatalf("run %+v, %v", run, err)
	}
}

// A project's run stopped by the time limit of the size check stays running
// with its project, step and position; after a restart the next check goes
// on with that run, for that project alone, to 80% of its share.
func TestDiskShareRunGoesOnAfterARestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database := scenarioFlood(t, dir, 20, 5, 3000)
	maxBytes := limitFor(measured(t, database, 2), 20, 3)
	share := store.ShareLimit(maxBytes, 20)
	quiet := scenarioSnapshot(t, database, 1)
	stopped := stopProjectRun(t, database, 2, maxBytes)

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database = openOnDisk(t, dir)
	clockAt(database, scenarioNow)

	// The run that went on is no new run.
	if runs := checkSize(t, database, maxBytes); len(runs) != 0 {
		t.Fatalf("new runs after the restart: %+v", runs)
	}

	run := storedRun(t, database, stopped.Sequence)

	if run.Project != 2 || run.Status != cleanupDone || run.Goal != stopped.Goal || run.Steps[0].Number != stopped.Steps[0].Number {
		t.Fatalf("the run stopped: %+v; after the restart: %+v", stopped, run)
	}

	onlyProject(t, run, 2)

	if history, _ := database.CleanupHistory(ctx, 10); len(history) != 1 {
		t.Errorf("%d runs in the history, want the one that went on", len(history))
	}

	if freed, before := freedBy(run), freedBy(stopped); freed <= before {
		t.Errorf("the run freed %d after the restart, %d before", freed, before)
	}

	if after := scenarioSnapshot(t, database, 1); !sameContents(after, quiet) {
		t.Errorf("project 1 holds %s, held %s", after, quiet)
	}

	if onDisk := measured(t, database, 2); float64(onDisk) > 0.9*float64(share) || float64(onDisk) < 0.7*float64(share) {
		t.Errorf("project 2 measures %.3f of its share, want about 0.8", float64(onDisk)/float64(share))
	}

	if again := checkSize(t, database, maxBytes); len(again) != 0 {
		t.Errorf("the next check runs again: %+v", again)
	}
}

// A project's run left running ends without deleting more when its share or
// MAX_DATABASE_SIZE changed, or the project was deleted, saying why; the same
// check then starts the run the new limit calls for, if any.
func TestDiskShareStoppedRunEnds(t *testing.T) {
	for name, test := range map[string]struct {
		change   func(t *testing.T, database *Store, maxBytes int64) int64 // returns the limit of the next check
		reason   string
		newShare int // the share of the run started after it; 0: none
	}{
		"share lowered": {func(t *testing.T, database *Store, maxBytes int64) int64 {
			setShare(t, database, 2, 10)

			return maxBytes
		}, "stopped: the disk share of project-2 changed to 10%", 10},
		"share raised to 100%": {func(t *testing.T, database *Store, maxBytes int64) int64 {
			setShare(t, database, 2, 100)

			return maxBytes
		}, "stopped: the disk share of project-2 changed to 100%", 0},
		"limit raised": {func(t *testing.T, database *Store, maxBytes int64) int64 {
			return maxBytes * 10
		}, "", 0},
		"limit lowered": {func(t *testing.T, database *Store, maxBytes int64) int64 {
			return maxBytes / 2
		}, "", 20},
		"limit turned off": {func(t *testing.T, database *Store, maxBytes int64) int64 {
			return 0
		}, "stopped: MAX_DATABASE_SIZE turned off", 0},
		"project deleted": {func(t *testing.T, database *Store, maxBytes int64) int64 {
			if err := database.DeleteProject(context.Background(), 2); err != nil {
				t.Fatal(err)
			}

			return maxBytes
		}, "stopped: the project was deleted", 0},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			database := scenarioFlood(t, t.TempDir(), 20, 5, 3000)
			maxBytes := limitFor(measured(t, database, 2), 20, 3)
			stopped := stopProjectRun(t, database, 2, maxBytes)
			flood := scenarioSnapshot(t, database, 2)
			next := test.change(t, database, maxBytes)
			reason := test.reason

			if reason == "" {
				reason = limitChanged(next)
			}

			runs := checkSize(t, database, next)
			history, _ := database.CleanupHistory(ctx, 10)
			ended := history[len(history)-1]

			if ended.Sequence != stopped.Sequence || ended.Status != cleanupFailed || ended.Error != reason || freedBy(ended) != freedBy(stopped) {
				t.Fatalf("the stopped run ended as %+v, want failed with %q", ended, reason)
			}

			if test.newShare == 0 {
				if len(runs) != 0 {
					t.Fatalf("runs after it: %+v", runs)
				}

				if after := scenarioSnapshot(t, database, 2); !sameContents(after, flood) {
					t.Errorf("project 2 holds %s, held %s when its run stopped", after, flood)
				}

				return
			}

			// Over all projects too when the lowered limit is below the database.
			if len(runs) > 1 && runs[1].Project == AllProjects {
				runs = runs[:1]
			}

			if len(runs) != 1 || runs[0].Project != 2 || runs[0].Share != test.newShare || runs[0].Limit != store.ShareLimit(next, test.newShare) || runs[0].Status != cleanupDone {
				t.Fatalf("runs after it: %+v", runs)
			}

			onlyProject(t, runs[0], 2)
		})
	}
}

// Ingest goes on while a project's run deletes: the run deletes that
// project's records alone, every other one written meanwhile stays, and the
// indexes and counters agree afterwards.
func TestDiskShareRunBesideIngest(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	seedShares(t, database, 100, 10)
	clockAt(database, scenarioNow)

	for range 2 {
		writeProjectLogs(t, database, 2, 4000)
	}

	maxBytes := usage(t, database).OnDisk[2] * 100 / 10 * 2 / 3

	var (
		group             sync.WaitGroup
		logs, events      int
		stop, firstWrites = make(chan struct{}), make(chan struct{})
	)

	// At most besideRounds rounds: a writer faster than the run deletes would keep the project over its goal.
	const besideRounds = 200

	group.Go(func() {
		for round := range besideRounds {
			select {
			case <-stop:
				return
			default:
			}

			for project := 1; project <= 2; project++ {
				batch := make([]store.NewLog, 0, 20)

				for index := range 20 {
					batch = append(batch, store.NewLog{ProjectID: project, Timestamp: database.now(), Level: "error", Body: fmt.Sprintf("beside %d %d", round, index)})
				}

				if err := database.SaveLogs(ctx, batch, 0); err != nil {
					t.Error(err)

					return
				}

				_, err := database.SaveEvent(ctx, store.NewEvent{
					ProjectID: project, EventID: fmt.Sprintf("beside-%d-%d", project, round), Fingerprint: "beside", Title: "beside",
					Level: "error", Timestamp: database.now(), ReceivedAt: database.now(), Payload: []byte(`{"exception":{}}`),
				})

				if err != nil {
					t.Error(err)

					return
				}
			}

			logs, events = logs+20, events+1

			if round == 0 {
				close(firstWrites)
			}
		}
	})

	<-firstWrites

	if err := database.CheckSize(ctx, maxBytes, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}

	close(stop)
	group.Wait()

	runs, _ := database.CleanupHistory(ctx, 10)

	if len(runs) != 1 || runs[0].Project != 2 || runs[0].Status != cleanupDone || len(runs[0].Steps) == 0 {
		t.Fatalf("runs: %+v", runs)
	}

	onlyProject(t, runs[0], 2)

	if kept := projectLogs(t, database, 1); kept != logs {
		t.Errorf("project 1 keeps %d logs, %d were written", kept, logs)
	}

	if kept := storedEvents(database, "beside"); kept != events {
		t.Errorf("project 1 keeps %d events, %d were written", kept, events)
	}

	t.Logf("%d logs and %d events per project written during the run; project 2 keeps %d logs", logs, events, projectLogs(t, database, 2))
	checkCollections(t, database)
}

// What get_storage takes from the store: the run's sizes are the measured
// ones the Projects page shows, its limit is the share of MAX_DATABASE_SIZE,
// and the oldest kept moves as the run deletes a kind.
func TestDiskShareStorageNumbers(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	seedShares(t, database, 100, 20)

	old := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	clockAt(database, old)

	for project := 1; project <= 2; project++ {
		info := make([]store.NewLog, 0, 200)

		for index := range 200 {
			info = append(info, store.NewLog{ProjectID: project, Timestamp: old, Level: "info", Body: fmt.Sprintf("info %d", index)})
		}

		if err := database.SaveLogs(ctx, info, 0); err != nil {
			t.Fatal(err)
		}

		for index := range 20 {
			_, err := database.SaveEvent(ctx, store.NewEvent{
				ProjectID: project, EventID: fmt.Sprintf("old-%d-%d", project, index), Fingerprint: "old", Title: "old",
				Level: "error", Timestamp: old, ReceivedAt: old, Payload: []byte(`{"exception":{}}`),
			})

			if err != nil {
				t.Fatal(err)
			}
		}
	}

	hours := make([]time.Time, 4)

	for hour := range hours {
		hours[hour] = time.Date(2026, 11, 2, 9+hour, 0, 0, 0, time.UTC)
		clockAt(database, hours[hour])
		writeProjectLogs(t, database, 2, 500)
	}

	clockAt(database, scenarioNow)

	before := usage(t, database)
	want := store.OldestKept{LogsBelowError: old, ErrorLogs: hours[0], Errors: old}

	if !sameOldest(before.Oldest[2], want) {
		t.Fatalf("project 2 oldest %+v, want %+v", before.Oldest[2], want)
	}

	maxBytes := before.OnDisk[2] * 10 / 13 * 100 / 20
	runs := checkSizeOnly(t, database, maxBytes) // as the run left it, before its compaction

	if len(runs) != 1 || runs[0].Project != 2 {
		t.Fatalf("runs: %+v", runs)
	}

	run, after := runs[0], usage(t, database)

	if run.Limit != maxBytes*20/100 || run.Share != 20 {
		t.Errorf("run limit %d, share %d; want %d, 20", run.Limit, run.Share, maxBytes*20/100)
	}

	if math.Abs(percentOff(run.SizeBefore, before.OnDisk[2])) > 1 || math.Abs(percentOff(run.SizeAfter, after.OnDisk[2])) > 1 {
		t.Errorf("run %d → %d, the page shows %d → %d", run.SizeBefore, run.SizeAfter, before.OnDisk[2], after.OnDisk[2])
	}

	kept := after.Oldest[2]

	if !kept.LogsBelowError.IsZero() || !kept.ErrorLogs.After(hours[0]) || kept.ErrorLogs.After(hours[3]) || !kept.Errors.Equal(old) {
		t.Errorf("project 2 oldest after its run %+v (error logs from %s to %s)", kept, hours[0], hours[3])
	}

	if !sameOldest(after.Oldest[1], before.Oldest[1]) {
		t.Errorf("project 1 oldest %+v, was %+v", after.Oldest[1], before.Oldest[1])
	}

	for share := store.MinDiskShare; share <= store.MaxDiskShare; share += store.DiskShareStep {
		want := maxBytes * int64(share) / 100

		if share == store.MaxDiskShare {
			want = 0
		}

		if limit := store.ShareLimit(maxBytes, (store.Project{DiskShare: share}).Share()); limit != want {
			t.Errorf("share %d%%: limit %d, want %d", share, limit, want)
		}
	}

	if (store.Project{}).Share() != store.MaxDiskShare {
		t.Errorf("a project stored without a share has %d%%", (store.Project{}).Share())
	}
}

func sameOldest(first, second store.OldestKept) bool {
	return first.LogsBelowError.Equal(second.LogsBelowError) && first.ErrorLogs.Equal(second.ErrorLogs) && first.Errors.Equal(second.Errors)
}
