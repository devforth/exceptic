// Package services holds the rules of the application: the only way the UI
// pages and the agents' tools reach the database.
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
	OAuth    *oauth.Server // the MCP server's sign-in
}

// All are the services the UI pages and the agents' tools (package api)
// share, so both apply the same rules.
type All struct {
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

	return &All{
		Issues:       newIssues(database),
		Projects:     newProjects(database, dependencies.Registry, settings.Recording, settings.Notifications.PublicURL, settings.TrustedProxies),
		Connections:  connections,
		Deliveries:   newDeliveries(database),
		Verification: newVerification(dependencies.Logger, database, dependencies.Factory, connections),
		Logs:         newLogs(database),
		Agents:       newAgents(dependencies.OAuth, database),
		Users:        NewUsers(dependencies.Logger, database),
		Sessions:     NewSessions(database),
	}
}
