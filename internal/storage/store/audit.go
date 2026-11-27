package store

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"
)

// The records an audit entry is about.
const (
	AuditProject    = "project"
	AuditConnection = "connection"
	AuditUser       = "user"
	AuditSession    = "session"
	AuditAgent      = "agent"
	AuditIssue      = "issue"
	AuditLog        = "log"
)

// AuditResources are the records of the audit log, in the order its filter offers them.
var AuditResources = []string{AuditIssue, AuditLog, AuditProject, AuditConnection, AuditUser, AuditSession, AuditAgent}

// The actions of the audit log: create, edit and delete of a record, and
// the actions that change one field the way their name says.
const (
	AuditCreate        = "create"
	AuditEdit          = "edit"
	AuditDelete        = "delete"
	AuditPause         = "pause"
	AuditResume        = "resume"
	AuditResetKey      = "reset_key"
	AuditResetPassword = "reset_password"
	AuditRetry         = "retry"
	AuditConfirm       = "confirm"
	AuditUnlinkGoogle  = "unlink_google"
)

// AuditActions are the actions of the audit log, in the order its filter offers them.
var AuditActions = []string{
	AuditCreate, AuditEdit, AuditDelete, AuditPause, AuditResume, AuditResetKey, AuditResetPassword,
	AuditRetry, AuditConfirm, AuditUnlinkGoogle,
}

// AuditReplaced stands in a diff for a secret that changed (a password, a
// chat token): secrets never enter the audit log. A DSN is public: a reset
// keeps the old and the new one.
const AuditReplaced = "[replaced]"

// AuditActor is who made a change, as they were then: users get renamed and deleted.
// Without UserID it is the REST API's token.
type AuditActor struct {
	UserID string `json:",omitempty"`
	Name   string
	Email  string `json:",omitempty"`
	Role   string `json:",omitempty"`
}

// AuditEntry is one change an admin or a user made, kept for good: no
// code path deletes it.
type AuditEntry struct {
	ID        int64
	CreatedAt time.Time
	Actor     AuditActor
	// Source is how the change came: SourceUI, SourceMCP or SourceAPI.
	Source string
	// Agent is the coding agent that acted for the actor through MCP.
	Agent string `json:",omitempty"`
	// Address is the client's IP address.
	Address    string `json:",omitempty"`
	Resource   string
	Action     string
	RecordID   string
	RecordName string `json:",omitempty"`
	// ProjectIDs are the projects the record belonged to; none for a record
	// of every project or of none, which only admins see.
	ProjectIDs []int `json:",omitempty"`
	// Before and After are the fields that changed: only After for a
	// create, only Before for a delete.
	Before map[string]json.RawMessage `json:",omitempty"`
	After  map[string]json.RawMessage `json:",omitempty"`
	// Items are the records of a request that changed several (a bulk
	// action): one entry per request. RecordID, RecordName, Before and
	// After are then empty, and ProjectIDs are the items' projects.
	Items []AuditItem `json:",omitempty"`
}

// AuditItem is one of the records a request changed.
type AuditItem struct {
	RecordID   string
	RecordName string                     `json:",omitempty"`
	ProjectID  int                        `json:",omitempty"`
	Before     map[string]json.RawMessage `json:",omitempty"`
	After      map[string]json.RawMessage `json:",omitempty"`
}

// shownTo: the item's project is within access (one without a project only with every project).
func (item AuditItem) shownTo(access Access) bool {
	if item.ProjectID == 0 {
		return access.All()
	}

	return access.SeesEach([]int{item.ProjectID})
}

// VisibleTo: an entry of one record whose projects are all within access,
// or of several of which access sees one.
func (entry AuditEntry) VisibleTo(access Access) bool {
	if len(entry.Items) == 0 {
		return access.SeesEach(entry.ProjectIDs)
	}

	return slices.ContainsFunc(entry.Items, func(item AuditItem) bool { return item.shownTo(access) })
}

// ShownTo is entry with only the items access sees, and their projects.
func (entry AuditEntry) ShownTo(access Access) AuditEntry {
	if len(entry.Items) == 0 || access.All() {
		return entry
	}

	items := []AuditItem{}
	projectIDs := []int{}

	for _, item := range entry.Items {
		if item.shownTo(access) {
			items = append(items, item)
			projectIDs = append(projectIDs, item.ProjectID)
		}
	}

	entry.Items = items
	entry.ProjectIDs = slices.Compact(slices.Sorted(slices.Values(projectIDs)))

	return entry
}

// AuditFilter selects audit entries, newest first.
type AuditFilter struct {
	// Access: only entries it sees (AuditEntry.VisibleTo).
	Access Access
	// UserIDs: only changes of these users.
	UserIDs   []string
	Resources []string
	Actions   []string
	// ProjectIDs: only entries about one of these projects.
	ProjectIDs []int
	// Query: text in the actor, agent, address, record name or ID.
	Query        string
	Since, Until time.Time
	Limit        int
	Offset       int
	// Actors, when set, are the user IDs the viewer sees (User.SeesUser):
	// others are shown only by their role (ShowsActor), never searched or filtered by.
	Actors map[string]bool
}

// ShowsActor says whether the viewer may see who made a change (name,
// email, address, agent); the API token only by those who see everyone.
func (filter AuditFilter) ShowsActor(actor AuditActor) bool {
	return filter.Actors == nil || actor.UserID != "" && filter.Actors[actor.UserID]
}

// Matches says whether entry is selected by the filter, its page aside.
func (filter AuditFilter) Matches(entry AuditEntry) bool {
	switch {
	case !entry.VisibleTo(filter.Access),
		len(filter.UserIDs) > 0 && (!slices.Contains(filter.UserIDs, entry.Actor.UserID) || !filter.ShowsActor(entry.Actor)),
		len(filter.Resources) > 0 && !slices.Contains(filter.Resources, entry.Resource),
		len(filter.Actions) > 0 && !slices.Contains(filter.Actions, entry.Action),
		len(filter.ProjectIDs) > 0 && !slices.ContainsFunc(entry.ProjectIDs, func(id int) bool { return slices.Contains(filter.ProjectIDs, id) }),
		!filter.Since.IsZero() && entry.CreatedAt.Before(filter.Since),
		!filter.Until.IsZero() && !entry.CreatedAt.Before(filter.Until):
		return false
	}

	if filter.Query == "" {
		return true
	}

	texts := []string{entry.RecordName, entry.RecordID}

	for _, item := range entry.Items {
		if item.shownTo(filter.Access) {
			texts = append(texts, item.RecordName, item.RecordID)
		}
	}

	if filter.ShowsActor(entry.Actor) {
		texts = append(texts, entry.Actor.Name, entry.Actor.Email, entry.Agent, entry.Address)
	}

	return slices.ContainsFunc(texts, func(text string) bool { return textutil.ContainsFold(text, filter.Query) })
}

// AuditChoices are the values some audit entry has: the choices of the
// audit log's filters, in no order.
type AuditChoices struct {
	Resources  []string
	Actions    []string
	UserIDs    []string
	ProjectIDs []int // without 0, the project of entries of none
}
