package database

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/background"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/logging"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The cleanup by size; what it deletes and in which order: docs/configuration.md (Retention).
const (
	cleanupStart  = float64(config.CleanupStartPercent) / 100
	cleanupFiles  = float64(config.CleanupFilesPercent) / 100
	cleanupChunk  = 2000
	cleanupPassed = 10 * cleanupChunk
	cleanupDecode = 2 << 20
	cleanupRest   = 2 * time.Millisecond
)

// The records of a run are the store's.
type (
	CleanupRun    = store.CleanupRun
	CleanupStep   = store.CleanupStep
	CleanupCounts = store.CleanupCounts
)

const (
	cleanupRunning = store.CleanupRunning
	cleanupDone    = store.CleanupDone
	cleanupFailed  = store.CleanupFailed
)

// quietFor is how long ingest must be quiet before a forced compaction:
// under writes goleveldb repeats a range compaction as long as new files come.
var quietFor = 2 * time.Second

func (database *Store) quiet() bool {
	return time.Since(time.Unix(0, database.lastAppend.Load())) >= quietFor
}

// CompactInbox takes processed envelopes, credentials included, out of the
// files, when ingest is quiet: a deleted record keeps its bytes until a
// compaction reaches its key.
func (database *Store) CompactInbox(context.Context) error {
	if !database.quiet() || database.backups.Load() > 0 {
		return nil
	}

	_, err := database.compact(false, database.inbox)

	return err
}

// CleanupWake ticks after every config.CleanupWritten bytes written.
func (database *Store) CleanupWake() <-chan struct{} { return database.cleanupWake }

// Lane is the periodic jobs of the database (schedule: config/schedule.go),
// for the server and the load tests alike.
func (database *Store) Lane(maxBytes int64, logger *slog.Logger) background.Lane {
	return background.Lane{Name: "database", Jobs: []background.Job{
		{Name: "inbox compaction", AtStart: true, First: config.InboxCompactionAt, Every: config.CleanupInterval, Timeout: config.InboxCompactionTimeout,
			Note: "when ingest is quiet", Run: database.CompactInbox},
		{Name: "size check", AtStart: true, First: config.CleanupInterval, Every: config.CleanupInterval, Timeout: config.SizeCheckTimeout,
			Wake: database.CleanupWake(), Note: fmt.Sprintf("after %s written", config.FormatSize(config.CleanupWritten)),
			Run: func(ctx context.Context) error { return database.CheckSize(ctx, maxBytes, logger) }},
		{Name: "cleanup compaction", AtStart: true, First: config.CleanupCompactionAt, Every: config.CleanupInterval, Timeout: config.CleanupCompactionTimeout,
			Wake: database.compactionWake, Note: "after every cleanup that deleted something",
			Run: func(ctx context.Context) error { return database.CompactCleanups(ctx, logger) }},
		{Name: "project purge", AtStart: true, First: config.ProjectPurgeAt, Every: config.ProjectPurgeInterval, Timeout: config.ProjectPurgeTimeout,
			Note: "the data of deleted projects", Run: database.PurgeProjects},
		{Name: "project sizes", AtStart: true, First: config.BackfillAt, Every: config.CleanupInterval, Timeout: config.BackfillTimeout,
			Note: "counts data stored before them, once", Run: database.Backfill},
	}}
}

// AllProjects is the scope of the cleanup over the whole database; any other is a project's ID.
const AllProjects = 0

// CheckSize thins the events that turned a month old and runs the cleanups due (cleanupPass).
func (database *Store) CheckSize(ctx context.Context, maxBytes int64, logger *slog.Logger) error {
	if maxBytes > 0 {
		reduced, err := database.ReduceAging(ctx)

		if err != nil {
			return fmt.Errorf("thinning old events: %w", err)
		}

		if len(reduced) > 0 {
			logger.Debug("old events thinned", "step", stepLine(reduced[0]))
		}
	}

	runs, err := database.cleanupPass(ctx, maxBytes)

	for _, run := range runs {
		if run.Status == cleanupDone {
			database.logCleanup(logger, run)
		}
	}

	return err
}

// runFailed labels an error of a run with its scope and step; nil stays nil.
func (database *Store) runFailed(run CleanupRun, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("cleanup of %s, step %d: %w", database.scope(run.Project).name(), run.Reached, err)
}

