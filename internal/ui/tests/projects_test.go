package tests

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// setColor saves the color of project 1 as its settings page does; it
// returns the problems of a refused color.
func (server *testServer) setColor(color string) map[string]string {
	server.t.Helper()

	ctx := context.Background()
	projects := server.app.Services.Projects
	project, err := projects.Get(ctx, 1)

	if err != nil {
		server.t.Fatal(err)
	}

	form := projects.FormValues(project)
	form.Set("color", color)

	_, problems, err := projects.Save(ctx, form, project)

	if err != nil {
		server.t.Fatal(err)
	}

	return problems
}

// A project color comes as a stylesheet (the CSP blocks inline styles): the
// palette color until one is chosen, then the chosen one.
func TestProjectColor(t *testing.T) {
	server := newTestServer(t, password)

	// The stylesheet names the projects: only for logged-in users.
	if response := server.get("/project-colors.css"); response.Code != http.StatusSeeOther {
		t.Fatalf("anonymous: %d", response.Code)
	}

	server.login()

	stylesheet := regexp.MustCompile(`href="(/project-colors\.css\?v=[0-9a-f]+)"`)
	page := server.page("/issues")
	before := stylesheet.FindStringSubmatch(page)

	if before == nil {
		t.Fatal("pages must link the project colors")
	}

	if css := server.page(before[1]); !strings.Contains(css, `[data-project="backend"] { --project-color: #2563eb; }`) {
		t.Fatalf("palette color:\n%s", css)
	}

	// By the browser only, asked again each time: a shared cache would hand
	// the project names to anyone, and a kept copy would outlive the logout.
	if cache := server.get(before[1]).Header().Get("Cache-Control"); cache != "private, no-cache" {
		t.Errorf("Cache-Control = %q, want private, no-cache", cache)
	}

	if problems := server.setColor("#ABCDEF"); len(problems) > 0 {
		t.Fatalf("set color: %v", problems)
	}

	after := stylesheet.FindStringSubmatch(server.page("/issues"))

	if after[1] == before[1] {
		t.Fatal("the stylesheet URL must change with the colors, or browsers keep the old one")
	}

	if css := server.page(after[1]); !strings.Contains(css, `[data-project="backend"] { --project-color: #abcdef; }`) {
		t.Fatalf("chosen color:\n%s", css)
	}

	if problems := server.setColor("red; background: url(x)"); problems["color"] == "" {
		t.Fatalf("invalid color accepted: %v", problems)
	}
}
