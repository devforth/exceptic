package kv

import (
	"bytes"
	"errors"
	"fmt"
)

// recountBatch is how many records one batch of a recount counts.
const recountBatch = 2000

func (db *DB) recountKey(collection, name string) Key {
	return db.meta.With(Text("recount"), Text(collection), Text(name))
}

// Recount runs the recounts (Definition.Recount) left to do, once the
// opener has checked the database is its own, before anything else writes.
func (db *DB) Recount() error {
	for _, collection := range db.schema.collections {
		if err := collection.recount(db); err != nil {
			return err
		}
	}

	return nil
}

// markRecounted records the recount done: a new database has nothing to count.
func (collection *Collection[T]) markRecounted(db *DB, batch *Batch) {
	if name := collection.definition.Recount; name != "" {
		batch.Put(db.recountKey(collection.name, name), []byte{backfillDone})
	}
}

// recount counts the stored records into the tallies (Definition.Recount),
// by DB.Recount before anything else writes: each batch holds its counts and the
// key it goes on from, so a restart goes on there and nothing counts twice.
func (collection *Collection[T]) recount(db *DB) error {
	name := collection.definition.Recount

	if name == "" {
		return nil
	}

	key := db.recountKey(collection.name, name)
	value, err := db.Get(key)
	position := Key{}

	switch {
	case errors.Is(err, ErrNotFound):
	case err != nil:
		return err
	case len(value) == 1 && value[0] == backfillDone:
		return nil
	case len(value) > 0 && value[0] == backfillPending:
		position = Key(bytes.Clone(value[1:]))
	default:
		return fmt.Errorf("kv: recount %s: unknown state %q", name, value)
	}

	for position != nil {
		err := db.Update(true, func(batch *Batch) error {
			iterator := batch.Iterate(collection.primaryKey(position), prefixEnd(collection.prefix))
			counted := 0
			position = nil

			for valid := iterator.First(); valid; valid = iterator.Next() {
				if counted == recountBatch {
					position = Key(bytes.Clone(iterator.Key()[len(collection.prefix):]))

					break
				}

				record, err := collection.definition.Decode(iterator.Value())

				if err != nil {
					return errors.Join(fmt.Errorf("kv: %s %x: %w", collection.name, iterator.Key(), err), iterator.Close())
				}

				collection.countRecord(batch, record, 1)

				counted++
			}

			if err := iterator.Close(); err != nil {
				return err
			}

			state := []byte{backfillDone}

			if position != nil {
				state = append([]byte{backfillPending}, position...)
			}

			batch.Put(key, state)

			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}
