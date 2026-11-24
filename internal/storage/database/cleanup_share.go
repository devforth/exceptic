package database

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The runs a size check starts and how a project's size on disk is
// measured; the rules are in docs/configuration.md (Retention).

// cleanupTarget is a scope a check may clean, with the limit it is judged by.
type cleanupTarget struct {
	project int // AllProjects or a project's ID
	limit   int64
}

// cleanupTargets are the runs a check may start, in order: each project with
// a disk share below 100%, the smallest share first, then all projects.
// Projects wait while data stored before their counters is still counted:
// their bytes and the events' project index fall short meanwhile.
func (database *Store) cleanupTargets(maxBytes int64) ([]cleanupTarget, error) {
	if maxBytes <= 0 {
		return nil, nil
	}

	var targets []cleanupTarget

	if database.backfilled() {
		projects, err := database.projects.Primary().All(database.db)

		if err != nil {
			return nil, err
		}

		slices.SortFunc(projects, func(first, second store.Project) int {
			return cmp.Or(cmp.Compare(first.Share(), second.Share()), cmp.Compare(first.ID, second.ID))
		})

		for _, project := range projects {
			if limit := store.ShareLimit(maxBytes, project.Share()); limit > 0 {
				targets = append(targets, cleanupTarget{project.ID, limit})
			}
		}
	}

	return append(targets, cleanupTarget{project: AllProjects, limit: maxBytes}), nil
}

// stopReason says why a run left running ends instead of going on: it
// started with other steps (an upgrade), or its goal came from a limit its
// scope no longer has. Empty: it goes on.
func (database *Store) stopReason(run CleanupRun, maxBytes int64) (string, error) {
	if run.Order != cleanupOrder() {
		return "stopped: the cleanup's steps changed", nil
	}

	limit, share := max(maxBytes, 0), 0

	if run.Project != AllProjects && maxBytes > 0 {
		project, err := database.projects.Get(database.db, projectKey(run.Project))

		if errors.Is(err, kv.ErrNotFound) {
			return "stopped: the project was deleted", nil
		}

		if err != nil {
			return "", err
		}

		limit, share = store.ShareLimit(maxBytes, project.Share()), project.Share()

		if limit != run.Limit && share != run.Share {
			return fmt.Sprintf("stopped: the disk share of %s changed to %d%%", project.Name, share), nil
		}
	}

	if limit == run.Limit {
		return "", nil
	}

	return limitChanged(maxBytes), nil
}

// limitChanged is why a run stops when MAX_DATABASE_SIZE is no longer the one its goal came from.
func limitChanged(maxBytes int64) string {
	if maxBytes <= 0 {
		return "stopped: MAX_DATABASE_SIZE turned off"
	}

	return "stopped: MAX_DATABASE_SIZE changed to " + config.FormatSize(maxBytes)
}

// diskUsage is how the database's files divide: the ingest inbox, the
// records no project counts (users, sessions, settings, status history,
// notifications), and the issues, events and logs, whose size on disk per
// byte counted is ratio. The deleted and replaced ones (staleBytes) take
// their place in the files until a compaction and count as nobody's.
type diskUsage struct {
	files, inbox, other int64
	ratio               float64
}

// onDisk is the size on disk of counted bytes of issues, events and logs.
func (usage diskUsage) onDisk(counted int64) int64 { return int64(float64(counted) * usage.ratio) }

func (database *Store) diskUsage() (diskUsage, error) {
	var usage diskUsage

	files, err := database.db.Size()

	if err != nil {
		return usage, err
	}

	lower, upper := database.inbox.KeyRange()

	if usage.inbox, err = database.sizeThrough(lower, upper); err != nil {
		return usage, err
	}

	// The other records are the key ranges between the inbox and the counted collections.
	collections := [][2][]byte{keyRange(database.events), keyRange(database.inbox), keyRange(database.issues), keyRange(database.logs)}
	slices.SortFunc(collections, func(first, second [2][]byte) int { return bytes.Compare(first[0], second[0]) })

	from := []byte{}

	for _, keys := range append(collections, [2][]byte{{0xff}, nil}) {
		size, err := database.sizeFrom(from, keys[0])

		if err != nil {
			return usage, err
		}

		usage.other += size
		from = keys[1]
	}

	usage.files = files
	counted := database.logicalSize() + database.staleBytes()

	// Nothing counted: the files without the inbox are the other records, the journal included.
	if counted == 0 {
		usage.other = max(files-usage.inbox, 0)
	}

	usage.ratio = float64(max(files-usage.inbox-usage.other, 0)) / float64(max(counted, 1))

	return usage, nil
}

func keyRange(collection interface{ KeyRange() ([]byte, []byte) }) [2][]byte {
	lower, upper := collection.KeyRange()

	return [2][]byte{lower, upper}
}

// The engine finds a bound's offset in a table by the index of its blocks,
// whose keys are shortened past a block's last key: the block ending a range
// may count in the range after it. sizeThrough and sizeFrom move the bounds
// to keys, so each block counts in the range of its first key.

// sizeThrough is SizeOf up to the first key at or after upper.
func (database *Store) sizeThrough(lower, upper []byte) (int64, error) {
	next := database.db.Iterate(upper, nil)
	bound := []byte{0xff}

	if next.First() {
		bound = slices.Clone(next.Key())
	}

	if err := next.Close(); err != nil {
		return 0, err
	}

	return database.db.SizeOf(lower, bound)
}

// sizeFrom is SizeOf from the first key in the range; 0 when it holds none.
func (database *Store) sizeFrom(lower, upper []byte) (int64, error) {
	first := database.db.Iterate(lower, upper)
	found := first.First()

	if found {
		lower = slices.Clone(first.Key())
	}

	if err := first.Close(); err != nil || !found {
		return 0, err
	}

	return database.db.SizeOf(lower, upper)
}

// liveSize is what the database holds: its issues, events and logs on disk
// and the other records. The inbox and what deletes and rewrites left in the
// files are left out; records still in the write journal count at ratio.
func (database *Store) liveSize() (int64, diskUsage, error) {
	usage, err := database.diskUsage()

	return usage.onDisk(database.logicalSize()) + usage.other, usage, err
}

// projectOnDisk is a project's part of liveSize: its bytes counted on disk.
func (database *Store) projectOnDisk(project int) (int64, float64, error) {
	usage, err := database.diskUsage()

	return usage.onDisk(database.projectBytes(project)), usage.ratio, err
}

// staleBytes is what deletes and rewrites (the cleanup's, a project's
// purge, deleted issues, thinned events, repeated log messages) left in the
// logs and events, in bytes counted. Issues are left out: rewritten with
// every event, theirs is mostly freed by goleveldb's own compactions of
// their few files, which do not lower it.
func (database *Store) staleBytes() int64 {
	return database.logs.Stale(database.db) + database.events.Stale(database.db)
}

// compactStale compacts the logs and events once their stale records take
// more than config.StalePercent of the bytes counted: goleveldb frees some
// of them on its own without telling, and each project is then measured
// low by up to that part. It says whether it compacted.
func (database *Store) compactStale() (bool, error) {
	if database.backups.Load() > 0 || database.staleBytes()*100 <= config.StalePercent*database.logicalSize() {
		return false, nil
	}

	return true, database.compact(database.staleData()...)
}
