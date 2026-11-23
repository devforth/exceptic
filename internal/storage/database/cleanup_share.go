package database

import (
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

// stopReason says why a run left running ends instead of going on: its
// goal came from a limit its scope no longer has. Empty: it goes on.
func (database *Store) stopReason(run CleanupRun, maxBytes int64) (string, error) {
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

// projectOnDisk is a project's part of the files: its bytes counted times diskRatio.
func (database *Store) projectOnDisk(project int) (int64, float64, error) {
	ratio, err := database.diskRatio()

	return int64(float64(database.projectBytes(project)) * ratio), ratio, err
}

// diskRatio is the size on disk of a byte counted: the files without the
// ingest inbox's, per byte counted and per byte of the logs and events
// deleted or replaced since their last compaction (staleBytes), which keep
// their space until then and belong to no project.
func (database *Store) diskRatio() (float64, error) {
	size, err := database.db.Size()

	if err != nil {
		return 0, err
	}

	inbox, err := database.db.SizeOf(database.inbox.KeyRange())

	if err != nil {
		return 0, err
	}

	return float64(max(size-inbox, 0)) / float64(max(database.logicalSize()+database.staleBytes(), 1)), nil
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
// low by up to that part.
func (database *Store) compactStale() error {
	if database.backups.Load() > 0 || database.staleBytes()*100 <= config.StalePercent*database.logicalSize() {
		return nil
	}

	return database.compact(database.staleData()...)
}
