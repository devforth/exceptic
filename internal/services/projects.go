package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/storage/projects"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Projects answers what pages show about projects: names, DSNs, colors,
// unresolved issues and the storage used.
type Projects struct {
	store          store.Store
	registry       *projects.Registry
	recording      config.Recording
	retention      config.Retention
	publicURL      string
	trustedProxies []netip.Prefix
}

func newProjects(database store.Store, registry *projects.Registry, settings *config.Config) *Projects {
	return &Projects{
		store: database, registry: registry, recording: settings.Recording, retention: settings.Retention,
		publicURL: settings.Notifications.PublicURL, trustedProxies: settings.TrustedProxies,
	}
}

// List gives the current projects (a config.ProjectList).
func (service *Projects) List() config.Projects {
	return service.registry.List()
}

// RecordedLevels are the levels project records, highest first.
func (service *Projects) RecordedLevels(project config.Project) (events, logs []string) {
	return service.recording.RecordedLevels(project.EventLevels, project.LogLevels)
}

// ColorsByName is the shown color of every project, by name.
func (service *Projects) ColorsByName() map[string]string {
	colors := map[string]string{}

	for _, project := range service.List() {
		colors[project.Name] = project.Color
	}

	return colors
}

// FromEnv: the project comes from PROJECTS (its key is fixed; it cannot be deleted).
func (service *Projects) FromEnv(id int) bool {
	return service.registry.FromEnv(id)
}

// Get reads a project with its settings; store.ErrNotFound when missing.
func (service *Projects) Get(ctx context.Context, id int) (store.Project, error) {
	return service.registry.Get(ctx, id)
}

// Save applies the form to project and stores it: a new one when its ID is
// 0, the stored one otherwise.
// It returns the project as read from the form (with its id once saved) and
// the field errors when nothing was saved.
func (service *Projects) Save(ctx context.Context, form url.Values, project store.Project) (store.Project, Problems, error) {
	// A new project's id is random, so its palette color is not known yet:
	// the color the form showed is stored as chosen.
	paletteColor := ""

	if project.ID != 0 {
		paletteColor = service.Color(store.Project{ID: project.ID})
	}

	problems := readProjectForm(form, &project, service.recording, paletteColor)

	if len(problems) > 0 {
		return project, problems, nil
	}

	var err error

	if project.ID == 0 {
		project.ID, err = service.registry.Create(ctx, project)
	} else {
		err = service.registry.Update(ctx, project)
	}

	if errors.Is(err, store.ErrProjectConflict) {
		return project, Problems{"name": "Another project already has this name."}, nil
	}

	return project, nil, err
}

// Delete removes a project added in the UI with its data; one from
// PROJECTS cannot be deleted (projects.ErrFromEnv).
func (service *Projects) Delete(ctx context.Context, id int) error {
	return service.registry.Delete(ctx, id)
}

// Names lists the current projects for the project filter.
func (service *Projects) Names() []string {
	list := service.registry.List()
	names := make([]string, 0, len(list))

	for _, project := range list {
		names = append(names, project.Name)
	}

	return names
}

// IDs gives the ids of the ?project= names; lists.CleanListQuery has kept
// only known projects, by name, each once. None means all projects.
func (service *Projects) IDs(names []string) []int {
	var ids []int

	for _, name := range names {
		if project, ok := service.registry.List().Find(name); ok {
			ids = append(ids, project.ID)
		}
	}

	return ids
}

// Name labels data of a deleted project by its ID.
func (service *Projects) Name(projectID int) string {
	if project, ok := service.registry.List().ByID(projectID); ok {
		return project.Name
	}

	return fmt.Sprintf("project %d", projectID)
}

// URL is the address of path on this server, such as the MCP endpoint.
func (service *Projects) URL(r *http.Request, path string) string {
	return service.address(r).JoinPath(path).String()
}

