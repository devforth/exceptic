package api

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The endpoints that manage the server: they act as a
// user (ActsAsUser) through the same services. Users and Agents check what
// the user may change; the Admin flag guards projects and notifications.
// A user changes their own password only in the UI; an agent never mints
// MCP tokens.
func (handler *handlers) management() []Endpoint {
	return append([]Endpoint{
		Define(Endpoint{
			Name:        "get_account",
			Description: "Who the agent acts for: email, name, role (admin or user), which decides the tools offered, and the projects they see.",
			ReadOnly:    true, Transports: TransportMCP,
		}, handler.getAccount),
		Define(Endpoint{
			Name:        "update_account_name",
			Description: "Change the display name of the user the agent acts for. The password can only be changed in the UI.",
			Idempotent:  true, Transports: ActsAsUser,
		}, handler.updateAccountName),
		Define(Endpoint{
			Name:        "get_activity",
			Description: "How a user signs in: their sessions, their sign-in log and the Google account linked to them. Yourself without user_id, or a user you manage.",
			ReadOnly:    true, Transports: TransportUI,
		}, handler.getActivity),
		Define(Endpoint{
			Name:        "end_session",
			Description: "End a session of yourself (not the one you use) or of a user you may take over.",
			Transports:  TransportUI, Confirm: true,
		}, handler.endSession),
		Define(Endpoint{
			Name:        "unlink_google",
			Description: "Unlink the Google account of yourself or of a user you may take over and disconnect the sessions opened through it; it links again by email only after they sign in with the password.",
			Transports:  TransportUI, Confirm: true,
		}, handler.unlinkGoogle),
		Define(Endpoint{
			Name:        "set_theme",
			Description: "Save the web app's color theme of the signed-in user, so every browser they log in from shows it.",
			Idempotent:  true, Transports: TransportUI, OpenWithTemporaryPassword: true,
			Choices: map[string][]string{"theme": store.Themes},
		}, handler.setTheme),
		Define(Endpoint{
			Name:        "list_agents",
			Description: "Coding agents and personal tokens connected to MCP for the user (admins: user_id of a user whose projects they all have, from owners), with when each was last used; connected is false for a personal token never used.",
			ReadOnly:    true, Transports: ActsAsUser,
		}, handler.listAgents),
		Define(Endpoint{
			Name:        "disconnect_agent",
			Description: "Revoke a connected agent or personal token (grant_id from list_agents); it stops working at once.",
			Destructive: true, Transports: ActsAsUser,
		}, handler.disconnectAgent),
		Define(Endpoint{
			Name:        "get_mcp_setup",
			Description: "How to connect a coding agent: the MCP endpoint, its commands and .mcp.json, whether the viewer has a personal token, and warnings.",
			ReadOnly:    true, Transports: TransportUI,
		}, handler.getMCPSetup),

		Define(Endpoint{
			Name:        "get_project",
			Description: "Settings of a project: DSN, color (missing when it follows the palette), the color it shows and recorded levels.",
			ReadOnly:    true, Transports: ActsAsUser,
		}, handler.getProject),
		Define(Endpoint{
			Name:        "create_project",
			Description: "Add a project (one per service sending errors); returns its DSN for the app's Sentry SDK.",
			Transports:  ActsAsUser,
			Choices:     projectChoices,
		}, handler.createProject),
		Define(Endpoint{
			Name:        "update_project",
			Description: "Change a project's settings; fields not given keep their values, an empty string returns a setting to the default.",
			Idempotent:  true, Transports: ActsAsUser, Confirm: true,
			Choices: projectChoices,
		}, handler.updateProject),
		Define(Endpoint{
			Name:        "delete_project",
			Description: "Delete a project with all its issues, events and logs; apps using its DSN are rejected afterwards.",
			Destructive: true, Admin: true, Transports: ActsAsUser,
		}, handler.deleteProject),

		Define(Endpoint{
			Name:        "list_users",
			Description: "Users of the panel with their role, last login, projects and whether you may change their projects (can_manage, access_editable), and their name, role and password or delete them (can_take_over: only when you have all their projects). With only some projects yourself, you see only those of each user's projects.",
			ReadOnly:    true, Transports: ActsAsUser,
		}, handler.listUsers),
		Define(Endpoint{
			Name:        "get_user",
			Description: "A user with whether the viewer may change them and their projects.",
			ReadOnly:    true, Transports: TransportUI,
		}, handler.getUser),
		Define(Endpoint{
			Name:        "create_user",
			Description: "Add a user with a role (admin or user) and the projects they see (default: yours). Returns a temporary password, shown only this once, that the user must change at the first login: give it to the user privately.",
			Transports:  ActsAsUser, Confirm: true,
			Choices: map[string][]string{"role": store.AssignableRoles},
		}, handler.createUser),
		Define(Endpoint{
			Name:        "update_user",
			Description: "Change another user's name, role (admin or user) or projects; fields not given keep their values. The email is set when the user is added and never changes: for another one, delete the user and add a new one. projects replaces only the user's projects you see; the others stay. Name and role change only when you have all the user's projects (can_take_over). Not yourself (update_account_name).",
			Idempotent:  true, Transports: ActsAsUser, Confirm: true,
			Choices: map[string][]string{"role": store.AssignableRoles},
		}, handler.updateUser),
		Define(Endpoint{
			Name:        "reset_user_password",
			Description: "Give another user a new temporary password (returned once): their sessions end and their connected agents are disconnected. Only for a user whose projects you all have (can_take_over).",
			Destructive: true, Transports: ActsAsUser,
		}, handler.resetUserPassword),
		Define(Endpoint{
			Name:        "delete_user",
			Description: "Delete another user; their sessions and connected agents end. Their past status changes keep their name. Only for a user whose projects you all have (can_take_over).",
			Destructive: true, Transports: ActsAsUser,
		}, handler.deleteUser),
	}, handler.notificationEndpoints()...)
}

