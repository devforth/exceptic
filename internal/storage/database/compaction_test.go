package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/background"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// rangesEngine records the key ranges compacted.
type rangesEngine struct {
	kv.Engine

	mutex  sync.Mutex
	ranges []compactRange
}

func (engine *rangesEngine) CompactRange(lower, upper []byte) error {
	engine.mutex.Lock()
	engine.ranges = append(engine.ranges, compactRange{bytes.Clone(lower), bytes.Clone(upper)})
	engine.mutex.Unlock()

	return engine.Engine.CompactRange(lower, upper)
}

func (engine *rangesEngine) taken() []compactRange {
	engine.mutex.Lock()
	defer engine.mutex.Unlock()

	ranges := engine.ranges
	engine.ranges = nil

	return ranges
}

// openRecording opens a store in dir whose compactions are recorded.
func openRecording(t *testing.T, dir string) (*Store, *rangesEngine) {
	t.Helper()

	engine, err := openEngine(Location(dir))

	if err != nil {
		t.Fatal(err)
	}

	recording := &rangesEngine{Engine: engine}
	database, err := open(recording)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = database.Close() })
	clockAt(database, scenarioNow)

	return database, recording
}

func covers(ranges []compactRange, key []byte) bool {
	for _, part := range ranges {
		if bytes.Compare(key, part.lower) >= 0 && (part.upper == nil || bytes.Compare(key, part.upper) < 0) {
			return true
		}
	}

	return false
}

// pendingRun runs a cleanup over all projects that leaves its compaction pending.
func pendingRun(t *testing.T, database *Store) CleanupRun {
	t.Helper()

	live, _ := measuredUsage(t, database)
	run, err := database.Cleanup(context.Background(), AllProjects, live*100/99)

	if err != nil || run.Compaction == nil || run.Compaction.Status != store.CompactionPending {
		t.Fatalf("run %+v, %v", run, err)
	}

	return run
}

// The compaction after a run compacts the logs up to the sequence it
// started at: the newer ones, where ingest writes while it runs, are left
// out (goleveldb would go on compacting as long as they come), and what the
// run deleted is freed.
func TestCompactionLeavesWhatIngestWrites(t *testing.T) {
	database, engine := openRecording(t, t.TempDir())
	liveData(t, database)
	run := pendingRun(t, database)
	started := database.db.Peek()
	random := rand.New(rand.NewPCG(5, 5))
	database.compactFault = func() error {
		database.compactFault = nil

		scenarioLogs(t, database, random, 1, 200, "info")
		scenarioLogs(t, database, random, 1, 200, "error")

		return nil
	}

	engine.taken()
	compactCleanups(t, database)

	compaction := storedRun(t, database, run.Sequence).Compaction
	ranges := engine.taken()

	if compaction.Status != store.CompactionDone || compaction.FilesAfter >= compaction.FilesBefore || database.staleBytes() != 0 || len(ranges) < 2 {
		t.Fatalf("the compaction %+v in %d parts, stale %d", compaction, len(ranges), database.staleBytes())
	}

	written := 0

	err := database.logs.Primary().Keys(database.db, func(key kv.Key) (bool, error) {
		if sequenceOf(key) < started {
			return true, nil
		}

		written++

		if covers(ranges, append([]byte("log/"), key...)) {
			t.Errorf("compacted the log %d written while it ran", sequenceOf(key))
		}

		return true, nil
	})

	if err != nil || written != 400 {
		t.Fatalf("%d logs written meanwhile, %v", written, err)
	}
}

