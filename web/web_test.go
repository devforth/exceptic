package web

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const testScript = `console.log("start")`

func testApp(t *testing.T) http.Handler {
	t.Helper()

	app, err := Load(fstest.MapFS{
		fallbackPage:                    {Data: []byte(`<html><script src="/x.js"></script><script>` + testScript + `</script></html>`)},
		"_app/immutable/entry/start.js": {Data: []byte(`export {}`)},
		"_app/version.json":             {Data: []byte(`{}`)},
	})

	if err != nil {
		t.Fatal(err)
	}

	router := http.NewServeMux()
	app.Register(router, func(next http.Handler) http.Handler { return next })

	return router
}

func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

	return recorder
}

func TestServesPageForEveryAddress(t *testing.T) {
	handler := testApp(t)

	for _, path := range []string{"/", "/issues/k3x9q2mf", "/" + fallbackPage} {
		response := get(handler, path)

		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), testScript) {
			t.Fatalf("%s: %d %q", path, response.Code, response.Body.String())
		}

		if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
			t.Errorf("%s: Cache-Control %q", path, cache)
		}

		// The Origin check of the app's requests needs it: no-referrer makes browsers send Origin: null.
		if policy := response.Header().Get("Referrer-Policy"); policy != "same-origin" {
			t.Errorf("%s: Referrer-Policy %q", path, policy)
		}
	}
}

func TestPolicyAllowsOnlyInlineScriptsOfPage(t *testing.T) {
	sum := sha256.Sum256([]byte(testScript))
	policy := get(testApp(t), "/").Header().Get("Content-Security-Policy")
	want := "script-src 'self' 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"

	if !strings.Contains(policy, want) || !strings.Contains(policy, "style-src 'self';") {
		t.Fatalf("policy %q, want %q", policy, want)
	}
}

func TestServesBuiltFiles(t *testing.T) {
	handler := testApp(t)

	cases := map[string]string{
		"/_app/immutable/entry/start.js": "public, max-age=31536000, immutable",
		"/_app/version.json":             "no-cache",
	}

	for path, cache := range cases {
		response := get(handler, path)

		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != cache {
			t.Errorf("%s: %d, Cache-Control %q", path, response.Code, response.Header().Get("Cache-Control"))
		}
	}

	if response := get(handler, "/_app/immutable/entry/gone.js"); response.Code != http.StatusNotFound {
		t.Errorf("missing file: %d", response.Code)
	}

	if response := get(handler, "/_app/immutable/"); response.Code != http.StatusNotFound {
		t.Errorf("directory: %d", response.Code)
	}

	// Pages only: a form sent to an address no route takes is unknown.
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/projects", nil))

	if recorder.Code != http.StatusNotFound {
		t.Errorf("POST: %d", recorder.Code)
	}
}

func TestNotBuilt(t *testing.T) {
	if _, err := Load(fstest.MapFS{}); !errors.Is(err, ErrNotBuilt) {
		t.Fatalf("err %v", err)
	}
}

// The real build, when there is one (CI builds it before this test).
func TestEmbeddedBuild(t *testing.T) {
	app, err := New()

	if errors.Is(err, ErrNotBuilt) {
		t.Skip(err)
	}

	if err != nil {
		t.Fatal(err)
	}

	page := string(app.page)

	// From the root: the page is served at every address, so a relative path would break.
	if !strings.Contains(page, `"/`+immutableDir) {
		t.Error("the page does not load the app from /" + immutableDir)
	}

	// The theme's (app.html, before the first paint) and SvelteKit's, each allowed by its hash.
	scripts := inlineScript.FindAllStringSubmatch(page, -1)

	if len(scripts) != 2 {
		t.Errorf("%d scripts in the page, want the theme's and SvelteKit's", len(scripts))
	}

	for _, script := range scripts {
		sum := sha256.Sum256([]byte(script[2]))

		if hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"; !strings.Contains(app.policy, hash) {
			t.Errorf("the policy %q lacks the hash of the script %q", app.policy, script[2])
		}
	}

	if strings.Contains(page, "style=") || strings.Contains(page, "<style") {
		t.Error("inline style in the page: the CSP blocks it")
	}

	err = fs.WalkDir(app.files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		data, err := fs.ReadFile(app.files, name)

		if err != nil {
			return err
		}

		if strings.HasPrefix(name, appDir) && strings.Contains(string(data), `style="`) {
			t.Errorf("%s: a style attribute in markup, which the CSP blocks", name)
		}

		// SvelteKit's own error page has a <style>; src/error.html replaces it.
		if strings.Contains(string(data), "<style") {
			t.Errorf("%s: a <style> element, which the CSP blocks", name)
		}

		if strings.Contains(string(data), designSystemMarker) || strings.Contains(name, "DesignSystem") {
			t.Errorf("%s: the design-system page is for the dev server only", name)
		}

		return nil
	})

	if err != nil {
		t.Fatal(err)
	}
}

// The id of the dev-only component catalog (src/lib/design-system/DesignSystem.svelte).
const designSystemMarker = "exceptic-design-system"
