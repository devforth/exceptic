package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The audit log: every change an admin or a user makes through the web
// app, MCP or the REST API is recorded by its handler after it succeeded,
// with the fields that changed. Own-account changes, reads and refusals
// are not; secrets never are (store.AuditReplaced says one changed).

func (handler *handlers) auditEndpoints() []Endpoint {
	return []Endpoint{
		Define(Endpoint{
			Name: "list_audit",
			Description: "The audit log, newest first: each change admins and users made (agents through MCP for them) with who, how, from which address and the fields before and after. " +
				"Admins see every entry, users those of their projects.",
			ReadOnly: true, Transports: ActsAsUser,
			Choices: auditChoices,
		}, handler.listAudit),
		Define(Endpoint{
			Name: "get_audit_entry",
			Description: "One entry of the audit log with every record it changed (a request that changed several, such as a bulk resolve, " +
				"is one entry: list_audit gives their count).",
			ReadOnly: true, Transports: ActsAsUser,
		}, handler.getAuditEntry),
		Define(Endpoint{
			Name:        "count_audit",
			Description: "How many entries of the audit log match the filters of list_audit.",
			ReadOnly:    true, Transports: TransportUI,
			Choices: auditChoices,
		}, handler.countAudit),
		Define(Endpoint{
			Name:        "list_audit_choices",
			Description: "The records, actions, users and projects some entry of the audit log the caller sees has: the choices of its filters.",
			ReadOnly:    true, Transports: TransportUI,
		}, handler.listAuditChoices),
	}
}

var auditChoices = map[string][]string{"resource": store.AuditResources, "action": store.AuditActions}

// auditChange is a change to record: Before and After are views of the
// record (nil before a create, after a delete), of which only the fields
// that differ are kept.
type auditChange struct {
	Resource   string
	Action     string
	RecordID   string
	RecordName string
	ProjectIDs []int
	Before     any
	After      any
	// Replaced are secrets that changed: store.AuditReplaced on both sides.
	Replaced []string
	// Always: recorded without a changed field (a retry).
	Always bool
}

// audit records the changes of one request as the call's actor, in one
// entry (with an item per record when there are several); a change of no
// field is left out.
func (handler *handlers) audit(ctx context.Context, call Call, changes ...auditChange) {
	var items []store.AuditItem

	var kept []auditChange

	actor := store.AuditActor{Name: call.Actor.Name}

	if viewer := call.Viewer; viewer.ID != "" {
		actor = store.AuditActor{UserID: viewer.ID, Name: viewer.DisplayName(), Email: viewer.Email, Role: viewer.Role}
	}

	for _, change := range changes {
		// The views are the handlers' own types: they always encode.
		before, after, _ := services.AuditDiff(change.Before, change.After)

		for _, name := range change.Replaced {
			replaced := json.RawMessage(strconv.Quote(store.AuditReplaced))
			before, after = withField(before, name, replaced), withField(after, name, replaced)
		}

		if len(before) == 0 && len(after) == 0 && !change.Always {
			continue
		}

		item := store.AuditItem{RecordID: change.RecordID, RecordName: change.RecordName, Before: before, After: after}

		if len(change.ProjectIDs) == 1 {
			item.ProjectID = change.ProjectIDs[0]
		}

		items = append(items, item)
		kept = append(kept, change)
	}

	if len(kept) == 0 {
		return
	}

	entry := store.AuditEntry{
		Actor: actor, Source: call.Actor.Source, Agent: call.Agent, Address: call.IP,
		Resource: kept[0].Resource, Action: kept[0].Action,
	}

	for _, change := range kept {
		entry.ProjectIDs = append(entry.ProjectIDs, change.ProjectIDs...)

		if change.Action != entry.Action {
			entry.Action = store.AuditEdit
		}
	}

	entry.ProjectIDs = slices.Compact(slices.Sorted(slices.Values(entry.ProjectIDs)))

	if len(items) == 1 {
		entry.RecordID, entry.RecordName, entry.Before, entry.After = items[0].RecordID, items[0].RecordName, items[0].Before, items[0].After
	} else {
		entry.Items = items
	}

	handler.services.Audit.Record(ctx, entry)
}

func withField(fields map[string]json.RawMessage, name string, value json.RawMessage) map[string]json.RawMessage {
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}

	fields[name] = value

	return fields
}