// logCleanup writes a run as one record: sizes, the last step reached and a line per step that deleted something.
func (database *Store) logCleanup(logger *slog.Logger, run CleanupRun) {
	attributes := []any{logging.Block, "scope", database.scope(run.Project).name(), "limit", config.FormatSize(run.Limit), "size_before", config.FormatSize(run.SizeBefore)}

	if run.Compacted != run.SizeBefore {
		attributes = append(attributes, "compacted", config.FormatSize(run.Compacted))
	}

	attributes = append(attributes, "size_after", config.FormatSize(run.SizeAfter))

	if run.Files > 0 {
		attributes = append(attributes, "files", fmt.Sprintf("%s, over %d%% of the limit after compacting", config.FormatSize(run.Files), config.CleanupFilesPercent))
	}

	if run.Reached > 0 {
		attributes = append(attributes, "reached", fmt.Sprintf("step %d", run.Reached))
	} else {
		attributes = append(attributes, "deleted", "nothing, compacting was enough")
	}

	attributes = append(attributes, "duration", run.Duration.Round(time.Millisecond))

	for _, step := range run.Steps {
		attributes = append(attributes, fmt.Sprintf("step_%d", step.Number), stepLine(step))

		if step.Number == orphanStep {
			logger.Warn("records of missing issues deleted, which only a bug leaves", "scope", database.scope(run.Project).name(), "deleted", stepLine(step))
		}
	}

	logger.Info("cleanup", attributes...)
}

// stepLine reads "logs below error, freed 1.2 MB in 40ms: project 1: 300 logs; project 2: 12 logs".
func stepLine(step CleanupStep) string {
	var projects []string

	for _, project := range slices.Sorted(maps.Keys(step.Projects)) {
		projects = append(projects, fmt.Sprintf("project %d: %s", project, step.Projects[project]))
	}

	return fmt.Sprintf("%s, freed %s in %s: %s", step.Name, config.FormatSize(step.Freed), step.Duration.Round(time.Millisecond),
		strings.Join(projects, "; "))
}

func (database *Store) noteWritten() {
	written := database.db.Written()

	if written-database.checkedAt.Load() < config.CleanupWritten {
		return
	}

	database.checkedAt.Store(written)

	select {
	case database.cleanupWake <- struct{}{}:
	default:
	}
}

// cleanupPass is one pass of the size check: it goes on with a run left
// running, then starts the runs of cleanupTargets that are due, one after
// another. A run stopped by ctx stays running for the next pass. It returns
// the runs that ran.
func (database *Store) cleanupPass(ctx context.Context, maxBytes int64) ([]CleanupRun, error) {
	// Deleted issues, a purge or thinned events since the last pass; a run's
	// deleted records are left to its compaction, which records them.
	pending, err := database.pendingCompactions()

	if err == nil && len(pending) == 0 {
		_, err = database.compactStale()
	}

	if err != nil {
		return nil, fmt.Errorf("cleanup: %w", err)
	}

	runs, err := database.resumeCleanup(ctx, maxBytes)

	if err != nil || ctx.Err() != nil {
		return runs, err
	}

	targets, err := database.cleanupTargets(maxBytes)

	if err != nil {
		return runs, fmt.Errorf("cleanup: %w", err)
	}

	for _, target := range targets {
		run, err := database.cleanup(ctx, target)

		if run.Status != "" {
			runs = append(runs, run)
		}

		if err != nil {
			return runs, database.runFailed(run, err)
		}

		if ctx.Err() != nil {
			return runs, nil
		}
	}

	return runs, nil
}

// resumeCleanup goes on with the run a restart or the pass's time limit
// stopped, or ends it when its scope's limit changed meanwhile.
func (database *Store) resumeCleanup(ctx context.Context, maxBytes int64) ([]CleanupRun, error) {
	run, err := database.runningCleanup()

	if err != nil {
		return nil, fmt.Errorf("cleanup: %w", err)
	}

	if run.Status != cleanupRunning {
		return nil, nil
	}

	reason, err := database.stopReason(run, maxBytes)

	if err == nil && reason != "" {
		err = database.finishCleanup(&run, nil, reason)
	}

	if err != nil || reason != "" {
		return nil, database.runFailed(run, err)
	}

	err = database.advance(ctx, &run)

	if ctx.Err() == nil {
		err = database.finishCleanup(&run, err, "")
	}

	return []CleanupRun{run}, database.runFailed(run, err)
}

// runningCleanup is the newest run when it is still running, a zero run otherwise.
func (database *Store) runningCleanup() (CleanupRun, error) {
	latest, err := database.cleanups.Primary().Newest().First(database.db)

	if errors.Is(err, kv.ErrNotFound) || (err == nil && latest.Record.Status != cleanupRunning) {
		return CleanupRun{}, nil
	}

	return latest.Record, err
}

// errCleanupRunning: a run stopped by a restart or ctx is still to finish.
var errCleanupRunning = errors.New("a cleanup is still running")

// Cleanup is cleanup while no run is running: the size check goes on only with the newest.
func (database *Store) Cleanup(ctx context.Context, project int, limit int64) (CleanupRun, error) {
	if running, err := database.runningCleanup(); err != nil || running.Status != "" {
		return CleanupRun{}, cmp.Or(err, errCleanupRunning)
	}

	return database.cleanup(ctx, cleanupTarget{project, limit})
}

