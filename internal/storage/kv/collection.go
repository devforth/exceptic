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

	if old != nil {
		collection.tally(batch, old.Record, key, old.Size, -1)

		kept := map[string]bool{}

		for _, entry := range keys {
			kept[string(entry.key)] = true
		}

		for _, entry := range collection.indexKeys(old.Record) {
			if !kept[string(entry.key)] {
				batch.Delete(entry.key)
			}
		}
	}

	batch.Put(key, value)

	for _, entry := range keys {
		batch.Put(entry.key, key)
	}

	collection.tally(batch, record, key, len(value), 1)

	return Entry[T]{Key: key[len(collection.prefix):], Record: record, Size: len(value)}, nil
}

// Entry is a stored record with its key (without the prefix) and value size.
type Entry[T any] struct {
	Key    Key
	Record T
	Size   int
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
