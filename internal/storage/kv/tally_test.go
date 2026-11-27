package kv

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
)

// ticket is a record whose collection keeps tallies: how many tickets each
// project has per status.
type ticket struct {
	ID      uint64
	Project uint32
	Status  byte
	Seen    uint64
	Opened  uint64
}

func defineTickets(schema *Schema) *Collection[ticket] {
	return Define(schema, Definition[ticket]{
		Name: "ticket",
		Key:  func(record ticket) Key { return K(U64(record.ID)) },
		Indexes: []Index[ticket]{
			{Name: "seen", Key: func(record ticket) Key { return K(U8(record.Status), U64(record.Seen), U64(record.ID)) }},
			{Name: "opened", Key: func(record ticket) Key { return K(U64(record.Opened), U64(record.ID)) }},
		},
		Tally: func(record ticket) []Key { return []Key{K(Text("ticket"), U32(record.Project), U8(record.Status))} },
	})
}

type ticketStore struct {
	db      *DB
	tickets *Collection[ticket]
}

func openTickets(t *testing.T, engine Engine) ticketStore {
	t.Helper()

	schema := NewSchema()
	tickets := defineTickets(schema)
	db, err := OpenDB(engine, schema)

	if err != nil {
		t.Fatal(err)
	}

	return ticketStore{db: db, tickets: tickets}
}

func (store ticketStore) update(t *testing.T, change func(batch *Batch) error) {
	t.Helper()

	if err := store.db.Update(false, change); err != nil {
		t.Fatal(err)
	}
}

func (store ticketStore) tally(project uint32, status byte) int64 {
	return store.db.Tally(K(Text("ticket"), U32(project), U8(status)))
}

// Tallies count records per key: a write adds, a delete takes off, a
// rewrite moves the record between keys; they survive a reopen and Check
// finds them right.
func TestTallies(t *testing.T) {
	engine := newMemoryEngine()
	store := openTickets(t, engine)

	store.update(t, func(batch *Batch) error {
		for id := uint64(1); id <= 3; id++ {
			if err := store.tickets.Insert(batch, ticket{ID: id, Project: 1, Seen: id, Opened: id}); err != nil {
				return err
			}
		}

		return store.tickets.Insert(batch, ticket{ID: 4, Project: 2, Seen: 4, Opened: 4})
	})

	if store.tally(1, 0) != 3 || store.tally(2, 0) != 1 {
		t.Fatalf("after inserts: %v", store.db.Tallies())
	}

	// Ticket 2 is resolved: it moves from status 0 to status 1.
	store.update(t, func(batch *Batch) error {
		entry, err := store.tickets.GetEntry(batch, K(U64(2)))

		if err != nil {
			return err
		}

		changed := entry.Record
		changed.Status = 1
		_, err = store.tickets.Write(batch, changed, &entry)

		return err
	})

	if store.tally(1, 0) != 2 || store.tally(1, 1) != 1 {
		t.Fatalf("after the status change: %v", store.db.Tallies())
	}

	// A rewrite that keeps the status keeps the tally.
	store.update(t, func(batch *Batch) error {
		entry, err := store.tickets.GetEntry(batch, K(U64(1)))

		if err != nil {
			return err
		}

		changed := entry.Record
		changed.Seen = 10
		_, err = store.tickets.Write(batch, changed, &entry)

		return err
	})

	if store.tally(1, 0) != 2 {
		t.Fatalf("after a rewrite in the same status: %v", store.db.Tallies())
	}

	store.update(t, func(batch *Batch) error {
		if _, err := store.tickets.DeleteKey(batch, K(U64(4))); err != nil {
			return err
		}

		return store.tickets.Delete(batch, ticket{ID: 3, Project: 1, Seen: 3, Opened: 3})
	})

	if store.tally(1, 0) != 1 || store.tally(2, 0) != 0 {
		t.Fatalf("after deletes: %v", store.db.Tallies())
	}

	if _, ok := store.db.Tallies()[string(K(Text("ticket"), U32(2), U8(0)))]; ok {
		t.Error("a tally at zero is kept")
	}

	reopened := openTickets(t, engine)

	if reopened.tally(1, 0) != 1 || reopened.tally(1, 1) != 1 {
		t.Fatalf("after a reopen: %v", reopened.db.Tallies())
	}

	if problems, err := reopened.tickets.Check(reopened.db); err != nil || len(problems) > 0 {
		t.Fatalf("check: %v %v", problems, err)
	}

	// Check finds a tally that does not match the records.
	reopened.update(t, func(batch *Batch) error {
		batch.Tally(K(Text("ticket"), U32(1), U8(0)), 5)

		return nil
	})

	if problems, err := reopened.db.CheckTallies(); err != nil || len(problems) == 0 {
		t.Errorf("a wrong tally passed: %v %v", problems, err)
	}
}

