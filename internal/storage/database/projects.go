package database

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// deleteChunk is how many logs one batch of a project deletion removes.
const deleteChunk = 5000

// projectIDs: a project added in the UI gets a random 8-digit id (PROJECTS sets its own).
const (
	firstProjectID = 10_000_000
	projectIDRange = 90_000_000
)

// keepNone keeps "no level" (a set but empty list) apart from nil, "the default", through JSON.
func keepNone(project store.Project) store.Project {
	if project.LogLevels != nil && *project.LogLevels == nil {
		project.LogLevels = &[]string{}
	}

	return project
}

func (database *Store) ListProjects(context.Context) ([]store.Project, error) {
	return database.projects.Primary().All(database.db)
}

func (database *Store) GetProject(_ context.Context, id int) (store.Project, error) {
	if id < 1 || id > config.MaxProjectID {
		return store.Project{}, store.ErrNotFound
	}

	project, err := database.projects.Get(database.db, projectKey(id))

	return project, notFound(err)
}

// projectConflict turns a taken name or key into ErrProjectConflict.
func projectConflict(err error) error {
	if conflict := (kv.ErrConflict{}); errors.As(err, &conflict) {
		return fmt.Errorf("%w: %s", store.ErrProjectConflict, conflict.Index)
	}

	return err
}

// CreateProject adds a project with a random 8-digit id that is not taken,
// nor was by a project deleted since the start; a limited owner (ownerID,
// "" for none) gets it in the same batch, so they see it at once.
func (database *Store) CreateProject(_ context.Context, project store.Project, now time.Time, ownerID string) (int, error) {
	project.CreatedAt = now.UTC()

	err := database.db.Update(true, func(batch *kv.Batch) error {
		for {
			number, err := rand.Int(rand.Reader, big.NewInt(projectIDRange))

			if err != nil {
				return err
			}

			project.ID = firstProjectID + int(number.Int64())

			taken, err := database.projects.Has(batch, projectKey(project.ID))

			if err != nil {
				return err
			}

			if !taken && !database.goneProjects[project.ID] {
				break
			}
		}

		if err := projectConflict(database.projects.Insert(batch, keepNone(project))); err != nil {
			return err
		}

		if ownerID == "" {
			return nil
		}

		return database.addProjectToUser(batch, ownerID, project.ID)
	})

	return project.ID, err
}

// UpdateProject saves the settings of a project.
func (database *Store) UpdateProject(_ context.Context, project store.Project) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		stored, err := database.projects.Get(batch, projectKey(project.ID))

		if err != nil {
			return notFound(err)
		}

		project.CreatedAt = stored.CreatedAt

		return projectConflict(database.projects.Save(batch, keepNone(project)))
	})
}

// SeedProject writes a project from PROJECTS: created with its name if
// missing, otherwise only its key is set to the env value (the name and the
// settings from the web app stay). A name or key taken by another project is
// an error, so a typo in PROJECTS cannot take over another DSN.
func (database *Store) SeedProject(_ context.Context, project store.Project, now time.Time) error {
	err := database.db.Update(true, func(batch *kv.Batch) error {
		stored, err := database.projects.Get(batch, projectKey(project.ID))

		switch {
		case errors.Is(err, kv.ErrNotFound):
			project.CreatedAt = now.UTC()

			batch.AfterWrite(func() { delete(database.goneProjects, project.ID) })

			return projectConflict(database.projects.Insert(batch, project))
		case err != nil:
			return err
		case stored.PublicKey == project.PublicKey:
			return nil
		}

		stored.PublicKey = project.PublicKey

		return projectConflict(database.projects.Save(batch, stored))
	})

	if err != nil {
		return fmt.Errorf("project %d %q: %w", project.ID, project.Name, err)
	}

	return nil
}

// projectPurge marks a deleted project whose issues and logs are still to delete.
type projectPurge struct {
	ProjectID int `json:"project_id"`
}

func (database *Store) loadPurges() error {
	return database.purges.Primary().Each(database.db, func(entry kv.Entry[projectPurge]) (bool, error) {
		database.goneProjects[entry.Record.ProjectID] = true

		return true, nil
	})
}

// projectContact is when a project first sent a request with its key, of
// any kind (a session, a client report, an event): the project is connected.
type projectContact struct {
	ProjectID int       `json:"project_id"`
	At        time.Time `json:"at"`
}

func (database *Store) loadContacts() error {
	return database.contacts.Primary().Each(database.db, func(entry kv.Entry[projectContact]) (bool, error) {
		database.contacted[entry.Record.ProjectID] = entry.Record.At

		return true, nil
	})
}

