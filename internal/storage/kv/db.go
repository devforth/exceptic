package kv

import (
	"encoding/binary"
	"errors"
	"maps"
	"sync"
	"sync/atomic"
)

// DB is an engine with one writer, counters and sequence numbers on top.
type DB struct {
	Engine

	counterPrefix Key
	groupPrefix   Key
	stalePrefix   Key
	staledPrefix  Key
	meta          Key
	sequenceKey   Key
	// fresh: the engine held no key when opened.
	fresh bool

	writeMutex   sync.Mutex // one DB.Update at a time
	compactMutex sync.Mutex // one Collection.Compact at a time

	countersMutex sync.RWMutex
	counters      map[string]int64
	groups        map[string]int64
	// stale is per collection what its deletes and rewrites left in the
	// files since Collection.Compact last compacted it, in bytes counted.
	stale map[string]int64
	// staled is per collection all its deletes and rewrites ever added to
	// stale: a compaction's mark (Collection.StaleMark), so it takes off
	// only what no other compaction took (Collection.TakeStale).
	staled map[string]int64

	written atomic.Int64

	sequenceMutex sync.Mutex
	next          uint64 // the next sequence number
	reserved      uint64 // stored; numbers below it may be handed out
}

// sequenceBlock numbers are reserved at a time; after a crash the rest of the block is skipped.
const sequenceBlock = 10_000

// OpenDB opens a DB on engine and reserves its prefixes in schema.
func OpenDB(engine Engine, schema *Schema) (*DB, error) {
	meta := schema.Reserve("meta")
	db := &DB{
		Engine: engine, counterPrefix: schema.Reserve("count"), groupPrefix: schema.Reserve("group"), stalePrefix: schema.Reserve("stale"),
		staledPrefix: schema.Reserve("staled"),
		meta:         meta, sequenceKey: meta.With(Text("sequence")), counters: map[string]int64{}, groups: map[string]int64{}, stale: map[string]int64{}, staled: map[string]int64{},
	}

	iterator := engine.Iterate(nil, nil)
	db.fresh = !iterator.First()

	if err := iterator.Close(); err != nil {
		return nil, err
	}

	value, err := engine.Get(db.sequenceKey)

	switch {
	case errors.Is(err, ErrNotFound):
		db.next = 1
	case err != nil:
		return nil, err
	default:
		db.next = binary.BigEndian.Uint64(value)
	}

	db.reserved = db.next

	for prefix, values := range map[string]map[string]int64{
		string(db.counterPrefix): db.counters, string(db.groupPrefix): db.groups, string(db.stalePrefix): db.stale, string(db.staledPrefix): db.staled,
	} {
		iterator := engine.Iterate([]byte(prefix), prefixEnd([]byte(prefix)))

		for valid := iterator.First(); valid; valid = iterator.Next() {
			values[string(iterator.Key()[len(prefix):])] = int64(binary.BigEndian.Uint64(iterator.Value()))
		}

		if err := iterator.Close(); err != nil {
			return nil, err
		}
	}

	// Stored before staled was: all that is stale counts as added since.
	batch := NewBatch(engine)

	for collection, stale := range db.stale {
		if db.staled[collection] < stale {
			db.staled[collection] = stale
			batch.Put(append(db.staledPrefix.With(), collection...), binary.BigEndian.AppendUint64(nil, uint64(stale)))
		}
	}

	if batch.Len() > 0 {
		if err := db.Write(batch, true); err != nil {
			return nil, err
		}
	}

	if err := schema.openBackfills(db); err != nil {
		return nil, err
	}

	return db, nil
}

// Fresh reports whether the engine held no key when the DB was opened.
func (db *DB) Fresh() bool { return db.fresh }

// Next returns a sequence number larger than any before, also across restarts.
func (db *DB) Next() (uint64, error) {
	db.sequenceMutex.Lock()
	defer db.sequenceMutex.Unlock()

	if db.next >= db.reserved {
		reserved := db.next + sequenceBlock
		batch := NewBatch(db.Engine)
		batch.Put(db.sequenceKey, binary.BigEndian.AppendUint64(nil, reserved))

		if err := db.Write(batch, true); err != nil {
			return 0, err
		}

		db.reserved = reserved
	}

	number := db.next
	db.next++

	return number, nil
}

// Peek returns the number Next would return, without taking it.
func (db *DB) Peek() uint64 {
	db.sequenceMutex.Lock()
	defer db.sequenceMutex.Unlock()

	return db.next
}

// Update runs change on a new batch and writes it with its counters, one at a time.
// Nothing is written when change fails.
func (db *DB) Update(sync bool, change func(batch *Batch) error) error {
	db.writeMutex.Lock()
	defer db.writeMutex.Unlock()

	batch := NewBatch(db.Engine)

	if err := change(batch); err != nil {
		return err
	}

	db.countersMutex.RLock()
	totals := db.totals(batch, db.counterPrefix, db.counters, batch.counts)
	groupTotals := db.totals(batch, db.groupPrefix, db.groups, batch.groups)
	staleTotals := db.totals(batch, db.stalePrefix, db.stale, batch.stale)
	staledTotals := db.totals(batch, db.staledPrefix, db.staled, batch.staled)
	db.countersMutex.RUnlock()

	if err := db.Write(batch, sync); err != nil {
		return err
	}

	for _, delta := range batch.counts {
		db.written.Add(max(delta, 0))
	}

	db.countersMutex.Lock()
	keep(db.counters, totals)
	keep(db.groups, groupTotals)
	keep(db.stale, staleTotals)
	keep(db.staled, staledTotals)
	db.countersMutex.Unlock()

	for _, apply := range batch.after {
		apply()
	}

	return nil
}

// totals are the counters after the batch's changes, written into it under prefix.
func (db *DB) totals(batch *Batch, prefix Key, counters, changes map[string]int64) map[string]int64 {
	totals := make(map[string]int64, len(changes))

	for counter, delta := range changes {
		total := counters[counter] + delta
		totals[counter] = total
		key := append(prefix.With(), counter...)

		if total == 0 {
			batch.Delete(key)
		} else {
			batch.Put(key, binary.BigEndian.AppendUint64(nil, uint64(total)))
		}
	}

	return totals
}

// keep puts written totals into counters; a counter at zero is not kept.
func keep(counters, totals map[string]int64) {
	for counter, total := range totals {
		if total == 0 {
			delete(counters, counter)
		} else {
			counters[counter] = total
		}
	}
}

// Written returns the bytes updates have added to counters since the DB was opened.
func (db *DB) Written() int64 { return db.written.Load() }

// Counter returns the value of a counter.
func (db *DB) Counter(counter Key) int64 {
	db.countersMutex.RLock()

	defer db.countersMutex.RUnlock()

	return db.counters[string(counter)]
}

// Counters returns a copy of every counter.
func (db *DB) Counters() map[string]int64 {
	db.countersMutex.RLock()

	defer db.countersMutex.RUnlock()

	return maps.Clone(db.counters)
}

// Group returns a group counter.
func (db *DB) Group(group Key) int64 {
	db.countersMutex.RLock()

	defer db.countersMutex.RUnlock()

	return db.groups[string(group)]
}

// Groups returns a copy of every group counter.
func (db *DB) Groups() map[string]int64 {
	db.countersMutex.RLock()

	defer db.countersMutex.RUnlock()

	return maps.Clone(db.groups)
}
