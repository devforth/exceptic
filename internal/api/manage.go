package api

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/projects"
	"github.com/devforth/exceptic/internal/storage/store"
)

// The endpoints that manage the server, as the UI pages do: they act as a
// user (ActsAsUser) through the same services. Users and Agents check what
// the user may change; the Admin flag guards projects and notifications.
// A user changes their own password only in the UI; an agent never mints
// MCP tokens.
func (handler *handlers) management() []Endpoint {
	return append([]Endpoint{
		Define(Endpoint{
			Name:        "get_account",
			Description: "Who the agent acts for: username, name and role (superadmin, admin or user), which decides the tools offered.",
			ReadOnly:    true, Transports: ActsAsUser,
		}, handler.getAccount),
		Define(Endpoint{
			Name:        "update_account_name",
			Description: "Change the display name of the user the agent acts for. The password can only be changed in the UI.",
			Idempotent:  true, Transports: ActsAsUser,
		}, handler.updateAccountName),
		Define(Endpoint{
			Name:        "list_agents",
			Description: "Coding agents and personal tokens connected to MCP for the user (admins: user_id of a user they manage), with when each was last used.",
			ReadOnly:    true, Transports: ActsAsUser,
		}, handler.listAgents),
		Define(Endpoint{
			Name:        "disconnect_agent",
			Description: "Revoke a connected agent or personal token (grant_id from list_agents); it stops working at once.",
			Destructive: true, Transports: ActsAsUser,
		}, handler.disconnectAgent),

		Define(Endpoint{
			Name:        "get_project",
			Description: "Settings of a project: DSN, color (missing when it follows the palette), the color it shows and recorded levels.",
			ReadOnly:    true, Admin: true, Transports: ActsAsUser,
		}, handler.getProject),
		Define(Endpoint{
			Name:        "create_project",
			Description: "Add a project (one per service sending errors); returns its DSN for the app's Sentry SDK.",
			Admin:       true, Transports: ActsAsUser,
			Choices: projectChoices,
		}, handler.createProject),
		Define(Endpoint{
			Name:        "update_project",
			Description: "Change a project's settings; fields not given keep their values, an empty string returns a setting to the default. Projects from PROJECTS keep their name.",
			Idempotent:  true, Admin: true, Transports: ActsAsUser, Confirm: true,
			Choices: projectChoices,
		}, handler.updateProject),
		Define(Endpoint{
			Name:        "delete_project",
			Description: "Delete a project added in the UI with all its issues, events and logs; apps using its DSN are rejected afterwards.",
			Destructive: true, Admin: true, Transports: ActsAsUser,
		}, handler.deleteProject),

		Define(Endpoint{
			Name:        "list_users",
			Description: "Users of the panel with their role, last login and whether you may change them (can_manage).",
			ReadOnly:    true, Admin: true, Transports: ActsAsUser,
		}, handler.listUsers),
		Define(Endpoint{
			Name:        "get_user",
			Description: "A user with whether the viewer may change them.",
			ReadOnly:    true, Admin: true, Transports: TransportUI,
		}, handler.getUser),
		Define(Endpoint{
			Name:        "create_user",
			Description: "Add a user with a role (admin or user). Returns a temporary password, shown only this once, that the user must change at the first login: give it to the user privately.",
			Admin:       true, Transports: ActsAsUser, Confirm: true,
			Choices: map[string][]string{"role": store.AssignableRoles},
		}, handler.createUser),
		Define(Endpoint{
			Name:        "update_user",
			Description: "Change another user's name or role (admin or user); fields not given keep their values. Not the superadmin, not yourself (update_account_name).",
			Idempotent:  true, Admin: true, Transports: ActsAsUser, Confirm: true,
			Choices: map[string][]string{"role": store.AssignableRoles},
		}, handler.updateUser),
		Define(Endpoint{
			Name:        "reset_user_password",
			Description: "Give another user a new temporary password (returned once): their sessions end and their connected agents are disconnected.",
			Destructive: true, Admin: true, Transports: ActsAsUser,
		}, handler.resetUserPassword),
		Define(Endpoint{
			Name:        "delete_user",
			Description: "Delete another user; their sessions and connected agents end. Their past status changes keep their name.",
			Destructive: true, Admin: true, Transports: ActsAsUser,
		}, handler.deleteUser),
	}, handler.notificationEndpoints()...)
}

