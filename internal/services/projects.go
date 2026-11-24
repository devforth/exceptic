package services

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/projects"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Projects answers what the catalog shows about projects: names, DSNs,
// colors, unresolved issues and the storage used.
type Projects struct {
	store     store.Store
	registry  *projects.Registry
	recording config.Recording
	retention config.Retention
	dropped   DropCounter
}

func newProjects(database store.Store, registry *projects.Registry, settings *config.Config, dropped DropCounter) *Projects {
	return &Projects{store: database, registry: registry, recording: settings.Recording, retention: settings.Retention, dropped: dropped}
}

// List gives the current projects (a config.ProjectList).
func (service *Projects) List() config.Projects {
	return service.registry.List()
}

// Visible gives the current projects within access.
func (service *Projects) Visible(access store.Access) config.Projects {
	return access.Visible(service.List())
}

// RecordedLogLevels are the log levels project records, highest first.
func (service *Projects) RecordedLogLevels(project config.Project) []string {
	return service.recording.RecordedLogLevels(project.LogLevels)
}

// ColorsByName is the shown color of every project within access, by name.
func (service *Projects) ColorsByName(access store.Access) map[string]string {
	colors := map[string]string{}

	for _, project := range service.Visible(access) {
		colors[project.Name] = project.Color
	}

	return colors
}

// FromEnv: the project comes from PROJECTS (its key is fixed; it cannot be deleted).
func (service *Projects) FromEnv(id int) bool {
	return service.registry.FromEnv(id)
}

// Get reads a project with its settings; store.ErrNotFound when missing,
// ErrHidden outside access.
func (service *Projects) Get(ctx context.Context, access store.Access, id int) (store.Project, error) {
	if err := service.Check(access, id); err != nil {
		return store.Project{}, err
	}

	return service.registry.Get(ctx, id)
}

// Check: the project exists (store.ErrNotFound) and is within access (ErrHidden).
func (service *Projects) Check(access store.Access, id int) error {
	switch _, ok := service.List().ByID(id); {
	case !ok:
		return Missing("project", strconv.Itoa(id))
	case !access.Sees(id):
		return Hidden("project", strconv.Itoa(id))
	default:
		return nil
	}
}

// Find is the project of a name or ID among every project: store.ErrNotFound
// when there is none, ErrHidden outside access. The refusals name it as given.
func (service *Projects) Find(access store.Access, nameOrID string) (config.Project, error) {
	project, ok := service.List().Find(strings.TrimSpace(nameOrID))

	switch {
	case !ok:
		return project, Missing("project", strconv.Quote(nameOrID)+" (see list_projects)")
	case !access.Sees(project.ID):
		return project, Hidden("project", strconv.Quote(nameOrID))
	default:
		return project, nil
	}
}

// Save applies the form to project and stores it as viewer: a new one when
// its ID is 0 (it joins a limited viewer's access), the stored one
// otherwise (ErrHidden outside viewer's access). A disk share changed
// without every project is ErrForbidden, a taken name ErrConflict, each a
// Refusal at its field.
// It returns the project as read from the form (with its id once saved) and
// the field errors when nothing was saved.
func (service *Projects) Save(ctx context.Context, viewer store.User, form url.Values, project store.Project) (store.Project, Problems, error) {
	if project.ID != 0 && !viewer.ProjectAccess().Sees(project.ID) {
		return project, nil, Hidden("project", strconv.Itoa(project.ID))
	}

	// A new project's id is random, so its palette color is not known yet:
	// the color the form showed is stored as chosen.
	paletteColor := ""

	if project.ID != 0 {
		paletteColor = service.Color(store.Project{ID: project.ID})
	}

	problems := readProjectForm(form, &project, service.recording, paletteColor)
	access := viewer.ProjectAccess()

	if len(problems) > 0 {
		return project, problems, nil
	}

	var err error

	if project.ID == 0 {
		project.ID, err = service.registry.Create(ctx, project, viewer.ID)
	} else {
		err = service.registry.Update(ctx, project)
	}

	if errors.Is(err, store.ErrProjectConflict) {
		return project, nil, Refusal{Kind: ErrConflict, Problems: Problems{"name": service.nameProblem(access, project.Name)}}
	}

	return project, nil, err
}

// nameProblem says a name is taken without telling of a project outside access.
func (service *Projects) nameProblem(access store.Access, name string) string {
	if other, ok := service.List().Find(name); ok && access.Sees(other.ID) {
		return "Another project already has this name."
	}

	return "This name cannot be used."
}

// Delete removes a project added in the web app with its data; one from
// PROJECTS cannot be deleted (projects.ErrFromEnv).
func (service *Projects) Delete(ctx context.Context, access store.Access, id int) error {
	if err := service.Check(access, id); err != nil {
		return err
	}

	return service.registry.Delete(ctx, id)
}

