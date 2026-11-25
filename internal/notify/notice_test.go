package notify

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// A notice goes to the active connections that cover one of its projects,
// whatever levels they selected; one about no project only to those for
// every project.
func TestNoticeRouting(t *testing.T) {
	setup := newSetup(t)

	backendWarnings := &fakeChannel{}
	frontendErrors := &fakeChannel{}
	everything := &fakeChannel{}
	disabled := &fakeChannel{}

	setup.connect(backendWarnings, []int{1}, "warning")
	setup.connect(frontendErrors, []int{2}, "error")
	setup.connect(everything, nil, "fatal")

	disabledID := setup.connect(disabled, []int{1}, "error")
	connection, _ := setup.database.GetConnection(context.Background(), disabledID)
	connection.Active = false

	if err := setup.database.UpdateConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}

	notifier := setup.start(Options{})

	notifier.Notice([]int{1}, Notice{Heading: "backend"})
	notifier.Notice(nil, Notice{Heading: "server"})

	closeNotifier(t, notifier)

	headings := func(channel *fakeChannel) string {
		var result []string

		for _, notice := range channel.notices {
			result = append(result, notice.Heading)
		}

		return strings.Join(result, ", ")
	}

	if got := headings(backendWarnings); got != "backend" {
		t.Errorf("project 1, warnings: %q", got)
	}

	if got := headings(frontendErrors); got != "" {
		t.Errorf("project 2: %q", got)
	}

	if got := headings(everything); got != "backend, server" {
		t.Errorf("all projects, fatal only: %q", got)
	}

	if disabled.noticeCount() != 0 {
		t.Errorf("a disabled connection got %q", headings(disabled))
	}
}

// A cleanup notice names its scope, the data before and after and what each
// step deleted per project; a dropped notice the count, kind and reason.
func TestNoticeTexts(t *testing.T) {
	run := store.CleanupRun{
		SizeBefore: 2 << 30, SizeAfter: 3 << 29, Limit: 2 << 30, Duration: 29 * time.Second,
		Steps: []store.CleanupStep{
			{Number: 5, Name: "logs below error, oldest first", Projects: map[int]*store.CleanupCounts{2: {Logs: 25458}}},
			{Number: 6, Name: "events below error but the latest of their issue", Projects: map[int]*store.CleanupCounts{1: {Events: 1302}, 2: {Events: 231}}},
		},
	}
	notice := CleanupNotice(run, map[int]string{1: "backend", 2: "frontend"}, "all projects", "https://errors.example.com/projects/cleanups")
	text := notice.Heading + "\n" + strings.Join(notice.Lines, "\n") + "\n" + notice.Table.text()
	want := `Cleanup done: all projects
2.0 GB → 1.5 GB of 2.0 GB, 29s
Step                                                 Project   Deleted
5. logs below error, oldest first                    frontend  25,458 logs
6. events below error but the latest of their issue  backend   1,302 events
6. events below error but the latest of their issue  frontend  231 events`

	if text != want {
		t.Errorf("cleanup notice:\n%s\nwant:\n%s", text, want)
	}

	if projects := RunProjects(run); len(projects) != 2 || projects[0] != 1 || projects[1] != 2 {
		t.Errorf("run projects %v", projects)
	}

	dropped := DroppedNotice("frontend", "logs", "rate limit", 1200, "")

	if dropped.Heading != "frontend: data dropped" || dropped.Table.text() != "What  Why         Count\nlogs  rate limit  1,200" {
		t.Errorf("dropped notice %+v", dropped)
	}
}
