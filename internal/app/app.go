// Package app builds the server from its parts: one wiring for main and the tests.
package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/ingest"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/oauth"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/projects"
	"github.com/devforth/exceptic/internal/ui"
	"github.com/devforth/exceptic/web"
)

// Options are what a server is built from: main fills them from the
// environment, the tests with fakes.
type Options struct {
	Settings *config.Config
	Logger   *slog.Logger
	Store    *database.Store
	Factory  notify.Factory // the notification channels
	// Web is the single-page app (web.New in main, a small fake in tests); nil serves none.
	Web     *web.App
	Version string
}

// App is a built server; Run serves it.
type App struct {
	Handler  http.Handler
	Registry *projects.Registry
	OAuth    *oauth.Server
	Services *services.All

	options  Options
	notifier *notify.Notifier
	pipeline *ingest.Pipeline
}

func New(ctx context.Context, options Options) (*App, error) {
	settings, logger, db := options.Settings, options.Logger, options.Store

	// Projects live in the database; the ones in PROJECTS are written there with the same values at every start.
	registry, err := projects.Open(ctx, logger, db, settings.Projects)

	if err != nil {
		return nil, err
	}

	authorization := &oauth.Server{
		Name: settings.AppName, Store: db, PublicURL: settings.Notifications.PublicURL, TrustedProxies: settings.TrustedProxies, ResourcePath: api.MCPPath,
		Logger: logger,
	}
	all := services.New(services.Dependencies{
		Logger: logger, Store: db, Registry: registry, Settings: settings, Factory: options.Factory, OAuth: authorization,
	})

	// The first start creates the superadmin from env; later starts do not
	// use ADMIN_USERNAME/ADMIN_PASSWORD.
	if err := all.Users.EnsureSuperadmin(ctx, settings.AdminUsername, settings.AdminPassword); err != nil {
		return nil, err
	}

	// One login for the UI and the web app: its attempt limits live in memory.
	auth := session.NewAuth(logger, settings, all.Users, all.Sessions)
	webUI, err := ui.New(logger, settings, all, auth)

	if err != nil {
		return nil, err
	}

	// A nil *web.App must not become a non-nil server.Routes.
	var spa server.Routes

	if options.Web != nil {
		spa = options.Web
	}

	app := &App{Registry: registry, OAuth: authorization, Services: all, options: options}
	app.notifier = notify.New(notify.Options{
		Store: db, Channels: options.Factory, Logger: logger,
		RetryFor: settings.Notifications.RetryFor, KeepSent: settings.Notifications.KeepSent, KeepFailed: settings.Notifications.KeepFailed,
	})
	sink := ingest.StoreSink{Logger: logger, Notifier: app.notifier, PublicURL: settings.Notifications.PublicURL, Recording: settings.Recording}
	app.pipeline = ingest.NewPipeline(logger, settings, registry, db, sink)
	app.pipeline.MaxInboxBytes = config.MaxInboxBytes(settings.Retention.MaxDatabaseBytes)

	// One catalog of requests, served as the REST API and as MCP tools, on the same services as the UI.
	endpoints := api.Endpoints(registry, all)
	app.Handler = server.New(server.Options{
		Logger:         logger,
		Database:       db,
		Ingest:         app.pipeline.Handler(),
		API:            api.NewREST(logger, endpoints, settings.Notifications.PublicURL, settings.TrustedProxies),
		MCP:            api.NewMCP(logger, db, endpoints, authorization, settings.AppName, options.Version),
		OAuth:          authorization,
		UI:             webUI,
		Web:            spa,
		UIAPI:          api.NewUI(logger, endpoints, session.AgentRequests(auth, all.Agents), auth, settings.Notifications.PublicURL, settings.TrustedProxies),
		APIToken:       settings.APIToken,
		AllowedIPs:     settings.AllowedIPs,
		TrustedProxies: settings.TrustedProxies,
	}).Handler()

	return app, nil
}
