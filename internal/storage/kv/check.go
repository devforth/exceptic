package kv

import (
	"bytes"
	"errors"
	"fmt"
)

func (schema *Schema) addCollection(collection checkable) {
	schema.mutex.Lock()
	defer schema.mutex.Unlock()

	schema.collections = append(schema.collections, collection)
}

// Check runs Check on every collection of the schema.
func (schema *Schema) Check(reader Reader) ([]string, error) {
	var problems []string

	for _, collection := range schema.collections {
		found, err := collection.Check(reader)
		problems = append(problems, found...)

		if err != nil {
			return problems, err
		}
	}

	return problems, nil
}

// Counted adds up the record sizes per counter and per group counter over
// every collection of the schema.
func (schema *Schema) Counted(reader Reader) (counters, groups map[string]int64, err error) {
	counters, groups = map[string]int64{}, map[string]int64{}

	for _, collection := range schema.collections {
		sizes, groupSizes, err := collection.Counted(reader)

		if err != nil {
			return counters, groups, err
		}

		for counter, size := range sizes {
			counters[counter] += size
		}

		for counter, size := range groupSizes {
			groups[counter] += size
		}
	}

	return counters, groups, nil
}

// Counted adds up the stored record sizes per counter and per group counter,
// as Write counts them; DB.Counters and DB.Groups should hold the same.
func (collection *Collection[T]) Counted(reader Reader) (counters, groups map[string]int64, err error) {
	counters, groups = map[string]int64{}, map[string]int64{}

	if !collection.counted() {
		return counters, groups, nil
	}

	err = collection.Primary().Each(reader, func(entry Entry[T]) (bool, error) {
		batch := &Batch{counts: counters, groups: groups}
		collection.tally(batch, entry.Record, collection.primaryKey(entry.Key), entry.Size, 1)

		return true, nil
	})

	return counters, groups, err
}

// Check verifies that records and index keys point to each other.
func (collection *Collection[T]) Check(reader Reader) ([]string, error) {
	var problems []string

	err := collection.Primary().Each(reader, func(entry Entry[T]) (bool, error) {
		primary := collection.primaryKey(entry.Key)

		if !bytes.Equal(collection.KeyOf(entry.Record), entry.Key) {
			problems = append(problems, fmt.Sprintf("%s %x: stored under another key than its own", collection.name, primary))
		}

		reached := collection.reached(primary)

		for _, key := range collection.indexKeys(entry.Record) {
			if key.backfilled && !reached {
				continue
			}

			holder, err := reader.Get(key.key)

			switch {
			case errors.Is(err, ErrNotFound):
				problems = append(problems, fmt.Sprintf("%s %x: index %s key %x missing", collection.name, primary, key.index, key.key))
			case err != nil:
				return false, err
			case !bytes.Equal(holder, primary):
				problems = append(problems, fmt.Sprintf("%s %x: index %s key %x points to %x", collection.name, primary, key.index, key.key, holder))
			}
		}

		return true, nil
	})

	if err != nil {
		return problems, err
	}

	for _, entry := range collection.order {
		iterator := reader.Iterate(entry.prefix, prefixEnd(entry.prefix))

		for valid := iterator.First(); valid; valid = iterator.Next() {
			key, primary := bytes.Clone(iterator.Key()), bytes.Clone(iterator.Value())
			value, err := reader.Get(primary)

			if errors.Is(err, ErrNotFound) {
				problems = append(problems, fmt.Sprintf("%s.%s key %x points to missing %x", collection.name, entry.Name, key, primary))

				continue
			}

			if err != nil {
				_ = iterator.Close()

				return problems, err
			}

			record, err := collection.definition.Decode(value)

			if err != nil {
				_ = iterator.Close()

				return problems, err
			}

			if own := entry.Key(record); own == nil || !bytes.Equal(entry.fullKey(own), key) {
				problems = append(problems, fmt.Sprintf("%s.%s key %x is not the key of the record it points to", collection.name, entry.Name, key))
			}
		}

		if err := iterator.Close(); err != nil {
			return problems, err
		}
	}

	return problems, nil
}