// Name labels data of a deleted project by its ID.
func (service *Projects) Name(projectID int) string {
	if project, ok := service.registry.List().ByID(projectID); ok {
		return project.Name
	}

	return fmt.Sprintf("project %d", projectID)
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

// UnresolvedCounts gives the number of unresolved issues of each project
// within access, by id.
func (service *Projects) UnresolvedCounts(ctx context.Context, access store.Access) (map[int]int, error) {
	counts, err := service.store.UnresolvedCounts(ctx)

	maps.DeleteFunc(counts, func(id, _ int) bool { return !access.Sees(id) })

	return counts, err
}

// Storage is how much the database holds and how the cleanup keeps it under the limit.
type Storage struct {
	// UsedBytes is the data the database holds, as the cleanup measures it
	// (store.ProjectUsage.Live).
	UsedBytes  int64
	LimitBytes int64 // 0: no size limit
	// Used and Limit are the sizes as the server writes them (config.FormatSize).
	Used, Limit string
	Percent     int // of the limit
	// FilesBytes is what the files take: also the ingest queue, the write
	// journal and deleted records until their next compaction.
	FilesBytes int64
	Files      string
	// The cleanup starts at CleanupStartPercent of the limit and deletes down to CleanupStopPercent.
	CleanupStartPercent, CleanupStopPercent int
	// Projects are the parts of UsedBytes of the current projects the
	// viewer sees, in list order; Other is the rest (the projects not seen,
	// users, sessions, settings, history, data of deleted projects).
	// Without parts while Counting.
	Projects   []ProjectStorage
	OtherBytes int64
	Other      string
	// Counting: data stored before the project sizes is still being counted.
	Counting bool
}

// ProjectStorage is a project's part of the database on disk, as the
// cleanup by size measures it (store.ProjectUsage.OnDisk).
type ProjectStorage struct {
	ID    int
	Name  string
	Bytes int64
	Size  string
	// LastReceived is when its newest issue or log came; nil when none is stored.
	LastReceived *time.Time
	// LimitBytes is its disk share of the size limit; 0 at 100% or without a limit.
	LimitBytes int64
	// Oldest is what it still keeps, by the kinds the cleanup deletes.
	Oldest store.OldestKept
	// Dropped is what ingest dropped of it in the last 24 hours, largest first.
	Dropped []store.Dropped
}

// Storage is the data the whole database holds and its limit, which the
// cleanup acts on, with the parts of the projects within access.
func (service *Projects) Storage(ctx context.Context, access store.Access) (Storage, error) {
	usage, err := service.store.ProjectUsage(ctx)

	if err != nil {
		return Storage{}, err
	}

	used := usage.Live
	storage := Storage{
		UsedBytes: used, Used: config.FormatSize(used), FilesBytes: usage.Files, Files: config.FormatSize(usage.Files),
		CleanupStartPercent: config.CleanupStartPercent, CleanupStopPercent: config.CleanupStopPercent, Counting: usage.Counting,
	}

	if limit := service.retention.MaxDatabaseBytes; limit > 0 {
		storage.LimitBytes, storage.Limit = limit, config.FormatSize(limit)
		storage.Percent = int(min(100, used*100/limit))
	}

	storage.OtherBytes = used

	for _, project := range service.Visible(access) {
		part := ProjectStorage{ID: project.ID, Name: project.Name, Oldest: usage.Oldest[project.ID]}

		part.LimitBytes = store.ShareLimit(storage.LimitBytes, project.DiskShare)

		if service.dropped != nil {
			part.Dropped = service.dropped.Dropped(project.ID)
		}

		if last, ok := usage.LastReceived[project.ID]; ok {
			part.LastReceived = &last
		}

		if !usage.Counting {
			part.Bytes = min(storage.OtherBytes, usage.OnDisk[project.ID])
			part.Size = config.FormatSize(part.Bytes)
			storage.OtherBytes -= part.Bytes
		}

		storage.Projects = append(storage.Projects, part)
	}

	storage.Other = config.FormatSize(storage.OtherBytes)

	return storage, nil
}

// Contact is whether an app reaches a project.
type Contact struct {
	// Connected: it sent a request with its key, of any kind (SDKs send a
	// session at start), or has stored something.
	Connected bool
	// ConnectedAt is its first request; nil before, or when only data stored
	// before first requests were recorded tells it is connected.
	ConnectedAt *time.Time
	// Received: it has stored an error or a log.
	Received bool
}

// Contact tells whether an app reaches the project, for its setup page
// (Check's refusals).
func (service *Projects) Contact(ctx context.Context, access store.Access, projectID int) (Contact, error) {
	if err := service.Check(access, projectID); err != nil {
		return Contact{}, err
	}

	usage, err := service.store.ProjectUsage(ctx)
	_, received := usage.LastReceived[projectID]
	contact := Contact{Received: received, Connected: received}

	if at, ok := usage.FirstContact[projectID]; ok {
		contact.Connected, contact.ConnectedAt = true, &at
	}

	return contact, err
}

// Cleanups are the runs of the cleanup by size the store keeps, newest
// first, as access shows them (visibleRun).
func (service *Projects) Cleanups(ctx context.Context, access store.Access) ([]store.CleanupRun, error) {
	runs, err := service.store.CleanupHistory(ctx, store.CleanupRunsKept)
	shown := []store.CleanupRun{}

	for _, run := range runs {
		if run, ok := visibleRun(access, run); ok {
			shown = append(shown, run)
		}
	}

	return shown, err
}

// visibleRun is a run as access shows it: none of a project outside it; a
// run over the database with only the counts of the projects within it,
// without the steps that deleted only in others (its sizes and the freed
// bytes of the steps shown stay: they are the database's).
func visibleRun(access store.Access, run store.CleanupRun) (store.CleanupRun, bool) {
	if access.All() {
		return run, true
	}

	if run.Project != 0 && !access.Sees(run.Project) {
		return run, false
	}

	steps := make([]store.CleanupStep, 0, len(run.Steps))

	for _, step := range run.Steps {
		deleted := len(step.Projects) > 0
		step.Projects = maps.Clone(step.Projects)
		maps.DeleteFunc(step.Projects, func(id int, _ *store.CleanupCounts) bool { return !access.Sees(id) })

		if deleted && len(step.Projects) == 0 {
			continue // it deleted only in hidden projects
		}

		steps = append(steps, step)
	}

	run.Steps = steps

	return run, true
}
