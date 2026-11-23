package database

import (
	"context"
	"errors"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// backfillChunk is how many records one step of the backfill brings up to date.
const backfillChunk = 2000

// Backfill counts the data stored before the project counters (their bytes,
// and the events' project index) in steps, until done or ctx ends; the next
// pass goes on where it stopped. Once done it does nothing.
func (database *Store) Backfill(ctx context.Context) error {
	for _, step := range []func(*kv.DB, int) (bool, error){database.issues.BackfillStep, database.events.BackfillStep, database.logs.BackfillStep} {
		for done := false; !done; {
			if err := ctx.Err(); err != nil {
				return err
			}

			var err error

			if done, err = step(database.db, backfillChunk); err != nil {
				return err
			}
		}
	}

	return nil
}

// projectBytes is what a project's issues, events and logs take, before compression.
func (database *Store) projectBytes(projectID int) int64 {
	return database.db.Group(projectCounter(projectID))
}

// backfilled: every record is counted per project.
func (database *Store) backfilled() bool {
	return database.issues.Backfilled() && database.events.Backfilled() && database.logs.Backfilled()
}

func (database *Store) ProjectUsage(context.Context) (store.ProjectUsage, error) {
	usage := store.ProjectUsage{
		Bytes: map[int]int64{}, OnDisk: map[int]int64{}, LastReceived: map[int]time.Time{}, Oldest: map[int]store.OldestKept{},
		Counting: !database.backfilled(),
	}

	ratio, err := database.diskRatio()

	if err != nil {
		return usage, err
	}

	for counter, size := range database.db.Groups() {
		projectID := int(kv.Decode([]byte(counter), 0).U32())
		usage.Bytes[projectID], usage.OnDisk[projectID] = size, int64(float64(size)*ratio)
	}

	database.issuesMutex.RLock()

	for _, issue := range database.table.byID {
		if issue.LastSeen.After(usage.LastReceived[issue.ProjectID]) {
			usage.LastReceived[issue.ProjectID] = issue.LastSeen
		}
	}

	database.issuesMutex.RUnlock()

	projects, err := database.projects.Primary().All(database.db)

	if err != nil {
		return usage, err
	}

	for _, project := range projects {
		projectID := project.ID
		newest, err := database.newestLog(projectID)

		if err != nil {
			return usage, err
		}

		if newest.After(usage.LastReceived[projectID]) {
			usage.LastReceived[projectID] = newest
		}

		if usage.Oldest[projectID], err = database.oldestKept(projectID); err != nil {
			return usage, err
		}
	}

	return usage, nil
}

// oldestKept reads the oldest record of each kind the cleanup deletes, by the project's indexes.
func (database *Store) oldestKept(projectID int) (store.OldestKept, error) {
	var kept store.OldestKept

	scope := cleanupScope{database: database, project: projectID}

	for _, logs := range []struct {
		ranks  []byte
		oldest *time.Time
	}{{minorRanks, &kept.LogsBelowError}, {majorRanks, &kept.ErrorLogs}} {
		queries := make([]kv.Query[logRecord], 0, len(logs.ranks))

		for _, rank := range logs.ranks {
			queries = append(queries, scope.logs(rank))
		}

		err := oldestFirst(database.db, queries, func(entry kv.Entry[logRecord]) bool {
			*logs.oldest = entry.Record.Timestamp

			return false
		})

		if err != nil {
			return kept, err
		}
	}

	queries := make([]kv.Query[eventRecord], 0, len(allRanks))

	for _, rank := range allRanks {
		queries = append(queries, scope.events(rank).Head())
	}

	err := oldestFirst(database.db, queries, func(entry kv.Entry[eventRecord]) bool {
		kept.Errors = entry.Record.ReceivedAt

		return false
	})

	return kept, err
}

// newestLog is the time of a project's newest log, zero when it has none:
// the newest of each level by the project index, then the latest of them.
func (database *Store) newestLog(projectID int) (time.Time, error) {
	var newest kv.Key

	for _, rank := range allRanks {
		err := database.logs.By("project", kv.U32(uint32(projectID)), kv.U8(rank)).Newest().Limit(1).Keys(database.db, func(key kv.Key) (bool, error) {
			if newest == nil || sequenceOf(key) > sequenceOf(newest) {
				newest = key
			}

			return false, nil
		})

		if err != nil {
			return time.Time{}, err
		}
	}

	if newest == nil {
		return time.Time{}, nil
	}

	record, err := database.logs.Get(database.db, newest)

	if errors.Is(err, kv.ErrNotFound) { // deleted meanwhile
		return time.Time{}, nil
	}

	return record.Timestamp, err
}
