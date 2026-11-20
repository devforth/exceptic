package fixtures

import (
	"testing"
	"testing/fstest"

	"github.com/devforth/exceptic/web"
)

// WebPage marks the stand-in page of the web app.
const WebPage = "exceptic web app"

// WebApp is a stand-in build of the web app (web/), so full-server tests
// serve the same addresses whether the real one is built or not.
func WebApp(t testing.TB) *web.App {
	t.Helper()

	app, err := web.Load(fstest.MapFS{"200.html": {Data: []byte("<!doctype html><title>" + WebPage + "</title><script>start()</script>")}})

	if err != nil {
		t.Fatal(err)
	}

	return app
}