// The views of the records as the audit log keeps them: what a user
// changes and reads, never a secret.

type projectAudit struct {
	Name           string   `json:"name"`
	Color          string   `json:"color"`
	LogLevels      []string `json:"log_levels"`
	DiskShare      int      `json:"disk_share"`
	AllowedOrigins []string `json:"allowed_origins"`
	Paused         bool     `json:"paused"`
}

func (handler *handlers) projectAudit(project store.Project) projectAudit {
	levels := handler.services.Projects.RecordedLogLevels(config.Project{LogLevels: project.LogLevels})

	return projectAudit{
		Name: project.Name, Color: handler.services.Projects.Color(project), LogLevels: levels, DiskShare: project.Share(),
		AllowedOrigins: append([]string{}, project.AllowedOrigins...), Paused: project.Paused,
	}
}

func projectChange(action string, project store.Project, before, after any) auditChange {
	return auditChange{
		Resource: store.AuditProject, Action: action, RecordID: strconv.Itoa(project.ID), RecordName: project.Name,
		ProjectIDs: []int{project.ID}, Before: before, After: after,
	}
}

type userAudit struct {
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	AllProjects bool     `json:"all_projects"`
	Projects    []string `json:"projects"`
}

func (handler *handlers) userAudit(user store.User) userAudit {
	access := user.ProjectAccess()
	view := userAudit{Email: user.Email, Name: user.Name, Role: user.Role, AllProjects: access.All(), Projects: []string{}}

	for _, id := range access.ProjectIDs {
		view.Projects = append(view.Projects, handler.auditProjectName(id))
	}

	return view
}

// userProjects tags an entry about users: the projects they had; none
// when one had every project, which only admins see.
func userProjects(users ...store.User) []int {
	var ids []int

	for _, user := range users {
		access := user.ProjectAccess()

		if access.All() {
			return nil
		}

		ids = append(ids, access.ProjectIDs...)
	}

	return ids
}

func userChange(action string, user store.User, before, after any, also ...store.User) auditChange {
	return auditChange{
		Resource: store.AuditUser, Action: action, RecordID: user.ID, RecordName: user.Email,
		ProjectIDs: userProjects(append([]store.User{user}, also...)...), Before: before, After: after,
	}
}

type connectionAudit struct {
	Name             string   `json:"name"`
	Type             string   `json:"type"`
	Destination      string   `json:"destination"`
	AllProjects      bool     `json:"all_projects"`
	Projects         []string `json:"projects"`
	Levels           []string `json:"levels"`
	Active           bool     `json:"active"`
	IncludeIssueLink bool     `json:"include_issue_link"`
}

func (handler *handlers) connectionAudit(connection store.Connection) connectionAudit {
	view := connectionAudit{
		Name: connection.Name, Type: connection.Type, Destination: handler.services.Connections.Destination(connection),
		AllProjects: connection.AllProjects, Projects: []string{}, Levels: connection.Levels, Active: connection.Active,
		IncludeIssueLink: !connection.HideIssueLink,
	}

	for _, id := range connection.ProjectIDs {
		view.Projects = append(view.Projects, handler.auditProjectName(id))
	}

	return view
}

func connectionChange(action string, connection store.Connection, before, after any) auditChange {
	change := auditChange{
		Resource: store.AuditConnection, Action: action, RecordID: connection.ID, RecordName: connection.Name,
		Before: before, After: after,
	}

	if !connection.AllProjects {
		change.ProjectIDs = connection.ProjectIDs
	}

	return change
}

type issueAudit struct {
	Status            string     `json:"status"`
	IgnoredUntil      *time.Time `json:"ignored_until,omitempty"`
	IgnoredUntilCount int        `json:"ignored_until_events,omitempty"`
}

type deletedIssueAudit struct {
	Title  string `json:"title"`
	Level  string `json:"level"`
	Status string `json:"status"`
	Events int    `json:"events"`
}