// cleanup starts a run when its scope is due and no backup runs, takes it
// as far as needed and saves it. A run stopped by ctx stays running.
func (database *Store) cleanup(ctx context.Context, target cleanupTarget) (CleanupRun, error) {
	// A backup's snapshot keeps deleted records in the files: neither deleting
	// nor compacting frees space while it runs, and a compaction would not
	// free what the stale counts say it did.
	if target.limit <= 0 || database.backups.Load() > 0 {
		return CleanupRun{}, nil
	}

	run, err := database.startCleanup(target)

	if err != nil || run.Status != cleanupRunning {
		return run, err
	}

	err = database.advance(ctx, &run)

	if ctx.Err() != nil {
		return run, err
	}

	return run, database.finishCleanup(&run, err, "")
}

// startCleanup saves a run when its scope's data is at cleanupStart of the
// limit, or over all projects when the files are over cleanupFiles of it
// and still are after compactBefore. At cleanupStart alone it compacts
// nothing: that moves the data held by a percent or two
// (TestCompactingBeforeAtScale), the old versions of issues goleveldb frees
// on its own. Its goal, in bytes counted, frees
// CleanupRun.Need; a deleted project gets none.
func (database *Store) startCleanup(target cleanupTarget) (CleanupRun, error) {
	run := CleanupRun{StartedAt: database.now().UTC(), Limit: target.limit, Project: target.project, Order: cleanupOrder()}
	scope := database.scope(target.project)
	due := func(size int64) bool { return float64(size) >= cleanupStart*float64(run.Limit) }
	overflows := func(usage diskUsage) bool {
		return scope.all() && float64(usage.files) > cleanupFiles*float64(run.Limit)
	}

	share, found, err := scope.share()

	if err != nil || !found {
		return run, err
	}

	run.Share = share
	size, usage, err := scope.size()

	if err != nil || (!due(size) && !overflows(usage)) {
		return run, err
	}

	run.SizeBefore, run.Compacted = size, size

	if overflows(usage) {
		compacted, err := scope.compactBefore()

		if err == nil && compacted {
			run.Compacted, usage, err = scope.size()
		}

		if err != nil {
			return run, err
		}
	}

	if overflows(usage) {
		run.Files = usage.files
	}

	if !due(run.Compacted) && run.Files == 0 {
		run.Status, run.SizeAfter = cleanupDone, run.Compacted
		run.Duration = database.now().Sub(run.StartedAt)

		return run, database.saveCleanup(&run)
	}

	run.Goal = scope.counted() - int64(float64(run.Need(config.CleanupStopPercent))/max(usage.ratio, 1e-9))
	run.Ratio = usage.ratio
	run.Status, run.Reached = cleanupRunning, 1

	return run, database.saveCleanup(&run)
}

// finishCleanup saves a run, failed with err or the reason it was stopped,
// otherwise done; one that deleted something waits for its compaction
// (CompactCleanups), which it wakes.
func (database *Store) finishCleanup(run *CleanupRun, err error, stopped string) error {
	if err == nil {
		run.SizeAfter, _, err = database.scope(run.Project).size()
	}

	if len(run.Steps) > 0 {
		run.Compaction = &store.CleanupCompaction{Status: store.CompactionPending}

		defer func() {
			select {
			case database.compactionWake <- struct{}{}:
			default:
			}
		}()
	}

	run.Status, run.Duration, run.Error = cleanupDone, database.now().Sub(run.StartedAt), stopped

	if err != nil {
		run.Error = err.Error()
	}

	if run.Error != "" {
		run.Status = cleanupFailed
	}

	return errors.Join(err, database.db.Update(true, func(batch *kv.Batch) error { return database.cleanups.Save(batch, *run) }))
}

// saveCleanup stores a new run and keeps the newest store.CleanupRunsKept of them.
func (database *Store) saveCleanup(run *CleanupRun) error {
	sequence, err := database.db.Next()

	if err != nil {
		return err
	}

	run.Sequence = sequence

	return database.db.Update(true, func(batch *kv.Batch) error {
		if err := database.cleanups.Insert(batch, *run); err != nil {
			return err
		}

		var stored []kv.Key

		err := database.cleanups.Primary().Keys(database.db, func(key kv.Key) (bool, error) {
			stored = append(stored, key)

			return true, nil
		})

		for _, key := range stored[:max(0, len(stored)+1-store.CleanupRunsKept)] {
			if err == nil {
				_, err = database.cleanups.DeleteKey(batch, key)
			}
		}

		return err
	})
}

func (database *Store) CleanupHistory(_ context.Context, limit int) ([]CleanupRun, error) {
	return database.cleanups.Primary().Newest().Limit(limit).All(database.db)
}

func (database *Store) logicalSize() int64 {
	var total int64

	for _, size := range database.db.Counters() {
		total += size
	}

	return total
}

// The ranks of the levels below error (logs and events share them: warning
// is warn): those logs go whenever the cleanup runs, those events before any
// error; error and fatal ones only while needed.
var (
	minorRanks = allRanks[:levelRank("error")]
	majorRanks = allRanks[levelRank("error"):]
)

// cleanupStep deletes in batches: each batch reads the data as it is then,
// under the write lock, so what arrives meanwhile is judged by its rule too.
type cleanupStep struct {
	name   string
	always bool // runs in full; the others stop once the goal is reached
	batch  cleanupBatch
}