// Account and agents.

type userOutput struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
	// DisplayName is the name, or the username without one.
	DisplayName        string     `json:"display_name"`
	Role               string     `json:"role"`
	MustChangePassword bool       `json:"must_change_password,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
}

func userView(user store.User) userOutput {
	return userOutput{
		ID: user.ID, Username: user.Username, Name: user.Name, DisplayName: user.DisplayName(), Role: user.Role,
		MustChangePassword: user.MustChangePassword, CreatedAt: user.CreatedAt, LastLoginAt: user.LastLoginAt,
	}
}

// managedUser is a user as an administrator sees them: CanManage says
// whether the viewer may change them (store.User.CanManage).
type managedUser struct {
	User      userOutput `json:"user"`
	CanManage bool       `json:"can_manage"`
}

func managedView(viewer, user store.User) managedUser {
	return managedUser{User: userView(user), CanManage: viewer.CanManage(user)}
}

type noInput struct{}

func (handler *handlers) getAccount(_ context.Context, call Call, _ noInput) (userOutput, error) {
	return userView(call.Viewer), nil
}

type accountNameInput struct {
	Name string `json:"name" jsonschema:"The new display name; empty shows the username."`
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

	return userView(user), err
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
}

type agentsOutput struct {
	User   string        `json:"user"`
	Agents []agentOutput `json:"agents"`
}

func (handler *handlers) listAgents(ctx context.Context, call Call, input listAgentsInput) (agentsOutput, error) {
	page, err := handler.services.Agents.Page(ctx, call.Viewer, input.UserID)

	if err != nil {
		return agentsOutput{}, err
	}

	// Page falls back to the viewer's own agents: say so instead.
	if input.UserID != "" && page.Owner.ID != input.UserID {
		return agentsOutput{}, ErrNotFound
	}

	output := agentsOutput{User: page.Owner.Username, Agents: []agentOutput{}}

	for _, grant := range page.Grants {
		output.Agents = append(output.Agents, agentOutput{GrantID: grant.ID, Name: call.Text(grant.Name), Kind: grant.Kind, CreatedAt: grant.CreatedAt, LastUsedAt: grant.LastUsedAt})
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
	if err := handler.services.Agents.Disconnect(ctx, call.Viewer, input.GrantID); err != nil {
		return doneOutput{}, err
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
}

// findProject reads the project given by name or ID; an unknown one is not found.
func (handler *handlers) findProject(ctx context.Context, nameOrID string) (store.Project, error) {
	project, ok := handler.projects.List().Find(nameOrID)

	if !ok {
		return store.Project{}, fmt.Errorf("unknown project %q (see list_projects): %w", nameOrID, ErrNotFound)
	}

	return handler.services.Projects.Get(ctx, project.ID)
}

func (handler *handlers) settings(call Call, project store.Project) projectSettings {
	shown := config.Project{ID: project.ID, Name: project.Name, PublicKey: project.PublicKey, EventLevels: project.EventLevels, LogLevels: project.LogLevels}

	return projectSettings{projectView: handler.projectView(call, shown), Color: project.Color, ShownColor: handler.services.Projects.Color(project)}
}

// projectDefaults is what the form of a new project starts with.
type projectDefaults struct {
	ShownColor  string   `json:"shown_color"`
	EventLevels []string `json:"event_levels"`
	LogLevels   []string `json:"log_levels"`
}

func (handler *handlers) getProjectDefaults(context.Context, Call, noInput) (projectDefaults, error) {
	defaults := handler.services.Projects.FormValues(store.Project{})

	return projectDefaults{ShownColor: handler.services.Projects.Color(store.Project{}), EventLevels: defaults["event_level"], LogLevels: defaults["log_level"]}, nil
}

type projectIDInput struct {
	Project int `json:"project" jsonschema:"Project ID."`
}

// projectSetup is how an app is connected to a project.
type projectSetup struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	DSN  string `json:"dsn"`
	// Received: the project has stored an error or a log.
	Received  bool          `json:"received"`
	Platforms []sdkPlatform `json:"platforms"`
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
	project, ok := handler.projects.List().ByID(input.Project)

	if !ok || call.Address == nil {
		return projectSetup{}, ErrNotFound
	}

	received, err := handler.services.Projects.Received(ctx, project.ID)
	setup := projectSetup{ID: project.ID, Name: project.Name, DSN: project.DSN(call.Address), Received: received, Platforms: []sdkPlatform{}}

	for _, sdk := range services.SDKSetups(setup.DSN) {
		setup.Platforms = append(setup.Platforms, sdkPlatform{
			ID: sdk.Platform, Label: sdk.Label, Install: sdk.Install, Where: sdk.Where, Init: sdk.Init, Test: sdk.Test, AgentPrompt: sdk.AgentPrompt,
		})
	}

	return setup, err
}

func (handler *handlers) getProject(ctx context.Context, call Call, input projectInput) (projectSettings, error) {
	project, err := handler.findProject(ctx, input.Project)

	if err != nil {
		return projectSettings{}, err
	}

	return handler.settings(call, project), nil
}

// projectFields are the settings a tool may change; nil keeps the value.
type projectFields struct {
	Color       *string   `json:"color,omitempty" jsonschema:"#rrggbb, or empty for the palette color."`
	EventLevels *[]string `json:"event_levels,omitempty" jsonschema:"Event levels to store; an empty list stores none."`
	LogLevels   *[]string `json:"log_levels,omitempty" jsonschema:"Log levels to store; an empty list stores none."`
}

var projectChoices = map[string][]string{"event_levels": config.Levels, "log_levels": config.LogLevels}

// apply writes the given fields over the form.
func (fields projectFields) apply(form url.Values) {
	if fields.Color != nil {
		form.Set("color", *fields.Color)
	}

	if fields.EventLevels != nil {
		form["event_level"] = *fields.EventLevels
	}

	if fields.LogLevels != nil {
		form["log_level"] = *fields.LogLevels
	}
}

// projectFormFields names the form's fields as the tools do.
var projectFormFields = map[string]string{"event_level": "event_levels", "log_level": "log_levels"}

type createProjectInput struct {
	Name string `json:"name" jsonschema:"Lowercase letters, digits, '-' and '_', starting with a letter or digit, not digits only (that reads as an ID): backend, frontend, worker."`
	projectFields
}

func (handler *handlers) saveProject(ctx context.Context, call Call, form url.Values, project store.Project) (projectSettings, error) {
	saved, problems, err := handler.services.Projects.Save(ctx, form, project)

	if err != nil {
		return projectSettings{}, err
	}

	if err := problemsError(problems, projectFormFields); err != nil {
		return projectSettings{}, err
	}

	// As stored: a new project's key is made by the store.
	saved, err = handler.services.Projects.Get(ctx, saved.ID)

	return handler.settings(call, saved), err
}

func (handler *handlers) createProject(ctx context.Context, call Call, input createProjectInput) (projectSettings, error) {
	form := handler.services.Projects.FormValues(store.Project{})
	form.Set("name", input.Name)
	input.apply(form)

	return handler.saveProject(ctx, call, form, store.Project{})
}

type updateProjectInput struct {
	Project string  `json:"project" jsonschema:"Project name or ID."`
	Name    *string `json:"name,omitempty" jsonschema:"A new name (not for projects from PROJECTS)."`
	projectFields
}

func (handler *handlers) updateProject(ctx context.Context, call Call, input updateProjectInput) (projectSettings, error) {
	project, err := handler.findProject(ctx, input.Project)

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

func (handler *handlers) deleteProject(ctx context.Context, _ Call, input projectInput) (doneOutput, error) {
	project, err := handler.findProject(ctx, input.Project)

	if err != nil {
		return doneOutput{}, err
	}

	if err := handler.services.Projects.Delete(ctx, project.ID); errors.Is(err, projects.ErrFromEnv) {
		return doneOutput{}, Invalid("project %q comes from PROJECTS: remove it there", project.Name)
	} else if err != nil {
		return doneOutput{}, err
	}

	return doneOutput{Done: "deleted project " + project.Name}, nil
}

// Users.

func (handler *handlers) listUsers(ctx context.Context, call Call, _ noInput) (Page[managedUser], error) {
	users, err := handler.services.Users.List(ctx)
	output := Page[managedUser]{Items: []managedUser{}}

	for _, user := range users {
		output.Items = append(output.Items, managedView(call.Viewer, user))
	}

	return output, err
}

func (handler *handlers) getUser(ctx context.Context, call Call, input userInput) (managedUser, error) {
	user, err := handler.services.Users.Get(ctx, input.UserID)

	return managedView(call.Viewer, user), err
}

type createUserInput struct {
	Username string `json:"username" jsonschema:"Login name: letters, digits and . _ @ -."`
	Name     string `json:"name,omitempty" jsonschema:"Display name."`
	Role     string `json:"role" jsonschema:"The role."`
}

type passwordOutput struct {
	User              userOutput `json:"user"`
	TemporaryPassword string     `json:"temporary_password"`
	Note              string     `json:"note"`
}

const temporaryPasswordNote = "Shown only this once. Give it to the user privately; they choose their own password at the first login. Do not store it."

func (handler *handlers) createUser(ctx context.Context, call Call, input createUserInput) (passwordOutput, error) {
	id, password, problems, err := handler.services.Users.Create(ctx, call.Viewer, services.UserInput{Username: input.Username, Name: input.Name, Role: input.Role})

	if err != nil {
		return passwordOutput{}, err
	}

	if err := problemsError(problems, nil); err != nil {
		return passwordOutput{}, err
	}

	return handler.withPassword(ctx, id, password)
}

func (handler *handlers) withPassword(ctx context.Context, id string, password string) (passwordOutput, error) {
	user, err := handler.services.Users.Get(ctx, id)

	return passwordOutput{User: userView(user), TemporaryPassword: password, Note: temporaryPasswordNote}, err
}

type userInput struct {
	UserID string `json:"user_id" jsonschema:"id from list_users."`
}

type updateUserInput struct {
	UserID string  `json:"user_id" jsonschema:"id from list_users."`
	Name   *string `json:"name,omitempty" jsonschema:"Display name."`
	Role   *string `json:"role,omitempty" jsonschema:"The role."`
}

func (handler *handlers) updateUser(ctx context.Context, call Call, input updateUserInput) (userOutput, error) {
	user, err := handler.services.Users.Get(ctx, input.UserID)

	if err != nil {
		return userOutput{}, err
	}

	change := services.UserInput{Username: user.Username, Name: user.Name, Role: user.Role}

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

	user, err = handler.services.Users.Get(ctx, input.UserID)

	return userView(user), err
}

func (handler *handlers) resetUserPassword(ctx context.Context, call Call, input userInput) (passwordOutput, error) {
	password, err := handler.services.Users.ResetPassword(ctx, call.Viewer, input.UserID)

	if err != nil {
		return passwordOutput{}, err
	}

	return handler.withPassword(ctx, input.UserID, password)
}

func (handler *handlers) deleteUser(ctx context.Context, call Call, input userInput) (doneOutput, error) {
	if err := handler.services.Users.Delete(ctx, call.Viewer, input.UserID); err != nil {
		return doneOutput{}, err
	}

	return doneOutput{Done: "deleted user " + input.UserID}, nil
}