// address is where clients reach the server (see server.PublicURL).
func (service *Projects) address(r *http.Request) *url.URL {
	return server.PublicURL(r, service.publicURL, service.trustedProxies)
}

// Color is the color a project shows: its own, or the palette color of
// its id (for a new project, the next one after the existing ids).
func (service *Projects) Color(project store.Project) string {
	if project.Color != "" {
		return project.Color
	}

	id := project.ID

	if id == 0 {
		for _, existing := range service.registry.List() {
			id = max(id, existing.ID)
		}

		id++
	}

	return config.DefaultProjectColor(id)
}

// UnresolvedCounts gives the number of unresolved issues of each project, by id.
func (service *Projects) UnresolvedCounts(ctx context.Context) (map[int]int, error) {
	return service.store.UnresolvedCounts(ctx)
}

// DatabaseSize is how many bytes the database holds.
func (service *Projects) DatabaseSize(ctx context.Context) (int64, error) {
	return service.store.DatabaseSize(ctx)
}

// Storage is how much the database holds and how the cleanup keeps it under the limit.
type Storage struct {
	UsedBytes  int64
	LimitBytes int64 // 0: no size limit
	// Used and Limit are the sizes as the server writes them (config.FormatSize).
	Used, Limit string
	Percent     int // of the limit
	// CleanupStopPercent: at the limit the cleanup deletes down to this percent of it.
	CleanupStopPercent int
	// Projects are the current projects' parts of UsedBytes, in list order;
	// Other is the rest (the ingest queue, data of deleted projects, users,
	// notifications). Without parts while Counting.
	Projects   []ProjectStorage
	OtherBytes int64
	Other      string
	// Counting: data stored before the project sizes is still being counted.
	Counting bool
}

// ProjectStorage is a project's part of the database on disk: its issues,
// events and logs, in the proportion of their bytes before compression.
type ProjectStorage struct {
	ID    int
	Name  string
	Bytes int64
	Size  string
	// LastReceived is when its newest issue or log came; nil when none is stored.
	LastReceived *time.Time
}

func (service *Projects) Storage(ctx context.Context) (Storage, error) {
	used, err := service.DatabaseSize(ctx)

	if err != nil {
		return Storage{}, err
	}

	usage, err := service.store.ProjectUsage(ctx)

	if err != nil {
		return Storage{}, err
	}

	storage := Storage{UsedBytes: used, Used: config.FormatSize(used), CleanupStopPercent: config.CleanupStopPercent, Counting: usage.Counting}

	if limit := service.retention.MaxDatabaseBytes; limit > 0 {
		storage.LimitBytes, storage.Limit = limit, config.FormatSize(limit)
		storage.Percent = int(min(100, used*100/limit))
	}

	storage.OtherBytes = used

	for _, project := range service.List() {
		part := ProjectStorage{ID: project.ID, Name: project.Name}

		if last, ok := usage.LastReceived[project.ID]; ok {
			part.LastReceived = &last
		}

		if !usage.Counting && usage.Counted > 0 {
			part.Bytes = min(storage.OtherBytes, int64(float64(used)*float64(usage.Bytes[project.ID])/float64(usage.Counted)))
			part.Size = config.FormatSize(part.Bytes)
			storage.OtherBytes -= part.Bytes
		}

		storage.Projects = append(storage.Projects, part)
	}

	storage.Other = config.FormatSize(storage.OtherBytes)

	return storage, nil
}

// Received reports whether a project has stored anything: an error or a log.
func (service *Projects) Received(ctx context.Context, projectID int) (bool, error) {
	usage, err := service.store.ProjectUsage(ctx)
	_, received := usage.LastReceived[projectID]

	return received, err
}

// Cleanups are every run of the cleanup by size the store keeps, newest first.
func (service *Projects) Cleanups(ctx context.Context) ([]store.CleanupRun, error) {
	return service.store.CleanupHistory(ctx, store.CleanupRunsKept)
}
