package kv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
)

// Definition declares a collection: name, primary key, indexes and encoding.
type Definition[T any] struct {
	Name    string
	Key     func(T) Key
	Indexes []Index[T]
	// Encode and Decode default to JSON.
	Encode func(T) ([]byte, error)
	Decode func([]byte) (T, error)
	// DecodeHead, when set, decodes what a walk with Head needs: the fields
	// the keys and lists use, without the large part of the value.
	DecodeHead func([]byte) (T, error)
	// Counter, when set, counts the bytes a record takes, index keys included.
	Counter func(T) Key
	// Group, when set, counts the same bytes once more under another key
	// (such as the record's project), apart from Counter's: groups add up
	// to no total of the database (DB.Groups).
	Group func(T) Key
	// Backfill names what the collection gained after records were stored.
	Backfill *Backfill
	// Tally, when set, counts each record once under each of its keys (how
	// many records of a project have a status, say), kept exact in the same
	// batch as the record (DB.Tally). A Backfill may add indexes but no
	// Group: tallies count every record from the start, reached or not.
	Tally func(T) []Key
}

// Index declares an index; its keys hold the record's primary key as their value.
type Index[T any] struct {
	Name string
	// Key returns nil to leave the record out. A non-unique index key must
	// still be unique per record: end it with the record's own key parts.
	Key func(T) Key
	// Unique refuses a key another record holds (ErrConflict).
	Unique bool
}

// Collection is built with Define.
type Collection[T any] struct {
	name       string
	prefix     Key
	definition Definition[T]
	indexes    map[string]*index[T]
	order      []*index[T]
	// position is where the backfill goes on: records at or after it are not
	// reached yet; nil once done (backfill.go).
	position atomic.Pointer[Key]
}

type index[T any] struct {
	Index[T]
	prefix     Key
	backfilled bool // added by the collection's Backfill
}

// Define registers a collection under "<name>/" and its indexes under "<name>.<index>/".
func Define[T any](schema *Schema, definition Definition[T]) *Collection[T] {
	if definition.Encode == nil {
		definition.Encode = func(record T) ([]byte, error) { return json.Marshal(record) }
	}

	if definition.Decode == nil {
		definition.Decode = func(value []byte) (T, error) {
			var record T

			return record, json.Unmarshal(value, &record)
		}
	}

	collection := &Collection[T]{
		name: definition.Name, prefix: Key(definition.Name + "/"), definition: definition, indexes: map[string]*index[T]{},
	}

	if definition.Tally != nil && definition.Backfill != nil && definition.Backfill.Group {
		panic(fmt.Sprintf("kv: collection %s cannot have both Tally and a Backfill of its Group", definition.Name))
	}

	schema.register(string(collection.prefix), "collection "+definition.Name)
	schema.addCollection(collection)

	for _, declared := range definition.Indexes {
		prefix := Key(definition.Name + "." + declared.Name + "/")
		schema.register(string(prefix), "index "+definition.Name+"."+declared.Name)

		entry := &index[T]{Index: declared, prefix: prefix}
		entry.backfilled = definition.Backfill != nil && slices.Contains(definition.Backfill.Indexes, declared.Name)

		if entry.backfilled && declared.Unique {
			panic(fmt.Sprintf("kv: backfilled index %s.%s cannot be unique", definition.Name, declared.Name))
		}

		collection.indexes[declared.Name] = entry
		collection.order = append(collection.order, entry)
	}

	return collection
}

// KeyRange covers the keys of the collection and its indexes, and nothing else.
func (collection *Collection[T]) KeyRange() (lower, upper []byte) {
	return []byte(collection.name + "."), prefixEnd(collection.prefix)
}

// Stale is what the collection's deletes and rewrites left in the files
// since Compact last ran, in bytes counted: the engine frees a deleted
// record's space only when a compaction reaches it. Its own compactions are
// not told, so it may count bytes they freed.
func (collection *Collection[T]) Stale(db *DB) int64 {
	db.countersMutex.RLock()

	defer db.countersMutex.RUnlock()

	return db.stale[collection.name]
}

