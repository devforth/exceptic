package database_test

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

// The cleanup on a large database (storetest.OpenSeed, scale 1), measured only on request:
// CLEANUP_BENCH_DIR=$PWD/data/bench-cleanup go test ./internal/storage/database -run TestCleanupBench -v -timeout 2h
// (-run TestCleanupBench/deep for one scenario; start is the normal start at
// CleanupStartPercent of the data held, the others limits of the files). The first run builds the seed,
// which takes a few minutes. Use a directory on a disk: on tmpfs (/tmp on many
// systems) a sync costs nothing.
func TestCleanupBench(t *testing.T) {
	base := os.Getenv("CLEANUP_BENCH_DIR")

	if base == "" {
		t.Skip("set CLEANUP_BENCH_DIR")
	}

	for _, scenario := range []struct {
		name  string
		limit float64 // of the size on disk; 0: the data held just at the start
	}{{"start", 0}, {"always", 0.95}, {"half", 0.5}, {"deep", 0.15}} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			db, _ := storetest.OpenSeed(t, base, 1)
			size, err := db.DatabaseSize(ctx)

			if err != nil {
				t.Fatal(err)
			}

			limit := int64(float64(size) * scenario.limit)

			if scenario.limit == 0 {
				limit = startLimit(ctx, t, db, size)
			}

			probeCtx, stop := context.WithCancel(ctx)
			probed := benchProbe(probeCtx, db)
			started := time.Now()
			run, err := db.Cleanup(ctx, database.AllProjects, limit)
			elapsed := time.Since(started)

			stop()

			probe := <-probed

			if err != nil || probe.err != nil {
				t.Fatal(err, probe.err)
			}

			var stepsTime time.Duration

			for _, step := range run.Steps {
				var total store.CleanupCounts

				for _, counts := range step.Projects {
					total.Add(*counts)
				}

				stepsTime += step.Duration
				t.Logf("step %d, %s: freed %s in %s (%s)", step.Number, step.Name, config.FormatSize(step.Freed), step.Duration.Round(time.Millisecond), total)
			}

			waits := probe.waits
			left, _ := run.Left()

			slices.Sort(waits)
			t.Logf("limit %s, size %s -> %s (%s left of the data it started with), reached step %d, total %s: steps %s, writes, syncs, rests and compaction %s; ingest batches %d, p50 %s, p99 %s, max %s",
				config.FormatSize(limit), config.FormatSize(run.SizeBefore), config.FormatSize(run.SizeAfter), config.FormatSize(left), run.Reached,
				elapsed.Round(time.Millisecond), stepsTime.Round(time.Millisecond), (elapsed - stepsTime).Round(time.Millisecond),
				len(waits), storetest.Percentile(waits, 50), storetest.Percentile(waits, 99), storetest.Percentile(waits, 100))

			// The run's compaction, a pass of its own on the lane (CompactCleanups).
			probeCtx, stop = context.WithCancel(ctx)
			probed = benchProbe(probeCtx, db)
			started = time.Now()
			err = db.CompactCleanups(ctx, slog.New(slog.DiscardHandler))
			compacting := time.Since(started)

			stop()

			probe = <-probed
			files, sizeErr := db.DatabaseSize(ctx)

			if err != nil || probe.err != nil || sizeErr != nil {
				t.Fatal(err, probe.err, sizeErr)
			}

			slices.Sort(probe.waits)
			t.Logf("compaction %s, files %s -> %s; ingest batches %d, p50 %s, p99 %s, max %s",
				compacting.Round(time.Millisecond), config.FormatSize(size), config.FormatSize(files),
				len(probe.waits), storetest.Percentile(probe.waits, 50), storetest.Percentile(probe.waits, 99), storetest.Percentile(probe.waits, 100))

			if scenario.limit == 0 && run.Files > 0 {
				t.Errorf("the files started the run: %s", config.FormatSize(run.Files))
			}

			if run.Reached == 0 || len(run.Steps) == 0 || run.SizeAfter >= limit {
				t.Errorf("the cleanup deleted nothing or stayed over the limit: %+v", run)
			}

			if problems, err := db.Verify(); err != nil || len(problems) > 0 {
				t.Errorf("verify: %v %v", problems, err)
			}
		})
	}
}

// startLimit is the limit whose CleanupStartPercent the data held just
// reaches, with the files under CleanupFilesPercent of it: a run without the
// safety net. A seed is written without deletes, so its files are the data
// held plus the inbox and the journal, well under that.
func startLimit(ctx context.Context, t *testing.T, db *database.Store, files int64) int64 {
	t.Helper()

	usage, err := db.ProjectUsage(ctx)

	if err != nil {
		t.Fatal(err)
	}

	limit := usage.Live * 100 / config.CleanupStartPercent

	if files*100 >= limit*config.CleanupFilesPercent {
		t.Fatalf("the seed's files %s are over %d%% of %s: the run would start from the files", config.FormatSize(files), config.CleanupFilesPercent, config.FormatSize(limit))
	}

	return limit
}

type probeResult struct {
	waits []time.Duration
	err   error
}

// benchProbe writes an ingest-sized batch every 20 ms and reports how long each took.
func benchProbe(ctx context.Context, db *database.Store) <-chan probeResult {
	result := make(chan probeResult, 1)

	go func() {
		var (
			probe probeResult
			index int
		)

		random := rand.New(rand.NewPCG(7, 7))

		for ctx.Err() == nil {
			envelope := database.Envelope{Events: []store.NewEvent{storetest.SeedEvent(random, 1, fmt.Sprintf("probe-%d", index%5), storetest.SeedNow)},
				Logs: storetest.SeedLogs(random, storetest.SeedNow, 10, index)}
			started := time.Now()

			if _, probe.err = db.Write([]database.Envelope{envelope}); probe.err != nil {
				break
			}

			probe.waits = append(probe.waits, time.Since(started))
			index++

			select {
			case <-ctx.Done():
			case <-time.After(20 * time.Millisecond):
			}
		}

		result <- probe
	}()

	return result
}
