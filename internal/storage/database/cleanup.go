package database

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sort"
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
	cleanupStop   = float64(config.CleanupStopPercent) / 100
	cleanupChunk  = 2000
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

	return database.compactInbox()
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

// runFailed labels an error of a run with its scope and step.
func (database *Store) runFailed(run CleanupRun, err error) error {
	return fmt.Errorf("cleanup of %s, step %d: %w", database.scopeName(run.Project), run.Reached, err)
}

// scopeName names a run's scope in the log.
func (database *Store) scopeName(project int) string {
	if project == AllProjects {
		return "all projects"
	}

	if stored, err := database.projects.Get(database.db, projectKey(project)); err == nil {
		return stored.Name
	}

	return fmt.Sprintf("project %d", project)
}

// logCleanup writes a run as one record: sizes, the last step reached and a line per step that deleted something.
func (database *Store) logCleanup(logger *slog.Logger, run CleanupRun) {
	attributes := []any{logging.Block, "scope", database.scopeName(run.Project), "limit", config.FormatSize(run.Limit), "size_before", config.FormatSize(run.SizeBefore),
		"compacted", config.FormatSize(run.Compacted), "size_after", config.FormatSize(run.SizeAfter)}

	if run.Reached > 0 {
		attributes = append(attributes, "reached", fmt.Sprintf("step %d", run.Reached))
	} else {
		attributes = append(attributes, "deleted", "nothing, compacting was enough")
	}

	attributes = append(attributes, "duration", run.Duration.Round(time.Millisecond))

	for _, step := range run.Steps {
		attributes = append(attributes, fmt.Sprintf("step_%d", step.Number), stepLine(step))
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

// compactInbox takes processed envelopes, credentials included, out of the
// files: a deleted record keeps its bytes until a compaction reaches its key.
func (database *Store) compactInbox() error {
	return database.db.CompactRange(database.inbox.KeyRange())
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

// cleanupPass is one pass of the size check: it first goes on with a run a
// restart stopped (unless its limit changed meanwhile), then cleans each
// project over its disk share, the smallest share first, then all projects
// if the database is still at maxBytes (0: no limit). The runs go one after
// another, never two at once; a run stopped by ctx stays running and goes on
// at the next pass. It returns the runs that ran.
func (database *Store) cleanupPass(ctx context.Context, maxBytes int64) ([]CleanupRun, error) {
	var runs []CleanupRun

	run, err := database.runningCleanup()

	if err != nil {
		return nil, fmt.Errorf("cleanup: %w", err)
	}

	if run.Status == cleanupRunning {
		limit, reason, err := database.scopeLimit(run, maxBytes)

		if err != nil {
			return nil, database.runFailed(run, err)
		}

		if run.Limit != limit {
			// Its goal came from another limit: it ends, and the targets below start from the new one.
			if err := database.finishCleanup(&run, nil, reason); err != nil {
				return nil, database.runFailed(run, err)
			}
		} else {
			err = database.advance(ctx, &run)

			if ctx.Err() == nil {
				err = database.finishCleanup(&run, err, "")
			}

			if runs = append(runs, run); err != nil {
				return runs, database.runFailed(run, err)
			}

			if ctx.Err() != nil {
				return runs, nil
			}
		}
	}

	if maxBytes <= 0 {
		return runs, nil
	}

	targets, err := database.shareTargets(maxBytes)

	if err != nil {
		return runs, fmt.Errorf("cleanup: %w", err)
	}

	for _, target := range append(targets, cleanupTarget{AllProjects, maxBytes}) {
		run, err := database.cleanup(ctx, target.project, target.limit)

		if run.Status != "" {
			runs = append(runs, run)
		}

		if err != nil {
			return runs, database.runFailed(CleanupRun{Project: target.project, Reached: run.Reached}, err)
		}

		if ctx.Err() != nil {
			return runs, nil
		}
	}

	return runs, nil
}

// runningCleanup is the newest run when it is still running, a zero run otherwise.
func (database *Store) runningCleanup() (CleanupRun, error) {
	latest, err := database.cleanups.Primary().Newest().First(database.db)

	if errors.Is(err, kv.ErrNotFound) || (err == nil && latest.Record.Status != cleanupRunning) {
		return CleanupRun{}, nil
	}

	return latest.Record, err
}

// cleanupTarget is what a run cleans and down from which limit.
type cleanupTarget struct {
	project int
	limit   int64
}

// shareTargets are the projects with a disk share below 100%, the smallest
// first, each with its limit. None while data stored before the project
// counters is still counted: their bytes and the events' project index fall short.
func (database *Store) shareTargets(maxBytes int64) ([]cleanupTarget, error) {
	if !database.backfilled() {
		return nil, nil
	}

	projects, err := database.projects.Primary().All(database.db)

	if err != nil {
		return nil, err
	}

	slices.SortFunc(projects, func(first, second store.Project) int {
		return cmp.Or(cmp.Compare(first.Share(), second.Share()), cmp.Compare(first.ID, second.ID))
	})

	var targets []cleanupTarget

	for _, project := range projects {
		if limit := store.ShareLimit(maxBytes, project.Share()); limit > 0 {
			targets = append(targets, cleanupTarget{project.ID, limit})
		}
	}

	return targets, nil
}

// scopeLimit is the limit a run's scope has now (0: none), and why a run
// from another limit stops.
func (database *Store) scopeLimit(run CleanupRun, maxBytes int64) (int64, string, error) {
	if run.Project == AllProjects || maxBytes <= 0 {
		return max(maxBytes, 0), limitChanged(maxBytes), nil
	}

	project, err := database.projects.Get(database.db, projectKey(run.Project))

	if errors.Is(err, kv.ErrNotFound) {
		return 0, "stopped: the project was deleted", nil
	}

	if err != nil {
		return 0, "", err
	}

	limit := store.ShareLimit(maxBytes, project.Share())

	if run.Share != project.Share() {
		return limit, fmt.Sprintf("stopped: the disk share of %s changed to %d%%", project.Name, project.Share()), nil
	}

	return limit, limitChanged(maxBytes), nil
}

// errCleanupRunning: a run stopped by a restart or ctx is still to finish.
var errCleanupRunning = errors.New("a cleanup is still running")

// Cleanup is cleanup while no run is running: the size check goes on only with the newest.
func (database *Store) Cleanup(ctx context.Context, project int, limit int64) (CleanupRun, error) {
	if running, err := database.runningCleanup(); err != nil || running.Status != "" {
		return CleanupRun{}, cmp.Or(err, errCleanupRunning)
	}

	return database.cleanup(ctx, project, limit)
}

// cleanup cleans one scope, a project (its ID) or AllProjects, when it is at
// cleanupStart of limit and no backup runs: it starts a run, takes it as far
// as needed and saves it. A run stopped by ctx stays running; cleanupPass goes on with it.
func (database *Store) cleanup(ctx context.Context, project int, limit int64) (CleanupRun, error) {
	// A backup's snapshot keeps deleted records on disk, which would make the goal too low.
	if limit <= 0 || database.backups.Load() > 0 {
		return CleanupRun{}, nil
	}

	run, err := database.startCleanup(project, limit)

	if err != nil || run.Status != cleanupRunning {
		return run, err
	}

	err = database.advance(ctx, &run)

	if ctx.Err() != nil {
		return run, err
	}

	return run, database.finishCleanup(&run, err, "")
}

// startCleanup saves a run whose goal is cleanupStop of limit when its
// scope is at cleanupStart of it. Over all projects it first compacts, when
// ingest is quiet, what collects deleted and replaced records (the inbox,
// the issues rewritten with every event): that may be enough.
func (database *Store) startCleanup(project int, limit int64) (CleanupRun, error) {
	run := CleanupRun{StartedAt: database.now().UTC(), Limit: limit, Project: project}

	if project != AllProjects {
		return run, database.startProjectCleanup(&run)
	}

	size, err := database.db.Size()

	if err != nil || float64(size) < cleanupStart*float64(limit) {
		return run, err
	}

	run.SizeBefore = size

	if database.quiet() {
		ranges := []func() ([]byte, []byte){database.inbox.KeyRange, database.issues.KeyRange, database.status.KeyRange}

		if err := database.compact(append(ranges, database.uncompactedRanges()...)); err != nil {
			return run, err
		}
	}

	if run.Compacted, err = database.db.Size(); err != nil {
		return run, err
	}

	if float64(run.Compacted) < cleanupStart*float64(limit) {
		run.Status, run.SizeAfter = cleanupDone, run.Compacted
		run.Duration = database.now().Sub(run.StartedAt)

		return run, database.saveCleanup(&run)
	}

	// The counters count bytes before compression; the ratio converts.
	logical := database.logicalSize()
	ratio := float64(run.Compacted) / float64(max(logical, 1))
	run.Goal = logical - int64((float64(run.Compacted)-cleanupStop*float64(limit))/ratio)
	run.Status, run.Reached = cleanupRunning, 1

	return run, database.saveCleanup(&run)
}

// startProjectCleanup starts the run of a project at cleanupStart of its
// limit, by its part of the size on disk as the Projects page shows it;
// compacting is left to the run over all projects.
func (database *Store) startProjectCleanup(run *CleanupRun) error {
	project, err := database.projects.Get(database.db, projectKey(run.Project))

	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}

	if err != nil {
		return err
	}

	run.Share = project.Share()
	onDisk, ratio, err := database.projectOnDisk(run.Project)

	if err != nil || float64(onDisk) < cleanupStart*float64(run.Limit) {
		return err
	}

	run.SizeBefore, run.Compacted = onDisk, onDisk
	run.Goal = int64(cleanupStop * float64(run.Limit) / ratio)
	run.Status, run.Reached = cleanupRunning, 1

	return database.saveCleanup(run)
}

// projectOnDisk is a project's part of the size on disk, its bytes counted
// times diskRatio, and that ratio.
func (database *Store) projectOnDisk(project int) (int64, float64, error) {
	ratio, err := database.diskRatio()

	return int64(float64(database.projectBytes(project)) * ratio), ratio, err
}

// diskRatio is the size on disk of a byte counted. The inbox's files, with
// the envelopes processed but not compacted away, are no project's; what
// single projects' runs deleted is still in the files, so it is counted as
// if it were there, until compacted.
func (database *Store) diskRatio() (float64, error) {
	size, err := database.db.Size()

	if err != nil {
		return 0, err
	}

	inbox, err := database.db.SizeOf(database.inbox.KeyRange())

	if err != nil {
		return 0, err
	}

	return float64(max(size-inbox, 0)) / float64(max(database.logicalSize()+database.uncompacted.Load(), 1)), nil
}

// scopeBytes is what a run's scope holds, before compression.
func (database *Store) scopeBytes(project int) int64 {
	if project == AllProjects {
		return database.logicalSize()
	}

	return database.projectBytes(project)
}

// finishCleanup saves a run, failed with err or the reason it was stopped,
// otherwise done. A run over all projects first compacts what it deleted
// from. A project's run does not: compacting a collection rewrites every
// project's records in it, so the space comes back when the database
// compacts those keys on its own or the run over all projects does.
func (database *Store) finishCleanup(run *CleanupRun, err error, stopped string) error {
	if err == nil && run.Project == AllProjects {
		err = database.compactCleaned(run)
	}

	if err == nil && run.Project == AllProjects {
		run.SizeAfter, err = database.db.Size()
	} else if err == nil {
		run.SizeAfter, _, err = database.projectOnDisk(run.Project)
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

// limitChanged is why a run whose goal came from another limit than maxBytes stops.
func limitChanged(maxBytes int64) string {
	if maxBytes <= 0 {
		return "stopped: MAX_DATABASE_SIZE turned off"
	}

	return "stopped: MAX_DATABASE_SIZE changed to " + config.FormatSize(maxBytes)
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

// compactCleaned compacts the collections the run deleted from, and those
// projects' runs deleted from, so the disk space comes back now.
func (database *Store) compactCleaned(run *CleanupRun) error {
	var logs, events, issues bool

	for _, step := range run.Steps {
		for _, counts := range step.Projects {
			logs = logs || counts.Logs > 0
			events = events || counts.Events > 0 || counts.Thinned > 0
			issues = issues || counts.Issues > 0
		}
	}

	pending := database.uncompacted.Load() > 0

	var ranges []func() ([]byte, []byte)

	if logs || pending {
		ranges = append(ranges, database.logs.KeyRange)
	}

	if events || pending {
		ranges = append(ranges, database.events.KeyRange)
	}

	if issues {
		ranges = append(ranges, database.issues.KeyRange, database.status.KeyRange)
	}

	return database.compact(ranges)
}

// uncompactedRanges are the logs and events, when projects' runs deleted from them since they were last compacted.
func (database *Store) uncompactedRanges() []func() ([]byte, []byte) {
	if database.uncompacted.Load() == 0 {
		return nil
	}

	return []func() ([]byte, []byte){database.logs.KeyRange, database.events.KeyRange}
}

// compact compacts key ranges; with the logs and events among them, what
// projects' runs deleted is out of the files.
func (database *Store) compact(ranges []func() ([]byte, []byte)) error {
	logs, _ := database.logs.KeyRange()
	events, _ := database.events.KeyRange()
	compacted := 0

	for _, keyRange := range ranges {
		lower, upper := keyRange()

		if err := database.db.CompactRange(lower, upper); err != nil {
			return err
		}

		if bytes.Equal(lower, logs) || bytes.Equal(lower, events) {
			compacted++
		}
	}

	if compacted == 2 {
		database.uncompacted.Store(0)
	}

	return nil
}

// loadUncompacted adds up what the runs of single projects freed since the
// newest run over all projects, which compacted what they deleted.
func (database *Store) loadUncompacted() error {
	var freed int64

	err := database.cleanups.Primary().Newest().Each(database.db, func(entry kv.Entry[CleanupRun]) (bool, error) {
		if entry.Record.Project == AllProjects {
			return false, nil
		}

		freed += freedBy(entry.Record)

		return true, nil
	})

	database.uncompacted.Store(freed)

	return err
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
	decoded  int // bytes of payloads parsed (thinning), which is what takes the time
}

// full says the batch has done enough: cleanupChunk records or cleanupDecode
// bytes parsed (the write lock is held meanwhile), or freed what is needed.
func (cursor *cleanupCursor) full() bool {
	return cursor.records >= cleanupChunk || cursor.decoded >= cleanupDecode || (!cursor.always && -cursor.batch.Counted() >= cursor.need)
}

func (cursor *cleanupCursor) number(offset int) uint64 {
	return kv.Decode(cursor.position, offset).U64()
}

// cleanupSteps are the steps in the order of docs/configuration.md; their
// number is their place. Each reads and deletes only the records of project
// (AllProjects: every record).
func (database *Store) cleanupSteps(project int) []cleanupStep {
	now := database.now()
	month, week := now.Add(-config.CleanupAge), now.Add(-config.CleanupRecentAge)
	scope := cleanupScope{database: database, project: project}

	return []cleanupStep{
		{"logs older than a month", true, scope.deleteLogs(allRanks, func() uint64 { return database.sequenceFrom(month) })},
		{"events older than a month beyond the newest of their issue", true, scope.trimIssues(config.OldEventsPerIssue, month, func(issue *issueRecord) bool {
			return len(issue.EventDays) > 0 && issue.EventDays[0].Day <= dayOf(month)
		})},
		{"events of resolved and ignored issues but the latest", true, scope.trimIssues(1, time.Time{}, func(issue *issueRecord) bool {
			return issue.Status != store.StatusUnresolved
		})},
		{"logs below error", true, scope.deleteLogs(minorRanks, nil)},
		{"events below error but the latest of their issue", false, scope.deleteOldestEvents(time.Time{}, minorRanks)},
		{"error and fatal logs", false, scope.deleteOldestLogs(majorRanks)},
		{"events beyond the newest of their issue", false, scope.trimIssues(config.OldEventsPerIssue, time.Time{}, nil)},
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

// cleanupScope reads the records of one project, or of all (AllProjects).
type cleanupScope struct {
	database *Store

	project int
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
	steps := database.cleanupSteps(run.Project)

	for ctx.Err() == nil {
		if run.Reached > len(steps) {
			run.Reached = len(steps)

			return nil
		}

		step := steps[run.Reached-1]
		need := database.scopeBytes(run.Project) - run.Goal

		if !step.always && need <= 0 {
			// A step started has a position or deleted something; one not started does not count.
			if run.Position == nil && (len(run.Steps) == 0 || run.Steps[len(run.Steps)-1].Number != run.Reached) {
				run.Reached--
			}

			return nil
		}

		var next CleanupRun

		started := time.Now()

		err := database.update(func(writer *batchWriter) error {
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

		if run.Project != AllProjects {
			database.uncompacted.Add(freedBy(next) - freedBy(*run))
		}

		*run = next

		select {
		case <-ctx.Done():
		case <-time.After(cleanupRest):
		}
	}

	return ctx.Err()
}

// freedBy is what a run freed so far, in bytes of the counters.
func freedBy(run CleanupRun) int64 {
	var freed int64

	for _, step := range run.Steps {
		freed += step.Freed
	}

	return freed
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

// eachIssue visits, by ID from the cursor on, the issues of the scope
// accepts takes as they are now; visit says whether it is done with one. Position: issue ID.
func (scope cleanupScope) eachIssue(accepts func(*issueRecord) bool,
	visit func(writer *batchWriter, cursor *cleanupCursor, issue *issueRecord) (bool, error),
) cleanupBatch {
	database := scope.database
	wants := func(issue *issueRecord) bool { return scope.has(issue) && accepts(issue) }

	var ids []string // sorted once per step; each batch checks the issues as they are then

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		if ids == nil {
			ids = database.issueIDs(wants)
		}

		from := kv.Decode(cursor.position, 0).Text()

		for _, id := range ids[sort.SearchStrings(ids, from):] {
			// The table changes only under the write lock, which this batch holds.
			database.issuesMutex.RLock()
			stored, ok := database.table.byID[id]
			database.issuesMutex.RUnlock()

			if !ok || !wants(stored) {
				continue
			}

			if cursor.full() {
				cursor.position = kv.K(kv.Text(id))

				return false, nil
			}

			issue, _ := writer.issue(id)
			done, err := visit(writer, cursor, issue)

			if err != nil || !done {
				cursor.position = kv.K(kv.Text(id))

				return false, err
			}
		}

		return true, nil
	}
}

// issueIDs lists, sorted, the issues wants accepts now.
func (database *Store) issueIDs(wants func(*issueRecord) bool) []string {
	database.issuesMutex.RLock()

	ids := []string{}

	for id, issue := range database.table.byID {
		if wants(issue) {
			ids = append(ids, id)
		}
	}

	database.issuesMutex.RUnlock()
	slices.Sort(ids)

	return ids
}

// trimIssues deletes the oldest events received before cutoff (zero: any)
// beyond keep, of the issues match accepts (nil: all).
func (scope cleanupScope) trimIssues(keep int, cutoff time.Time, match func(*issueRecord) bool) cleanupBatch {
	database := scope.database
	wants := func(issue *issueRecord) bool { return issue.Stored > keep && (match == nil || match(issue)) }

	return scope.eachIssue(wants, func(writer *batchWriter, cursor *cleanupCursor, issue *issueRecord) (bool, error) {
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
	})
}

// deleteOldestEvents deletes events of ranks received before cutoff (zero:
// any), oldest first whatever their level among those, but not the latest of
// an issue. Position: sequence.
func (scope cleanupScope) deleteOldestEvents(cutoff time.Time, ranks []byte) cleanupBatch {
	database := scope.database

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		from := cursor.number(0)
		queries := make([]kv.Query[eventRecord], 0, len(ranks))

		for _, rank := range ranks {
			queries = append(queries, scope.events(rank).From(kv.U64(from)).Head())
		}

		full := false

		err := oldestFirst(database.db, queries, func(entry kv.Entry[eventRecord]) bool {
			if !cutoff.IsZero() && !entry.Record.ReceivedAt.Before(cutoff) {
				return false
			}

			if full = cursor.full(); full {
				return false
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

	var issues []unseen // sorted once per step; each batch checks the issues as they are then

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		if issues == nil {
			database.issuesMutex.RLock()

			for _, issue := range database.table.byID {
				if wants(issue) {
					issues = append(issues, unseen{issue.LastSeen.UnixNano(), issue.ID})
				}
			}

			database.issuesMutex.RUnlock()
			slices.SortFunc(issues, compare)
		}

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

		start, _ := slices.BinarySearchFunc(issues, after, compare)

		for _, candidate := range issues[start:] {
			// Gone, or seen again since the list was made: the next run judges it.
			database.issuesMutex.RLock()
			stored, ok := database.table.byID[candidate.id]
			database.issuesMutex.RUnlock()

			if !ok || compare(candidate, after) <= 0 || !wants(stored) || stored.LastSeen.UnixNano() != candidate.lastSeen {
				continue
			}

			if cursor.full() {
				cursor.position = kv.K(kv.U64(uint64(after.lastSeen)), kv.Text(after.id))

				return false, nil
			}

			if candidate.id != started {
				from = 0
			}

			issue, _ := writer.issue(candidate.id)
			next, done, err := writer.deleteIssueEvents(issue.ID, from, take)

			if err != nil || !done {
				cursor.position = kv.K(kv.U64(uint64(after.lastSeen)), kv.Text(after.id), kv.Text(issue.ID), kv.U64(next))

				return false, err
			}

			if whole {
				if err := writer.deleteIssue(issue); err != nil {
					return false, err
				}

				cursor.records++
			}

			after = candidate
		}

		return true, nil
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