// Compact compacts the collection's keys, which frees what its deletes and
// rewrites left in the files, and takes that off Stale. What is deleted
// while it runs stays counted: the compaction may not have reached it.
func (collection *Collection[T]) Compact(db *DB) error {
	db.compactMutex.Lock()
	defer db.compactMutex.Unlock()

	mark := collection.StaleMark(db)

	if err := db.CompactRange(collection.KeyRange()); err != nil {
		return err
	}

	_, err := collection.TakeStale(db, mark)

	return err
}

// StaleMark is all the collection's deletes and rewrites have ever added to
// Stale: what a compaction starting now will free, for TakeStale.
func (collection *Collection[T]) StaleMark(db *DB) int64 {
	db.countersMutex.RLock()

	defer db.countersMutex.RUnlock()

	return db.staled[collection.name]
}

// TakeStale takes off Stale what was added up to mark (StaleMark when a
// compaction started) and no compaction has taken off yet, and returns it:
// two compactions over the same records take them off once, and records
// deleted after mark stay counted.
func (collection *Collection[T]) TakeStale(db *DB, mark int64) (int64, error) {
	var taken int64

	err := db.Update(true, func(batch *Batch) error {
		db.countersMutex.RLock()
		stale, staled := db.stale[collection.name], db.staled[collection.name]
		db.countersMutex.RUnlock()

		// staled-stale is what compactions took off so far.
		taken = min(max(mark-(staled-stale), 0), stale)
		batch.addStale(collection.name, -taken)

		return nil
	})

	if err != nil {
		return 0, err
	}

	return taken, nil
}

// Name is the collection's name, which its counters and Stale go by.
func (collection *Collection[T]) Name() string { return collection.name }

// PrefixEnd is the first key after every key that starts with prefix; nil when there is none.
func PrefixEnd(prefix []byte) []byte { return prefixEnd(prefix) }

// KeyOf returns the primary key of record, without the prefix.
func (collection *Collection[T]) KeyOf(record T) Key { return collection.definition.Key(record) }

func (collection *Collection[T]) primaryKey(key Key) []byte {
	return append(bytes.Clone(collection.prefix), key...)
}

// fullKey is key with the index's prefix.
func (entry *index[T]) fullKey(key Key) []byte { return append(bytes.Clone(entry.prefix), key...) }

func (collection *Collection[T]) indexKeys(record T) []indexKey {
	keys := make([]indexKey, 0, len(collection.order))

	for _, entry := range collection.order {
		if key := entry.Key(record); key != nil {
			keys = append(keys, indexKey{key: entry.fullKey(key), index: entry.Name, unique: entry.Unique, backfilled: entry.backfilled})
		}
	}

	return keys
}

type indexKey struct {
	key        []byte
	index      string
	unique     bool
	backfilled bool
}

// ErrConflict means a unique index already holds the key for another record.
type ErrConflict struct {
	Collection, Index string
}

func (err ErrConflict) Error() string {
	return fmt.Sprintf("kv: %s.%s is already taken", err.Collection, err.Index)
}

// Get reads the record under key, or returns ErrNotFound.
func (collection *Collection[T]) Get(reader Reader, key Key) (T, error) {
	value, err := reader.Get(collection.primaryKey(key))

	if err != nil {
		var zero T

		return zero, err
	}

	return collection.definition.Decode(value)
}

// GetValue returns the stored bytes under key, or ErrNotFound, for a caller that slices them instead of decoding a copy.
func (collection *Collection[T]) GetValue(reader Reader, key Key) ([]byte, error) {
	return reader.Get(collection.primaryKey(key))
}

// Has reports whether a record is stored under key, without reading its value.
func (collection *Collection[T]) Has(reader Reader, key Key) (bool, error) {
	primary := collection.primaryKey(key)
	iterator := reader.Iterate(primary, append(bytes.Clone(primary), 0))
	found := iterator.First()

	return found, iterator.Close()
}

