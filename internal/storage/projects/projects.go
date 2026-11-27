// Package projects keeps the list of projects: added and edited in the UI,
// cached in memory.
package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

type Registry struct {
	store store.Store
	// reloadMutex keeps two reloads from storing an older list last.
	reloadMutex sync.Mutex
	current     atomic.Pointer[config.Projects]
}

var _ config.ProjectList = (*Registry)(nil)

// Open loads the list of projects.
func Open(ctx context.Context, database store.Store) (*Registry, error) {
	registry := &Registry{store: database}

	if err := registry.Reload(ctx); err != nil {
		return nil, err
	}

	return registry, nil
}

// List returns the current projects.
func (registry *Registry) List() config.Projects {
	if current := registry.current.Load(); current != nil {
		return *current
	}

	return nil
}

// Reload reads the list from the database. Called after every change.
func (registry *Registry) Reload(ctx context.Context) error {
	registry.reloadMutex.Lock()
	defer registry.reloadMutex.Unlock()

	stored, err := registry.store.ListProjects(ctx)

	if err != nil {
		return fmt.Errorf("load projects: %w", err)
	}

	list := make(config.Projects, 0, len(stored))

	for _, project := range stored {
		color := project.Color

		if color == "" {
			color = config.DefaultProjectColor(project.ID)
		}

		list = append(list, config.Project{
			ID: project.ID, Name: project.Name, PublicKey: project.PublicKey, Color: color,
			LogLevels: project.LogLevels, DiskShare: project.Share(), AllowedOrigins: project.AllowedOrigins, Paused: project.Paused,
		})
	}

	registry.current.Store(&list)

	return nil
}

// Get returns a project as stored, for editing.
func (registry *Registry) Get(ctx context.Context, id int) (store.Project, error) {
	return registry.store.GetProject(ctx, id)
}

// Create adds a project with a new DSN key; returns its id. A limited
// owner (a user id, or "") gets it in their access at once.
func (registry *Registry) Create(ctx context.Context, project store.Project, ownerID string) (int, error) {
	project.PublicKey = NewPublicKey()

	id, err := registry.store.CreateProject(ctx, project, time.Now(), ownerID)

	if err != nil {
		return 0, err
	}

	return id, registry.Reload(ctx)
}

// Update saves a project's settings (the store keeps its key).
func (registry *Registry) Update(ctx context.Context, project store.Project) error {
	if err := registry.store.UpdateProject(ctx, project); err != nil {
		return err
	}

	return registry.Reload(ctx)
}

// SetPaused pauses or resumes storing what a project's apps send; ingest
// reads it from the list at once.
func (registry *Registry) SetPaused(ctx context.Context, id int, paused bool) error {
	if err := registry.store.SetProjectPaused(ctx, id, paused); err != nil {
		return err
	}

	return registry.Reload(ctx)
}

// RotateKey gives a project a new DSN key: the old one stops working at once.
func (registry *Registry) RotateKey(ctx context.Context, id int) error {
	if err := registry.store.RotateProjectKey(ctx, id, NewPublicKey(), time.Now()); err != nil {
		return err
	}

	return registry.Reload(ctx)
}

// Delete removes a project, then all its data, even when ctx ends first; a
// purge stopped by a restart goes on in the background job.
func (registry *Registry) Delete(ctx context.Context, id int) error {
	ctx = context.WithoutCancel(ctx)

	if err := registry.store.DeleteProject(ctx, id); err != nil {
		return err
	}

	if err := registry.Reload(ctx); err != nil {
		return err
	}

	return registry.store.PurgeProjects(ctx)
}

// NewPublicKey returns a random DSN key (32 hex characters).
func NewPublicKey() string {
	var key [16]byte

	_, _ = rand.Read(key[:])

	return hex.EncodeToString(key[:])
}
