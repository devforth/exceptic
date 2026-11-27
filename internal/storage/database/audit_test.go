package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The audit log lists newest first within an access, and nothing deletes it:
// not deleting the user who acted, deleting and purging the project it was
// about, nor the deepest cleanup.
func TestAuditIsNeverDeleted(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	clockAt(database, now)

	if _, err := database.CreateUser(ctx, store.User{Email: "root@example.com", Role: store.RoleAdmin, CreatedAt: now}, "hash"); err != nil {
		t.Fatal(err)
	}

	userID, err := database.CreateUser(ctx, store.User{Email: "admin@example.com", Role: store.RoleAdmin, CreatedAt: now}, "hash")

	if err != nil {
		t.Fatal(err)
	}

	projectID, err := database.CreateProject(ctx, store.Project{Name: "gone"}, now, "")

	if err != nil {
		t.Fatal(err)
	}

	actor := store.AuditActor{UserID: userID, Name: "admin", Email: "admin@example.com", Role: store.RoleAdmin}
	entries := []store.AuditEntry{
		{CreatedAt: now, Actor: actor, Source: store.SourceUI, Resource: store.AuditProject, Action: store.AuditCreate,
			RecordName: "gone", ProjectIDs: []int{projectID}, After: map[string]json.RawMessage{"name": json.RawMessage(`"gone"`)}},
		{CreatedAt: now, Actor: actor, Source: store.SourceUI, Resource: store.AuditUser, Action: store.AuditCreate, RecordName: "all@example.com"},
	}

	if err := database.AppendAudit(ctx, entries); err != nil {
		t.Fatal(err)
	}

	writeLogs(t, database, "error", 10, "log")
	writeEvents(t, database, "busy", "error", 5)

	if err := database.DeleteUser(ctx, userID); err != nil {
		t.Fatal(err)
	}

	if err := database.DeleteProject(ctx, projectID); err != nil {
		t.Fatal(err)
	}

	if err := database.PurgeProjects(ctx); err != nil {
		t.Fatal(err)
	}

	if _, _, err := database.cleanTo(ctx, 0); err != nil {
		t.Fatal(err)
	}

	listed, err := database.ListAudit(ctx, store.AuditFilter{})

	if err != nil || len(listed) != 2 || listed[0].Resource != store.AuditUser || listed[1].RecordName != "gone" || listed[1].Actor.Email != "admin@example.com" {
		t.Fatalf("audit after the deletes: %+v, %v", listed, err)
	}

	// A user of the project sees its entry, not the one of no project.
	seen, err := database.ListAudit(ctx, store.AuditFilter{Access: store.OnlyProjects(projectID)})

	if err != nil || len(seen) != 1 || seen[0].Resource != store.AuditProject {
		t.Errorf("seen within the project: %+v, %v", seen, err)
	}

	if count, more, err := database.CountAudit(ctx, store.AuditFilter{Resources: []string{store.AuditUser}}, 10); err != nil || count != 1 || more {
		t.Errorf("count of user entries %d (more %v), %v", count, more, err)
	}

	checkCollections(t, database)
}

// The filter's choices come from counts written with each entry: per
// project (the items' for several records, 0 for none), resource, action
// and user (none for the API token).
func TestAuditChoices(t *testing.T) {
	database := openTest(t)
	appendAuditSamples(t, database)
	checkAuditChoices(t, database)
}

// A database whose entries were stored before the counts recounts them once
// when opened, and never again.
func TestAuditChoicesRecountedAtOpen(t *testing.T) {
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	appendAuditSamples(t, database)
	forgetAuditChoices(t, database)

	for range 2 {
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}

		database = openOnDisk(t, dir)
		checkAuditChoices(t, database)
		checkCollections(t, database)
	}
}

