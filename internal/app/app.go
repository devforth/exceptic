// Package app builds the server from its parts: one wiring for main and the tests.
package app

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"time"

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
	"github.com/devforth/exceptic/internal/storage/store"
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

	// Projects live in the database; the ones in PROJECTS are created when missing and take their key from it at every start.
	registry, err := projects.Open(ctx, logger, db, settings.Projects)

	if err != nil {
		return nil, err
	}

	authorization := &oauth.Server{
		Name: settings.AppName, Store: db, PublicURL: settings.Notifications.PublicURL, TrustedProxies: settings.TrustedProxies, ResourcePath: api.MCPPath,
		Logger: logger,
	}
	notifier := notify.New(notify.Options{
		Store: db, Channels: options.Factory, Logger: logger,
		RetryFor: settings.Notifications.RetryFor, KeepSent: settings.Notifications.KeepSent, KeepFailed: settings.Notifications.KeepFailed,
	})
	sink := ingest.StoreSink{Logger: logger, Notifier: notifier, PublicURL: settings.Notifications.PublicURL, Recording: settings.Recording}
	pipeline := ingest.NewPipeline(logger, settings, registry, db, sink)
	pipeline.MaxInboxBytes = config.MaxInboxBytes(settings.Retention.MaxDatabaseBytes)

	if err := noticeStorage(db, registry, notifier, pipeline.Dropped(), logger, settings.Notifications.PublicURL); err != nil {
		_ = notifier.Close(ctx)

		return nil, err
	}

	all := services.New(services.Dependencies{
		Logger: logger, Store: db, Registry: registry, Settings: settings, Factory: options.Factory, Notifier: notifier, OAuth: authorization,
		Dropped: pipeline.Dropped(),
	})

	// Nothing creates users at start: the first admin is set up on the login page.
	if err := all.Users.LogSetup(ctx, settings.AdminPassword); err != nil {
		_ = notifier.Close(ctx)

		return nil, err
	}

	// One login for the web app: its attempt limits live in memory.
	auth := session.NewAuth(logger, settings, all.Users, all.Sessions)

	// A nil *web.App must not become a non-nil server.Routes.
	var spa server.Routes

	if options.Web != nil {
		spa = options.Web
	}

	app := &App{Registry: registry, OAuth: authorization, Services: all, options: options, notifier: notifier, pipeline: pipeline}

	// One catalog of requests, served as the REST API, as MCP tools and to the web app.
	endpoints := api.Endpoints(registry, all)
	plainRequests := api.LoginRequests(logger, auth, settings.AppName)
	maps.Copy(plainRequests, api.AgentRequests(logger, auth, all.Agents))
	app.Handler = server.New(server.Options{
		Logger:         logger,
		Database:       db,
		Ingest:         app.pipeline.Handler(),
		API:            api.NewREST(logger, endpoints, settings.Notifications.PublicURL, settings.TrustedProxies),
		MCP:            api.NewMCP(logger, db, registry, endpoints, authorization, settings.AppName, options.Version),
		OAuth:          authorization,
		Web:            spa,
		SignIn:         auth,
		UIAPI:          api.NewUI(logger, endpoints, plainRequests, auth, settings.Notifications.PublicURL, settings.TrustedProxies),
		Unsubscribe:    api.Unsubscribe{Logger: logger, AppName: settings.AppName, Connections: all.Connections, Unsubscriber: options.Factory.Unsubscribe},
		APIToken:       settings.APIToken,
		AllowedIPs:     settings.AllowedIPs,
		TrustedProxies: settings.TrustedProxies,
	}).Handler()

	return app, nil
}

// noticeStorage sends the connections of a project a notice at its first
// dropped data in a day, and at every cleanup run done that deleted its data.
func noticeStorage(db *database.Store, registry *projects.Registry, notifier *notify.Notifier, dropped *ingest.DropCounter, logger *slog.Logger, publicURL string) error {
	page := func(path string) string {
		if publicURL == "" {
			return ""
		}

		return publicURL + path
	}

	noticed, err := db.DropNotices()

	if err != nil {
		return err
	}

	dropped.OnFirstDrop(noticed, func(project config.Project, kind, reason string, count int64, at time.Time) {
		if err := db.MarkDropNotice(project.ID, at); err != nil {
			logger.Error("record the notice of dropped data", "project", project.Name, "err", err)
		}

		project.Color = cmp.Or(project.Color, config.DefaultProjectColor(project.ID))
		notifier.Notice([]int{project.ID}, notify.DroppedNotice(project, kind, reason, count, page("/projects")))
	})

	stepNames := map[int]string{}

	for index, step := range database.CleanupSteps() {
		stepNames[index+1] = step.Short
	}

	db.OnCleanupDone(func(run database.CleanupRun) {
		notifier.NoticeWithin(notify.RunProjects(run), cleanupNotice(run, registry.List(), stepNames, page("/projects/cleanups")))
	})

	return nil
}

// cleanupNotice writes the notice of a run as an access sees it, the way the
// Cleanup history shows it (services.VisibleRun): only the rows of its projects.
func cleanupNotice(run database.CleanupRun, projectList config.Projects, stepNames map[int]string, historyURL string) func(store.Access) (notify.Notice, bool) {
	byID := map[int]config.Project{}

	for _, project := range projectList {
		project.Color = cmp.Or(project.Color, config.DefaultProjectColor(project.ID))
		byID[project.ID] = project
	}

	scope := "all projects"

	if run.Project != database.AllProjects {
		scope = fmt.Sprintf("%s, down to its disk share of %d%%", byID[run.Project].Name, run.Share)
	}

	return func(access store.Access) (notify.Notice, bool) {
		shown, ok := services.VisibleRun(access, run)

		if !ok {
			return notify.Notice{}, false
		}

		return notify.CleanupNotice(shown.Run, byID, stepNames, scope, historyURL), true
	}
}
