package database

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// seedShares stores projects 1 to len(shares) with those disk shares.
func seedShares(t *testing.T, database *Store, shares ...int) {
	t.Helper()

	for index, share := range shares {
		project := store.Project{ID: index + 1, Name: fmt.Sprintf("project-%d", index+1), PublicKey: fmt.Sprintf("key-%d", index+1)}

		if err := database.SeedProject(context.Background(), project, database.now()); err != nil {
			t.Fatal(err)
		}

		project.DiskShare = share

		if err := database.UpdateProject(context.Background(), project); err != nil {
			t.Fatal(err)
		}
	}
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

// checkSize runs the size check of the database lane once.
func checkSize(t *testing.T, database *Store, maxBytes int64) []CleanupRun {
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

// One project floods; at the limit the cleanup over all projects deletes
// the oldest error logs first, today's logs of a quiet project included.
// A disk share on the flooding project cleans it alone, before that.
func TestDiskShareKeepsOtherProjects(t *testing.T) {
	for name, share := range map[string]int{"without a share": 100, "with a share on the flood": 50} {
		t.Run(name, func(t *testing.T) {
			database := openTest(t)
			seedShares(t, database, 100, share)
			clockAt(database, time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC))
			writeProjectLogs(t, database, 1, 20)
			clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
			writeProjectLogs(t, database, 2, 2000)

			size, _ := database.db.Size()
			runs := checkSize(t, database, size)
			quiet := projectLogs(t, database, 1)

			if share == 100 {
				if quiet != 0 || len(runs) != 1 || runs[0].Project != 0 {
					t.Fatalf("the quiet project keeps %d logs; runs %+v", quiet, runs)
				}

				return
			}

			if quiet != 20 || projectLogs(t, database, 2) >= 2000 {
				t.Fatalf("the quiet project keeps %d logs, the flood %d", quiet, projectLogs(t, database, 2))
			}

			if len(runs) != 1 || runs[0].Project != 2 || runs[0].Status != cleanupDone || runs[0].Limit != size/2 {
				t.Fatalf("runs: %+v", runs)
			}

			onlyProject(t, runs[0], 2)
		})
	}
}

// Projects over their share are cleaned one at a time, the smallest share
// first; the run over all projects follows only while the database is still
// at its limit.
func TestDiskShareRunsInOrder(t *testing.T) {
	for name, test := range map[string]struct {
		limitPercent int64
		projects     []int // the runs, in order; 0 is all projects
	}{
		"projects freed enough":     {100, []int{2, 1}},
		"projects freed too little": {70, []int{2, 1, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			database := openTest(t)
			seedShares(t, database, 10, 5, 100)
			clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
			writeProjectLogs(t, database, 1, 300)
			writeProjectLogs(t, database, 2, 300)
			writeProjectLogs(t, database, 3, 1400)

			size, _ := database.db.Size()
			runs := checkSize(t, database, size*test.limitPercent/100)

			if len(runs) != len(test.projects) {
				t.Fatalf("%d runs, want %d: %+v", len(runs), len(test.projects), runs)
			}

			for index, run := range runs {
				if run.Project != test.projects[index] || run.Status != cleanupDone {
					t.Fatalf("run %d: project %d, %s; want project %d", index+1, run.Project, run.Status, test.projects[index])
				}

				if run.Project != 0 {
					onlyProject(t, run, run.Project)
				}
			}

			if test.projects[len(test.projects)-1] != 0 && projectLogs(t, database, 3) != 1400 {
				t.Errorf("project 3 keeps %d logs, want 1400", projectLogs(t, database, 3))
			}
		})
	}
}

// A project's run stopped by a restart goes on with its project, step and
// position; one whose share or MAX_DATABASE_SIZE changed meanwhile ends, saying which.
func TestDiskShareRunGoesOn(t *testing.T) {
	const twoGB = int64(2 << 30)

	for name, test := range map[string]struct {
		share    int
		maxBytes int64
		reason   string
	}{
		"same share":    {20, twoGB, ""},
		"share changed": {30, twoGB, "stopped: the disk share of project-2 changed to 30%"},
		// 20% of 2 GB is 10% of 4 GB, another share: still the limit is what changed.
		"limit changed": {20, 2 * twoGB, "stopped: MAX_DATABASE_SIZE changed to 4.0 GB"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			database := openTest(t)
			seedShares(t, database, 100, test.share)
			clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
			writeProjectLogs(t, database, 1, 100)
			writeProjectLogs(t, database, 2, 100)

			run := CleanupRun{Status: cleanupRunning, Project: 2, Reached: 6, Goal: database.projectBytes(2) / 2, Limit: twoGB * 20 / 100, Share: 20}

			if err := database.saveCleanup(&run); err != nil {
				t.Fatal(err)
			}

			runs := checkSize(t, database, test.maxBytes)

			if projectLogs(t, database, 1) != 100 {
				t.Errorf("project 1 keeps %d logs, want 100", projectLogs(t, database, 1))
			}

			history, _ := database.CleanupHistory(ctx, 10)

			if len(runs) != 0 || len(history) != 1 || history[0].Project != 2 || history[0].Error != test.reason {
				t.Fatalf("runs %+v, history %+v", runs, history)
			}

			if test.reason != "" {
				if history[0].Status != cleanupFailed || projectLogs(t, database, 2) != 100 {
					t.Fatalf("stopped run: %+v, project 2 keeps %d logs", history[0], projectLogs(t, database, 2))
				}

				return
			}

			if left := projectLogs(t, database, 2); history[0].Status != cleanupDone || history[0].Steps[0].Number != 6 || left == 0 || left >= 100 {
				t.Fatalf("run: %+v, project 2 keeps %d logs", history[0], left)
			}

			onlyProject(t, history[0], 2)
		})
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
		issues, _ := database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{2}})

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
	if err := database.compactInbox(); err != nil {
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

// A project's run leaves what it deleted in the files until a compaction;
// the next check, after a restart too, does not count those bytes as the
// project's and clean it again.
func TestDiskShareRunOnce(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprintf("restart %t", restart), func(t *testing.T) { testDiskShareRunOnce(t, restart) })
	}
}

func testDiskShareRunOnce(t *testing.T, restart bool) {
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	seedShares(t, database, 100, 50)
	clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))

	random := rand.New(rand.NewPCG(1, 2))

	// Bodies that do not compress, so the data outweighs what the deletes add to the journal.
	for project, count := range map[int]int{1: 50, 2: 1000} {
		logs := make([]store.NewLog, 0, count)

		for range count {
			body := make([]byte, 2000)

			for at := range body {
				body[at] = 'a' + byte(random.IntN(26))
			}

			logs = append(logs, store.NewLog{ProjectID: project, Timestamp: database.now(), Level: "error", Body: string(body)})
		}

		if err := database.SaveLogs(context.Background(), logs, 0); err != nil {
			t.Fatal(err)
		}
	}

	if err := database.db.CompactRange(nil, nil); err != nil {
		t.Fatal(err)
	}

	size, _ := database.db.Size()
	maxBytes := size * 3 / 2
	first := checkSize(t, database, maxBytes)

	if len(first) != 1 || first[0].Project != 2 || len(first[0].Steps) == 0 {
		t.Fatalf("first check: %+v", first)
	}

	left := projectLogs(t, database, 2)

	if restart {
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}

		database = openOnDisk(t, dir)
		clockAt(database, time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC))
	}

	if again := checkSize(t, database, maxBytes); len(again) != 0 || projectLogs(t, database, 2) != left {
		t.Fatalf("second check: %+v; project 2 keeps %d logs of %d", again, projectLogs(t, database, 2), left)
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