// FirstContact is when the project first sent a request with its key; ok is
// false before that.
func (database *Store) FirstContact(projectID int) (at time.Time, ok bool) {
	database.contactsMutex.RLock()

	defer database.contactsMutex.RUnlock()

	at, ok = database.contacted[projectID]

	return at, ok
}

// MarkContact records the project's first contact at at, once: when it is
// known it neither reads nor writes the database. written says it was new.
func (database *Store) MarkContact(projectID int, at time.Time) (written bool, err error) {
	if _, known := database.FirstContact(projectID); known {
		return false, nil
	}

	err = database.db.Update(true, func(batch *kv.Batch) error {
		// Updates run one at a time: another may have marked it meanwhile.
		if _, known := database.FirstContact(projectID); known || database.goneProjects[projectID] {
			return nil
		}

		contact := projectContact{ProjectID: projectID, At: at.UTC()}
		written = true

		batch.AfterWrite(func() {
			database.contactsMutex.Lock()
			database.contacted[projectID] = contact.At
			database.contactsMutex.Unlock()
		})

		return database.contacts.Save(batch, contact)
	})

	return written && err == nil, err
}

// DeleteProject deletes the project and its place in connections and users'
// access in one batch,
// so its DSN stops at once and no later event or log of it is stored, and
// marks its data for PurgeProjects.
func (database *Store) DeleteProject(_ context.Context, id int) error {
	return database.db.Update(true, func(batch *kv.Batch) error {
		project, err := database.projects.Get(batch, projectKey(id))

		if err != nil {
			return notFound(err)
		}

		if err := database.projects.Delete(batch, project); err != nil {
			return err
		}

		if err := database.purges.Save(batch, projectPurge{ProjectID: id}); err != nil {
			return err
		}

		if err := database.contacts.Delete(batch, projectContact{ProjectID: id}); err != nil {
			return err
		}

		batch.AfterWrite(func() {
			database.goneProjects[id] = true

			database.contactsMutex.Lock()
			delete(database.contacted, id)
			database.contactsMutex.Unlock()
		})

		if err := database.dropProjectFromConnections(batch, id); err != nil {
			return err
		}

		return database.dropProjectFromUsers(batch, id)
	})
}

// PurgeProjects deletes the issues, events and logs of deleted projects in
// batches; one stopped (ctx, a restart) goes on at the next call.
func (database *Store) PurgeProjects(ctx context.Context) error {
	purges, err := database.purges.Primary().All(database.db)

	if err != nil {
		return err
	}

	for _, purge := range purges {
		if err := database.purgeProject(ctx, purge.ProjectID); err != nil {
			return err
		}
	}

	if len(purges) == 0 {
		return nil
	}

	// Not under ingest: compactStale takes what is left once it is over its share.
	_, err = database.compact(false, database.logs, database.events)

	return err
}

func (database *Store) purgeProject(ctx context.Context, id int) error {
	// Added again by PROJECTS since: its data stays with it.
	if again, err := database.projects.Has(database.db, projectKey(id)); err != nil || again {
		if err != nil {
			return err
		}

		return database.db.Update(true, func(batch *kv.Batch) error {
			_, err := database.purges.DeleteKey(batch, projectKey(id))

			return err
		})
	}

	// The project's issues by the index, in chunks that DeleteIssues spreads over batches.
	for ctx.Err() == nil {
		var issueIDs []string

		err := database.walkIssues(ctx, store.IssueFilter{ProjectIDs: []int{id}, Sort: store.SortLastSeenAsc}, func(issue issueRecord) bool {
			issueIDs = append(issueIDs, issue.ID)

			return len(issueIDs) < cleanupChunk
		})

		if err != nil {
			return err
		}

		if len(issueIDs) == 0 {
			break
		}

		if _, err := database.DeleteIssues(ctx, issueIDs); err != nil {
			return err
		}
	}

	// Each chunk goes on after the last log deleted, so no walk passes deleted keys again.
	logs := database.logs.By("project", kv.U32(uint32(id)))

	for ctx.Err() == nil {
		deleted := 0

		err := database.update(func(writer *batchWriter) error {
			err := logs.Limit(deleteChunk).Each(database.db, func(entry kv.Entry[logRecord]) (bool, error) {
				database.logs.DeleteEntry(writer.batch, entry)
				logs = database.logs.By("project", kv.U32(uint32(id))).From(kv.U8(levelRank(entry.Record.Level)), kv.U64(entry.Record.Sequence+1))
				deleted++

				return true, nil
			})

			if err != nil || deleted == deleteChunk {
				return err
			}

			_, err = database.purges.DeleteKey(writer.batch, projectKey(id))

			return err
		})

		if err != nil || deleted < deleteChunk {
			return err
		}
	}

	return ctx.Err()
}
