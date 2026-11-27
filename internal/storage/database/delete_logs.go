package database

import (
	"context"
	"errors"
	"slices"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// DeleteLogs deletes the logs of logIDs that exist and access sees, in
// batches of up to cleanupChunk, and returns them without their attributes.
// A log's key starts with its level, so each id is looked for under every level.
func (database *Store) DeleteLogs(ctx context.Context, access store.Access, logIDs []int64) ([]store.LogRecord, error) {
	ids := slices.Compact(slices.Sorted(slices.Values(logIDs)))
	deleted := []store.LogRecord{}

	for chunk := range slices.Chunk(ids, cleanupChunk) {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}

		var gone []store.LogRecord

		err := database.update(func(writer *batchWriter) error {
			for _, id := range chunk {
				for _, rank := range allRanks {
					record, err := database.logs.Get(database.db, logKey(rank, id))

					if errors.Is(err, kv.ErrNotFound) {
						continue
					}

					if err != nil {
						return err
					}

					// Outside access: as if it were missing.
					if !access.Sees(record.ProjectID) {
						break
					}

					if _, err := database.logs.DeleteKey(writer.batch, logKey(rank, id)); err != nil {
						return err
					}

					record.Attributes = nil
					gone = append(gone, logView(record))

					break
				}
			}

			return nil
		})

		if err != nil {
			return deleted, err
		}

		deleted = append(deleted, gone...)
	}

	return deleted, nil
}

func logKey(rank byte, logID int64) kv.Key {
	return kv.K(kv.U8(rank), kv.U64(uint64(logID)))
}