type cleanupBatch func(writer *batchWriter, cursor *cleanupCursor) (finished bool, err error)

// cleanupCursor is one batch of a step: where it starts and goes on, and how much it may do.
type cleanupCursor struct {
	batch    *kv.Batch
	position []byte
	need     int64 // bytes still to free; only steps that are not always stop there
	always   bool
	records  int
	passed   int // keys passed over without reading their records
	decoded  int // bytes of payloads parsed (thinning), which is what takes the time
}

// full says the batch has done enough: cleanupChunk records, cleanupPassed
// keys passed over or cleanupDecode bytes parsed (the write lock is held
// meanwhile), or freed what is needed.
func (cursor *cleanupCursor) full() bool {
	return cursor.records >= cleanupChunk || cursor.passed >= cleanupPassed || cursor.decoded >= cleanupDecode ||
		(!cursor.always && -cursor.batch.Counted() >= cursor.need)
}

func (cursor *cleanupCursor) number(offset int) uint64 {
	return kv.Decode(cursor.position, offset).U64()
}

// cleanupSteps are the steps in the order of docs/configuration.md; their
// number is their place. Each reads and deletes only the records of project
// (AllProjects: every record).
func (database *Store) cleanupSteps(project int) []cleanupStep {
	scope := database.scope(project)
	now := database.now()
	month, week := now.Add(-config.CleanupAge), now.Add(-config.CleanupRecentAge)

	return []cleanupStep{
		{"events and status changes of missing issues", true, scope.deleteOrphans()},
		{"logs older than a month", true, scope.deleteLogs(allRanks, func() uint64 { return database.sequenceFrom(month) })},
		// By first seen: an issue first seen within the month has no older event, so the walk stops at the first one.
		{"events older than a month beyond the newest of their issue", true, scope.trimIssues(config.OldEventsPerIssue, month, func(issue *issueRecord) bool {
			return len(issue.EventDays) > 0 && issue.EventDays[0].Day <= dayOf(month)
		}, issueWalk{index: "first", beyond: func(issue *issueRecord) bool { return dayOf(issue.FirstSeen) > dayOf(month) }})},
		// Only the resolved and ignored issues (an ignore whose time passed reads unresolved).
		{"events of resolved and ignored issues but the latest", true, scope.trimIssues(1, time.Time{}, func(issue *issueRecord) bool {
			return issue.At(now).Status != store.StatusUnresolved
		}, issueWalk{index: "seen", statuses: []string{store.StatusResolved, store.StatusIgnored}})},
		{"logs below error, oldest first", false, scope.deleteOldestLogs(minorRanks)},
		{"events below error but the latest of their issue", false, scope.deleteOldestEvents(time.Time{}, minorRanks)},
		{"events beyond the newest of their issue", false, scope.trimCopies()},
		{"error and fatal logs", false, scope.deleteOldestLogs(majorRanks)},
		{"events older than a week", false, scope.deleteOldestEvents(week, allRanks)},
		{"events of the last week", false, scope.deleteOldestEvents(time.Time{}, allRanks)},
		{"latest events of issues not seen for a week", false, scope.deleteUnseen(week, false)},
		{"whole issues", false, scope.deleteUnseen(time.Time{}, true)},
	}
}

// CleanupStepName is a step of the cleanup as the web app lists it.
type CleanupStepName struct {
	Name   string
	Always bool // runs in full; the others only while needed
}

// CleanupSteps are the names of the cleanup's steps, in order.
func CleanupSteps() []CleanupStepName {
	var names []CleanupStepName

	for _, step := range (&Store{now: time.Now}).cleanupSteps(AllProjects) {
		names = append(names, CleanupStepName{step.name, step.always})
	}

	return names
}

// cleanupOrder names the steps' order, saved with a run (CleanupRun.Order):
// a run left running under other steps ends, since its step and position
// would be another step's.
// cleanupPositions is raised when a step's position changes meaning (2: steps 3 and 4 walk issue indexes, step 7 none).
const cleanupPositions = 2

func cleanupOrder() string {
	hash := fnv.New64a()

	// The revision of the steps' positions: a step whose position changes meaning raises it.
	_, _ = fmt.Fprintln(hash, cleanupPositions)

	for _, step := range CleanupSteps() {
		_, _ = fmt.Fprintf(hash, "%s|%t\n", step.Name, step.Always)
	}

	return strconv.FormatUint(hash.Sum64(), 16)
}

// cleanupScope is what a run cleans: one project's records, or all of them
// (AllProjects). Its methods hold every difference between the two runs.
type cleanupScope struct {
	database *Store

	project int
}

func (database *Store) scope(project int) cleanupScope {
	return cleanupScope{database: database, project: project}
}

func (scope cleanupScope) all() bool { return scope.project == AllProjects }

// name names the scope in the log.
func (scope cleanupScope) name() string {
	if scope.all() {
		return "all projects"
	}

	if stored, err := scope.database.projects.Get(scope.database.db, projectKey(scope.project)); err == nil {
		return stored.Name
	}

	return fmt.Sprintf("project %d", scope.project)
}