// Account and agents.

type userOutput struct {
	userAccess
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
	// DisplayName is the name, or the email without one.
	DisplayName        string     `json:"display_name"`
	Role               string     `json:"role"`
	MustChangePassword bool       `json:"must_change_password,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	LastActiveAt       *time.Time `json:"last_active_at,omitempty"`
	// GoogleLinked: a Google account is linked to the user.
	GoogleLinked bool `json:"google_linked,omitempty"`
}

// userAccess is the projects a user sees, as much as the viewer may see
// of it (store.Access.Shown): a viewer with some projects is shown only
// those of the user's.
type userAccess struct {
	AllProjects bool     `json:"all_projects"`
	ProjectIDs  []int    `json:"project_ids"`
	Projects    []string `json:"projects"`
}

// userView is user as viewer sees them.
func (handler *handlers) userView(viewer, user store.User) userOutput {
	shown := user.ProjectAccess().Shown(viewer.ProjectAccess())
	access := userAccess{AllProjects: shown.All(), ProjectIDs: []int{}, Projects: []string{}}

	for _, id := range shown.ProjectIDs {
		access.ProjectIDs, access.Projects = append(access.ProjectIDs, id), append(access.Projects, handler.projectName(id))
	}

	return userOutput{
		userAccess: access,
		ID:         user.ID, Email: user.Email, Name: user.Name, DisplayName: user.DisplayName(), Role: user.Role,
		MustChangePassword: user.MustChangePassword, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt,
		LastActiveAt: user.LastActiveAt, GoogleLinked: user.GoogleSub != "",
	}
}

// managedUser is a user as an administrator sees them: CanManage says
// whether the viewer may change them at all (store.User.CanManage),
// CanTakeOver whether their name, role and password and delete them
// (store.User.CanTakeOver), and AccessEditable whether their projects;
// each reason says why not when the viewer may change the rest.
type managedUser struct {
	User           userOutput `json:"user"`
	CanManage      bool       `json:"can_manage"`
	CanTakeOver    bool       `json:"can_take_over"`
	TakeOverReason string     `json:"take_over_reason,omitempty"`
	AccessEditable bool       `json:"access_editable"`
	AccessReason   string     `json:"access_reason,omitempty"`
}

func (handler *handlers) managedView(viewer, user store.User) managedUser {
	editable, reason := handler.services.Users.AccessEditable(viewer, user)
	view := managedUser{
		User: handler.userView(viewer, user), CanManage: viewer.CanManage(user), CanTakeOver: viewer.CanTakeOver(user),
		AccessEditable: editable, AccessReason: reason,
	}

	if view.CanManage && !view.CanTakeOver {
		view.TakeOverReason = services.TakeOverProblem
	}

	return view
}

type noInput struct{}

func (handler *handlers) getAccount(_ context.Context, call Call, _ noInput) (userOutput, error) {
	return handler.userView(call.Viewer, call.Viewer), nil
}

type accountNameInput struct {
	Name string `json:"name" jsonschema:"The new display name; empty shows the email."`
}

func (handler *handlers) updateAccountName(ctx context.Context, call Call, input accountNameInput) (userOutput, error) {
	problems, err := handler.services.Users.UpdateName(ctx, call.Viewer, input.Name)

	if err != nil {
		return userOutput{}, err
	}

	if err := problemsError(problems, nil); err != nil {
		return userOutput{}, err
	}

	user, err := handler.services.Users.Get(ctx, call.Viewer.ID)

	return handler.userView(user, user), err
}

type themeInput struct {
	Theme string `json:"theme" jsonschema:"light or dark."`
}

type themeOutput struct {
	Theme string `json:"theme"`
}

func (handler *handlers) setTheme(ctx context.Context, call Call, input themeInput) (themeOutput, error) {
	return themeOutput(input), handler.services.Users.SetTheme(ctx, call.Viewer, input.Theme)
}

type listAgentsInput struct {
	UserID string `json:"user_id,omitempty" jsonschema:"Admins: a user they manage; the user the agent acts for when empty."`
}

type agentOutput struct {
	GrantID    string     `json:"grant_id"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	// Connected: false for a personal token never used, which is no connected app yet.
	Connected bool `json:"connected"`
}

type agentOwnerOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type agentsOutput struct {
	User   string `json:"user"`
	UserID string `json:"user_id"`
	// Owners: whose agents the caller may list, themselves included; only when more than one.
	Owners []agentOwnerOutput `json:"owners,omitempty"`
	Agents []agentOutput      `json:"agents"`
}

func (handler *handlers) listAgents(ctx context.Context, call Call, input listAgentsInput) (agentsOutput, error) {
	page, err := handler.services.Agents.Page(ctx, call.Viewer, input.UserID)

	if err != nil {
		return agentsOutput{}, err
	}

	output := agentsOutput{User: page.Owner.Email, UserID: page.Owner.ID, Agents: []agentOutput{}}

	if len(page.Owners) > 1 {
		for _, owner := range page.Owners {
			output.Owners = append(output.Owners, agentOwnerOutput{ID: owner.ID, Name: owner.DisplayName()})
		}
	}

	for _, grant := range page.Grants {
		output.Agents = append(output.Agents, agentOutput{
			GrantID: grant.ID, Name: call.Text(grant.Name), Kind: grant.Kind, CreatedAt: grant.CreatedAt, LastUsedAt: grant.LastUsedAt, Connected: grant.Connected(),
		})
	}

	return output, nil
}

type grantInput struct {
	GrantID string `json:"grant_id" jsonschema:"grant_id from list_agents."`
}

type doneOutput struct {
	Done string `json:"done"`
}

func (handler *handlers) disconnectAgent(ctx context.Context, call Call, input grantInput) (doneOutput, error) {
	grant, owner, err := handler.services.Agents.Disconnect(ctx, call.Viewer, input.GrantID)

	if err != nil {
		return doneOutput{}, err
	}

	// Only another user's agent: one's own is the account's.
	if owner.ID != call.Viewer.ID {
		handler.audit(ctx, call, auditChange{
			Resource: store.AuditAgent, Action: store.AuditDelete, RecordID: grant.ID, RecordName: grant.Name, ProjectIDs: userProjects(owner),
			Before: map[string]any{"name": grant.Name, "kind": grant.Kind, "user": owner.Email},
		})
	}

	return doneOutput{Done: "disconnected"}, nil
}

// Projects.

type projectInput struct {
	Project string `json:"project" jsonschema:"Project name or ID."`
}

// projectSettings is a project the way update_project takes it.
type projectSettings struct {
	projectView

	// Color is missing when the project follows the palette.
	Color string `json:"color,omitempty"`
	// ShownColor is the color it shows: its own or the palette's, #rrggbb.
	ShownColor string `json:"shown_color"`
	// DiskShare: the percent of MAX_DATABASE_SIZE it may hold before a cleanup of its own.
	DiskShare int `json:"disk_share"`
	// AllowedOrigins: the sites whose browsers may send to it; empty: any.
	AllowedOrigins []string `json:"allowed_origins"`
}

