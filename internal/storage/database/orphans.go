package database

import (
	"errors"

	"github.com/devforth/exceptic/internal/storage/kv"
)

// Events and status changes whose issue is not stored: the other steps go
// by issue and never reach them. Nothing should leave them, so finding any
// is a bug; the first step of every run deletes them.

// orphanStep is the number of that step.
const orphanStep = 1

// deleteOrphans deletes the status changes, then the events, of the issue IDs
// no stored issue has: over all projects every one, in a project's run those
// whose events are the project's. It reads a key per issue ID (eachOrphan)
// and the head of one event per missing issue, never every event.
// Position: phase (0 status changes, 1 events), issue ID, sequence in it.
func (scope cleanupScope) deleteOrphans() cleanupBatch {
	database := scope.database

	return func(writer *batchWriter, cursor *cleanupCursor) (bool, error) {
		decoder := kv.Decode(cursor.position, 0)
		phase, from, sequence := decoder.U8(), decoder.Text(), decoder.U64()

		if phase == 0 {
			if done, err := orphanPhase(cursor, scope, database.status, 0, from, sequence, func(entry kv.Entry[statusRecord], project int) {
				database.status.DeleteEntry(writer.batch, entry)
				writer.count(project).StatusChanges++
			}); err != nil || !done {
				return false, err
			}

			from, sequence = "", 0
		}

		return orphanPhase(cursor, scope, database.events, 1, from, sequence, func(entry kv.Entry[eventRecord], _ int) {
			writer.deleteEvent(entry)
		})
	}
}

// orphanPhase deletes the records of collection (keys: issue ID, sequence)
// of missing issues, from the issue ID from and the sequence in it on. A
// key read counts as a record, so a batch holds the write lock for at most
// cleanupChunk of them. It says whether it reached the end.
func orphanPhase[T any](cursor *cleanupCursor, scope cleanupScope, collection *kv.Collection[T], phase byte,
	from string, sequence uint64, remove func(entry kv.Entry[T], project int),
) (bool, error) {
	database := scope.database
	done := true

	err := eachOrphan(database.db, database, collection, from, func(id string, gone bool) (bool, error) {
		if id != from {
			sequence = 0
		}

		if cursor.full() {
			cursor.position, done = kv.K(kv.U8(phase), kv.Text(id), kv.U64(sequence)), false

			return false, nil
		}

		cursor.records++

		if !gone {
			return true, nil
		}

		project, ok, err := database.orphanProject(id)

		if err != nil || (!scope.all() && (!ok || project != scope.project)) {
			return err == nil, err
		}

		err = collection.Primary(kv.Text(id)).From(kv.U64(sequence)).Head().Each(database.db, func(entry kv.Entry[T]) (bool, error) {
			if cursor.full() {
				done = false

				return false, nil
			}

			remove(entry, project)

			cursor.records++

			key := kv.Decode(entry.Key, 0)
			key.Text()
			sequence = key.U64() + 1

			return true, nil
		})

		if !done {
			cursor.position = kv.K(kv.U8(phase), kv.Text(id), kv.U64(sequence))
		}

		return done && err == nil, err
	})

	return done, err
}

// orphanProject is the project of a missing issue's events; ok is false
// when it has none left.
func (database *Store) orphanProject(id string) (int, bool, error) {
	first, err := database.events.Primary(kv.Text(id)).Head().First(database.db)

	if errors.Is(err, kv.ErrNotFound) {
		return 0, false, nil
	}

	return first.Record.ProjectID, err == nil, err
}

// eachOrphan visits the issue IDs collection's keys start with, from the ID
// from on, seeking from one to the next, so it reads one key per ID however
// many records it has; gone says no stored issue has it. Inside an update
// the issue table is as stored.
func eachOrphan[T any](reader kv.Reader, database *Store, collection *kv.Collection[T], from string,
	visit func(id string, gone bool) (bool, error),
) error {
	query := collection.Primary()

	if from != "" {
		query = query.From(kv.Text(from))
	}

	return query.Groups(reader, issueGroup, func(key kv.Key) (bool, error) {
		id := kv.Decode(key, 0).Text()

		_, err := database.issues.GetValue(reader, kv.K(kv.Text(id)))

		switch {
		case errors.Is(err, kv.ErrNotFound):
			return visit(id, true)
		case err != nil:
			return false, err
		}

		return visit(id, false)
	})
}

// issueGroup is the issue ID a key of the events or status changes starts with.
func issueGroup(key kv.Key) kv.Key { return kv.K(kv.Text(kv.Decode(key, 0).Text())) }