// A database of another format is refused before the recount writes to it.
func TestRecountWaitsForTheFormat(t *testing.T) {
	dir := t.TempDir()
	database := openOnDisk(t, dir)
	appendAuditSamples(t, database)
	forgetAuditChoices(t, database)

	if err := database.db.Update(true, func(batch *kv.Batch) error { batch.Put(database.formatKey, []byte("0")); return nil }); err != nil {
		t.Fatal(err)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(dir); !errors.Is(err, errFormat) {
		t.Fatalf("another format opened: %v", err)
	}

	engine, err := openEngineReadOnly(Location(dir))

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = engine.Close() }()

	if _, err := engine.Get(kv.Key("meta/").With(kv.Text("recount"), kv.Text("audit"), kv.Text("choices"))); !errors.Is(err, kv.ErrNotFound) {
		t.Errorf("the refused database got the recount's mark: %v", err)
	}
}

// forgetAuditChoices deletes the audit tallies and the recount's mark, as
// in a database stored before the counts.
func forgetAuditChoices(t *testing.T, database *Store) {
	t.Helper()

	batch := kv.NewBatch(database.db.Engine)
	tallies := kv.Key("tally/").With(kv.Text("audit"))
	iterator := database.db.Iterate(tallies, kv.Key("tally0"))

	for valid := iterator.First(); valid; valid = iterator.Next() {
		if bytes.HasPrefix(iterator.Key(), tallies) {
			batch.Delete(bytes.Clone(iterator.Key()))
		}
	}

	if err := iterator.Close(); err != nil {
		t.Fatal(err)
	}

	batch.Delete(kv.Key("meta/").With(kv.Text("recount"), kv.Text("audit"), kv.Text("choices")))

	if batch.Len() < 2 {
		t.Fatalf("%d keys deleted, want the tallies and the recount's mark", batch.Len())
	}

	if err := database.db.Write(batch, true); err != nil {
		t.Fatal(err)
	}
}

func appendAuditSamples(t *testing.T, database *Store) {
	t.Helper()

	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	admin := store.AuditActor{UserID: "admin001", Name: "admin", Role: store.RoleAdmin}
	entries := []store.AuditEntry{
		{CreatedAt: now, Actor: admin, Resource: store.AuditProject, Action: store.AuditEdit, ProjectIDs: []int{1}},
		{CreatedAt: now, Actor: store.AuditActor{Name: "API token"}, Resource: store.AuditIssue, Action: store.AuditDelete, ProjectIDs: []int{2},
			Items: []store.AuditItem{{RecordID: "a", ProjectID: 2}, {RecordID: "b", ProjectID: 3}}},
		{CreatedAt: now, Actor: store.AuditActor{UserID: "user0001", Name: "user"}, Resource: store.AuditUser, Action: store.AuditCreate},
	}

	if err := database.AppendAudit(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
}

func checkAuditChoices(t *testing.T, database *Store) {
	t.Helper()

	ctx := context.Background()
	sorted := func(choices store.AuditChoices) store.AuditChoices {
		slices.Sort(choices.Resources)
		slices.Sort(choices.Actions)
		slices.Sort(choices.UserIDs)
		slices.Sort(choices.ProjectIDs)

		return choices
	}

	all, err := database.AuditChoices(ctx, nil)
	want := store.AuditChoices{
		Resources: []string{store.AuditIssue, store.AuditProject, store.AuditUser}, Actions: []string{store.AuditCreate, store.AuditDelete, store.AuditEdit},
		UserIDs: []string{"admin001", "user0001"}, ProjectIDs: []int{1, 2, 3},
	}

	if all = sorted(all); err != nil || !reflect.DeepEqual(all, want) {
		t.Errorf("every project's choices %+v %v, want %+v", all, err, want)
	}

	// Project 3 is only an item's: the API token's delete, no user.
	third, err := database.AuditChoices(ctx, []int{3})
	want = store.AuditChoices{Resources: []string{store.AuditIssue}, Actions: []string{store.AuditDelete}, ProjectIDs: []int{3}}

	if third = sorted(third); err != nil || !reflect.DeepEqual(third, want) {
		t.Errorf("project 3's choices %+v %v, want %+v", third, err, want)
	}
}