// GetEntry reads the record under key with its stored size, for Write.
func (collection *Collection[T]) GetEntry(reader Reader, key Key) (Entry[T], error) {
	value, err := reader.Get(collection.primaryKey(key))

	if err != nil {
		return Entry[T]{}, err
	}

	record, err := collection.definition.Decode(value)

	return Entry[T]{Key: key, Record: record, Size: len(value)}, err
}

// GetBy reads the record a unique index points to, or returns ErrNotFound.
func (collection *Collection[T]) GetBy(reader Reader, indexName string, key Key) (T, error) {
	primary, err := reader.Get(collection.index(indexName).fullKey(key))

	if err != nil {
		var zero T

		return zero, err
	}

	value, err := reader.Get(primary)

	if err != nil {
		var zero T

		return zero, err
	}

	return collection.definition.Decode(value)
}

// Taken reports whether a unique index holds key.
func (collection *Collection[T]) Taken(reader Reader, indexName string, key Key) (bool, error) {
	_, err := reader.Get(collection.index(indexName).fullKey(key))

	switch {
	case errors.Is(err, ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}

	return true, nil
}

func (collection *Collection[T]) index(name string) *index[T] {
	entry, ok := collection.indexes[name]

	if !ok {
		panic(fmt.Sprintf("kv: collection %s has no index %q", collection.name, name))
	}

	return entry
}

// Insert writes a record that is not stored yet; use Save to replace one.
func (collection *Collection[T]) Insert(batch *Batch, record T) error {
	_, err := collection.Write(batch, record, nil)

	return err
}

// Save writes a record, replacing the stored one and its old index keys.
func (collection *Collection[T]) Save(batch *Batch, record T) error {
	key := collection.primaryKey(collection.KeyOf(record))
	value, err := batch.Get(key)

	switch {
	case errors.Is(err, ErrNotFound):
		return collection.Insert(batch, record)
	case err != nil:
		return err
	}

	old, err := collection.definition.Decode(value)

	if err != nil {
		return fmt.Errorf("kv: %s %x: %w", collection.name, key, err)
	}

	_, err = collection.Write(batch, record, &Entry[T]{Record: old, Size: len(value)})

	return err
}

// Write writes record over old (nil when none is stored) without reading the
// stored version, for callers that keep it in memory.
func (collection *Collection[T]) Write(batch *Batch, record T, old *Entry[T]) (Entry[T], error) {
	key := collection.primaryKey(collection.KeyOf(record))
	value, err := collection.definition.Encode(record)

	if err != nil {
		return Entry[T]{}, fmt.Errorf("kv: %s: %w", collection.name, err)
	}

	keys := collection.indexKeys(record)

	for _, entry := range keys {
		if !entry.unique {
			continue
		}

		holder, err := batch.Get(entry.key)

		switch {
		case errors.Is(err, ErrNotFound):
		case err != nil:
			return Entry[T]{}, err
		case !bytes.Equal(holder, key):
			return Entry[T]{}, ErrConflict{Collection: collection.name, Index: entry.index}
		}
	}

	// Index keys the old record had too stay as they are: neither deleted nor put again.
	unchanged := map[string]bool{}

	if old != nil {
		collection.tally(batch, old.Record, key, old.Size, -1)
		collection.countRecord(batch, old.Record, -1)

		current := map[string]bool{}

		for _, entry := range keys {
			current[string(entry.key)] = true
		}

		for _, entry := range collection.indexKeys(old.Record) {
			if current[string(entry.key)] {
				unchanged[string(entry.key)] = true

				// A key not written again leaves nothing stale in the files.
				if collection.counted() && (!entry.backfilled || collection.reached(key)) {
					batch.addStale(collection.name, -int64(len(entry.key)+len(key)))
				}
			} else {
				batch.Delete(entry.key)
			}
		}
	}

	batch.Put(key, value)

	for _, entry := range keys {
		if !unchanged[string(entry.key)] {
			batch.Put(entry.key, key)
		}
	}

	collection.tally(batch, record, key, len(value), 1)
	collection.countRecord(batch, record, 1)

	return Entry[T]{Key: key[len(collection.prefix):], Record: record, Size: len(value)}, nil
}

// Entry is a stored record with its key (without the prefix) and value size.
type Entry[T any] struct {
	Key    Key
	Record T
	Size   int
	// Rest is, for an entry of a walk, the walked key after the walk's
	// prefix (an index key's too): what Merge orders several walks by.
	Rest Key
}

// Delete deletes record with its index keys; DeleteEntry avoids re-encoding it.
func (collection *Collection[T]) Delete(batch *Batch, record T) error {
	size := 0

	if collection.counted() {
		value, err := collection.definition.Encode(record)

		if err != nil {
			return fmt.Errorf("kv: %s: %w", collection.name, err)
		}

		size = len(value)
	}

	collection.remove(batch, record, size)

	return nil
}

// DeleteEntry deletes a record read by a walk.
func (collection *Collection[T]) DeleteEntry(batch *Batch, entry Entry[T]) {
	collection.remove(batch, entry.Record, entry.Size)
}

func (collection *Collection[T]) remove(batch *Batch, record T, valueSize int) {
	key := collection.primaryKey(collection.KeyOf(record))
	batch.Delete(key)

	for _, entry := range collection.indexKeys(record) {
		batch.Delete(entry.key)
	}

	collection.tally(batch, record, key, valueSize, -1)
	collection.countRecord(batch, record, -1)
}

// countRecord adds (sign 1) or takes away (-1) the record from its tallies.
func (collection *Collection[T]) countRecord(batch *Batch, record T, sign int64) {
	if collection.definition.Tally == nil {
		return
	}

	for _, tally := range collection.definition.Tally(record) {
		batch.Tally(tally, sign)
	}
}

// Tallied counts the stored records per tally, as Write counts them;
// DB.Tallies should hold the same.
func (collection *Collection[T]) Tallied(reader Reader) (map[string]int64, error) {
	tallies := map[string]int64{}

	if collection.definition.Tally == nil {
		return tallies, nil
	}

	err := collection.Primary().Each(reader, func(entry Entry[T]) (bool, error) {
		for _, tally := range collection.definition.Tally(entry.Record) {
			tallies[string(tally)]++
		}

		return true, nil
	})

	return tallies, err
}

// DeleteKey deletes the record under key; false when there is none.
func (collection *Collection[T]) DeleteKey(batch *Batch, key Key) (bool, error) {
	value, err := batch.Get(collection.primaryKey(key))

	switch {
	case errors.Is(err, ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}

	record, err := collection.definition.Decode(value)

	if err != nil {
		return false, fmt.Errorf("kv: %s: %w", collection.name, err)
	}

	collection.remove(batch, record, len(value))

	return true, nil
}

// recordSize counts the key, value and each index key with its value.
func recordSize(keySize, valueSize int, keys []indexKey) int {
	size := keySize + valueSize

	for _, entry := range keys {
		size += len(entry.key) + keySize
	}

	return size
}

func (collection *Collection[T]) counted() bool {
	return collection.definition.Counter != nil || collection.definition.Group != nil
}

// tally adds (sign 1) or takes away (-1) the bytes of the record stored
// under primary to its counters. A record the backfill has not reached is
// counted as before it: without the backfilled indexes, in no group the
// backfill adds.
func (collection *Collection[T]) tally(batch *Batch, record T, primary []byte, valueSize int, sign int64) {
	if !collection.counted() {
		return
	}

	keys := collection.indexKeys(record)
	reached := collection.reached(primary)

	if !reached {
		keys = slices.DeleteFunc(keys, func(entry indexKey) bool { return entry.backfilled })
	}

	size := sign * int64(recordSize(len(primary), valueSize, keys))

	if sign < 0 {
		batch.addStale(collection.name, -size)
	}

	if collection.definition.Counter != nil {
		batch.Count(collection.definition.Counter(record), size)
	}

	if group := collection.group(record); group != nil && (reached || !collection.definition.Backfill.Group) {
		batch.Group(group, size)
	}
}

func (collection *Collection[T]) group(record T) Key {
	if collection.definition.Group == nil {
		return nil
	}

	return collection.definition.Group(record)
}
