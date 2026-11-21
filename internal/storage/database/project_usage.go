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

// backfilled: every record is counted per project.
func (database *Store) backfilled() bool {
	return database.issues.Backfilled() && database.events.Backfilled() && database.logs.Backfilled()
}

func (database *Store) ProjectUsage(context.Context) (store.ProjectUsage, error) {
	usage := store.ProjectUsage{
		Bytes: map[int]int64{}, LastReceived: map[int]time.Time{}, Counting: !database.backfilled(),
		Counted: database.logicalSize() + database.inboxBytes.Load(),
	}

	for counter, size := range database.db.Groups() {
		usage.Bytes[int(kv.Decode([]byte(counter), 0).U32())] = size
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
	}

	return usage, nil
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
