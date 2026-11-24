package database

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/devforth/exceptic/internal/platform/background"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Compactions go in parts, so the lane can run its other jobs between them
// (CompactCleanups), and so each ends promptly under ingest: goleveldb's
// CompactRange goes on while new tables overlap its range, and those of
// fresh writes span every collection. A collection is its indexes in one
// part (small), then its records: the logs per level and the ingest inbox
// up to the sequence the compaction started at, since newer keys hold
// nothing it frees and are where ingest writes, the others (keyed by issue)
// cut at compactPart bytes of the files. Each call costs a few seconds
// under ingest whatever its size (it flushes the write buffer first), so
// the parts are few.
const compactPart = 256 << 20

// arrivalLeads are the collections whose keys end with the arrival
// sequence, with the bytes before it: none, or the level's rank.
var arrivalLeads = map[string]int{"inbox": 0, "log": 1}

type compactRange struct{ lower, upper []byte }

// compactable is a collection: compacting it frees what its deletes and rewrites left in the files.
type compactable interface {
	Name() string
	KeyRange() (lower, upper []byte)
	Stale(db *kv.DB) int64
	StaleMark(db *kv.DB) int64
	TakeStale(db *kv.DB, mark int64) (int64, error)
}

func (database *Store) compactables() []compactable {
	return []compactable{database.inbox, database.issues, database.status, database.events, database.logs}
}

// compact compacts collections in parts, then takes off what they held of
// deleted records when it started and no other compaction took off. Unless
// forced it compacts only while ingest is quiet and says whether it
// finished; forced, each part refuses ingest for its time (compactRange).
func (database *Store) compact(forced bool, collections ...compactable) (bool, error) {
	if err := database.compactFaults(); err != nil {
		return false, err
	}

	marks := make([]int64, len(collections))

	for index, collection := range collections {
		marks[index] = collection.StaleMark(database.db)
	}

	parts, err := database.compactParts(collections, database.db.Peek(), nil)

	for _, part := range parts {
		if err != nil {
			return false, err
		}

		if !forced && !database.quiet() {
			return false, nil
		}

		err = database.compactRange(part)
	}

	for index, collection := range collections {
		if err == nil {
			_, err = collection.TakeStale(database.db, marks[index])
		}
	}

	return err == nil, err
}

// compactRange compacts a part with ingest's writes held: goleveldb's range
// compaction goes on for as long as new files overlap its range, and under
// ingest every file written holds keys of the whole database, so under
// writes it would hold the lane for as long as they come while goleveldb's
// own compactions wait (measured: parts of minutes, level 1 past a GB).
// Envelopes arriving meanwhile get 429 (ErrCompacting), the writer waits.
func (database *Store) compactRange(part compactRange) error {
	database.ingestWrites.Lock()
	defer database.ingestWrites.Unlock()

	if database.whileCompacting != nil {
		database.whileCompacting()
	}

	return database.db.CompactRange(part.lower, part.upper)
}

func (database *Store) compactFaults() error {
	if database.compactFault == nil {
		return nil
	}

	return database.compactFault()
}

// compactParts are the key ranges compacting collections takes, in key
// order, from from on (nil: all of them).
func (database *Store) compactParts(collections []compactable, below uint64, from []byte) ([]compactRange, error) {
	var parts []compactRange

	for _, collection := range collections {
		lower, upper := collection.KeyRange()
		records := []byte(collection.Name() + "/")
		parts = append(parts, compactRange{lower, records})

		found, err := database.spreadParts(records, upper)

		if lead, ok := arrivalLeads[collection.Name()]; ok {
			found, err = database.arrivalParts(records, upper, lead, below)
		}

		if err != nil {
			return nil, err
		}

		parts = append(parts, found...)
	}

	slices.SortFunc(parts, func(first, second compactRange) int { return bytes.Compare(first.lower, second.lower) })

	left := parts[:0]

	for _, part := range parts {
		if from != nil && bytes.Compare(part.upper, from) <= 0 {
			continue
		}

		if from != nil && bytes.Compare(part.lower, from) < 0 {
			part.lower = from
		}

		left = append(left, part)
	}

	return left, nil
}

