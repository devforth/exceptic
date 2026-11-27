package database

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The audit log: appended to, read newest first, and never deleted from;
// it counts as the other records in the database's size.

func (database *Store) AppendAudit(_ context.Context, entries []store.AuditEntry) error {
	for index := range entries {
		sequence, err := database.db.Next()

		if err != nil {
			return err
		}

		entries[index].ID = int64(sequence)
		entries[index].CreatedAt = entries[index].CreatedAt.UTC()
	}

	return database.db.Update(true, func(batch *kv.Batch) error {
		for _, entry := range entries {
			if err := database.audit.Insert(batch, entry); err != nil {
				return err
			}
		}

		return nil
	})
}

func (database *Store) auditQuery(ctx context.Context, filter store.AuditFilter) kv.Query[store.AuditEntry] {
	return database.audit.Primary().Newest().WithContext(ctx).Where(filter.Matches)
}

func (database *Store) ListAudit(ctx context.Context, filter store.AuditFilter) ([]store.AuditEntry, error) {
	entries := []store.AuditEntry{}
	skipped := 0

	err := database.auditQuery(ctx, filter).Each(database.db, func(entry kv.Entry[store.AuditEntry]) (bool, error) {
		if skipped < filter.Offset {
			skipped++

			return true, nil
		}

		entries = append(entries, entry.Record)

		return filter.Limit <= 0 || len(entries) < filter.Limit, nil
	})

	return entries, err
}

func (database *Store) GetAudit(_ context.Context, id int64) (store.AuditEntry, error) {
	if id <= 0 {
		return store.AuditEntry{}, store.ErrNotFound
	}

	entry, err := database.audit.Get(database.db, sequenceKey(uint64(id)))

	if errors.Is(err, kv.ErrNotFound) {
		return store.AuditEntry{}, store.ErrNotFound
	}

	return entry, err
}

func (database *Store) CountAudit(ctx context.Context, filter store.AuditFilter, most int) (int, bool, error) {
	return database.auditQuery(ctx, filter).Count(database.db, most)
}

// AuditChoices reads the tallies only (auditTallies): no entry is read.
func (database *Store) AuditChoices(_ context.Context, projectIDs []int) (store.AuditChoices, error) {
	choices := store.AuditChoices{}
	seen := map[byte]map[string]bool{auditFacetResource: {}, auditFacetAction: {}, auditFacetUser: {}}

	for key := range database.db.Tallies() {
		decoder := kv.Decode(kv.Key(key), 0)

		if decoder.Text() != "audit" {
			continue
		}

		project := int(decoder.U32())

		if len(projectIDs) > 0 && !slices.Contains(projectIDs, project) {
			continue
		}

		if project != 0 && !slices.Contains(choices.ProjectIDs, project) {
			choices.ProjectIDs = append(choices.ProjectIDs, project)
		}

		if values, ok := seen[decoder.U8()]; ok {
			values[decoder.Text()] = true
		}
	}

	choices.Resources = slices.Collect(maps.Keys(seen[auditFacetResource]))
	choices.Actions = slices.Collect(maps.Keys(seen[auditFacetAction]))
	choices.UserIDs = slices.Collect(maps.Keys(seen[auditFacetUser]))

	return choices, nil
}
