package database

import (
	"errors"
	"math"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
)

// ErrCompacting: a compaction part runs, during which ingest writes nothing; ingest answers 429.
var ErrCompacting = errors.New("the database is compacting its files")

// ErrInboxFull and ErrOverShare: AppendWithin refused the envelope by its InboxLimit.
var (
	ErrInboxFull = errors.New("the inbox is full")
	ErrOverShare = errors.New("the project holds its share of the inbox")
)

// InboxLimit bounds what AppendWithin accepts: the inbox already holding
// more than Total, or a project holding something going over Own, refuses it.
type InboxLimit struct {
	Total, Own int64
}

// Append writes an entry without a bound (AppendWithin).
func (database *Store) Append(projectID int, received time.Time, body []byte) (InboxEntry, error) {
	return database.AppendWithin(projectID, received, body, InboxLimit{Total: math.MaxInt64, Own: math.MaxInt64})
}

// AppendWithin writes without a sync: the writer's next synced batch takes it to disk.
// While a compaction part runs it refuses at once (ErrCompacting). Its bytes
// are counted before the write, checked against limit under one lock, so
// concurrent requests never all pass on one count.
func (database *Store) AppendWithin(projectID int, received time.Time, body []byte, limit InboxLimit) (entry InboxEntry, err error) {
	if !database.ingestWrites.TryRLock() {
		// Refused, it still counts as ingest: the next part waits for quiet.
		database.lastAppend.Store(time.Now().UnixNano())

		return InboxEntry{}, ErrCompacting
	}

	defer database.ingestWrites.RUnlock()

	size := int64(inboxHeader + len(body))

	if err := database.reserveInbox(projectID, size, limit); err != nil {
		return InboxEntry{}, err
	}

	defer func() {
		if err != nil {
			database.addInbox(projectID, -size)
		}
	}()

	sequence, err := database.db.Next()

	if err != nil {
		return InboxEntry{}, err
	}

	entry = InboxEntry{Sequence: sequence, ProjectID: projectID, Received: received.UTC(), Size: int(size)}
	batch := kv.NewBatch(database.db.Engine)

	if err := database.inbox.Insert(batch, inboxRecord{InboxEntry: entry, Body: body}); err != nil {
		return InboxEntry{}, err
	}

	if err := database.db.Write(batch, false); err != nil {
		return InboxEntry{}, err
	}

	database.lastAppend.Store(time.Now().UnixNano())

	return entry, nil
}

// reserveInbox counts size bytes of the project's next entry unless limit refuses them.
func (database *Store) reserveInbox(projectID int, size int64, limit InboxLimit) error {
	database.inboxMutex.Lock()

	defer database.inboxMutex.Unlock()

	queued := database.inboxProjects[projectID]

	switch {
	case database.inboxBytes.Load() > limit.Total:
		return ErrInboxFull
	case queued > 0 && queued+size > limit.Own:
		return ErrOverShare
	}

	database.inboxBytes.Add(size)
	database.inboxProjects[projectID] = queued + size

	return nil
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

// InboxBytesOf returns the size of a project's waiting envelopes.
func (database *Store) InboxBytesOf(projectID int) int64 {
	database.inboxMutex.Lock()

	defer database.inboxMutex.Unlock()

	return database.inboxProjects[projectID]
}

// addInbox counts bytes of a project's entries entering (size > 0) or leaving the inbox.
func (database *Store) addInbox(projectID int, size int64) {
	database.inboxBytes.Add(size)
	database.inboxMutex.Lock()

	defer database.inboxMutex.Unlock()

	if left := database.inboxProjects[projectID] + size; left != 0 {
		database.inboxProjects[projectID] = left
	} else {
		delete(database.inboxProjects, projectID)
	}
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
	return database.inbox.Primary().Head().Each(database.db, func(entry kv.Entry[inboxRecord]) (bool, error) {
		database.addInbox(entry.Record.ProjectID, int64(entry.Record.Size))

		return true, nil
	})
}
