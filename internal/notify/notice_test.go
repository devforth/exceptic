package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/platform/config"
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
	projects := map[int]config.Project{1: {ID: 1, Name: "backend", Color: "#2563eb"}, 2: {ID: 2, Name: "frontend", Color: "#db2777"}}
	notice := CleanupNotice(run, projects, map[int]string{5: "Logs below error", 6: "Events below error"}, "all projects", "https://errors.example.com/projects/cleanups")
	text := notice.Heading + "\n" + strings.Join(notice.Lines, "\n") + "\n" + notice.Table.text(chatTableWidth)
	want := `Cleanup done: all projects
2.0 GB → 1.5 GB of 2.0 GB, 29s
Step  Project   Deleted
5     frontend  25,458 logs
6     backend   1,302 events
6     frontend  231 events`

	if text != want {
		t.Errorf("cleanup notice:\n%s\nwant:\n%s", text, want)
	}

	if projects := RunProjects(run); len(projects) != 2 || projects[0] != 1 || projects[1] != 2 {
		t.Errorf("run projects %v", projects)
	}

	dropped := DroppedNotice(config.Project{ID: 2, Name: "frontend"}, "logs", "rate limit", 1200, "")

	if dropped.Heading != "frontend: data dropped" || dropped.Table.text(chatTableWidth) != "What  Why         Count\nlogs  rate limit  1,200" {
		t.Errorf("dropped notice %+v", dropped)
	}
}

// A table wider than its width narrows its widest column and wraps the
// cell onto more lines, the other columns padded alike; rows of several
// lines are told apart by an empty line.
func TestNoticeTableWraps(t *testing.T) {
	table := NoticeTable{Columns: []string{"Step", "Project", "Deleted"}, Rows: [][]NoticeCell{
		{{Text: "5"}, {Text: "a project with a very long name"}, {Text: "25,458 logs"}},
		{{Text: "6"}, {Text: "backend"}, {Text: "1,302 events"}},
	}}
	want := `Step  Project               Deleted
5     a project with a      25,458 logs
      very long name

6     backend               1,302 events`

	if got := table.text(chatTableWidth); got != want {
		t.Errorf("table:\n%s\nwant:\n%s", got, want)
	}

	for line := range strings.SplitSeq(table.text(chatTableWidth), "\n") {
		if len([]rune(line)) > chatTableWidth {
			t.Errorf("line over %d characters: %q", chatTableWidth, line)
		}
	}
}

// A notice whose table is longer than a Telegram message keeps the rows that
// fit and says how many more there were, so Telegram does not refuse it.
func TestTelegramNoticeFitsAMessage(t *testing.T) {
	server, _, payload := recordingServer(t, http.StatusOK, nil, `{"ok":true}`)
	table := &NoticeTable{Columns: []string{"Step", "Project", "Deleted"}}

	for number := range 300 {
		table.Rows = append(table.Rows, []NoticeCell{{Text: "5"}, {Text: fmt.Sprintf("project %d with a long name", number)}, {Text: "25,458 logs, 1,302 events"}})
	}

	notice := Notice{Heading: "Cleanup done: all projects", Lines: []string{"2.0 GB → 1.5 GB of 2.0 GB, 29s"}, Table: table, URL: "https://errors.example.com/projects/cleanups", LinkText: "Open Cleanup history"}

	if err := (Telegram{BotToken: "1:token", ChatID: "-100", apiURL: server.URL}).SendNotice(context.Background(), notice); err != nil {
		t.Fatal(err)
	}

	var message struct{ Text string }

	if err := json.Unmarshal(*payload, &message); err != nil {
		t.Fatal(err)
	}

	shown := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html.UnescapeString(message.Text), "")

	if length := utf8.RuneCountInString(shown); length > telegramTextLimit || !strings.Contains(shown, "project 0 with") || !regexp.MustCompile(`…and \d+ more rows`).MatchString(shown) || !strings.Contains(shown, "Open Cleanup history") {
		t.Errorf("%d characters:\n%s", length, shown)
	}
}

// The delivery log reads a notice as its label and heading, not as an issue.
func TestDescribeNotice(t *testing.T) {
	payload, _ := json.Marshal(DroppedNotice(config.Project{ID: 2, Name: "frontend"}, "logs", "rate limit", 3, ""))

	if got := Describe(kindNotice, payload); got.Label != "Data dropped" || got.Title != "frontend: data dropped" || got.IssueID != "" {
		t.Errorf("notice described as %+v", got)
	}
}

// A notice built per connection is written from the projects each covers;
// one covering none of its projects gets nothing, and its message is about
// the projects the connection covers only.
func TestNoticeWithinEachConnection(t *testing.T) {
	setup := newSetup(t)

	backend, frontend, other, everything := &fakeChannel{}, &fakeChannel{}, &fakeChannel{}, &fakeChannel{}

	setup.connect(backend, []int{1})
	setup.connect(frontend, []int{2})
	setup.connect(other, []int{3})
	setup.connect(everything, nil)

	notifier := setup.start(Options{})
	names := map[int]string{1: "backend", 2: "frontend"}

	notifier.NoticeWithin([]int{1, 2}, func(access store.Access) (Notice, bool) {
		var shown []string

		for _, id := range []int{1, 2} {
			if access.Sees(id) {
				shown = append(shown, names[id])
			}
		}

		return Notice{Heading: strings.Join(shown, ", ")}, len(shown) > 0
	})

	closeNotifier(t, notifier)

	for channel, want := range map[*fakeChannel]string{backend: "backend", frontend: "frontend", everything: "backend, frontend"} {
		if channel.noticeCount() != 1 || channel.notices[0].Heading != want {
			t.Errorf("want %q, got %+v", want, channel.notices)
		}
	}

	if other.noticeCount() != 0 {
		t.Errorf("a connection of another project got %+v", other.notices)
	}

	messages, err := setup.database.ListOutbox(context.Background(), store.OutboxFilter{})

	if err != nil {
		t.Fatal(err)
	}

	projectsOf := map[string][]int{"backend": {1}, "frontend": {2}, "backend, frontend": {1, 2}}

	for _, message := range messages {
		var notice Notice

		if err := json.Unmarshal(message.Payload, &notice); err != nil || !slices.Equal(message.ProjectIDs, projectsOf[notice.Heading]) {
			t.Errorf("message %q is about %v", notice.Heading, message.ProjectIDs)
		}
	}
}

// A notice built per access is built once for the connections that see the
// same of its projects, however many they are.
func TestNoticeWithinBuildsOncePerAccess(t *testing.T) {
	setup := newSetup(t)
	channels := []*fakeChannel{{}, {}, {}, {}, {}}

	setup.connect(channels[0], []int{1})
	setup.connect(channels[1], []int{1, 3}) // sees the same of the notice's projects: 1
	setup.connect(channels[2], nil)
	setup.connect(channels[3], nil)
	setup.connect(channels[4], []int{1, 2})

	notifier := setup.start(Options{})

	var builds atomic.Int32

	notifier.NoticeWithin([]int{1, 2}, func(access store.Access) (Notice, bool) {
		builds.Add(1)

		return Notice{Heading: fmt.Sprint(access.All())}, true
	})

	closeNotifier(t, notifier)

	// Three: project 1, every project, and projects 1 and 2 (all of the notice's, but not every project).
	if builds.Load() != 3 {
		t.Errorf("built %d times for 5 connections, want 3", builds.Load())
	}

	for index, channel := range channels {
		if channel.noticeCount() != 1 {
			t.Errorf("connection %d got %d notices", index, channel.noticeCount())
		}
	}
}