// A rewrite writes only the index keys that changed: one whose key stays
// the same is neither deleted nor put again.
func TestWriteKeepsUnchangedIndexKeys(t *testing.T) {
	store := openTickets(t, newMemoryEngine())

	store.update(t, func(batch *Batch) error {
		return store.tickets.Insert(batch, ticket{ID: 1, Project: 1, Seen: 1, Opened: 1})
	})

	writes := 0

	store.update(t, func(batch *Batch) error {
		entry, err := store.tickets.GetEntry(batch, K(U64(1)))

		if err != nil {
			return err
		}

		changed := entry.Record
		changed.Seen = 2 // moves "seen"; "opened" stays
		_, err = store.tickets.Write(batch, changed, &entry)
		writes = batch.Len()

		return err
	})

	// The record, the old "seen" key deleted, the new one put.
	if writes != 3 {
		t.Errorf("writes %d, want 3: the unchanged opened key was written again", writes)
	}

	if problems, err := store.tickets.Check(store.db); err != nil || len(problems) > 0 {
		t.Fatalf("check: %v %v", problems, err)
	}
}

// A rewrite leaves stale only what it wrote again: the old value and the
// index keys that moved, not the keys that stayed.
func TestRewriteStaleLeavesUnchangedKeysOut(t *testing.T) {
	schema := NewSchema()
	items := Define(schema, Definition[ticket]{
		Name: "counted",
		Key:  func(record ticket) Key { return K(U64(record.ID)) },
		Indexes: []Index[ticket]{
			{Name: "seen", Key: func(record ticket) Key { return K(U64(record.Seen), U64(record.ID)) }},
			{Name: "opened", Key: func(record ticket) Key { return K(U64(record.Opened), U64(record.ID)) }},
		},
		Counter: func(ticket) Key { return K(Text("counted")) },
	})
	db, err := OpenDB(newMemoryEngine(), schema)

	if err != nil {
		t.Fatal(err)
	}

	var entry Entry[ticket]

	_ = db.Update(false, func(batch *Batch) error {
		entry, err = items.Write(batch, ticket{ID: 1, Seen: 1, Opened: 1}, nil)

		return err
	})

	_ = db.Update(false, func(batch *Batch) error {
		changed := entry.Record
		changed.Seen = 2
		_, err = items.Write(batch, changed, &entry)

		return err
	})

	primary := len(items.primaryKey(K(U64(1))))
	movedKey := len(items.index("seen").fullKey(K(U64(1), U64(1))))
	want := int64(primary + entry.Size + movedKey + primary)

	if stale := items.Stale(db); stale != want {
		t.Errorf("stale %d, want %d: the record and the moved key only", stale, want)
	}
}

// Merge walks several prefixes of an index as one list ordered by what
// follows their prefix: tickets of two statuses by when they were seen,
// both ways, the keys a walk skips taking their turn.
func TestMergeBySuffix(t *testing.T) {
	store := openTickets(t, newMemoryEngine())

	store.update(t, func(batch *Batch) error {
		for _, record := range []ticket{
			{ID: 1, Status: 0, Seen: 50}, {ID: 2, Status: 1, Seen: 40}, {ID: 3, Status: 0, Seen: 30},
			{ID: 4, Status: 1, Seen: 20}, {ID: 5, Status: 0, Seen: 10}, {ID: 6, Status: 1, Seen: 40},
		} {
			if err := store.tickets.Insert(batch, record); err != nil {
				return err
			}
		}

		return nil
	})

	walks := func() []Query[ticket] {
		return []Query[ticket]{store.tickets.By("seen", U8(0)), store.tickets.By("seen", U8(1)).Where(func(record ticket) bool { return record.ID != 4 })}
	}
	order := func(newest bool) []uint64 {
		t.Helper()

		var seen []uint64

		err := Merge(store.db, walks(), newest, func(entry Entry[ticket]) (bool, error) {
			seen = append(seen, entry.Record.ID)

			return true, nil
		})

		if err != nil {
			t.Fatal(err)
		}

		return seen
	}

	// Ties (seen 40) go by the key that follows, the ID.
	if got := order(true); !slices.Equal(got, []uint64{1, 6, 2, 3, 5}) {
		t.Errorf("newest first %v", got)
	}

	if got := order(false); !slices.Equal(got, []uint64{5, 3, 2, 6, 1}) {
		t.Errorf("oldest first %v", got)
	}

	var first []uint64

	_ = Merge(store.db, walks(), true, func(entry Entry[ticket]) (bool, error) {
		first = append(first, entry.Record.ID)

		return len(first) < 2, nil
	})

	if !slices.Equal(first, []uint64{1, 6}) {
		t.Errorf("stopped after two: %v", first)
	}
}

