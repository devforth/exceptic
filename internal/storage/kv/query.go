package kv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
)

// Query walks the keys of a collection or an index under a prefix, in key
// order or newest first.
type Query[T any] struct {
	collection   *Collection[T]
	index        *index[T] // nil: the primary keys
	prefix       Key
	lower, upper Key
	newest       bool
	limit        int
	where        func(T) bool
	raw          func(value []byte) bool
	keyed        func(key Key) bool
	head         bool
	ctx          context.Context // nil: the walk is not stopped
}

// Primary walks the records whose key starts with parts.
func (collection *Collection[T]) Primary(parts ...Part) Query[T] {
	return Query[T]{collection: collection, prefix: collection.prefix.With(parts...)}
}

// By walks the index name from the keys that start with parts.
func (collection *Collection[T]) By(name string, parts ...Part) Query[T] {
	entry := collection.index(name)

	return Query[T]{collection: collection, index: entry, prefix: entry.prefix.With(parts...)}
}

// From keeps the keys at or after the prefix followed by parts.
func (query Query[T]) From(parts ...Part) Query[T] {
	query.lower = query.prefix.With(parts...)

	return query
}

// Before keeps the keys before the prefix followed by parts.
func (query Query[T]) Before(parts ...Part) Query[T] {
	query.upper = query.prefix.With(parts...)

	return query
}

// Newest walks from the last key to the first.
func (query Query[T]) Newest() Query[T] {
	query.newest = true

	return query
}

// Limit stops after most records (0: no limit).
func (query Query[T]) Limit(most int) Query[T] {
	query.limit = most

	return query
}

// WithContext stops the walk with ctx's error once ctx ends.
func (query Query[T]) WithContext(ctx context.Context) Query[T] {
	query.ctx = ctx

	return query
}

// Where keeps the records match accepts; the walk still reads every record.
func (query Query[T]) Where(match func(T) bool) Query[T] {
	query.where = match

	return query
}

// Head decodes only the head of each record (Definition.DecodeHead) when the
// collection has one; such a record can be deleted but not written back.
func (query Query[T]) Head() Query[T] {
	query.head = true

	return query
}

// Raw skips, before decoding, the records whose stored value match refuses:
// a cheap prefilter; Where still decides on the record.
func (query Query[T]) Raw(match func(value []byte) bool) Query[T] {
	query.raw = match

	return query
}

// Keyed skips the records whose primary key (without the prefix) match
// refuses, before reading them: on an index, before the record it points to.
func (query Query[T]) Keyed(match func(key Key) bool) Query[T] {
	query.keyed = match

	return query
}

func (query Query[T]) bounds() ([]byte, []byte) {
	lower, upper := []byte(query.prefix), prefixEnd(query.prefix)

	if query.lower != nil && bytes.Compare(query.lower, lower) > 0 {
		lower = query.lower
	}

	if query.upper != nil && (upper == nil || bytes.Compare(query.upper, upper) < 0) {
		upper = query.upper
	}

	return lower, upper
}

// Each calls visit for each record until it returns false or the limit is reached.
// Index keys pointing to missing records are skipped.
func (query Query[T]) Each(reader Reader, visit func(Entry[T]) (bool, error)) error {
	cursor := query.Open(reader)

	for {
		entry, ok := cursor.Next()

		if !ok {
			return cursor.Close()
		}

		more, err := visit(entry)

		if err != nil || !more {
			return errors.Join(err, cursor.Close())
		}
	}
}

// Keys walks the primary keys without reading the records; Where does not apply.
func (query Query[T]) Keys(reader Reader, visit func(key Key) (bool, error)) error {
	lower, upper := query.bounds()
	iterator := reader.Iterate(lower, upper)
	found := 0

	move, valid := iterator.Next, iterator.First()

	if query.newest {
		move, valid = iterator.Prev, iterator.Last()
	}

	for ; valid; valid = move() {
		key := iterator.Key()

		if query.index != nil {
			key = iterator.Value()
		}

		more, err := visit(bytes.Clone(key[len(query.collection.prefix):]))

		if err != nil || !more {
			return errors.Join(err, iterator.Close())
		}

		if found++; query.limit > 0 && found >= query.limit {
			break
		}
	}

	return iterator.Close()
}

// Groups visits, in key order, the first primary key of each group: the keys
// that start with what group returns for that first key (a prefix of it). It
// seeks past each group, so it reads one key per group, however large; Where
// and Limit do not apply.
func (query Query[T]) Groups(reader Reader, group func(key Key) Key, visit func(first Key) (bool, error)) error {
	if query.index != nil || query.newest {
		panic("kv: Groups walks the primary keys in order")
	}

	lower, upper := query.bounds()
	iterator := reader.Iterate(lower, upper)
	base := query.collection.prefix

	for valid := iterator.First(); valid; {
		key := bytes.Clone(iterator.Key())
		more, err := visit(key[len(base):])

		if err != nil || !more {
			return errors.Join(err, iterator.Close())
		}

		prefix := group(key[len(base):])

		if len(prefix) == 0 || !bytes.HasPrefix(key[len(base):], prefix) {
			return errors.Join(fmt.Errorf("kv: group %x is not a prefix of key %x", prefix, key), iterator.Close())
		}

		next := prefixEnd(append(bytes.Clone(base), prefix...))

		if next == nil {
			break
		}

		valid = iterator.Seek(next)
	}

	return iterator.Close()
}

