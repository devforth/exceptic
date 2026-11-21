package kv

import (
	"bytes"
	"errors"
	"fmt"
)

// Backfill names what a collection gained after records were stored: its
// Group counter (Group true) and the Indexes listed. A database that held
// records then brings them up to date in steps (Collection.BackfillStep);
// until a step reaches a record, the record is counted as before. A new
// database has nothing to bring up to date. A backfilled index cannot be
// unique, and a later change needs a new Name.
type Backfill struct {
	Name    string
	Indexes []string
	Group   bool
}

// The state of a backfill under meta/backfill/<collection>/<name>: done, or the key it goes on from.
const (
	backfillDone    = 'd'
	backfillPending = 'p'
)

func (db *DB) backfillKey(collection, name string) Key {
	return db.meta.With(Text("backfill"), Text(collection), Text(name))
}

// openBackfills reads where each backfill stands; a new database records them done.
func (schema *Schema) openBackfills(db *DB) error {
	if !db.fresh {
		for _, collection := range schema.collections {
			if err := collection.openBackfill(db); err != nil {
				return err
			}
		}

		return nil
	}

	batch := NewBatch(db.Engine)

	for _, collection := range schema.collections {
		collection.markBackfilled(db, batch)
	}

	if batch.Len() == 0 {
		return nil
	}

	return db.Write(batch, true)
}

func (collection *Collection[T]) backfillName() string {
	if collection.definition.Backfill == nil {
		return ""
	}

	return collection.definition.Backfill.Name
}

func (collection *Collection[T]) markBackfilled(db *DB, batch *Batch) {
	if name := collection.backfillName(); name != "" {
		batch.Put(db.backfillKey(collection.name, name), []byte{backfillDone})
	}
}

func (collection *Collection[T]) openBackfill(db *DB) error {
	name := collection.backfillName()

	if name == "" {
		return nil
	}

	value, err := db.Get(db.backfillKey(collection.name, name))

	switch {
	case errors.Is(err, ErrNotFound):
		start := Key{}
		collection.position.Store(&start)
	case err != nil:
		return err
	case len(value) == 1 && value[0] == backfillDone:
	case len(value) > 0 && value[0] == backfillPending:
		position := Key(bytes.Clone(value[1:]))
		collection.position.Store(&position)
	default:
		return fmt.Errorf("kv: backfill %s: unknown state %q", name, value)
	}

	return nil
}

// Backfilled reports whether every record is up to date: no backfill, or it is done.
func (collection *Collection[T]) Backfilled() bool { return collection.position.Load() == nil }

// reached reports whether the record under primary is up to date.
func (collection *Collection[T]) reached(primary []byte) bool {
	position := collection.position.Load()

	return position == nil || bytes.Compare(primary[len(collection.prefix):], *position) < 0
}

// BackfillStep brings up to most records up to date in one update and
// records where it stopped, so a restart goes on from there; true once
// every record is.
func (collection *Collection[T]) BackfillStep(db *DB, most int) (bool, error) {
	position := collection.position.Load()

	if position == nil {
		return true, nil
	}

	var next *Key

	err := db.Update(false, func(batch *Batch) error {
		next = nil
		iterator := batch.Iterate(collection.primaryKey(*position), prefixEnd(collection.prefix))
		filled := 0

		for valid := iterator.First(); valid; valid = iterator.Next() {
			if filled == most {
				following := Key(bytes.Clone(iterator.Key()[len(collection.prefix):]))
				next = &following

				break
			}

			if err := collection.fill(batch, bytes.Clone(iterator.Key()), iterator.Value()); err != nil {
				return errors.Join(err, iterator.Close())
			}

			filled++
		}

		if err := iterator.Close(); err != nil {
			return err
		}

		state := []byte{backfillDone}

		if next != nil {
			state = append([]byte{backfillPending}, *next...)
		}

		batch.Put(db.backfillKey(collection.name, collection.definition.Backfill.Name), state)
		batch.AfterWrite(func() { collection.position.Store(next) })

		return nil
	})

	return err == nil && next == nil, err
}

// fill adds the backfilled index keys of a stored record and the bytes its
// counters lacked.
func (collection *Collection[T]) fill(batch *Batch, primary, value []byte) error {
	decode := collection.definition.Decode

	if collection.definition.DecodeHead != nil {
		decode = collection.definition.DecodeHead
	}

	record, err := decode(value)

	if err != nil {
		return fmt.Errorf("kv: %s %x: %w", collection.name, primary, err)
	}

	keys := collection.indexKeys(record)
	added := 0

	for _, entry := range keys {
		if entry.backfilled {
			batch.Put(entry.key, primary)
			added += len(entry.key) + len(primary)
		}
	}

	if collection.definition.Counter != nil {
		batch.Count(collection.definition.Counter(record), int64(added))
	}

	if group := collection.group(record); group != nil {
		if collection.definition.Backfill.Group {
			batch.Group(group, int64(recordSize(len(primary), len(value), keys)))
		} else {
			batch.Group(group, int64(added))
		}
	}

	return nil
}
