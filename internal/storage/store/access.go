package store

import (
	"errors"
	"slices"

	"github.com/devforth/exceptic/internal/platform/config"
)

// Access is the projects a user sees. Its zero value is every project,
// added later ones included, which is also how users stored before it read.
type Access struct {
	Limited bool `json:"limited,omitempty"`
	// ProjectIDs are the projects seen when Limited, sorted; empty: none.
	ProjectIDs []int `json:"project_ids,omitempty"`
}

// ErrAccessLocked: the viewer may not make that change of a user's access.
var ErrAccessLocked = errors.New("access cannot be changed")

// ErrTakeOverLocked: the viewer may not change the name or role of a user
// who has projects the viewer does not (User.CoveredBy).
var ErrTakeOverLocked = errors.New("name and role cannot be changed")

// OnlyProjects is access to these projects alone.
func OnlyProjects(ids ...int) Access {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)

	return Access{Limited: true, ProjectIDs: slices.Compact(sorted)}
}

// All: every project, future ones included.
func (access Access) All() bool {
	return !access.Limited
}

// Sees reports whether the project is within access.
func (access Access) Sees(projectID int) bool {
	return access.All() || slices.Contains(access.ProjectIDs, projectID)
}

// Covers: every project other sees, access sees too.
func (access Access) Covers(other Access) bool {
	if access.All() {
		return true
	}

	if other.All() {
		return false
	}

	for _, id := range other.ProjectIDs {
		if !access.Sees(id) {
			return false
		}
	}

	return true
}

// Shares: access and other have a project in common (every project counts
// as each; no project as none).
func (access Access) Shares(other Access) bool {
	switch {
	case access.All():
		return other.All() || len(other.ProjectIDs) > 0
	case other.All():
		return len(access.ProjectIDs) > 0
	}

	return slices.ContainsFunc(access.ProjectIDs, other.Sees)
}

// ChangeableBy: viewer may change this access (a user's). Access to every
// project is changed only by someone who has it.
func (access Access) ChangeableBy(viewer Access) bool {
	return viewer.All() || !access.All()
}

// SeesEach: every project of ids is within access; with none given (data
// that does not say its projects), only access to every project.
func (access Access) SeesEach(ids []int) bool {
	if access.All() {
		return true
	}

	return len(ids) > 0 && !slices.ContainsFunc(ids, func(id int) bool { return !access.Sees(id) })
}

// Restrict narrows the project filter of a list (ids) to access: as is for
// every project, otherwise its projects within access, or all of access's
// when none is requested. It is false when nothing can match; callers then
// answer empty, since an empty filter means every project.
func (access Access) Restrict(ids *[]int) bool {
	if access.All() {
		return true
	}

	if len(*ids) == 0 {
		*ids = slices.Clone(access.ProjectIDs)
	} else {
		*ids = access.within(*ids)
	}

	return len(*ids) > 0
}

// within are the projects of ids within access.
func (access Access) within(ids []int) []int {
	var seen []int

	for _, id := range ids {
		if access.Sees(id) {
			seen = append(seen, id)
		}
	}

	return seen
}

// Visible are the projects of the list within access.
func (access Access) Visible(projects config.Projects) config.Projects {
	if access.All() {
		return projects
	}

	visible := config.Projects{}

	for _, project := range projects {
		if access.Sees(project.ID) {
			visible = append(visible, project)
		}
	}

	return visible
}

// Shown is the part of this access (a user's) that viewer is shown: every
// project when it is all, otherwise its projects viewer sees.
func (access Access) Shown(viewer Access) Access {
	if access.All() || viewer.All() {
		return access
	}

	return OnlyProjects(viewer.within(access.ProjectIDs)...)
}

// Replace is this access (a user's, as stored) after viewer saved chosen,
// what viewer was shown of it changed: viewer's projects as chosen, the
// others kept. ErrAccessLocked when viewer may not: only access to every
// project gives every project, and changes a user who has it.
func (access Access) Replace(viewer, chosen Access) (Access, error) {
	switch {
	case viewer.All():
		return normalized(chosen), nil
	case access.All() && chosen.All():
		return access, nil // shown as every project and sent back unchanged
	case !access.ChangeableBy(viewer), !viewer.Covers(chosen):
		return access, ErrAccessLocked
	}

	ids := slices.Clone(chosen.ProjectIDs)

	for _, id := range access.ProjectIDs {
		if !viewer.Sees(id) {
			ids = append(ids, id)
		}
	}

	return OnlyProjects(ids...), nil
}

func normalized(access Access) Access {
	if access.All() {
		return Access{}
	}

	return OnlyProjects(access.ProjectIDs...)
}