// findProject reads the project given by name or ID: an unknown one is
// not found, one the call does not see forbidden.
func (handler *handlers) findProject(ctx context.Context, call Call, nameOrID string) (store.Project, error) {
	project, err := handler.services.Projects.Find(call.Access(), nameOrID)

	if err != nil {
		return store.Project{}, err
	}

	return handler.services.Projects.Get(ctx, call.Access(), project.ID)
}

func (handler *handlers) settings(call Call, project store.Project) projectSettings {
	shown := config.Project{ID: project.ID, Name: project.Name, PublicKey: project.PublicKey, LogLevels: project.LogLevels}

	return projectSettings{
		projectView: handler.projectView(call, shown), Color: project.Color, ShownColor: handler.services.Projects.Color(project), DiskShare: project.Share(),
		AllowedOrigins: append([]string{}, project.AllowedOrigins...),
	}
}

// projectDefaults is what the form of a new project starts with.
type projectDefaults struct {
	ShownColor string   `json:"shown_color"`
	LogLevels  []string `json:"log_levels"`
	DiskShare  int      `json:"disk_share"`
	// AllowedOrigins: empty, any site.
	AllowedOrigins []string `json:"allowed_origins"`
}

func (handler *handlers) getProjectDefaults(_ context.Context, call Call, _ noInput) (projectDefaults, error) {
	defaults := handler.services.Projects.FormValues(store.Project{})

	return projectDefaults{
		ShownColor: handler.services.Projects.Color(store.Project{}), LogLevels: defaults["log_level"], DiskShare: store.Project{}.Share(),
		AllowedOrigins: []string{},
	}, nil
}

type projectIDInput struct {
	Project int `json:"project" jsonschema:"Project ID."`
}

