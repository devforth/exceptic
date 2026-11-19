// Package projects keeps the list of projects: seeded from PROJECTS at
// startup, editable in the UI, cached in memory.
package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

type Registry struct {
	store store.Store
	env   config.Projects
	// reloadMutex keeps two reloads from storing an older list last.
	reloadMutex sync.Mutex
	current     atomic.Pointer[config.Projects]
}

var _ config.ProjectList = (*Registry)(nil)

// Open seeds the projects of PROJECTS and loads the list.
func Open(ctx context.Context, logger *slog.Logger, database store.Store, env config.Projects) (*Registry, error) {
	registry := &Registry{store: database, env: env}

	for _, project := range env {
		if existing, err := database.GetProject(ctx, project.ID); err == nil && (existing.Name != project.Name || existing.PublicKey != project.PublicKey) {
			logger.Warn("project changed by PROJECTS", "id", project.ID, "name", project.Name, "previous_name", existing.Name,
				"key_changed", existing.PublicKey != project.PublicKey)
		}

		if err := database.SeedProject(ctx, store.Project{ID: project.ID, Name: project.Name, PublicKey: project.PublicKey}, time.Now()); err != nil {
			return nil, fmt.Errorf("PROJECTS: %w", err)
		}
	}

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

// FromEnv reports whether a project is defined in PROJECTS: its name and key
// are set there, not in the UI, and it cannot be deleted in the UI.
func (registry *Registry) FromEnv(id int) bool {
	_, ok := registry.env.ByID(id)

	return ok
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
			EventLevels: project.EventLevels, LogLevels: project.LogLevels,
		})
	}

	registry.current.Store(&list)

	return nil
}

// Get returns a project as stored, for editing.
func (registry *Registry) Get(ctx context.Context, id int) (store.Project, error) {
	return registry.store.GetProject(ctx, id)
}

// Create adds a project with a new DSN key; returns its id.
func (registry *Registry) Create(ctx context.Context, project store.Project) (int, error) {
	project.PublicKey = NewPublicKey()

	id, err := registry.store.CreateProject(ctx, project, time.Now())

	if err != nil {
		return 0, err
	}

	return id, registry.Reload(ctx)
}

// Update saves a project. For a project from PROJECTS only the color and levels
// change: its name and key are kept as env has them.
func (registry *Registry) Update(ctx context.Context, project store.Project) error {
	if envProject, ok := registry.env.ByID(project.ID); ok {
		project.Name, project.PublicKey = envProject.Name, envProject.PublicKey
	}

	if err := registry.store.UpdateProject(ctx, project); err != nil {
		return err
	}

	return registry.Reload(ctx)
}

// ErrFromEnv: a project from PROJECTS cannot be deleted in the UI.
var ErrFromEnv = fmt.Errorf("the project is defined in PROJECTS; remove it there first")

// Delete removes a project, then all its data, even when ctx ends first; a
// purge stopped by a restart goes on in the background job.
func (registry *Registry) Delete(ctx context.Context, id int) error {
	if registry.FromEnv(id) {
		return ErrFromEnv
	}

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