// arrivalParts is a part per lead (a level) from prefix up to the sequence
// below, and the gaps between the leads holding records (only deleted ones
// are left there), compacted whole. The inbox, without a lead, is one part.
func (database *Store) arrivalParts(prefix, end []byte, lead int, below uint64) ([]compactRange, error) {
	if lead == 0 {
		return []compactRange{{prefix, append(bytes.Clone(prefix), kv.K(kv.U64(below))...)}}, nil
	}

	var parts []compactRange

	gap := func(lower, upper []byte) error {
		size, err := database.db.SizeOf(lower, upper)

		if size > 0 {
			parts = append(parts, compactRange{lower, upper})
		}

		return err
	}

	iterator := database.db.Iterate(prefix, end)
	lower := prefix

	for valid := iterator.First(); valid; valid = iterator.Seek(lower) {
		key := iterator.Key()
		start := bytes.Clone(key[:min(len(key), len(prefix)+lead)])

		if err := gap(lower, start); err != nil {
			return nil, errors.Join(err, iterator.Close())
		}

		parts = append(parts, compactRange{start, append(bytes.Clone(start), kv.K(kv.U64(below))...)})

		if lower = kv.PrefixEnd(start); lower == nil {
			break
		}
	}

	if err := iterator.Close(); err != nil || lower == nil {
		return parts, err
	}

	return parts, gap(lower, end)
}

// spreadParts cover the keys from prefix to end, cut after the first byte
// of a key once a part takes compactPart bytes of the files.
func (database *Store) spreadParts(prefix, end []byte) ([]compactRange, error) {
	iterator := database.db.Iterate(prefix, end)
	parts := []compactRange{{prefix, end}}

	for valid := iterator.First(); valid; {
		key := iterator.Key()
		cut := kv.PrefixEnd(key[:min(len(key), len(prefix)+1)])
		last := &parts[len(parts)-1]

		if cut == nil || bytes.Compare(cut, last.upper) >= 0 {
			break
		}

		size, err := database.db.SizeOf(last.lower, cut)

		if err != nil {
			return nil, errors.Join(err, iterator.Close())
		}

		if size >= compactPart {
			upper := last.upper
			last.upper = cut
			parts = append(parts, compactRange{cut, upper})
		}

		valid = iterator.Seek(cut)
	}

	return parts, iterator.Close()
}

// staleData is the logs and events, of those that hold deleted or replaced records.
func (database *Store) staleData() []compactable {
	var stale []compactable

	if database.logs.Stale(database.db) > 0 {
		stale = append(stale, database.logs)
	}

	if database.events.Stale(database.db) > 0 {
		stale = append(stale, database.events)
	}

	return stale
}

// compactAfter is what the compaction after a run compacts: over all
// projects what holds deleted records, the logs and events and, when it
// deleted issues, the issues and status history; after a project's run the
// logs and events only once the stale records are a sizable part of the
// files (compactStale). Nothing: it is not needed.
func (scope cleanupScope) compactAfter(run CleanupRun) []compactable {
	database := scope.database

	if !scope.all() {
		if !database.staleOver() {
			return nil
		}

		return database.staleData()
	}

	collections := database.staleData()

	if issuesDeleted(run) {
		collections = append(collections, database.issues, database.status)
	}

	return collections
}

// CompactCleanups is the compaction after the runs: each run that deleted
// something waits for it with its deleted records still in the files, after
// a restart too. It records on the run how long it took and what the files
// lost. The runs over all projects go first, since theirs frees the
// projects' runs' deleted records too, then the projects' runs, oldest
// first. While a backup runs it waits: the backup's snapshot keeps the
// deleted records in the files. It goes in parts and yields
// (background.ErrYielded) to another job of the lane that is due, the size
// check above all, and goes on right after it.
func (database *Store) CompactCleanups(ctx context.Context, logger *slog.Logger) error {
	runs, err := database.pendingCompactions()

	if err != nil {
		return fmt.Errorf("cleanup compaction: %w", err)
	}

	for _, run := range runs {
		if ctx.Err() != nil || database.backups.Load() > 0 {
			return nil
		}

		if err := database.compactAfterRun(ctx, run, logger); err != nil {
			return err
		}
	}

	return nil
}

