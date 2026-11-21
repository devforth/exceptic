// Package controllers holds the HTTP handlers of the UI, one file per page or
// group of pages.
package controllers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/ui/lists"
	"github.com/devforth/exceptic/internal/ui/presenters"
	"github.com/devforth/exceptic/internal/ui/requests"
	"github.com/devforth/exceptic/internal/ui/views"
)

// Controllers holds what the handlers need.
type Controllers struct {
	logger       *slog.Logger
	retention    config.Retention
	views        *views.Renderer
	auth         *session.Auth
	issues       *services.Issues
	projects     *services.Projects
	connections  *services.Connections
	deliveries   *services.Deliveries
	verification *services.Verification
	logs         *services.Logs
	agents       *services.Agents
	users        *services.Users
	// For the MCP page: whether ALLOWED_IPS limits access.
	ipsRestricted bool
	appName       string // APP_NAME

	connectionViews presenters.Connections
}

// Dependencies are the parts Controllers is built from.
type Dependencies struct {
	Logger   *slog.Logger
	Settings *config.Config
	Views    *views.Renderer
	Auth     *session.Auth
	Services *services.All
}

func New(dependencies Dependencies) *Controllers {
	settings := dependencies.Settings
	all := dependencies.Services

	return &Controllers{
		logger:        dependencies.Logger,
		retention:     settings.Retention,
		views:         dependencies.Views,
		auth:          dependencies.Auth,
		issues:        all.Issues,
		projects:      all.Projects,
		connections:   all.Connections,
		deliveries:    all.Deliveries,
		verification:  all.Verification,
		logs:          all.Logs,
		agents:        all.Agents,
		users:         all.Users,
		ipsRestricted: len(settings.AllowedIPs) > 0,
		appName:       settings.AppName,
		connectionViews: presenters.Connections{
			Chat: all.Connections.Chat, Notifications: settings.Notifications, Recording: settings.Recording,
			Projects: all.Projects, ProjectName: all.Projects.Name,
		},
	}
}

// Home sends the site root to the issues.
func (controller *Controllers) Home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/issues", http.StatusSeeOther)
}

func (controller *Controllers) serverError(w http.ResponseWriter, r *http.Request, err error) {
	controller.views.ServerError(w, r, err)
}

// renderList renders a list page, or only the list for htmx (filters, page links).
func (controller *Controllers) renderList(w http.ResponseWriter, r *http.Request, page string, fragment lists.ListFragment, data any) {
	if requests.HTMXTarget(r) == fragment.Target {
		controller.views.RenderFragment(w, r, page, fragment.Target, data)

		return
	}

	controller.views.Render(w, r, http.StatusOK, page, data)
}

// NotFoundPage is the 404 page: for unknown addresses, missing records, and
// what the viewer's role does not allow, so it looks as if it did not exist.
// htmx requests get plain text: a page must not be swapped into a fragment.
func (controller *Controllers) NotFoundPage(w http.ResponseWriter, r *http.Request) {
	if requests.IsHTMX(r) {
		http.NotFound(w, r)

		return
	}

	controller.views.NotFound(w, r)
}

// pathID reads a record id (store.ValidID) from the path value name; a 404
// page otherwise (false).
func (controller *Controllers) pathID(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := r.PathValue(name)

	if !store.ValidID(id) {
		controller.NotFoundPage(w, r)

		return "", false
	}

	return id, true
}

// failed answers an error of a service call: 404 for a missing record or a
// change the role does not allow, 500 otherwise. False without an error.
func (controller *Controllers) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrNotFound), errors.Is(err, services.ErrForbidden):
		controller.NotFoundPage(w, r)
	default:
		controller.serverError(w, r, err)
	}

	return true
}

// redirectAfterPost sends the browser to target after a form POST. htmx
// would swap a redirect response into the page, so it gets HX-Location
// (a client-side navigation) instead.
func redirectAfterPost(w http.ResponseWriter, r *http.Request, target string) {
	if requests.IsHTMX(r) {
		w.Header().Set("HX-Location", target)

		return
	}

	http.Redirect(w, r, target, http.StatusSeeOther)
}
