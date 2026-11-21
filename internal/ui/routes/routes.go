// Package routes is the route table of the UI: each pattern mapped to a
// controller, wrapped in the middleware it needs.
package routes

import (
	"net/http"
	"net/netip"
	"strings"

	"github.com/devforth/exceptic/internal/oauth"
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
		"GET /{$}":                      controller.Home,
		"GET /issues":                   controller.IssuesPage,
		"GET /issues/{issueID}":         controller.IssuePage,
		"POST /issues/{issueID}/action": controller.IssueAction, // deleting needs an admin (services.Issues)
		"POST /issues/bulk":             controller.BulkIssueAction,
		"GET /projects/cleanups":        controller.CleanupsPage,

		"GET /agents":                       controller.AgentsPage,
		"POST /agents/token":                controller.NewAgentToken,
		"POST /agents/{grantID}/disconnect": controller.DisconnectAgent, // someone else's needs an admin (services.Agents)
		"GET " + oauth.AuthorizePath:        controller.AuthorizePage,
		"POST " + oauth.AuthorizePath:       controller.Authorize,

		"GET " + services.ProjectColorsPath: controller.ProjectColors, // names the projects
	}

	// Settings only admins (and the superadmin) may change.
	admin := group{
		"GET /projects/new":                 controller.NewProjectPage,
		"POST /projects":                    controller.CreateProject,
		"GET /projects/{projectID}":         controller.ProjectPage,
		"POST /projects/{projectID}":        controller.UpdateProject,
		"POST /projects/{projectID}/delete": controller.DeleteProject,

		"GET /notifications":                           controller.ConnectionsPage,
		"GET /notifications/log":                       controller.DeliveriesPage,
		"GET /notifications/new":                       controller.NewConnectionPage,
		"POST /notifications":                          controller.CreateConnection,
		"GET /notifications/{connectionID}":            controller.ConnectionPage,
		"POST /notifications/{connectionID}":           controller.UpdateConnection,
		"POST /notifications/email/start":              controller.EmailStart,
		"POST /notifications/email/resend":             controller.EmailResend,
		"POST /notifications/email/confirm":            controller.EmailConfirm,
		"POST /notifications/email/change":             controller.EmailChange,
		"POST /notifications/{connectionID}/test":      controller.TestConnection,
		"POST /notifications/test-chat":                controller.TestChat,
		"POST /notifications/{connectionID}/test-chat": controller.TestChat,
		"POST /notifications/{connectionID}/retry":     controller.RetryConnection,
		"POST /notifications/{connectionID}/active":    controller.SetConnectionActive,
		"POST /notifications/{connectionID}/delete":    controller.DeleteConnection,
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