// pendingCompactions is the runs waiting for their compaction, in the order CompactCleanups takes them.
func (database *Store) pendingCompactions() ([]CleanupRun, error) {
	runs, err := database.cleanups.Primary().All(database.db)

	if err != nil {
		return nil, err
	}

	runs = slices.DeleteFunc(runs, func(run CleanupRun) bool {
		return run.Compaction == nil || run.Compaction.Status != store.CompactionPending
	})
	allFirst := func(run CleanupRun) int {
		if run.Project == AllProjects {
			return 0
		}

		return 1
	}

	slices.SortStableFunc(runs, func(first, second CleanupRun) int { return cmp.Compare(allFirst(first), allFirst(second)) })

	return runs, nil
}

// errCompactionInterrupted is the error of an attempt that never returned.
const errCompactionInterrupted = "did not finish: the server stopped or the compaction panicked"

func (database *Store) saveRun(run CleanupRun) error {
	return database.db.Update(true, func(batch *kv.Batch) error { return database.cleanups.Save(batch, run) })
}

// compactAfterRun compacts after a run, in parts, and records it; a failure
// leaves it pending for the job's next pass until store.CompactionAttempts.
// Each attempt is saved before it starts, so one that never returns (a
// panic, a killed process) counts too, and so is each part done, so the
// next pass goes on from it. A pass that yields is no attempt.
func (database *Store) compactAfterRun(ctx context.Context, run CleanupRun, logger *slog.Logger) error {
	scope := database.scope(run.Project)
	previous := *run.Compaction
	failed := func(err error) error { return fmt.Errorf("cleanup compaction of %s: %w", scope.name(), err) }

	if previous.Running {
		previous.Error = errCompactionInterrupted
	}

	if previous.Attempts >= store.CompactionAttempts {
		run.Compaction = &store.CleanupCompaction{Status: store.CompactionFailed, Attempts: previous.Attempts, Error: previous.Error}

		return database.saveRun(run)
	}

	progress := previous.Progress

	if progress == nil {
		var err error

		if progress, err = database.startCompaction(scope, run); err != nil || progress == nil {
			if err != nil {
				return failed(err)
			}

			logger.Debug("cleanup compaction not needed: nothing left to compact or under the stale share", "scope", scope.name(), "stale_percent", config.StalePercent)

			run.Compaction = &store.CleanupCompaction{Status: store.CompactionSkipped, Attempts: previous.Attempts}

			return database.saveRun(run)
		}
	}

	attempt := previous.Attempts + 1
	run.Compaction = &store.CleanupCompaction{Status: store.CompactionPending, Attempts: attempt, Error: previous.Error, Running: true, Progress: progress}

	if err := database.saveRun(run); err != nil {
		return failed(err)
	}

	finished, err := database.compactParted(ctx, &run)

	switch {
	case errors.Is(err, background.ErrYielded) || err == nil && !finished:
		run.Compaction = &store.CleanupCompaction{Status: store.CompactionPending, Attempts: previous.Attempts, Error: previous.Error, Progress: progress}

		return errors.Join(database.saveRun(run), err)
	case err == nil:
		return database.finishCompaction(scope, run, logger)
	}

	compaction := run.Compaction
	compaction.Running, compaction.Error = false, err.Error()

	if compaction.Attempts >= store.CompactionAttempts {
		run.Compaction = &store.CleanupCompaction{Status: store.CompactionFailed, Attempts: compaction.Attempts, Error: compaction.Error}
	}

	return errors.Join(failed(err), database.saveRun(run))
}

