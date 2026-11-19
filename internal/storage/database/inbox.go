package database

import (
	"errors"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
)

// Append writes without a sync: the writer's next synced batch takes it to disk.
func (database *Store) Append(projectID int, received time.Time, body []byte) (InboxEntry, error) {
	sequence, err := database.db.Next()

	if err != nil {
		return InboxEntry{}, err
	}

	entry := InboxEntry{Sequence: sequence, ProjectID: projectID, Received: received.UTC(), Size: inboxHeader + len(body)}
	batch := kv.NewBatch(database.db.Engine)

	if err := database.inbox.Insert(batch, inboxRecord{InboxEntry: entry, Body: body}); err != nil {
		return InboxEntry{}, err
	}

	if err := database.db.Write(batch, false); err != nil {
		return InboxEntry{}, err
	}

	database.inboxBytes.Add(int64(entry.Size))
	database.lastAppend.Store(time.Now().UnixNano())

	return entry, nil
}

// InboxBody returns the body of an entry; found is false once it is stored.
func (database *Store) InboxBody(sequence uint64) (body []byte, found bool, err error) {
	value, err := database.inbox.GetValue(database.db, sequenceKey(sequence))

	switch {
	case errors.Is(err, kv.ErrNotFound):
		return nil, false, nil
	case err != nil:
		return nil, false, err
	}

	body, err = inboxBody(value)

	return body, err == nil, err
}

// InboxBytes returns the size of the waiting envelopes.
func (database *Store) InboxBytes() int64 {
	return database.inboxBytes.Load()
}

// Pending returns up to limit entries after the sequence number, oldest first, except skipped ones; bodies are not read.
func (database *Store) Pending(after uint64, limit int, skip func(sequence uint64) bool) ([]InboxEntry, error) {
	var entries []InboxEntry

	query := database.inbox.Primary().From(kv.U64(after + 1)).Head()

	err := query.Each(database.db, func(entry kv.Entry[inboxRecord]) (bool, error) {
		if skip == nil || !skip(entry.Record.Sequence) {
			entries = append(entries, entry.Record.InboxEntry)
		}

		return len(entries) < limit, nil
	})

	return entries, err
}

func (database *Store) loadInboxSize() error {
	var size int64

	err := database.inbox.Primary().Head().Each(database.db, func(entry kv.Entry[inboxRecord]) (bool, error) {
		size += int64(entry.Size)

		return true, nil
	})

	database.inboxBytes.Store(size)

	return err
}
