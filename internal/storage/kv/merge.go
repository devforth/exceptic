package kv

import (
	"bytes"
	"errors"
)

// Merge walks queries as one list ordered by what follows each one's prefix
// (Entry.Rest): oldest first, or newest first with newest; equal ones in the
// order of queries. It visits the records the walks keep until visit returns
// false; keys a walk skips advance it in their turn, so the walks move
// together and one ended by its context ends the merge.
func Merge[T any](reader Reader, queries []Query[T], newest bool, visit func(entry Entry[T]) (bool, error)) error {
	type head struct {
		cursor *Cursor[T]
		entry  Entry[T]
		kept   bool
		valid  bool
	}

	heads := make([]*head, 0, len(queries))

	defer func() {
		for _, current := range heads {
			_ = current.cursor.Close()
		}
	}()

	step := func(current *head) {
		current.entry, current.kept, current.valid = current.cursor.Step()
		// A skipped key's Rest refers to the iterator: keep it past the next step.
		current.entry.Rest = bytes.Clone(current.entry.Rest)
	}

	for _, query := range queries {
		if newest {
			query = query.Newest()
		}

		current := &head{cursor: query.Open(reader)}
		heads = append(heads, current)
		step(current)
	}

	for {
		var next *head

		for _, current := range heads {
			if !current.valid {
				continue
			}

			if next == nil {
				next = current

				continue
			}

			order := bytes.Compare(current.entry.Rest, next.entry.Rest)

			if (newest && order > 0) || (!newest && order < 0) {
				next = current
			}
		}

		if next == nil {
			break
		}

		for _, current := range heads {
			if current.cursor.Stopped() {
				return current.cursor.err
			}
		}

		if next.kept {
			more, err := visit(next.entry)

			if err != nil || !more {
				return err
			}
		}

		step(next)
	}

	var errs []error

	for _, current := range heads {
		errs = append(errs, current.cursor.err)
	}

	return errors.Join(errs...)
}
