// Package presenters turns stored data into the view types of contracts,
// without I/O.
package presenters

// ProjectName names a project by id, also one that no longer exists
// (services.Projects.Name).
type ProjectName func(projectID int) string