// issueChanges are the changes of issues read before and after; one
// missing after was deleted when deleting, else it is left out (deleted by
// someone else meanwhile).
func issueChanges(before, after []store.Issue, deleting bool) []auditChange {
	var changes []auditChange

	for _, old := range before {
		change := auditChange{
			Resource: store.AuditIssue, Action: store.AuditEdit, RecordID: old.ID, RecordName: textutil.Shorten(old.Title, auditNameLength),
			ProjectIDs: []int{old.ProjectID},
		}
		index := slices.IndexFunc(after, func(issue store.Issue) bool { return issue.ID == old.ID })

		switch {
		case index < 0 && !deleting:
			continue
		case index < 0:
			change.Action, change.Before = store.AuditDelete, deletedIssueAudit{Title: old.Title, Level: old.Level, Status: old.Status, Events: old.EventCount}
		default:
			change.Before, change.After = issueAuditOf(old), issueAuditOf(after[index])
		}

		changes = append(changes, change)
	}

	return changes
}

func issueAuditOf(issue store.Issue) issueAudit {
	return issueAudit{Status: issue.Status, IgnoredUntil: issue.IgnoredUntil, IgnoredUntilCount: issue.IgnoredUntilCount}
}

type logAudit struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Body    string    `json:"body"`
	TraceID string    `json:"trace_id,omitempty"`
}

// auditNameLength cuts a record's name (an issue's title, a log line); a
// log's body is kept to auditBodyLength.
const (
	auditNameLength = 120
	auditBodyLength = 1000
)

func logChanges(logs []store.LogRecord) []auditChange {
	changes := make([]auditChange, 0, len(logs))

	for _, log := range logs {
		changes = append(changes, auditChange{
			Resource: store.AuditLog, Action: store.AuditDelete, RecordID: strconv.FormatInt(log.ID, 10),
			RecordName: textutil.Shorten(log.Body, auditNameLength), ProjectIDs: []int{log.ProjectID},
			Before: logAudit{Time: log.Timestamp, Level: log.Level, Body: textutil.Shorten(log.Body, auditBodyLength), TraceID: log.TraceID},
		})
	}

	return changes
}

// auditProjectName is a project's name, or its ID once it is deleted.
func (handler *handlers) auditProjectName(id int) string {
	return cmp.Or(handler.projectName(id), "#"+strconv.Itoa(id))
}

// The list.

type auditFiltersInput struct {
	UserID   []string `json:"user_id,omitempty" jsonschema:"Only changes of these users (ids from list_users)."`
	Resource []string `json:"resource,omitempty" jsonschema:"Only changes of these kinds of record."`
	Action   []string `json:"action,omitempty" jsonschema:"Only these actions."`
	Project  []string `json:"project,omitempty" jsonschema:"Project names or IDs: only changes about one of them."`
	Query    string   `json:"query,omitempty" jsonschema:"Text in the user's name or email, the agent, the address, or the record's name or ID."`
	Since    string   `json:"since,omitempty" jsonschema:"RFC 3339 time: only changes at or after it."`
	Until    string   `json:"until,omitempty" jsonschema:"RFC 3339 time: only changes before it."`
}

type listAuditInput struct {
	auditFiltersInput
	Limit  int    `json:"limit,omitempty" jsonschema:"Entries per page."`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page."`
}

type auditOutput struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	// The user as they were; no UserID for the REST API's token.
	UserID    string `json:"user_id,omitempty"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email,omitempty"`
	UserRole  string `json:"user_role,omitempty"`
	// Source is how the change came: ui, mcp or api.
	Source string `json:"source"`
	// Agent is the coding agent that acted for the user through MCP.
	Agent      string   `json:"agent,omitempty"`
	IP         string   `json:"ip,omitempty"`
	Resource   string   `json:"resource"`
	Action     string   `json:"action"`
	RecordID   string   `json:"record_id"`
	RecordName string   `json:"record_name,omitempty"`
	ProjectIDs []int    `json:"project_ids"`
	Projects   []string `json:"projects"`
	// Before and After are the fields that changed.
	Before map[string]json.RawMessage `json:"before"`
	After  map[string]json.RawMessage `json:"after"`
	// Exists: the record is still there and the viewer may open it (the web app links to it).
	Exists bool `json:"exists,omitempty"`
	// ItemCount: the request changed several records, listed by get_audit_entry.
	ItemCount int `json:"item_count,omitempty"`
}

