package database

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"slices"
	"time"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// EnqueueOutbox adds the message about projectIDs once per channel, due immediately.
func (database *Store) EnqueueOutbox(_ context.Context, channels []string, projectIDs []int, kind string, payload []byte, now time.Time) error {
	ids := make([]int64, len(channels))

	for index := range channels {
		sequence, err := database.db.Next()

		if err != nil {
			return err
		}

		ids[index] = int64(sequence)
	}

	return database.db.Update(true, func(batch *kv.Batch) error {
		for index, channel := range channels {
			message := store.OutboxRecord{
				OutboxMessage: store.OutboxMessage{ID: ids[index], Channel: channel, Kind: kind, ProjectIDs: projectIDs, Payload: payload, CreatedAt: now.UTC()},
				Status:        store.OutboxPending, NextAttemptAt: now.UTC(), UpdatedAt: now.UTC(),
			}

			if err := database.outbox.Insert(batch, message); err != nil {
				return err
			}
		}

		return nil
	})
}

// DueOutbox returns the pending messages of a channel whose next attempt is
// due, oldest first; a message waiting for its retry does not hold back newer
// ones. Only the due ones are read with their payload.
func (database *Store) DueOutbox(_ context.Context, channel string, now time.Time, limit int) ([]store.OutboxMessage, error) {
	var messages []store.OutboxMessage

	err := database.outbox.By("pending", kv.Text(channel)).Head().Each(database.db, func(entry kv.Entry[store.OutboxRecord]) (bool, error) {
		if entry.Record.NextAttemptAt.After(now) {
			return true, nil
		}

		message, err := database.outbox.Get(database.db, entry.Key)

		switch {
		case errors.Is(err, kv.ErrNotFound): // deleted meanwhile with its connection
			return true, nil
		case err != nil:
			return false, err
		}

		messages = append(messages, message.OutboxMessage)

		return len(messages) < limit, nil
	})

	return messages, err
}

// changeOutbox rewrites a message in one batch; a deleted one is left alone.
func (database *Store) changeOutbox(id int64, change func(message *store.OutboxRecord)) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		entry, err := database.outbox.GetEntry(batch, sequenceKey(uint64(id)))

		if err != nil {
			return ignoreMissing(err)
		}

		message := entry.Record
		change(&message)
		_, err = database.outbox.Write(batch, message, &entry)

		return err
	})
}

// ignoreMissing: a message deleted meanwhile (its connection went) needs no update.
func ignoreMissing(err error) error {
	if notFound(err) == store.ErrNotFound {
		return nil
	}

	return err
}

func (database *Store) MarkOutboxSent(_ context.Context, id int64, now time.Time) error {
	return database.changeOutbox(id, func(message *store.OutboxRecord) {
		message.Status, message.Attempts, message.LastError, message.UpdatedAt = store.OutboxSent, message.Attempts+1, "", now.UTC()
	})
}

// MarkOutboxRetry records a failed attempt and when to try again.
func (database *Store) MarkOutboxRetry(_ context.Context, id int64, lastError string, next, now time.Time) error {
	return database.changeOutbox(id, func(message *store.OutboxRecord) {
		message.Attempts, message.LastError, message.NextAttemptAt, message.UpdatedAt = message.Attempts+1, lastError, next.UTC(), now.UTC()
	})
}

// MarkOutboxFailed gives up on a message after its last attempt.
func (database *Store) MarkOutboxFailed(_ context.Context, id int64, lastError string, now time.Time) error {
	return database.changeOutbox(id, func(message *store.OutboxRecord) {
		message.Status, message.Attempts, message.LastError, message.UpdatedAt = store.OutboxFailed, message.Attempts+1, lastError, now.UTC()
	})
}

// RetryOutboxNow makes the waiting messages of a channel due now: after its
// connection is fixed they need not wait for the next step of the schedule.
func (database *Store) RetryOutboxNow(_ context.Context, channel string, now time.Time) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		return database.outbox.By("pending", kv.Text(channel)).Each(batch, func(entry kv.Entry[store.OutboxRecord]) (bool, error) {
			if !entry.Record.NextAttemptAt.After(now) {
				return true, nil
			}

			message := entry.Record
			message.NextAttemptAt = now.UTC()
			_, err := database.outbox.Write(batch, message, &entry)

			return true, err
		})
	})
}

