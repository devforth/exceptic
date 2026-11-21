package controllers

import "net/http"

// Logout is the menu's Log out of the old pages; the web app has its own.
func (controller *Controllers) Logout(w http.ResponseWriter, r *http.Request) {
	controller.auth.EndSession(r.Context(), w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
