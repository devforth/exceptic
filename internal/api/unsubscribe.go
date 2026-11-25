package api

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
)

// unsubscribeStyle is the one style of the unsubscribe page, allowed by its hash.
const unsubscribeStyle = `:root{color-scheme:light dark}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#eeeef3;color:#1d1d22;font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif}
main{box-sizing:border-box;width:min(440px,calc(100% - 32px));padding:28px;background:#fff;border:1px solid #e4e4ea;border-radius:10px}
.app{margin:0 0 16px;font-weight:700;font-size:14px}
h1{margin:0;font-size:20px;line-height:1.3}
p{margin:8px 0 0;color:#64646f}
button{margin-top:20px;padding:10px 18px;border:0;border-radius:6px;background:#4f46e5;color:#fff;font:600 14px/1.2 inherit;cursor:pointer}
button:focus-visible{outline:2px solid #4f46e5;outline-offset:2px}
@media (prefers-color-scheme:dark){body{background:#121216;color:#e8e8ee}main{background:#1c1c22;border-color:#33333d}p{color:#a3a3ae}}`

var unsubscribePage = template.Must(template.New("unsubscribe").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Heading}} · {{.AppName}}</title><style>` + unsubscribeStyle + `</style></head>
<body><main>
<p class="app">{{.AppName}}</p>
<h1>{{.Heading}}</h1>
<p>{{.Text}}</p>
{{if .Confirm}}<form method="post"><button type="submit">Stop these emails</button></form>{{end}}
</main></body></html>`))

var unsubscribePolicy = func() string {
	sum := sha256.Sum256([]byte(unsubscribeStyle))

	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"
}()

type unsubscribeView struct {
	AppName, Heading, Text string
	Confirm                bool
}

// Unsubscribe serves the links that stop emails (notify.UnsubscribePath):
// GET shows a button, since mail scanners open every link, and POST, the
// button or a mail provider's own one-click request (RFC 8058), pauses the
// connections sending to the address. Open to everyone like ingest, since
// providers send the request from their servers: the signed token is the
// only key, and a wrong one tells nothing about the address.
type Unsubscribe struct {
	Logger       *slog.Logger
	AppName      string
	Connections  *services.Connections
	Unsubscriber notify.Unsubscriber
}

// Register mounts the page on router.
func (handler Unsubscribe) Register(router *http.ServeMux) {
	router.HandleFunc("GET "+notify.UnsubscribePath+"{destination}", handler.serve)
	router.HandleFunc("POST "+notify.UnsubscribePath+"{destination}", handler.serve)
}

func (handler Unsubscribe) serve(w http.ResponseWriter, r *http.Request) {
	destination := r.PathValue("destination")
	view := unsubscribeView{AppName: handler.AppName}
	status := http.StatusOK

	switch {
	case !store.ValidID(destination) || !handler.Unsubscriber.Valid(destination, r.URL.Query().Get("token")):
		status, view.Heading, view.Text = http.StatusNotFound, "This link does not work", "It may have been cut by your mail client. Open it again from the email, or change the connection on the Notifications page."
	case r.Method == http.MethodGet:
		view.Heading, view.Text, view.Confirm = "Stop these emails?", "The button pauses every notification to this address. An admin can resume them on the Notifications page.", true
	default:
		address, paused, err := handler.Connections.Unsubscribe(r.Context(), destination)

		switch {
		case errors.Is(err, store.ErrNotFound):
			status, view.Heading, view.Text = http.StatusNotFound, "Nothing to stop", "No notifications are sent to this address any more."
		case err != nil:
			handler.Logger.Error("unsubscribe", "destination", destination, "err", err)

			status, view.Heading, view.Text = http.StatusInternalServerError, "Not stopped", "The server could not pause the notifications. Try again in a moment."
		default:
			handler.Logger.Info("notifications to an address paused from its email", "address", address, "connections", paused)

			view.Heading, view.Text = "Emails stopped", "Notifications to "+address+" are paused. An admin can resume them on the Notifications page."
		}
	}

	server.SecurityHeaders(w.Header(), unsubscribePolicy)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	_ = unsubscribePage.Execute(w, view)
}