// share is a project's disk share, 0 over all projects; found is false
// for a project deleted meanwhile.
func (scope cleanupScope) share() (int, bool, error) {
	if scope.all() {
		return 0, true, nil
	}

	project, err := scope.database.projects.Get(scope.database.db, projectKey(scope.project))

	if errors.Is(err, kv.ErrNotFound) {
		return 0, false, nil
	}

	return project.Share(), err == nil, err
}

// counted is what the scope holds, in bytes before compression.
func (scope cleanupScope) counted() int64 {
	if scope.all() {
		return scope.database.logicalSize()
	}

	return scope.database.projectBytes(scope.project)
}

// size is the scope's data as the cleanup measures it: what the database
// holds (liveSize) or a project's part of it, with how the files divide.
func (scope cleanupScope) size() (int64, diskUsage, error) {
	if scope.all() {
		return scope.database.liveSize()
	}

	usage, err := scope.database.diskUsage()

	return usage.onDisk(scope.database.projectBytes(scope.project)), usage, err
}

// compactBefore compacts, over all projects whose files overflow, what
// holds deleted records: the logs and events with deleted or replaced
// records in them, the inbox and the issues rewritten with every event
// (under ingest too); the files then hold no deleted records, so they start
// a run only when deleting is what frees them. It says whether it compacted.
// A project's run compacts nothing before: that would rewrite every
// project's records, and its size leaves out what they hold (diskUsage).
func (scope cleanupScope) compactBefore() (bool, error) {
	database := scope.database

	if !scope.all() {
		return false, nil
	}

	collections := append(database.staleData(), database.inbox, database.issues, database.status)

	// Forced: the files are over the limit, so it compacts under ingest too, each part refusing it for its time.
	return database.compact(true, collections...)
}

// logs walks the logs of a level by arrival.
func (scope cleanupScope) logs(rank byte) kv.Query[logRecord] {
	if scope.project == AllProjects {
		return scope.database.logs.Primary(kv.U8(rank))
	}

	return scope.database.logs.By("project", kv.U32(uint32(scope.project)), kv.U8(rank))
}

// events walks the events of a level by arrival.
func (scope cleanupScope) events(rank byte) kv.Query[eventRecord] {
	if scope.project == AllProjects {
		return scope.database.events.By("age", kv.U8(rank))
	}

	return scope.database.events.By("project", kv.U32(uint32(scope.project)), kv.U8(rank))
}

// has says whether an issue is in the scope.
func (scope cleanupScope) has(issue *issueRecord) bool {
	return scope.project == AllProjects || issue.ProjectID == scope.project
}

// agingSteps thin the events older than a month as they age, in the background; they delete nothing.
func (database *Store) agingSteps() []cleanupStep {
	return []cleanupStep{{"details of events older than a month", true, database.thinBefore(database.now().Add(-config.CleanupAge))}}
}

// ReduceAging runs agingSteps to the end and says what they did; only what
// aged since the last call is left to do, so it is cheap when run often.
func (database *Store) ReduceAging(ctx context.Context) ([]CleanupStep, error) {
	var done []CleanupStep

	for _, step := range database.agingSteps() {
		run := CleanupRun{Reached: 1}

		for finished := false; !finished && ctx.Err() == nil; {
			started := time.Now()

			err := database.update(func(writer *batchWriter) error {
				writer.counted = map[int]*CleanupCounts{}
				cursor := &cleanupCursor{batch: writer.batch, position: run.Position, always: true}

				var err error

				finished, err = step.batch(writer, cursor)
				// The run is in memory only: a restart starts over, and the steps skip what is done.
				writer.batch.AfterWrite(func() {
					run = withBatch(run, step.name, writer.counted, -writer.batch.Counted(), time.Since(started))
					run.Position = cursor.position
				})

				return err
			})

			if err != nil {
				return done, err
			}

			if !finished {
				select {
				case <-ctx.Done():
				case <-time.After(cleanupRest):
				}
			}
		}

		done = append(done, run.Steps...)
	}

	return done, ctx.Err()
}

// advance runs the run's steps batch by batch from where it is; each batch
// saves the run with it, so the progress and the deletes are written together.
func (database *Store) advance(ctx context.Context, run *CleanupRun) error {
	scope := database.scope(run.Project)
	steps := database.cleanupSteps(run.Project)

	for ctx.Err() == nil {
		if run.Reached > len(steps) {
			run.Reached = len(steps)

			return nil
		}

		step := steps[run.Reached-1]
		need := scope.counted() - run.Goal

		if !step.always && need <= 0 {
			// A step started has a position or deleted something; one not started does not count.
			if run.Position == nil && (len(run.Steps) == 0 || run.Steps[len(run.Steps)-1].Number != run.Reached) {
				run.Reached--
			}

			return nil
		}

		var next CleanupRun

		started := time.Now()

		err := database.cleanupUpdate(func(writer *batchWriter) error {
			writer.counted = map[int]*CleanupCounts{}
			cursor := &cleanupCursor{batch: writer.batch, position: run.Position, need: need, always: step.always}
			finished, err := step.batch(writer, cursor)

			if err != nil {
				return err
			}

			next = withBatch(*run, step.name, writer.counted, -writer.batch.Counted(), time.Since(started))
			next.Position = cursor.position

			if finished {
				next.Reached, next.Position = next.Reached+1, nil
			}

			return database.cleanups.Save(writer.batch, next)
		})

		if err != nil {
			return err
		}

		*run = next

		select {
		case <-ctx.Done():
		case <-time.After(cleanupRest):
		}
	}

	return ctx.Err()
}

