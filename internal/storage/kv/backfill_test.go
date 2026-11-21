package kv

import (
	"maps"
	"testing"
)

// grouped is the test collection after it gained a counter per project and
// an index by level, with the backfill of both.
func grouped(schema *Schema) *Collection[item] {
	return Define(schema, Definition[item]{
		Name: "item",
		Key:  func(record item) Key { return K(U64(record.ID)) },
		Indexes: []Index[item]{
			{Name: "project", Key: func(record item) Key { return K(U32(record.Project), U8(record.Level), U64(record.ID)) }},
			{Name: "trace", Key: func(record item) Key { return KIf(record.Trace != "", Text(record.Trace), U64(record.ID)) }},
			{Name: "name", Key: func(record item) Key { return KIf(record.Name != "", Text(record.Name)) }, Unique: true},
			{Name: "level", Key: func(record item) Key { return K(U8(record.Level), U64(record.ID)) }},
		},
		Counter:  func(record item) Key { return K(Text("item"), U8(record.Level)) },
		Group:    func(record item) Key { return K(U32(record.Project)) },
		Backfill: &Backfill{Name: "projects", Indexes: []string{"level"}, Group: true},
	})
}

func openGrouped(t *testing.T, engine Engine) (testStore, *Schema) {
	t.Helper()

	schema := NewSchema()
	items := grouped(schema)
	db, err := OpenDB(engine, schema)

	if err != nil {
		t.Fatal(err)
	}

	return testStore{db: db, items: items}, schema
}

// counts checks that the counters and groups hold what the records take, as Verify does.
func counts(t *testing.T, store testStore, schema *Schema) {
	t.Helper()

	counters, groups, err := schema.Counted(store.db)

	if err != nil {
		t.Fatal(err)
	}

	if !maps.Equal(counters, store.db.Counters()) || !maps.Equal(groups, store.db.Groups()) {
		t.Fatalf("counters %v, groups %v; records take %v, %v", store.db.Counters(), store.db.Groups(), counters, groups)
	}

	if problems, err := schema.Check(store.db); err != nil || len(problems) > 0 {
		t.Fatalf("check: %v %v", problems, err)
	}
}

// Records stored before a collection gained a group counter and an index
// are brought up to date step by step, across a restart, while records are
// written and deleted on both sides of where it stands; the counters then
// hold what a database written with them from the start holds.
func TestBackfill(t *testing.T) {
	engine := newMemoryEngine()
	before := openStore(t, engine)

	for id := uint64(1); id <= 9; id++ {
		before.insert(t, item{ID: id, Project: uint32(1 + id%2), Level: byte(id % 3), Trace: "t"})
	}

	store, schema := openGrouped(t, engine)

	if store.items.Backfilled() || len(store.db.Groups()) != 0 {
		t.Fatalf("backfilled %v, groups %v", store.items.Backfilled(), store.db.Groups())
	}

	counts(t, store, schema)

	step := func(want bool) {
		t.Helper()

		if done, err := store.items.BackfillStep(store.db, 3); err != nil || done != want {
			t.Fatalf("step: %v %v, want done %v", done, err, want)
		}

		counts(t, store, schema)
	}

	change := func(write func(batch *Batch) error) {
		t.Helper()

		if err := store.db.Update(false, write); err != nil {
			t.Fatal(err)
		}

		counts(t, store, schema)
	}

	step(false) // 1-3 reached

	// Behind it and ahead of it: changed, deleted and new records.
	change(func(batch *Batch) error {
		return store.items.Save(batch, item{ID: 2, Project: 2, Level: 1, Name: "two"})
	})
	change(func(batch *Batch) error {
		return store.items.Save(batch, item{ID: 7, Project: 1, Level: 2, Name: "seven"})
	})
	change(func(batch *Batch) error {
		_, err := store.items.DeleteKey(batch, K(U64(3)))

		return err
	})
	change(func(batch *Batch) error {
		_, err := store.items.DeleteKey(batch, K(U64(5)))

		return err
	})
	change(func(batch *Batch) error { return store.items.Insert(batch, item{ID: 20, Project: 3, Level: 1}) })

	store, schema = openGrouped(t, engine) // goes on after a restart

	if store.items.Backfilled() {
		t.Fatal("a restart forgot the backfill")
	}

	step(false)
	step(true)

	if !store.items.Backfilled() {
		t.Fatal("not backfilled")
	}

	records, err := store.items.Primary().All(store.db)

	if err != nil {
		t.Fatal(err)
	}

	fresh, freshSchema := openGrouped(t, newMemoryEngine())

	if !fresh.items.Backfilled() {
		t.Fatal("a new database waits for a backfill")
	}

	fresh.insert(t, records...)
	counts(t, fresh, freshSchema)

	if !maps.Equal(fresh.db.Counters(), store.db.Counters()) || !maps.Equal(fresh.db.Groups(), store.db.Groups()) {
		t.Errorf("backfilled %v %v, written new %v %v", store.db.Counters(), store.db.Groups(), fresh.db.Counters(), fresh.db.Groups())
	}

	if levelOne, err := store.items.By("level", U8(1)).All(store.db); err != nil || len(levelOne) != 4 {
		t.Errorf("level 1 by the new index: %v %v", ids(levelOne), err)
	}

	store, _ = openGrouped(t, engine)

	if !store.items.Backfilled() {
		t.Error("done is forgotten after a restart")
	}

	if fresh, _ = openGrouped(t, fresh.db.Engine); !fresh.items.Backfilled() {
		t.Error("a database new with the backfill waits for it after a restart")
	}
}
