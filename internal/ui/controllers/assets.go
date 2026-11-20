package controllers

import (
	"net/http"

	"github.com/devforth/exceptic/internal/ui/middleware"
)

// ProjectColors serves the colors of project badges. Inline styles are
// blocked by the CSP, so colors chosen in the UI come as a stylesheet; its
// URL carries a version of the content, so a change is fetched at once.
func (controller *Controllers) ProjectColors(w http.ResponseWriter, _ *http.Request) {
	colors := controller.projects.Colors()

	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	// It names the projects: no shared cache may keep it, and the browser
	// asks again each time, so it does not outlive a logout.
	w.Header().Set("Cache-Control", "private, "+middleware.CacheUnversioned)

	_, _ = w.Write([]byte(colors.CSS))
}