// withBatch returns a copy of the run with a batch of its current step added.
func withBatch(run CleanupRun, name string, counted map[int]*CleanupCounts, freed int64, took time.Duration) CleanupRun {
	steps := make([]CleanupStep, len(run.Steps))

	for index, step := range run.Steps {
		step.Projects = maps.Clone(step.Projects)

		for project, counts := range step.Projects {
			copied := *counts
			step.Projects[project] = &copied
		}

		steps[index] = step
	}

	run.Steps = steps

	if len(counted) == 0 {
		return run
	}

	if len(steps) == 0 || steps[len(steps)-1].Number != run.Reached {
		run.Steps = append(run.Steps, CleanupStep{Number: run.Reached, Name: name, Projects: map[int]*CleanupCounts{}})
	}

	step := &run.Steps[len(run.Steps)-1]
	step.Freed += freed
	step.Duration += took

	for project, counts := range counted {
		total := step.Projects[project]

		if total == nil {
			total = &CleanupCounts{}
			step.Projects[project] = total
		}

		total.Add(*counts)
	}

	return run
}

// deleteLogs deletes logs of ranks, lowest level and oldest first, that
// arrived before the sequence bound returns (nil: any). Position: rank, sequence.
func (scope cleanupScope) deleteLogs(ranks []byte, bound func() uint64) cleanupBatch {
	database := scope.database

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		start, from := kv.Decode(cursor.position, 0).U8(), cursor.number(1)
		before := uint64(math.MaxUint64)

		if bound != nil {
			before = bound()
		}

		for _, rank := range ranks {
			if rank < start {
				continue
			}

			if rank > start {
				from = 0
			}

			full := false

			err := scope.logs(rank).From(kv.U64(from)).Before(kv.U64(before)).Each(database.db, func(entry kv.Entry[logRecord]) (bool, error) {
				if full = cursor.full(); full {
					return false, nil
				}

				database.logs.DeleteEntry(writer.batch, entry)
				writer.count(entry.Record.ProjectID).Logs++
				cursor.records++
				from = entry.Record.Sequence + 1

				return true, nil
			})

			if err != nil || full {
				cursor.position = kv.K(kv.U8(rank), kv.U64(from))

				return false, err
			}
		}

		return true, nil
	}
}

// deleteOldestLogs deletes logs of ranks oldest first, whatever their level. Position: sequence.
func (scope cleanupScope) deleteOldestLogs(ranks []byte) cleanupBatch {
	database := scope.database

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		from := cursor.number(0)
		queries := make([]kv.Query[logRecord], 0, len(ranks))

		for _, rank := range ranks {
			queries = append(queries, scope.logs(rank).From(kv.U64(from)))
		}

		full := false

		err := oldestFirst(database.db, queries, func(entry kv.Entry[logRecord]) bool {
			if full = cursor.full(); full {
				return false
			}

			database.logs.DeleteEntry(writer.batch, entry)
			writer.count(entry.Record.ProjectID).Logs++
			cursor.records++
			from = entry.Record.Sequence + 1

			return true
		})

		cursor.position = kv.K(kv.U64(from))

		return !full, err
	}
}

// eachIndexed visits the issues of the scope in the order of an index of
// issues (schema.go), from where the cursor stopped: the prefixes of the
// statuses given (nil: all), oldest first, until beyond says no
// later one can be wanted; accepts takes them as they are now (this batch
// holds the write lock); visit says whether it is done with one. Position:
// the index key after its prefix (value, ID) of the issue to go on with.
func (scope cleanupScope) eachIndexed(index string, statuses []string, beyond, accepts func(*issueRecord) bool,
	visit func(writer *batchWriter, cursor *cleanupCursor, issue *issueRecord) (bool, error),
) cleanupBatch {
	database := scope.database
	wants := func(issue *issueRecord) bool { return scope.has(issue) && accepts(issue) }

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		var filter store.IssueFilter

		if !scope.all() {
			filter.ProjectIDs = []int{scope.project}
		}

		var queries []kv.Query[issueRecord]

		for _, prefix := range database.issuePrefixes(filter) {
			if statuses != nil && !slices.Contains(statuses, prefix.status) {
				continue
			}

			query := database.issues.By(index, kv.U8(statusRank(prefix.status)), kv.U32(uint32(prefix.project)), kv.U8(levelRank(prefix.level)))

			if len(cursor.position) > 0 {
				query = query.FromRest(cursor.position)
			}

			queries = append(queries, query)
		}

		finished := true
		stop := func(entry kv.Entry[issueRecord]) {
			cursor.position, finished = bytes.Clone(entry.Rest), false
		}

		err := kv.Merge(writer.batch, queries, false, func(entry kv.Entry[issueRecord]) (bool, error) {
			if beyond != nil && beyond(&entry.Record) {
				return false, nil
			}

			stored, ok := writer.peekIssue(entry.Record.ID)
			wanted := ok && wants(stored)

			if !wanted {
				cursor.passed++
			}

			if cursor.full() {
				stop(entry)

				return false, nil
			}

			if !wanted {
				return true, nil
			}

			issue, _ := writer.issue(entry.Record.ID)
			done, err := visit(writer, cursor, issue)

			if err != nil || !done {
				stop(entry)

				return false, err
			}

			return true, nil
		})

		return finished, err
	}
}