// Cursor reads a walk one record at a time.
type Cursor[T any] struct {
	query    Query[T]
	reader   Reader
	iterator Iterator
	started  bool
	found    int
	err      error
	stopped  bool
}

// Open starts the walk; Close the cursor when done.
func (query Query[T]) Open(reader Reader) *Cursor[T] {
	lower, upper := query.bounds()

	return &Cursor[T]{query: query, reader: reader, iterator: reader.Iterate(lower, upper)}
}

// Next returns false at the end or on an error, which Close returns.
func (cursor *Cursor[T]) Next() (Entry[T], bool) {
	for {
		entry, kept, ok := cursor.Step()

		if !ok || kept {
			return entry, ok
		}
	}
}

// Step reads one key: its entry and kept when the walk keeps the record; a
// key it skips (Raw, Where, an index key without its record) gives an entry
// with only its Key, valid until the next Step. ok is false at the end, on
// an error and when the context ends the walk.
func (cursor *Cursor[T]) Step() (entry Entry[T], kept, ok bool) {
	query, iterator := cursor.query, cursor.iterator

	if cursor.err != nil || (query.limit > 0 && cursor.found >= query.limit) {
		return Entry[T]{}, false, false
	}

	if query.ctx != nil && query.ctx.Err() != nil {
		cursor.err, cursor.stopped = query.ctx.Err(), true

		return Entry[T]{}, false, false
	}

	var valid bool

	switch {
	case !cursor.started && query.newest:
		valid = iterator.Last()
	case !cursor.started:
		valid = iterator.First()
	case query.newest:
		valid = iterator.Prev()
	default:
		valid = iterator.Next()
	}

	cursor.started = true

	if !valid {
		return Entry[T]{}, false, false
	}

	entry, found, err := query.read(cursor.reader, iterator)

	if err != nil {
		cursor.err = err

		return Entry[T]{}, false, false
	}

	if found && (query.where == nil || query.where(entry.Record)) {
		cursor.found++

		return entry, true, true
	}

	return Entry[T]{Key: iterator.Key()}, false, true
}

// Stopped reports whether the walk's context ended it.
func (cursor *Cursor[T]) Stopped() bool { return cursor.stopped }

// Close releases the walk and returns its first error.
func (cursor *Cursor[T]) Close() error {
	return errors.Join(cursor.err, cursor.iterator.Close())
}

// read decodes the record at the iterator, following an index to its record.
func (query Query[T]) read(reader Reader, iterator Iterator) (Entry[T], bool, error) {
	key, value := iterator.Key(), iterator.Value()
	primary := key

	if query.index != nil {
		primary = value
	}

	if query.keyed != nil && !query.keyed(Key(primary[len(query.collection.prefix):])) {
		return Entry[T]{}, false, nil
	}

	if query.index != nil {
		primary := bytes.Clone(value)
		stored, err := reader.Get(primary)

		if errors.Is(err, ErrNotFound) {
			return Entry[T]{}, false, nil
		}

		if err != nil {
			return Entry[T]{}, false, err
		}

		key, value = primary, stored
	}

	if query.raw != nil && !query.raw(value) {
		return Entry[T]{}, false, nil
	}

	decode := query.collection.definition.Decode

	if query.head && query.collection.definition.DecodeHead != nil {
		decode = query.collection.definition.DecodeHead
	}

	record, err := decode(value)

	if err != nil {
		return Entry[T]{}, false, fmt.Errorf("kv: %s %x: %w", query.collection.name, key, err)
	}

	return Entry[T]{Key: bytes.Clone(key[len(query.collection.prefix):]), Record: record, Size: len(value)}, true, nil
}

// All returns the records of the walk.
func (query Query[T]) All(reader Reader) ([]T, error) {
	var records []T

	err := query.Each(reader, func(entry Entry[T]) (bool, error) {
		records = append(records, entry.Record)

		return true, nil
	})

	return records, err
}

// First returns the first record of the walk, or ErrNotFound.
func (query Query[T]) First(reader Reader) (Entry[T], error) {
	var first *Entry[T]

	err := query.Limit(1).Each(reader, func(entry Entry[T]) (bool, error) {
		first = &entry

		return false, nil
	})

	if err == nil && first == nil {
		err = ErrNotFound
	}

	if first == nil {
		return Entry[T]{}, err
	}

	return *first, err
}

// Count counts the records up to most (0: all); more says there are more.
// Without Where, Raw and Keyed it counts keys and reads no record, so an index key
// whose record is gone counts too.
func (query Query[T]) Count(reader Reader, most int) (count int, more bool, err error) {
	visit := func() bool {
		if most > 0 && count == most {
			more = true

			return false
		}

		count++

		return true
	}

	if query.where == nil && query.raw == nil && query.keyed == nil {
		err = query.Limit(0).Keys(reader, func(Key) (bool, error) { return visit(), nil })
	} else {
		err = query.Limit(0).Each(reader, func(Entry[T]) (bool, error) { return visit(), nil })
	}

	return count, more, err
}
