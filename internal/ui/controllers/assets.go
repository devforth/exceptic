package controllers

import (
	"net/http"

	"github.com/devforth/exceptic/internal/ui/middleware"
	"github.com/devforth/exceptic/internal/ui/views"
)

// ProjectColors serves the colors of project badges. Inline styles are
// blocked by the CSP, so colors chosen in the UI come as a stylesheet; its
// URL carries a version of the content, so it can be cached for good.
func (controller *Controllers) ProjectColors(w http.ResponseWriter, r *http.Request) {
	colors := views.ProjectStylesheet(controller.projects.List())

	w.Header().Set("Content-Type", "text/css; charset=utf-8")

	// Private: the stylesheet names the projects, so no shared cache may keep it.
	if r.URL.Query().Get("v") == colors.Version {
		w.Header().Set("Cache-Control", "private, "+middleware.CacheVersioned)
	} else {
		w.Header().Set("Cache-Control", middleware.CacheUnversioned)
	}

	_, _ = w.Write([]byte(colors.CSS))
}
