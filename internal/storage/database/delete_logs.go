package database

import (
	"context"
	"slices"

	"github.com/devforth/exceptic/internal/storage/kv"
)

// DeleteLogs deletes the logs of logIDs that exist, in batches of up to
// cleanupChunk; it says how many. A log's key starts with its level, so
// each id is looked for under every level.
func (database *Store) DeleteLogs(ctx context.Context, logIDs []int64) (int, error) {
	ids := slices.Compact(slices.Sorted(slices.Values(logIDs)))
	deleted := 0

	for chunk := range slices.Chunk(ids, cleanupChunk) {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}

		gone := 0

		err := database.update(func(writer *batchWriter) error {
			for _, id := range chunk {
				for _, rank := range allRanks {
					found, err := database.logs.DeleteKey(writer.batch, logKey(rank, id))

					if err != nil {
						return err
					}

					if found {
						gone++

						break
					}
				}
			}

			return nil
		})

		if err != nil {
			return deleted, err
		}

		deleted += gone
	}

	return deleted, nil
}

func logKey(rank byte, logID int64) kv.Key {
	return kv.K(kv.U8(rank), kv.U64(uint64(logID)))
}