// CleanOutbox deletes sent messages finished before sentBefore and failed ones
// before failedBefore, so the outbox does not grow forever.
func (database *Store) CleanOutbox(_ context.Context, sentBefore, failedBefore time.Time) (int64, error) {
	var deleted int64

	err := database.db.Update(true, func(batch *kv.Batch) error {
		for status, before := range map[string]time.Time{store.OutboxSent: sentBefore, store.OutboxFailed: failedBefore} {
			err := database.outbox.By("status", kv.Text(status)).Head().Each(batch, func(entry kv.Entry[store.OutboxRecord]) (bool, error) {
				if entry.Record.UpdatedAt.Before(before) {
					database.outbox.DeleteEntry(batch, entry)

					deleted++
				}

				return true, nil
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	return deleted, err
}

// HasPendingOutbox says whether a channel has messages still to send, due or not.
func (database *Store) HasPendingOutbox(_ context.Context, channel string) (bool, error) {
	count, _, err := database.outbox.By("pending", kv.Text(channel)).Count(database.db, 1)

	return count > 0, err
}

// PendingChannels lists the channels with messages still to send, so their workers start after a restart.
func (database *Store) PendingChannels(context.Context) ([]string, error) {
	var channels []string

	err := database.outbox.By("pending").Head().Each(database.db, func(entry kv.Entry[store.OutboxRecord]) (bool, error) {
		if len(channels) == 0 || channels[len(channels)-1] != entry.Record.Channel {
			channels = append(channels, entry.Record.Channel)
		}

		return true, nil
	})

	return channels, err
}

// outboxQuery walks the messages of a filter, newest first, each filter by an index.
func (database *Store) outboxQuery(filter store.OutboxFilter) kv.Query[store.OutboxRecord] {
	var query kv.Query[store.OutboxRecord]

	switch {
	case filter.Channel != "":
		query = database.outbox.By("channel", kv.Text(filter.Channel))
	case filter.Status != "":
		query = database.outbox.By("status", kv.Text(filter.Status))
	default:
		query = database.outbox.Primary()
	}

	if filter.Channels != nil || !filter.Access.All() || (filter.Status != "" && filter.Channel != "") {
		query = query.Where(func(message store.OutboxRecord) bool {
			return (filter.Status == "" || message.Status == filter.Status) &&
				(filter.Channels == nil || slices.Contains(filter.Channels, message.Channel)) &&
				filter.Access.SeesEach(message.ProjectIDs)
		})
	}

	return query.Newest()
}

// ListOutbox returns outbox messages for the delivery log, newest first.
func (database *Store) ListOutbox(_ context.Context, filter store.OutboxFilter) ([]store.OutboxRecord, error) {
	var (
		records []store.OutboxRecord
		skipped int
	)

	err := database.outboxQuery(filter).Each(database.db, func(entry kv.Entry[store.OutboxRecord]) (bool, error) {
		if skipped < filter.Offset {
			skipped++

			return true, nil
		}

		records = append(records, entry.Record)

		return filter.Limit <= 0 || len(records) < filter.Limit, nil
	})

	return records, err
}

// CountOutbox counts the messages ListOutbox would page through.
func (database *Store) CountOutbox(_ context.Context, filter store.OutboxFilter) (int, error) {
	count, _, err := database.outboxQuery(filter).Head().Count(database.db, 0)

	return count, err
}

// DeliveryHealthByChannel sums up the delivery of every channel with messages.
// A sent message has no error, so the latest error belongs to a retrying or failed one.
func (database *Store) DeliveryHealthByChannel(context.Context) (map[string]store.DeliveryHealth, error) {
	health := map[string]store.DeliveryHealth{}

	err := database.outbox.Primary().Head().Each(database.db, func(entry kv.Entry[store.OutboxRecord]) (bool, error) {
		message, channel := entry.Record, health[entry.Record.Channel]

		switch message.Status {
		case store.OutboxPending:
			channel.Pending++

			if message.Attempts > 0 {
				channel.Retrying++
			}
		case store.OutboxFailed:
			channel.Failed++
		case store.OutboxSent:
			if message.UpdatedAt.After(channel.LastSentAt) {
				channel.LastSentAt = message.UpdatedAt
			}
		}

		if message.LastError != "" && message.UpdatedAt.After(channel.LastErrorAt) {
			channel.LastError, channel.LastErrorAt = message.LastError, message.UpdatedAt
		}

		health[message.Channel] = channel

		return true, nil
	})

	return health, err
}

// Outbox messages keep their payload after a JSON head, so a walk does not
// decode it as base64, and a walk with Head does not read it.
func encodeOutbox(message store.OutboxRecord) ([]byte, error) {
	head := message
	head.Payload = nil
	tail := binary.BigEndian.AppendUint32(make([]byte, 0, 4+len(message.Payload)), uint32(len(message.Payload)))

	return withTail(head, append(tail, message.Payload...))
}

func decodeOutbox(value []byte) (store.OutboxRecord, error) {
	var message store.OutboxRecord

	end, err := decodeHead(value, &message)

	if err != nil {
		return message, err
	}

	tail := value[end:]

	if len(tail) < 4 || int(binary.BigEndian.Uint32(tail)) > len(tail)-4 {
		return message, errShortRecord
	}

	// Bytes after the payload are an event copied by an earlier version.
	message.Payload = bytes.Clone(tail[4 : 4+int(binary.BigEndian.Uint32(tail))])

	return message, nil
}

func decodeOutboxHead(value []byte) (store.OutboxRecord, error) {
	var message store.OutboxRecord

	_, err := decodeHead(value, &message)

	return message, err
}
