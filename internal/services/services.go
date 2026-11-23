// Package services holds the rules of the application: the only way the
// request catalog (package api: REST, MCP and the web app) reaches the database.
package services

import (
	"log/slog"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/projects"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Dependencies are what the services are built from.
type Dependencies struct {
	Logger   *slog.Logger
	Store    store.Store
	Registry *projects.Registry
	Settings *config.Config
	Factory  notify.Factory
	Notifier *notify.Notifier // sends the outbox; nil in tests that need none
	OAuth    *oauth.Server    // the MCP server's sign-in
}

// All are the services the request catalog (package api) and the login
// (session) share, so every transport applies the same rules.
type All struct {
	// AppName names the server to its users (APP_NAME).
	AppName      string
	Issues       *Issues
	Projects     *Projects
	Connections  *Connections
	Deliveries   *Deliveries
	Verification *Verification
	Logs         *Logs
	Agents       *Agents
	Users        *Users
	Sessions     *Sessions
}

func New(dependencies Dependencies) *All {
	settings, database := dependencies.Settings, dependencies.Store
	connections := newConnections(dependencies.Logger, database, dependencies.Factory, dependencies.Registry, settings)
	connections.notifier = dependencies.Notifier
	users := NewUsers(dependencies.Logger, database)
	users.projects = dependencies.Registry

	return &All{
		AppName:      settings.AppName,
		Issues:       newIssues(dependencies.Logger, database),
		Projects:     newProjects(database, dependencies.Registry, settings),
		Connections:  connections,
		Deliveries:   newDeliveries(database, connections),
		Verification: newVerification(dependencies.Logger, database, dependencies.Factory, connections),
		Logs:         newLogs(database),
		Agents:       newAgents(dependencies.OAuth, database, settings),
		Users:        users,
		Sessions:     NewSessions(database),
	}
}