// issueWalk is how a step walks the issues (eachIndexed): the index, oldest
// first, the statuses, and when no later issue can be wanted.
type issueWalk struct {
	index    string
	statuses []string
	beyond   func(*issueRecord) bool
}

// trimIssues deletes the oldest events received before cutoff (zero: any)
// beyond keep, of the issues match accepts (nil: all), in the walk's order.
func (scope cleanupScope) trimIssues(keep int, cutoff time.Time, match func(*issueRecord) bool, walk issueWalk) cleanupBatch {
	wants := func(issue *issueRecord) bool { return issue.Stored > keep && (match == nil || match(issue)) }

	return scope.eachIndexed(walk.index, walk.statuses, walk.beyond, wants, scope.trimEvents(keep, cutoff))
}

// trimCopies trims every issue down to its newest config.OldEventsPerIssue
// events, those storing the most first. It walks the trim index, which holds
// only the issues storing more, from its top at every batch: one trimmed
// leaves it, one stopped halfway is still at its top, and one that stored
// more meanwhile is in it again, so none is passed over (a position kept in
// an index of counts would pass over the issues whose count grew between
// batches). Until the index is backfilled, the issues not reached yet are not in it.
func (scope cleanupScope) trimCopies() cleanupBatch {
	database := scope.database
	trim := scope.trimEvents(config.OldEventsPerIssue, time.Time{})

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		projects := []int{scope.project}

		if scope.all() {
			projects = nil

			for _, prefix := range database.issuePrefixes(store.IssueFilter{}) {
				if !slices.Contains(projects, prefix.project) {
					projects = append(projects, prefix.project)
				}
			}
		}

		queries := make([]kv.Query[issueRecord], 0, len(projects))

		for _, project := range projects {
			queries = append(queries, database.issues.By("trim", kv.U32(uint32(project))))
		}

		finished := true

		err := kv.Merge(writer.batch, queries, true, func(entry kv.Entry[issueRecord]) (bool, error) {
			if cursor.full() {
				finished = false

				return false, nil
			}

			issue, ok := writer.issue(entry.Record.ID)

			if !ok || issue.Stored <= config.OldEventsPerIssue {
				cursor.passed++

				return true, nil
			}

			done, err := trim(writer, cursor, issue)

			if err != nil || !done {
				finished = false

				return false, err
			}

			return true, nil
		})

		return finished, err
	}
}

// trimEvents deletes an issue's oldest events received before cutoff (zero:
// any) beyond keep, and says whether it got to the end of them.
func (scope cleanupScope) trimEvents(keep int, cutoff time.Time) func(writer *batchWriter, cursor *cleanupCursor, issue *issueRecord) (bool, error) {
	database := scope.database

	return func(writer *batchWriter, cursor *cleanupCursor, issue *issueRecord) (bool, error) {
		done := true

		err := database.events.Primary(kv.Text(issue.ID)).Head().Each(database.db, func(entry kv.Entry[eventRecord]) (bool, error) {
			if issue.Stored <= keep || (!cutoff.IsZero() && !entry.Record.ReceivedAt.Before(cutoff)) {
				return false, nil
			}

			if cursor.full() {
				done = false

				return false, nil
			}

			writer.deleteEvent(entry)

			cursor.records++

			return true, nil
		})

		return done, err
	}
}

