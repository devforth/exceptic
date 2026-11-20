package web

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
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

	for _, path := range []string{Base + "/", Base + "/issues/k3x9q2mf", Base + "/" + fallbackPage} {
		response := get(handler, path)

		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), testScript) {
			t.Fatalf("%s: %d %q", path, response.Code, response.Body.String())
		}

		if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
			t.Errorf("%s: Cache-Control %q", path, cache)
		}
	}
}

func TestPolicyAllowsOnlyInlineScriptsOfPage(t *testing.T) {
	sum := sha256.Sum256([]byte(testScript))
	policy := get(testApp(t), Base+"/").Header().Get("Content-Security-Policy")
	want := "script-src 'self' 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "';"

	if !strings.Contains(policy, want) || !strings.Contains(policy, "style-src 'self';") {
		t.Fatalf("policy %q, want %q", policy, want)
	}
}

func TestServesBuiltFiles(t *testing.T) {
	handler := testApp(t)

	cases := map[string]string{
		Base + "/_app/immutable/entry/start.js": "public, max-age=31536000, immutable",
		Base + "/_app/version.json":             "no-cache",
	}

	for path, cache := range cases {
		response := get(handler, path)

		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != cache {
			t.Errorf("%s: %d, Cache-Control %q", path, response.Code, response.Header().Get("Cache-Control"))
		}
	}

	if response := get(handler, Base+"/_app/immutable/entry/gone.js"); response.Code != http.StatusNotFound {
		t.Errorf("missing file: %d", response.Code)
	}

	if response := get(handler, Base+"/_app/immutable/"); response.Code != http.StatusNotFound {
		t.Errorf("directory: %d", response.Code)
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

	if !strings.Contains(page, `"`+Base+"/"+immutableDir) {
		t.Errorf("the page does not load the app from %s (paths.base in vite.config.ts)", Base)
	}

	if count := len(inlineScript.FindAllStringSubmatch(page, -1)); count != 1 {
		t.Errorf("%d scripts in the page, want SvelteKit's one", count)
	}

	if strings.Contains(page, "style=") || strings.Contains(page, "<style") {
		t.Error("inline style in the page: the CSP blocks it")
	}
}