// A compaction yields between its parts to a size check woken meanwhile,
// which then runs at once, and goes on right after it; the history records
// it once done, with the time it took compacting.
func TestCompactionYieldsToTheSizeCheck(t *testing.T) {
	database := liveStore(t)
	run := pendingRun(t, database)
	wake := make(chan struct{}, 1)

	var seen atomic.Pointer[store.CleanupCompaction]

	database.compactFault = func() error {
		database.compactFault = nil

		wake <- struct{}{}

		time.Sleep(50 * time.Millisecond) // the lane takes the wake

		return nil
	}

	check := background.Job{Name: "size check", First: time.Hour, Every: time.Hour, Wake: wake, Run: func(context.Context) error {
		seen.Store(storedRun(t, database, run.Sequence).Compaction)

		return nil
	}}
	compaction := background.Job{Name: "cleanup compaction", AtStart: true, First: time.Hour, Every: time.Hour, Run: func(ctx context.Context) error {
		return database.CompactCleanups(ctx, slog.New(slog.DiscardHandler))
	}}
	stop := background.Start(context.Background(), slog.New(slog.DiscardHandler), background.Lane{Name: "database", Jobs: []background.Job{compaction, check}})

	for deadline := time.Now().Add(10 * time.Second); storedRun(t, database, run.Sequence).Compaction.Status == store.CompactionPending; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the compaction did not finish")
		}
	}

	stop()

	if during := seen.Load(); during == nil || during.Status != store.CompactionPending || during.Running || during.Progress == nil || during.Progress.Reached == nil {
		t.Errorf("the size check saw %+v", during)
	}

	done := storedRun(t, database, run.Sequence).Compaction

	if done.Status != store.CompactionDone || done.Attempts != 1 || done.Duration <= 0 || done.StartedAt.IsZero() ||
		done.FilesAfter >= done.FilesBefore || done.Progress != nil {
		t.Errorf("the compaction %+v", done)
	}
}