// deleteOldestEvents deletes events of ranks received before cutoff (zero:
// any), oldest first whatever their level among those, but not the latest of
// an issue. The events of issues with one stored event are passed over by
// their keys, as the issue table counts them, without being read.
// Position: sequence.
func (scope cleanupScope) deleteOldestEvents(cutoff time.Time, ranks []byte) cleanupBatch {
	database := scope.database

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		from := cursor.number(0)
		several := func(key kv.Key) bool {
			issue, ok := writer.peekIssue(kv.Decode(key, 0).Text())

			return ok && issue.Stored > 1
		}
		queries := make([]kv.Query[eventRecord], 0, len(ranks))

		for _, rank := range ranks {
			queries = append(queries, scope.events(rank).From(kv.U64(from)).Head().Keyed(several))
		}

		full := false
		// Keys passed over are not decoded, so they stop at the cutoff's sequence (time marks).
		var stopAt uint64

		if !cutoff.IsZero() {
			stopAt = database.sequenceFrom(cutoff)
		}

		err := oldestKeysFirst(database.db, queries, func(entry kv.Entry[eventRecord], kept bool) bool {
			if kept && !cutoff.IsZero() && !entry.Record.ReceivedAt.Before(cutoff) {
				return false
			}

			if !kept && stopAt > 0 && sequenceOf(entry.Key) >= stopAt {
				return false
			}

			if full = cursor.full(); full {
				return false
			}

			if !kept {
				cursor.passed++
				from = sequenceOf(entry.Key) + 1

				return true
			}

			from = entry.Record.Sequence + 1
			cursor.records++

			// The latest event of an issue is the one it was last seen with.
			if issue, ok := writer.issue(entry.Record.IssueID); ok && issue.Stored > 1 && entry.Record.ReceivedAt.Before(issue.LastSeen) {
				writer.deleteEvent(entry)
			}

			return true
		})

		cursor.position = kv.K(kv.U64(from))

		return !full, err
	}
}

// deleteUnseen deletes, longest unseen first, the events of the issues last
// seen before seenBefore (zero: any) or, with whole, the issues.
// Position: last seen and ID of the last issue done, then the ID of an issue
// whose events took more than a batch and the sequence its next batch starts at.
func (scope cleanupScope) deleteUnseen(seenBefore time.Time, whole bool) cleanupBatch {
	database := scope.database

	type unseen struct {
		lastSeen int64
		id       string
	}

	compare := func(first, second unseen) int {
		return cmp.Or(cmp.Compare(first.lastSeen, second.lastSeen), strings.Compare(first.id, second.id))
	}

	wants := func(issue *issueRecord) bool {
		return scope.has(issue) && (whole || issue.Stored > 0) && (seenBefore.IsZero() || issue.LastSeen.Before(seenBefore))
	}

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		after := unseen{math.MinInt64, ""}
		started, from := "", uint64(0)

		if len(cursor.position) > 0 {
			decoder := kv.Decode(cursor.position, 0)
			after = unseen{int64(decoder.U64()), decoder.Text()}
			started, from = decoder.Text(), decoder.U64()
		}

		take := func() bool {
			if cursor.full() {
				return false
			}

			cursor.records++

			return true
		}

		// Longest unseen first: the "seen" index of every status and level of
		// the scope's projects, merged, from where the last batch stopped.
		var filter store.IssueFilter

		if !scope.all() {
			filter.ProjectIDs = []int{scope.project}
		}

		var queries []kv.Query[issueRecord]

		for _, prefix := range database.issuePrefixes(filter) {
			query := database.issues.By("seen", kv.U8(statusRank(prefix.status)), kv.U32(uint32(prefix.project)), kv.U8(levelRank(prefix.level)))

			if after.id != "" {
				query = query.From(kv.U64(uint64(max(after.lastSeen, 0))))
			}

			queries = append(queries, query)
		}

		finished := true

		err := kv.Merge(writer.batch, queries, false, func(entry kv.Entry[issueRecord]) (bool, error) {
			candidate := unseen{entry.Record.LastSeen.UnixNano(), entry.Record.ID}

			if compare(candidate, after) <= 0 {
				return true, nil
			}

			// The index is in last seen order: past seenBefore, nothing more is wanted.
			if !seenBefore.IsZero() && !entry.Record.LastSeen.Before(seenBefore) {
				return false, nil
			}

			stored, ok := writer.peekIssue(candidate.id)
			wanted := ok && wants(stored)

			if !wanted {
				cursor.passed++
			}

			if cursor.full() {
				cursor.position, finished = kv.K(kv.U64(uint64(after.lastSeen)), kv.Text(after.id)), false

				return false, nil
			}

			if !wanted {
				return true, nil
			}

			if candidate.id != started {
				from = 0
			}

			issue, _ := writer.issue(candidate.id)
			next, done, err := writer.deleteIssueEvents(issue.ID, from, take)

			if err != nil || !done {
				cursor.position, finished = kv.K(kv.U64(uint64(after.lastSeen)), kv.Text(after.id), kv.Text(issue.ID), kv.U64(next)), false

				return false, err
			}

			if whole {
				if err := writer.deleteIssue(issue); err != nil {
					return false, err
				}

				cursor.records++
			}

			after = candidate

			return true, nil
		})

		return finished, err
	}
}

// count is the counts of a project in this batch; a batch outside the
// cleanup counts nothing.
func (writer *batchWriter) count(project int) *CleanupCounts {
	if writer.counted == nil {
		return &CleanupCounts{}
	}

	counts := writer.counted[project]

	if counts == nil {
		counts = &CleanupCounts{}
		writer.counted[project] = counts
	}

	return counts
}
