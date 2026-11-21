// Package routes is the route table of the UI: each pattern mapped to a
// controller, wrapped in the middleware it needs.
package routes

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/session"
	"github.com/devforth/exceptic/internal/ui/controllers"
	"github.com/devforth/exceptic/internal/ui/middleware"
	"github.com/devforth/exceptic/internal/ui/views"
)

// AccountPath is the web app's account page, the only page open with a
// temporary password: the old pages send such a user there.
const AccountPath = "/account"

// Register adds the UI routes. protect is the IP allowlist.
func Register(router *http.ServeMux, protect func(http.Handler) http.Handler, controller *controllers.Controllers, auth *session.Auth, trustedProxies []netip.Prefix) {
	registrar := registrar{router: router, protect: protect, sameOrigin: middleware.SameOrigin(trustedProxies)}

	// Open to anyone who passes the IP allowlist: the old pages' Log out (the
	// login and the account page are the web app's).
	public := group{
		"POST /logout": controller.Logout,
	}

	// The dashboard: every route needs a login and the user's own password; any role.
	dashboard := group{
		"GET /{$}": controller.Home, // to the web app's issue list

		"GET " + services.ProjectColorsPath: controller.ProjectColors, // names the projects
	}

	// Settings only admins (and the superadmin) may change.
	admin := group{
	}

	login, passwordChanged := middleware.RequireLogin(auth), middleware.RequirePasswordChanged(AccountPath)

	registrar.add(public)
	registrar.add(dashboard, login, passwordChanged)
	registrar.add(admin, login, passwordChanged, middleware.RequireAdmin(controller.NotFoundPage))

	static := http.StripPrefix("/static/", middleware.StaticCache(http.FileServerFS(views.Static())))
	router.Handle("GET /static/", protect(middleware.SecurityHeaders(static)))
}

// group is a set of routes that share their middleware.
type group map[string]http.HandlerFunc

// registrar adds groups of routes with the protection every route gets.
type registrar struct {
	router     *http.ServeMux
	protect    func(http.Handler) http.Handler // the IP allowlist
	sameOrigin func(http.HandlerFunc) http.HandlerFunc
}

// add registers the routes, each wrapped in wrap (outermost first). Every
// POST also gets the same-origin check (CSRF), inside the others: a group
// cannot forget it.
func (registrar registrar) add(routes group, wrap ...func(http.HandlerFunc) http.HandlerFunc) {
	for pattern, route := range routes {
		if strings.HasPrefix(pattern, http.MethodPost+" ") {
			route = registrar.sameOrigin(route)
		}

		for index := len(wrap) - 1; index >= 0; index-- {
			route = wrap[index](route)
		}

		registrar.router.Handle(pattern, registrar.protect(middleware.SecurityHeaders(route)))
	}
}