// projectSetup is how an app is connected to a project.
type projectSetup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	DSN  string `json:"dsn"`
	// Connected: an app sent a request with the project's key, of any kind
	// (a session at the SDK's start too), or the project has stored data.
	Connected bool `json:"connected"`
	// ConnectedAt is the first such request; null before it, or for data
	// stored before first requests were recorded.
	ConnectedAt *time.Time `json:"connected_at"`
	// Received: the project has stored an error or a log.
	Received  bool          `json:"received"`
	Platforms []sdkPlatform `json:"platforms"`
	// Paused: what its apps send is answered but not stored.
	Paused bool `json:"paused"`
	// KeyChangedAt is when the DSN was last reset; null before the first reset (CreatedAt then).
	KeyChangedAt *time.Time `json:"key_changed_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

type sdkPlatform struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Install string `json:"install"`
	Where   string `json:"where"`
	Init    string `json:"init"`
	Test    string `json:"test"`
	// AgentPrompt asks a coding agent to do the same.
	AgentPrompt string `json:"agent_prompt"`
}

func (handler *handlers) getProjectSetup(ctx context.Context, call Call, input projectIDInput) (projectSetup, error) {
	if call.Address == nil {
		return projectSetup{}, errors.New("project setup without the server's address")
	}

	contact, err := handler.services.Projects.Contact(ctx, call.Access(), input.Project)

	if err != nil {
		return projectSetup{}, err
	}

	stored, err := handler.services.Projects.Get(ctx, call.Access(), input.Project)

	if err != nil {
		return projectSetup{}, err
	}

	project, _ := handler.projects.List().ByID(input.Project)
	setup := projectSetup{
		ID: project.ID, Name: project.Name, DSN: project.DSN(call.Address),
		Connected: contact.Connected, ConnectedAt: contact.ConnectedAt, Received: contact.Received, Platforms: []sdkPlatform{},
		KeyChangedAt: stored.KeyChangedAt, CreatedAt: stored.CreatedAt, Paused: stored.Paused,
	}

	for _, sdk := range services.SDKSetups(setup.DSN) {
		setup.Platforms = append(setup.Platforms, sdkPlatform{
			ID: sdk.Platform, Label: sdk.Label, Install: sdk.Install, Where: sdk.Where, Init: sdk.Init, Test: sdk.Test, AgentPrompt: sdk.AgentPrompt,
		})
	}

	return setup, err
}

func (handler *handlers) getProject(ctx context.Context, call Call, input projectInput) (projectSettings, error) {
	project, err := handler.findProject(ctx, call, input.Project)

	if err != nil {
		return projectSettings{}, err
	}

	return handler.settings(call, project), nil
}

// projectFields are the settings a tool may change; nil keeps the value.
type projectFields struct {
	Color     *string   `json:"color,omitempty" jsonschema:"#rrggbb, or empty for the palette color."`
	LogLevels *[]string `json:"log_levels,omitempty" jsonschema:"Log levels to store; an empty list stores none. Events of every level are stored."`
	DiskShare *int      `json:"disk_share,omitempty" jsonschema:"Percent of MAX_DATABASE_SIZE the project may hold, in steps of 5; above it the cleanup deletes its own data first. 100: no limit of its own."`
	// AllowedOrigins: maxItems is config.MaxAllowedOrigins.
	AllowedOrigins *[]string `json:"allowed_origins,omitempty" jsonschema:"Sites whose browsers may send to the project, as scheme://host[:port] (https://app.example.com, https://*.example.com for subdomains); an empty list: any site. Backend SDKs are not affected."`
}

var projectChoices = map[string][]string{"log_levels": config.LogLevels}

// apply writes the given fields over the form.
func (fields projectFields) apply(form url.Values) {
	if fields.Color != nil {
		form.Set("color", *fields.Color)
	}

	if fields.LogLevels != nil {
		form["log_level"] = *fields.LogLevels
	}

	if fields.DiskShare != nil {
		form.Set("disk_share", strconv.Itoa(*fields.DiskShare))
	}

	if fields.AllowedOrigins != nil {
		// An empty list still sends the field, so it clears them.
		form["allowed_origin"] = append([]string{""}, *fields.AllowedOrigins...)
	}
}

// projectFormFields names the form's fields as the tools do.
var projectFormFields = map[string]string{"log_level": "log_levels", "allowed_origin": "allowed_origins"}

type createProjectInput struct {
	Name string `json:"name" jsonschema:"Lowercase letters, digits, '-' and '_', starting with a letter or digit, not digits only (that reads as an ID): backend, frontend, worker."`
	projectFields
}

func (handler *handlers) saveProject(ctx context.Context, call Call, form url.Values, project store.Project) (projectSettings, error) {
	saved, problems, err := handler.services.Projects.Save(ctx, call.Viewer, form, project)

	if err != nil {
		return projectSettings{}, formError(err, projectFormFields)
	}

	if err := problemsError(problems, projectFormFields); err != nil {
		return projectSettings{}, err
	}

	// As stored: a new project's key is made by the store. Read with every
	// project: the caller's access read before the save lacks a new one.
	saved, err = handler.services.Projects.Get(ctx, store.Access{}, saved.ID)

	if err != nil {
		return projectSettings{}, err
	}

	if project.ID == 0 {
		handler.audit(ctx, call, projectChange(store.AuditCreate, saved, nil, handler.projectAudit(saved)))
	} else {
		handler.audit(ctx, call, projectChange(store.AuditEdit, saved, handler.projectAudit(project), handler.projectAudit(saved)))
	}

	return handler.settings(call, saved), nil
}

func (handler *handlers) createProject(ctx context.Context, call Call, input createProjectInput) (projectSettings, error) {
	form := handler.services.Projects.FormValues(store.Project{})
	form.Set("name", input.Name)
	input.apply(form)

	return handler.saveProject(ctx, call, form, store.Project{})
}

type updateProjectInput struct {
	Project string  `json:"project" jsonschema:"Project name or ID."`
	Name    *string `json:"name,omitempty" jsonschema:"A new name."`
	projectFields
}

func (handler *handlers) updateProject(ctx context.Context, call Call, input updateProjectInput) (projectSettings, error) {
	project, err := handler.findProject(ctx, call, input.Project)

	if err != nil {
		return projectSettings{}, err
	}

	form := handler.services.Projects.FormValues(project)

	if input.Name != nil {
		form.Set("name", *input.Name)
	}

	input.apply(form)

	return handler.saveProject(ctx, call, form, project)
}

func (handler *handlers) deleteProject(ctx context.Context, call Call, input projectInput) (doneOutput, error) {
	project, err := handler.findProject(ctx, call, input.Project)

	if err != nil {
		return doneOutput{}, err
	}

	if err := handler.services.Projects.Delete(ctx, call.Access(), project.ID); err != nil {
		return doneOutput{}, err
	}

	handler.audit(ctx, call, projectChange(store.AuditDelete, project, handler.projectAudit(project), nil))

	return doneOutput{Done: "deleted project " + project.Name}, nil
}

// Users.

func (handler *handlers) listUsers(ctx context.Context, call Call, _ noInput) (Page[managedUser], error) {
	users, err := handler.services.Users.List(ctx, call.Viewer)
	output := Page[managedUser]{Items: []managedUser{}}

	for _, user := range users {
		output.Items = append(output.Items, handler.managedView(call.Viewer, user))
	}

	return output, err
}

func (handler *handlers) getUser(ctx context.Context, call Call, input userInput) (managedUser, error) {
	user, err := handler.services.Users.Visible(ctx, call.Viewer, input.UserID)

	return handler.managedView(call.Viewer, user), err
}

type createUserInput struct {
	Email string `json:"email" jsonschema:"Email address, the login."`
	Name  string `json:"name,omitempty" jsonschema:"Display name."`
	Role  string `json:"role" jsonschema:"The role."`
	accessFields
}

// accessFields choose the projects a user sees; nil keeps them (a new
// user gets the viewer's).
type accessFields struct {
	AllProjects *bool     `json:"all_projects,omitempty" jsonschema:"Every project, including future ones (only someone who has every project may give it)."`
	Projects    *[]string `json:"projects,omitempty" jsonschema:"Project names or IDs the user sees, when all_projects is not true; an empty list: none."`
}

// access reads the fields (projectsField's refusals).
func (handler *handlers) access(call Call, fields accessFields) (*store.Access, error) {
	switch {
	case fields.AllProjects != nil && *fields.AllProjects:
		return &store.Access{}, nil
	case fields.Projects == nil && fields.AllProjects == nil:
		return nil, nil
	}

	var names []string

	if fields.Projects != nil {
		names = *fields.Projects
	}

	ids, err := handler.projectsField(call, "projects", names)

	if err != nil {
		return nil, err
	}

	access := store.OnlyProjects(ids...)

	return &access, nil
}

type passwordOutput struct {
	User              userOutput `json:"user"`
	TemporaryPassword string     `json:"temporary_password"`
	Note              string     `json:"note"`
}

const temporaryPasswordNote = "Shown only this once. Give it to the user privately; they choose their own password at the first login. Do not store it."

func (handler *handlers) createUser(ctx context.Context, call Call, input createUserInput) (passwordOutput, error) {
	access, err := handler.access(call, input.accessFields)

	if err != nil {
		return passwordOutput{}, err
	}

	id, password, problems, err := handler.services.Users.Create(ctx, call.Viewer, services.UserInput{Email: input.Email, Name: input.Name, Role: input.Role, Access: access})

	if err != nil {
		return passwordOutput{}, err
	}

	if err := problemsError(problems, nil); err != nil {
		return passwordOutput{}, err
	}

	return handler.withPassword(ctx, call, id, password, func(user store.User) auditChange {
		return userChange(store.AuditCreate, user, nil, handler.userAudit(user))
	})
}

// withPassword answers user id with the temporary password given them, recording change.
func (handler *handlers) withPassword(ctx context.Context, call Call, id string, password string, change func(store.User) auditChange) (passwordOutput, error) {
	user, err := handler.services.Users.Get(ctx, id)

	if err == nil {
		handler.audit(ctx, call, change(user))
	}

	return passwordOutput{User: handler.userView(call.Viewer, user), TemporaryPassword: password, Note: temporaryPasswordNote}, err
}

type userInput struct {
	UserID string `json:"user_id" jsonschema:"id from list_users."`
}

type updateUserInput struct {
	UserID string  `json:"user_id" jsonschema:"id from list_users."`
	Name   *string `json:"name,omitempty" jsonschema:"Display name."`
	Role   *string `json:"role,omitempty" jsonschema:"The role."`
	accessFields
}

func (handler *handlers) updateUser(ctx context.Context, call Call, input updateUserInput) (userOutput, error) {
	user, err := handler.services.Users.Visible(ctx, call.Viewer, input.UserID)

	if err != nil {
		return userOutput{}, err
	}

	access, err := handler.access(call, input.accessFields)

	if err != nil {
		return userOutput{}, err
	}

	change := services.UserInput{Email: user.Email, Name: user.Name, Role: user.Role, Access: access}

	if input.Name != nil {
		change.Name = *input.Name
	}

	if input.Role != nil {
		change.Role = *input.Role
	}

	problems, err := handler.services.Users.Update(ctx, call.Viewer, input.UserID, change)

	if err != nil {
		return userOutput{}, err
	}

	if err := problemsError(problems, nil); err != nil {
		return userOutput{}, err
	}

	updated, err := handler.services.Users.Get(ctx, input.UserID)

	if err != nil {
		return userOutput{}, err
	}

	handler.audit(ctx, call, userChange(store.AuditEdit, updated, handler.userAudit(user), handler.userAudit(updated), user))

	return handler.userView(call.Viewer, updated), nil
}

func (handler *handlers) resetUserPassword(ctx context.Context, call Call, input userInput) (passwordOutput, error) {
	password, err := handler.services.Users.ResetPassword(ctx, call.Viewer, input.UserID)

	if err != nil {
		return passwordOutput{}, err
	}

	return handler.withPassword(ctx, call, input.UserID, password, func(user store.User) auditChange {
		change := userChange(store.AuditResetPassword, user, nil, nil)
		change.Replaced = []string{"password"}

		return change
	})
}

func (handler *handlers) deleteUser(ctx context.Context, call Call, input userInput) (doneOutput, error) {
	// Read for the audit log; Delete checks the rest.
	user, _ := handler.services.Users.Get(ctx, input.UserID)

	if err := handler.services.Users.Delete(ctx, call.Viewer, input.UserID); err != nil {
		return doneOutput{}, err
	}

	handler.audit(ctx, call, userChange(store.AuditDelete, user, handler.userAudit(user), nil))

	return doneOutput{Done: "deleted user " + input.UserID}, nil
}

// resetProjectKey gives the project a new DSN and answers its setup with it.
// The audit log keeps the old and new DSN: a DSN is public by design, and
// the old one no longer works.
func (handler *handlers) resetProjectKey(ctx context.Context, call Call, input projectIDInput) (projectSetup, error) {
	before, err := handler.services.Projects.Get(ctx, call.Access(), input.Project)

	if err != nil {
		return projectSetup{}, err
	}

	if err := handler.services.Projects.ResetKey(ctx, call.Access(), input.Project); err != nil {
		return projectSetup{}, err
	}

	if after, err := handler.services.Projects.Get(ctx, call.Access(), input.Project); err == nil {
		dsn := func(project store.Project) map[string]string {
			return map[string]string{"dsn": config.Project{ID: project.ID, PublicKey: project.PublicKey}.DSN(call.Address)}
		}

		handler.audit(ctx, call, projectChange(store.AuditResetKey, after, dsn(before), dsn(after)))
	}

	return handler.getProjectSetup(ctx, call, input)
}

type setProjectActiveInput struct {
	Project string `json:"project" jsonschema:"Project name or ID."`
	Active  bool   `json:"active" jsonschema:"false pauses recording, true resumes it."`
}

// setProjectActive pauses or resumes recording a project.
func (handler *handlers) setProjectActive(ctx context.Context, call Call, input setProjectActiveInput) (projectView, error) {
	project, err := handler.services.Projects.Find(call.Access(), input.Project)

	if err != nil {
		return projectView{}, err
	}

	stored, err := handler.services.Projects.Get(ctx, call.Access(), project.ID)

	if err != nil {
		return projectView{}, err
	}

	if err := handler.services.Projects.SetPaused(ctx, call.Access(), project.ID, !input.Active); err != nil {
		return projectView{}, err
	}

	action := store.AuditResume

	if !input.Active {
		action = store.AuditPause
	}

	handler.audit(ctx, call, projectChange(action, stored, map[string]bool{"paused": stored.Paused}, map[string]bool{"paused": !input.Active}))

	project, _ = handler.projects.List().ByID(project.ID)

	return handler.projectView(call, project), nil
}
