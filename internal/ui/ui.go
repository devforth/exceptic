// Package ui is the composition root of the web interface (server-rendered
// HTML with htmx); the code lives in its sub-packages.
package ui

import (
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/ui/controllers"
	"github.com/devforth/exceptic/internal/ui/requests"
	"github.com/devforth/exceptic/internal/ui/routes"
	"github.com/devforth/exceptic/internal/ui/views"
)

type Handler struct {
	controllers    *controllers.Controllers
	auth           *session.Auth
	trustedProxies []netip.Prefix
}

// New builds the UI on the services every part shares (services.New) and
// the login (session.NewAuth), both built once in app.New.
func New(logger *slog.Logger, settings *config.Config, all *services.All, auth *session.Auth) (*Handler, error) {
	renderer, err := views.New(logger, all.Projects, all.Issues, requests.Viewer, settings.AppName)

	if err != nil {
		return nil, err
	}

	return &Handler{
		controllers: controllers.New(controllers.Dependencies{
			Logger: logger, Settings: settings, Views: renderer, Auth: auth, Services: all,
		}),
		auth: auth, trustedProxies: settings.TrustedProxies,
	}, nil
}

// Register adds the UI routes. protect is the IP allowlist.
func (handler *Handler) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	routes.Register(router, protect, handler.controllers, handler.auth, handler.trustedProxies)
}