type auditItemOutput struct {
	RecordID   string                     `json:"record_id"`
	RecordName string                     `json:"record_name,omitempty"`
	Project    string                     `json:"project,omitempty"`
	Before     map[string]json.RawMessage `json:"before"`
	After      map[string]json.RawMessage `json:"after"`
	Exists     bool                       `json:"exists,omitempty"`
}

type auditEntryOutput struct {
	auditOutput
	Items []auditItemOutput `json:"items"`
}

type getAuditEntryInput struct {
	ID int64 `json:"id" jsonschema:"The entry's id from list_audit."`
}

func (handler *handlers) getAuditEntry(ctx context.Context, call Call, input getAuditEntryInput) (auditEntryOutput, error) {
	entry, filter, err := handler.services.Audit.Get(ctx, call.Viewer, input.ID)

	if err != nil {
		return auditEntryOutput{}, err
	}

	output := auditEntryOutput{auditOutput: handler.auditView(ctx, call, filter, entry), Items: []auditItemOutput{}}

	for _, item := range entry.Items {
		shown := auditItemOutput{
			RecordID: item.RecordID, RecordName: call.Text(item.RecordName),
			Before: auditFields(call, item.Before), After: auditFields(call, item.After),
		}

		if item.ProjectID != 0 {
			shown.Project = handler.auditProjectName(item.ProjectID)
		}

		if call.Transport == TransportUI && entry.Action != store.AuditDelete {
			shown.Exists = handler.recordExists(ctx, call, store.AuditEntry{Resource: entry.Resource, RecordID: item.RecordID})
		}

		output.Items = append(output.Items, shown)
	}

	return output, nil
}

func (handler *handlers) auditFilter(call Call, input auditFiltersInput) (store.AuditFilter, error) {
	filter := store.AuditFilter{UserIDs: input.UserID, Resources: input.Resource, Actions: input.Action, Query: strings.TrimSpace(input.Query)}

	var err error

	if filter.ProjectIDs, err = handler.projectIDs(call, input.Project); err != nil {
		return filter, err
	}

	if filter.Since, err = parseTime("since", input.Since); err != nil {
		return filter, err
	}

	filter.Until, err = parseTime("until", input.Until)

	return filter, err
}

func (handler *handlers) listAudit(ctx context.Context, call Call, input listAuditInput) (Page[auditOutput], error) {
	output := Page[auditOutput]{Items: []auditOutput{}}
	filter, err := handler.auditFilter(call, input.auditFiltersInput)

	if err != nil {
		return output, err
	}

	limit := pageLimit(input.Limit)

	if filter.Offset, err = parseOffset(input.Cursor); err != nil {
		return output, err
	}

	filter.Limit = limit + 1
	entries, filter, err := handler.services.Audit.List(ctx, call.Viewer, filter)

	if err != nil {
		return output, err
	}

	entries, more := store.Page(entries, limit)

	for _, entry := range entries {
		output.Items = append(output.Items, handler.auditView(ctx, call, filter, entry))
	}

	if more {
		output.NextCursor = strconv.Itoa(filter.Offset + limit)
	}

	return output, nil
}

func (handler *handlers) auditView(ctx context.Context, call Call, filter store.AuditFilter, entry store.AuditEntry) auditOutput {
	view := auditOutput{
		ID: entry.ID, CreatedAt: entry.CreatedAt, UserID: entry.Actor.UserID, UserName: entry.Actor.Name, UserEmail: entry.Actor.Email,
		UserRole: entry.Actor.Role, Source: entry.Source, Agent: call.Text(entry.Agent), IP: entry.Address,
		Resource: entry.Resource, Action: entry.Action, RecordID: entry.RecordID, RecordName: call.Text(entry.RecordName),
		ProjectIDs: append([]int{}, entry.ProjectIDs...), Projects: []string{},
		Before: auditFields(call, entry.Before), After: auditFields(call, entry.After),
	}

	if call.ForAgent() {
		// People's addresses do not go to a model provider.
		view.IP = ""
	}

	if !filter.ShowsActor(entry.Actor) {
		view.UserID, view.UserEmail, view.Agent, view.IP = "", "", "", ""
		view.UserName = hiddenActorName(entry.Actor)
	}

	for _, id := range entry.ProjectIDs {
		view.Projects = append(view.Projects, handler.auditProjectName(id))
	}

	if len(entry.Items) > 0 {
		view.ItemCount = len(entry.Items)
		view.RecordName = itemsName(len(entry.Items), entry.Resource)
		// What every record changed alike (the status of a bulk resolve), for the list's message.
		view.Before = auditFields(call, commonFields(entry.Items, func(item store.AuditItem) map[string]json.RawMessage { return item.Before }))
		view.After = auditFields(call, commonFields(entry.Items, func(item store.AuditItem) map[string]json.RawMessage { return item.After }))
	} else if call.Transport == TransportUI && entry.Action != store.AuditDelete {
		view.Exists = handler.recordExists(ctx, call, entry)
	}

	return view
}

