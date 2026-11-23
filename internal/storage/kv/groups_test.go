package kv

import (
	"fmt"
	"testing"
)

// countingReader counts the iterator moves of the walks it serves.
type countingReader struct {
	Reader

	moves *int
}

func (reader countingReader) Iterate(lower, upper []byte) Iterator {
	return countingIterator{reader.Reader.Iterate(lower, upper), reader.moves}
}

type countingIterator struct {
	Iterator

	moves *int
}

func (it countingIterator) First() bool { *it.moves++; return it.Iterator.First() }

func (it countingIterator) Next() bool { *it.moves++; return it.Iterator.Next() }

func (it countingIterator) Seek(key []byte) bool { *it.moves++; return it.Iterator.Seek(key) }

type member struct {
	Group    string
	Sequence uint64
}

func firstText(key Key) Key { return K(Text(Decode(key, 0).Text())) }

// Groups reads one key per group, however many keys a group has, and goes
// on from a position.
func TestGroupsSeekPastEachGroup(t *testing.T) {
	schema := NewSchema()
	members := Define(schema, Definition[member]{
		Name: "member",
		Key:  func(record member) Key { return K(Text(record.Group), U64(record.Sequence)) },
	})
	// Another collection after it, whose keys a walk must not reach.
	others := defineItems(schema)
	db, err := OpenDB(newMemoryEngine(), schema)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	sizes := map[string]int{"a": 1000, "a\x00b": 2, "ab": 3, "b": 1, "c": 500}

	err = db.Update(false, func(batch *Batch) error {
		for group, size := range sizes {
			for sequence := range size {
				if err := members.Insert(batch, member{group, uint64(sequence)}); err != nil {
					return err
				}
			}
		}

		return others.Insert(batch, item{ID: 1})
	})

	if err != nil {
		t.Fatal(err)
	}

	walk := func(query Query[member]) ([]string, int) {
		t.Helper()

		var seen []string

		moves := 0
		err := query.Groups(countingReader{db, &moves}, firstText, func(key Key) (bool, error) {
			decoder := Decode(key, 0)
			seen = append(seen, fmt.Sprintf("%s/%d", decoder.Text(), decoder.U64()))

			return true, nil
		})

		if err != nil {
			t.Fatal(err)
		}

		return seen, moves
	}

	seen, moves := walk(members.Primary())

	if want := "[a/0 a\x00b/0 ab/0 b/0 c/0]"; fmt.Sprint(seen) != want {
		t.Errorf("groups %q, want %q", seen, want)
	}

	// The first key, then one seek per group.
	if moves != len(sizes)+1 {
		t.Errorf("%d moves for %d groups of %d keys", moves, len(sizes), 1506)
	}

	if seen, _ := walk(members.Primary().From(Text("ab"))); fmt.Sprint(seen) != "[ab/0 b/0 c/0]" {
		t.Errorf("from ab: %q", seen)
	}
}
