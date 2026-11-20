// Package web serves the single-page app: the SvelteKit project in this
// folder, built into dist/spa (make web) and embedded in the binary.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"errors"
	"io/fs"
	"net/http"
	"regexp"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Base is where the app is served (paths.base in vite.config.ts): the
// server-rendered UI keeps the root until the app replaces it.
const Base = "/next"

const (
	// fallbackPage is the page of every address (adapter-static's fallback).
	fallbackPage = "200.html"
	// immutableDir holds files whose names carry a hash of their content.
	immutableDir = "_app/immutable/"
	// appDir holds only built files: a missing one is a 404, not the page.
	appDir = "_app/"
)

var ErrNotBuilt = errors.New("the web app is not built (make web)")

type App struct {
	files  fs.FS
	page   []byte
	policy string
}

// New serves the embedded build.
func New() (*App, error) {
	files, err := fs.Sub(dist, "dist/spa")

	if err != nil {
		return nil, err
	}

	return Load(files)
}

// Load serves the build in files, the contents of dist/spa.
func Load(files fs.FS) (*App, error) {
	page, err := fs.ReadFile(files, fallbackPage)

	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotBuilt
	}

	if err != nil {
		return nil, err
	}

	return &App{files: files, page: page, policy: contentSecurityPolicy(page)}, nil
}

// Register adds the app's routes. protect is the IP allowlist; the page holds
// no data, the requests it makes check the login.
func (app *App) Register(router *http.ServeMux, protect func(http.Handler) http.Handler) {
	router.Handle("GET "+Base+"/", protect(http.HandlerFunc(app.serve)))
}

func (app *App) serve(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Content-Security-Policy", app.policy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "same-origin")
	header.Set("X-Frame-Options", "DENY")

	name := strings.TrimPrefix(r.URL.Path, Base+"/")

	if name != fallbackPage && app.isFile(name) {
		if strings.HasPrefix(name, immutableDir) {
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			header.Set("Cache-Control", "no-cache")
		}

		http.ServeFileFS(w, r, app.files, name)

		return
	}

	if strings.HasPrefix(name, appDir) {
		http.NotFound(w, r)

		return
	}

	// The page can show data once it has loaded it, so no cache keeps it.
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Type", "text/html; charset=utf-8")

	_, _ = w.Write(app.page)
}

func (app *App) isFile(name string) bool {
	if !fs.ValidPath(name) {
		return false
	}

	info, err := fs.Stat(app.files, name)

	return err == nil && !info.IsDir()
}

var inlineScript = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script>`)

// contentSecurityPolicy allows scripts and styles only from the server, and
// the page's own inline scripts (SvelteKit starts the app with one) by hash.
func contentSecurityPolicy(page []byte) string {
	scripts := "'self'"

	for _, match := range inlineScript.FindAllSubmatch(page, -1) {
		if strings.Contains(strings.ToLower(string(match[1])), "src=") {
			continue
		}

		sum := sha256.Sum256(match[2])
		scripts += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}

	return "default-src 'self'; script-src " + scripts +
		"; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; form-action 'self'; base-uri 'none'"
}