// itemsName names the records of an entry by their count: "3 issues", "1 issue".
func itemsName(count int, resource string) string {
	if count == 1 {
		return "1 " + resource
	}

	return fmt.Sprintf("%d %ss", count, resource)
}

// commonFields are the fields every item has with the same value on one side.
func commonFields(items []store.AuditItem, side func(store.AuditItem) map[string]json.RawMessage) map[string]json.RawMessage {
	common := map[string]json.RawMessage{}

	if len(items) == 0 {
		return common
	}

	for name, value := range side(items[0]) {
		if !slices.ContainsFunc(items[1:], func(item store.AuditItem) bool { return !bytes.Equal(side(item)[name], value) }) {
			common[name] = value
		}
	}

	return common
}

// hiddenActorName names an actor the viewer does not see by role only.
func hiddenActorName(actor store.AuditActor) string {
	switch {
	case actor.UserID == "":
		return actor.Name
	case actor.Role == store.RoleAdmin:
		return "Admin"
	}

	return "User"
}

// auditFields are a diff's fields; for an agent issue titles and log
// lines, written by the monitored apps, are redacted like elsewhere.
func auditFields(call Call, fields map[string]json.RawMessage) map[string]json.RawMessage {
	shown := map[string]json.RawMessage{}

	for name, value := range fields {
		var text string

		if call.ForAgent() && json.Unmarshal(value, &text) == nil {
			value, _ = json.Marshal(call.Text(text))
		}

		shown[name] = value
	}

	return shown
}

// recordExists: the entry's record is still there and the viewer sees it,
// so the web app can link to its page.
func (handler *handlers) recordExists(ctx context.Context, call Call, entry store.AuditEntry) bool {
	access := call.Access()

	switch entry.Resource {
	case store.AuditProject:
		id, err := strconv.Atoi(entry.RecordID)

		return err == nil && handler.services.Projects.Check(access, id) == nil
	case store.AuditUser:
		_, err := handler.services.Users.Visible(ctx, call.Viewer, entry.RecordID)

		return err == nil
	case store.AuditConnection:
		_, err := handler.services.Connections.Get(ctx, access, entry.RecordID)

		return err == nil && call.Viewer.CanAdminister()
	case store.AuditIssue:
		_, err := handler.services.Issues.Get(ctx, access, entry.RecordID)

		return err == nil
	}

	return false
}

func (handler *handlers) countAudit(ctx context.Context, call Call, input auditFiltersInput) (countOutput, error) {
	filter, err := handler.auditFilter(call, input)

	if err != nil {
		return countOutput{}, err
	}

	count, more, err := handler.services.Audit.Count(ctx, call.Viewer, filter)

	return countOutput{Count: count, More: more}, err
}

type auditChoicesOutput struct {
	// Resources and Actions are in the order of the filter's choices.
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
	UserIDs   []string `json:"user_ids"`
	// Projects are names, in the order of the projects' list.
	Projects []string `json:"projects"`
}

func (handler *handlers) listAuditChoices(ctx context.Context, call Call, _ noInput) (auditChoicesOutput, error) {
	present, err := handler.services.Audit.Choices(ctx, call.Viewer)
	inOrder := func(known, values []string) []string {
		return slices.DeleteFunc(slices.Clone(known), func(value string) bool { return !slices.Contains(values, value) })
	}

	slices.Sort(present.UserIDs)

	return auditChoicesOutput{
		Resources: inOrder(store.AuditResources, present.Resources), Actions: inOrder(store.AuditActions, present.Actions),
		UserIDs: append([]string{}, present.UserIDs...), Projects: handler.projectNames(present.ProjectIDs).Projects,
	}, err
}