// startCompaction is the progress of a compaction about to start, nil when
// none is needed.
func (database *Store) startCompaction(scope cleanupScope, run CleanupRun) (*store.CompactionProgress, error) {
	collections := scope.compactAfter(run)

	if len(collections) == 0 {
		return nil, nil
	}

	files, err := database.db.Size()

	if err != nil {
		return nil, err
	}

	progress := &store.CompactionProgress{StartedAt: database.now().UTC(), FilesBefore: files, Below: database.db.Peek(), Stale: map[string]int64{}}

	for _, collection := range collections {
		progress.Stale[collection.Name()] = collection.StaleMark(database.db)
	}

	return progress, nil
}

// compactParted compacts the parts of the run's compaction left, saving the
// progress after each, and says whether it finished. Each part waits for
// ingest to be quiet (it refuses ingest while it runs, compactRange); between
// them it stops at ctx's end or a backup, or yields to a job of the lane that
// is due (background.ErrYielded).
func (database *Store) compactParted(ctx context.Context, run *CleanupRun) (bool, error) {
	progress := run.Compaction.Progress
	started := time.Now()

	defer func() { progress.Took += time.Since(started) }()

	if err := database.compactFaults(); err != nil {
		return false, err
	}

	var collections []compactable

	for _, collection := range database.compactables() {
		if _, ok := progress.Stale[collection.Name()]; ok {
			collections = append(collections, collection)
		}
	}

	parts, err := database.compactParts(collections, progress.Below, progress.Reached)

	for index, part := range parts {
		if err != nil {
			return false, err
		}

		switch {
		// Each part refuses ingest while it runs, so it waits for ingest to be quiet.
		case !database.quiet():
			return false, nil
		case index == 0:
		case ctx.Err() != nil || database.backups.Load() > 0:
			return false, nil
		case background.Waiting(ctx):
			return false, background.ErrYielded
		}

		if err = database.compactRange(part); err == nil {
			progress.Reached = part.upper
			err = database.saveRun(*run)
		}
	}

	return err == nil, err
}

// finishCompaction takes off what the compaction freed from the stale
// counts, unless another compaction took it off meanwhile, and records it
// on the run.
func (database *Store) finishCompaction(scope cleanupScope, run CleanupRun, logger *slog.Logger) error {
	progress := run.Compaction.Progress
	compaction := &store.CleanupCompaction{Status: store.CompactionDone, StartedAt: progress.StartedAt, Duration: progress.Took,
		FilesBefore: progress.FilesBefore, Attempts: run.Compaction.Attempts}

	var (
		stale int64
		err   error
	)

	for _, collection := range database.compactables() {
		mark, ok := progress.Stale[collection.Name()]

		if !ok || err != nil {
			continue
		}

		var taken int64

		taken, err = collection.TakeStale(database.db, mark)

		if collection == compactable(database.logs) || collection == compactable(database.events) {
			stale += taken
		}
	}

	usage, sizeErr := database.diskUsage()

	if err = errors.Join(err, sizeErr); err != nil {
		run.Compaction.Running, run.Compaction.Error = false, err.Error()

		return errors.Join(fmt.Errorf("cleanup compaction of %s: %w", scope.name(), err), database.saveRun(run))
	}

	// What it compacted away, measured apart from data arriving meanwhile, or what the files lost when that is more.
	compaction.FilesAfter = usage.files
	compaction.Freed = max(usage.onDisk(stale), compaction.FilesBefore-compaction.FilesAfter, 0)
	run.Compaction = compaction

	logger.Info("cleanup compaction", "scope", scope.name(), "run_started", run.StartedAt.Format(time.RFC3339), "files_before", config.FormatSize(compaction.FilesBefore),
		"files_after", config.FormatSize(compaction.FilesAfter), "freed", config.FormatSize(compaction.Freed), "duration", compaction.Duration.Round(time.Millisecond))

	return database.saveRun(run)
}

func issuesDeleted(run CleanupRun) bool {
	for _, step := range run.Steps {
		for _, counts := range step.Projects {
			if counts.Issues > 0 {
				return true
			}
		}
	}

	return false
}