// A compaction stopped between its parts (its time limit, a restart) goes
// on from the part it reached.
func TestCompactionGoesOnFromItsPart(t *testing.T) {
	dir := t.TempDir()
	database, engine := openRecording(t, dir)
	liveData(t, database)
	run := pendingRun(t, database)
	ctx, cancel := context.WithCancel(context.Background())
	database.compactFault = func() error {
		database.compactFault = nil

		cancel()

		return nil
	}

	engine.taken()

	if err := database.CompactCleanups(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}

	first := engine.taken()
	stopped := storedRun(t, database, run.Sequence).Compaction

	if len(first) != 1 || stopped.Status != store.CompactionPending || stopped.Running || stopped.Attempts != 0 || !bytes.Equal(stopped.Progress.Reached, first[0].upper) {
		t.Fatalf("stopped after %d parts: %+v", len(first), stopped)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	database, engine = openRecording(t, dir)
	compactCleanups(t, database)

	rest := engine.taken()

	if done := storedRun(t, database, run.Sequence).Compaction; done.Status != store.CompactionDone || done.Attempts != 1 || len(rest) == 0 ||
		covers(rest, first[0].lower) || database.staleBytes() != 0 {
		t.Errorf("then %d parts, compacting %q again: %+v, stale %d", len(rest), first[0].lower, done, database.staleBytes())
	}
}

// The compaction after a run on a database of about 170 MB per scale (half
// of it logs, a fourth of those deleted by the run), while a writer stores
// about 3 MB a second as ingest does (envelopes into the inbox, then their
// logs and the events of new issues): one CompactRange per collection, as
// before, against the parts. Measured only on request:
// COMPACTION_LOAD=6 go test ./internal/storage/database -run TestCompactionUnderIngestLoad -v -timeout 1h
// (2026-11-24, scale 6, 1 GB: whole 16.8 s and 20.4 s, two calls of up to
// about 10 s; in parts 15.9 s, five parts of 2.2 to 3.7 s.)
func TestCompactionUnderIngestLoad(t *testing.T) {
	scale, _ := strconv.Atoi(os.Getenv("COMPACTION_LOAD"))

	if scale == 0 {
		t.Skip("set COMPACTION_LOAD to a scale")
	}

	for _, whole := range []bool{true, false} {
		t.Run(fmt.Sprintf("whole=%t", whole), func(t *testing.T) {
			database := openOnDisk(t, t.TempDir())
			seedShares(t, database, 100)

			random := rand.New(rand.NewPCG(3, 3))

			for range 60 * scale {
				scenarioLogs(t, database, random, 1, 5000, "info")
			}

			scenarioEvents(t, database, random, 1, "old", 20000*scale, 1, "error", time.Now().Add(-time.Hour))
			time.Sleep(5 * time.Second) // goleveldb's own compactions settle

			live, _ := measuredUsage(t, database)

			if _, err := database.Cleanup(context.Background(), AllProjects, live); err != nil {
				t.Fatal(err)
			}

			files, _ := database.db.Size()
			stop := make(chan struct{})

			var (
				written atomic.Int64
				writer  sync.WaitGroup
			)

			writer.Go(func() { ingestLoad(t, database, stop, 3<<20, &written) })
			time.Sleep(3 * time.Second)

			started := time.Now()

			var err error

			if whole {
				err = errors.Join(database.logs.Compact(database.db), database.events.Compact(database.db))
			} else {
				parts, _ := database.compactParts([]compactable{database.logs, database.events}, database.db.Peek(), nil)

				for _, part := range parts {
					size, _ := database.db.SizeOf(part.lower, part.upper)
					began := time.Now()
					err = errors.Join(err, database.db.CompactRange(part.lower, part.upper))
					t.Logf("part %q to %q, %d MB: %s", part.lower[:min(len(part.lower), 12)], part.upper[:min(len(part.upper), 12)], size>>20, time.Since(began).Round(time.Millisecond))
				}
			}

			t.Logf("files %d MB; compacted in %s, %d MB written meanwhile, err %v", files>>20, time.Since(started).Round(time.Millisecond), written.Load()>>20, err)
			close(stop)
			writer.Wait()
		})
	}
}

// ingestLoad writes as ingest does, about rate bytes a second, until stop.
func ingestLoad(t *testing.T, database *Store, stop <-chan struct{}, rate int64, written *atomic.Int64) {
	random := rand.New(rand.NewPCG(9, 9))
	started := time.Now()

	for round := 0; ; round++ {
		select {
		case <-stop:
			return
		default:
		}

		if ahead := time.Duration(float64(written.Load())/float64(rate)*1e9) - time.Since(started); ahead > 0 {
			time.Sleep(ahead)
		}

		body := []byte(scenarioText(random, 40_000))
		entry, err := database.Append(1, database.now(), body)

		if err != nil {
			t.Error(err)

			return
		}

		logs := make([]store.NewLog, 0, 100)

		for range 100 {
			logs = append(logs, store.NewLog{ProjectID: 1, Timestamp: database.now(), Level: "info", Body: scenarioText(random, 300),
				TraceID: strconv.FormatUint(random.Uint64(), 16), SpanID: strconv.FormatUint(random.Uint64(), 16)})
		}

		events := make([]store.NewEvent, 0, 10)

		for index := range 10 {
			fingerprint := fmt.Sprintf("load-%d-%d", round, index)
			events = append(events, store.NewEvent{ProjectID: 1, EventID: fingerprint, Fingerprint: fingerprint, Title: "load", Level: "error",
				Timestamp: database.now(), ReceivedAt: database.now(), Payload: fmt.Appendf(nil, `{"extra":{"note":%q}}`, scenarioText(random, 2000))})
		}

		if _, err := database.Write([]Envelope{{Sequence: entry.Sequence, InboxSize: entry.Size, Logs: logs, Events: events}}); err != nil {
			t.Error(err)

			return
		}

		written.Add(int64(len(body)) + 100*350 + 10*2100)
	}
}

// Where the time of a run over all projects goes outside its steps while
// ingest writes about 3 MB a second (ingestLoad): the run's loop as
// advance runs it, each batch timed apart into the wait for the write lock,
// the step's work, the commit (the issues and counters written, the synced
// write) and the rest between batches. Measured only on request:
// CLEANUP_TIMES=4 CLEANUP_TIMES_DIR=$PWD/data/bench-times go test ./internal/storage/database -run TestCleanupTimesUnderIngest -v -timeout 1h
// (CLEANUP_TIMES_RATE: MB a second, 3 by default; the directory on a disk, a sync on tmpfs costs nothing.)
func TestCleanupTimesUnderIngest(t *testing.T) {
	scale, _ := strconv.Atoi(os.Getenv("CLEANUP_TIMES"))

	if scale == 0 {
		t.Skip("set CLEANUP_TIMES to a scale")
	}

	dir := t.TempDir()

	if base := os.Getenv("CLEANUP_TIMES_DIR"); base != "" {
		if err := os.MkdirAll(base, 0o700); err != nil {
			t.Fatal(err)
		}

		dir, _ = os.MkdirTemp(base, "run-")

		t.Cleanup(func() { _ = os.RemoveAll(dir) })
	}

	database := openOnDisk(t, dir)
	seedShares(t, database, 100)

	random := rand.New(rand.NewPCG(3, 3))

	for range 40 * scale {
		scenarioLogs(t, database, random, 1, 5000, "info")
	}

	scenarioEvents(t, database, random, 1, "single", 10000*scale, 1, "warning", time.Now().Add(-time.Hour))
	scenarioEvents(t, database, random, 1, "loop", 50, 40*scale, "error", time.Now().Add(-time.Hour))
	time.Sleep(5 * time.Second) // goleveldb's own compactions settle

	stop := make(chan struct{})

	var (
		written atomic.Int64
		writer  sync.WaitGroup
	)

	rate, _ := strconv.Atoi(os.Getenv("CLEANUP_TIMES_RATE"))

	writer.Go(func() { ingestLoad(t, database, stop, int64(max(rate, 3))<<20, &written) })
	time.Sleep(3 * time.Second)

	live, _ := measuredUsage(t, database)

	started := time.Now()
	run, err := database.startCleanup(cleanupTarget{AllProjects, live * 100 / 120})
	startTook := time.Since(started)

	if err != nil || run.Status != cleanupRunning {
		t.Fatalf("run %+v, %v", run, err)
	}

	steps := database.cleanupSteps(AllProjects)
	scope := database.scope(AllProjects)

	var lockWait, work, commit, rest, counting time.Duration

	batches := 0

	for run.Reached <= len(steps) {
		step := steps[run.Reached-1]
		before := time.Now()
		need := scope.counted() - run.Goal
		counting += time.Since(before)

		if !step.always && need <= 0 {
			break
		}

		var next CleanupRun

		var locked, worked time.Time

		began := time.Now()

		err := database.cleanupUpdate(func(writer *batchWriter) error {
			locked = time.Now()
			writer.counted = map[int]*CleanupCounts{}
			cursor := &cleanupCursor{batch: writer.batch, position: run.Position, need: need, always: step.always}
			finished, err := step.batch(writer, cursor)

			next = withBatch(run, step.name, writer.counted, -writer.batch.Counted(), time.Since(began))
			next.Position = cursor.position

			if finished {
				next.Reached, next.Position = next.Reached+1, nil
			}

			worked = time.Now()

			return errors.Join(err, database.cleanups.Save(writer.batch, next))
		})

		committed := time.Now()

		if err != nil {
			t.Fatal(err)
		}

		run, batches = next, batches+1
		lockWait, work, commit = lockWait+locked.Sub(began), work+worked.Sub(locked), commit+committed.Sub(worked)

		time.Sleep(cleanupRest)

		rest += time.Since(committed)
	}

	before := time.Now()
	err = database.finishCleanup(&run, nil, "")
	finishTook := time.Since(before)

	close(stop)
	writer.Wait()

	var stepsTook time.Duration

	for _, step := range run.Steps {
		stepsTook += step.Duration
		t.Logf("step %d %s: %s", step.Number, step.Name, stepLine(step))
	}

	total := time.Since(started)
	t.Logf("run %s, steps %s, outside them %s; %d batches: waiting for the write lock %s, the steps' work %s, commits %s, rests %s, "+
		"reading the counters %s, start (sizes) %s, finish (size, save) %s; %d MB written meanwhile, err %v",
		total.Round(time.Millisecond), stepsTook.Round(time.Millisecond), (total - stepsTook).Round(time.Millisecond), batches,
		lockWait.Round(time.Millisecond), work.Round(time.Millisecond), commit.Round(time.Millisecond), rest.Round(time.Millisecond),
		counting.Round(time.Millisecond), startTook.Round(time.Millisecond), finishTook.Round(time.Millisecond), written.Load()>>20, err)
}

// A compaction that another took the stale bytes off from while it was
// stopped between its parts (the compaction before a new run, a project's
// purge) takes nothing off again: logs deleted after that stay counted,
// though they may lie in the parts it had passed.
func TestCompactionsTakeStaleOffOnce(t *testing.T) {
	database := liveStore(t)
	run := pendingRun(t, database)
	ctx, cancel := context.WithCancel(context.Background())
	database.compactFault = func() error {
		database.compactFault = nil

		cancel()

		return nil
	}

	if err := database.CompactCleanups(ctx, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}

	if _, err := database.compact(true, database.staleData()...); err != nil || database.staleBytes() != 0 {
		t.Fatalf("stale %d after compacting, %v", database.staleBytes(), err)
	}

	var ids []int64

	err := database.logs.Primary().Keys(database.db, func(key kv.Key) (bool, error) {
		ids = append(ids, int64(sequenceOf(key)))

		return len(ids) < 50, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if deleted, err := database.DeleteLogs(context.Background(), store.Access{}, ids); err != nil || deleted != len(ids) {
		t.Fatalf("deleted %d of %d, %v", deleted, len(ids), err)
	}

	deleted := database.staleBytes()

	compactCleanups(t, database)

	if done := storedRun(t, database, run.Sequence).Compaction; done.Status != store.CompactionDone || database.staleBytes() != deleted || deleted == 0 {
		t.Errorf("stale %d after the compaction %+v, %d deleted after the other", database.staleBytes(), done, deleted)
	}
}

// A compaction part holds ingest's writes for its time: goleveldb's range
// compaction goes on for as long as new files overlap its range, so none may
// come. An envelope is refused at once (ErrCompacting, ingest answers 429),
// a batch of the writer waits for the part to end and is written then.
func TestCompactionPartHoldsIngest(t *testing.T) {
	database := liveStore(t)
	written := make(chan error, 1)

	var appendErr error

	database.whileCompacting = func() {
		database.whileCompacting = nil
		_, appendErr = database.Append(1, scenarioNow, []byte(`{}`))

		go func() {
			_, err := database.Write([]Envelope{{}})

			written <- err
		}()

		select {
		case err := <-written:
			t.Errorf("a batch was written during a compaction part (%v)", err)
		case <-time.After(200 * time.Millisecond):
		}
	}

	if finished, err := database.compact(true, database.logs); err != nil || !finished {
		t.Fatal(finished, err)
	}

	if !errors.Is(appendErr, ErrCompacting) {
		t.Errorf("an envelope during a part: %v, want ErrCompacting", appendErr)
	}

	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the batch held by the part was never written")
	}
}

// The compaction after a run, and the stale compaction of the size check,
// wait until ingest is quiet, since each part refuses ingest while it runs:
// under a flood they stay pending and compact nothing, the run not counting
// an attempt; once it is quiet the compaction is done.
func TestCompactionWaitsForQuietIngest(t *testing.T) {
	database, engine := openRecording(t, t.TempDir())
	liveData(t, database)
	run := pendingRun(t, database)

	quietFor = time.Hour

	t.Cleanup(func() { quietFor = 0 })
	database.lastAppend.Store(time.Now().UnixNano())
	engine.taken()
	compactCleanups(t, database)

	if compacted, err := database.compact(false, database.staleData()...); err != nil || compacted {
		t.Errorf("the stale compaction under ingest: compacted %v, %v", compacted, err)
	}

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionPending || compaction.Attempts != 0 ||
		compaction.Running || len(engine.taken()) != 0 {
		t.Fatalf("under ingest: %+v", compaction)
	}

	quietFor = 0

	compactCleanups(t, database)

	if compaction := storedRun(t, database, run.Sequence).Compaction; compaction.Status != store.CompactionDone || len(engine.taken()) == 0 {
		t.Fatalf("once quiet: %+v", compaction)
	}
}

// A run done that deleted something is told to OnCleanupDone once, with
// its steps; a check under the limit tells nothing.
func TestCleanupDoneIsTold(t *testing.T) {
	database := liveStore(t)

	var told []CleanupRun

	database.OnCleanupDone(func(run CleanupRun) { told = append(told, run) })

	live, _ := measuredUsage(t, database)

	if _, err := database.Cleanup(context.Background(), AllProjects, live*10); err != nil {
		t.Fatal(err)
	}

	if len(told) != 0 {
		t.Fatalf("a check under the limit told %d runs", len(told))
	}

	run := pendingRun(t, database)

	if len(told) != 1 || told[0].Sequence != run.Sequence || told[0].Status != store.CleanupDone || len(told[0].Steps) == 0 {
		t.Fatalf("told %+v, want the run %d with its steps", told, run.Sequence)
	}
}