// A walk ended by its context ends the merge with the context's error.
func TestMergeStopsWithItsContext(t *testing.T) {
	store := openTickets(t, newMemoryEngine())

	store.update(t, func(batch *Batch) error {
		for id := uint64(1); id <= 4; id++ {
			if err := store.tickets.Insert(batch, ticket{ID: id, Status: byte(id % 2), Seen: id}); err != nil {
				return err
			}
		}

		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	visited := 0

	err := Merge(store.db, []Query[ticket]{store.tickets.By("seen", U8(0)).WithContext(ctx), store.tickets.By("seen", U8(1))}, true,
		func(Entry[ticket]) (bool, error) {
			visited++

			cancel()

			return true, nil
		})

	if !errors.Is(err, context.Canceled) || visited > 2 {
		t.Errorf("visited %d, err %v: the merge went on past its context", visited, err)
	}
}

// A walk goes on from the entry it stopped at.
func TestFromRest(t *testing.T) {
	store := openTickets(t, newMemoryEngine())

	store.update(t, func(batch *Batch) error {
		for id := uint64(1); id <= 5; id++ {
			if err := store.tickets.Insert(batch, ticket{ID: id, Seen: id * 10}); err != nil {
				return err
			}
		}

		return nil
	})

	var rest Key

	_ = store.tickets.By("seen", U8(0)).Limit(3).Each(store.db, func(entry Entry[ticket]) (bool, error) {
		rest = entry.Rest

		return true, nil
	})

	var up []uint64

	_ = store.tickets.By("seen", U8(0)).FromRest(rest).Each(store.db, func(entry Entry[ticket]) (bool, error) {
		up = append(up, entry.Record.ID)

		return true, nil
	})

	if !slices.Equal(up, []uint64{3, 4, 5}) {
		t.Errorf("from the third on: %v", up)
	}
}

// A Tally added to records stored without one is counted once by Recount, in
// batches, and never again; a new database has nothing to count.
func TestRecountAtOpen(t *testing.T) {
	engine := newMemoryEngine()
	define := func(recount bool) (*DB, *Collection[ticket]) {
		schema := NewSchema()
		definition := Definition[ticket]{Name: "ticket", Key: func(record ticket) Key { return K(U64(record.ID)) }}

		if recount {
			definition.Tally = func(record ticket) []Key { return []Key{K(Text("ticket"), U32(record.Project), U8(record.Status))} }
			definition.Recount = "tickets"
		}

		tickets := Define(schema, definition)
		db, err := OpenDB(engine, schema)

		if err == nil {
			err = db.Recount()
		}

		if err != nil {
			t.Fatal(err)
		}

		return db, tickets
	}

	db, tickets := define(false)
	stored := recountBatch*2 + 7

	if err := db.Update(false, func(batch *Batch) error {
		for id := 1; id <= stored; id++ {
			if err := tickets.Insert(batch, ticket{ID: uint64(id), Project: uint32(id % 2)}); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		db, _ = define(true)

		if odd := db.Tally(K(Text("ticket"), U32(1), U8(0))); odd != int64(stored/2+1) {
			t.Fatalf("odd tickets counted %d, want %d", odd, stored/2+1)
		}

		if problems, err := db.CheckTallies(); err != nil || len(problems) > 0 {
			t.Fatalf("check: %v %v", problems, err)
		}
	}

	engine = newMemoryEngine()
	db, tickets = define(true)

	if value, err := db.Get(db.recountKey("ticket", "tickets")); err != nil || !bytes.Equal(value, []byte{backfillDone}) {
		t.Fatalf("a new database's recount %q %v, want done", value, err)
	}

	if err := db.Update(false, func(batch *Batch) error { return tickets.Insert(batch, ticket{ID: 1, Project: 3}) }); err != nil {
		t.Fatal(err)
	}

	if db, _ = define(true); db.Tally(K(Text("ticket"), U32(3), U8(0))) != 1 {
		t.Errorf("a new database counted again: %v", db.Tallies())
	}
}
