package notify

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// kindNotice is an outbox message about the server's storage (Notice).
const kindNotice = "notice"

// Notice is a message about the server's storage, not an issue: data a
// project's SDKs sent was dropped, or a cleanup run is done. It goes to the
// connections that cover one of its projects, whatever their levels.
type Notice struct {
	Heading string
	Lines   []string
	// URL is the page of the web app that shows more; empty without PUBLIC_URL.
	URL      string
	LinkText string
}

// noticeFor is a notice and the projects it is about, on its way to the dispatcher.
type noticeFor struct {
	notice     Notice
	projectIDs []int
}

// Notice queues a notice about projects (none: only the connections for all
// projects get it). Like Notify it never blocks.
func (notifier *Notifier) Notice(projectIDs []int, notice Notice) {
	if notifier.options.Store == nil {
		return
	}

	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()

	if notifier.closed {
		return
	}

	select {
	case notifier.queue <- queued{notice: &noticeFor{notice: notice, projectIDs: slices.Clone(projectIDs)}}:
	default:
		notifier.options.Logger.Warn("notification queue full, dropping a notice", "heading", notice.Heading)
	}
}

// routeNotice writes a notice for every connection that covers one of its
// projects; notices are few (a day's first drop, a cleanup run), so no flood limit.
func (notifier *Notifier) routeNotice(notice noticeFor) {
	connections, err := notifier.options.Store.ListConnections(notifier.ctx)

	if err != nil {
		notifier.options.Logger.Error("read notification connections", "err", err)

		return
	}

	var send []string

	for _, connection := range connections {
		if coversAny(connection, notice.projectIDs) {
			send = append(send, connection.ID)
		}
	}

	if len(send) > 0 {
		notifier.enqueue(send, notice.projectIDs, kindNotice, notice.notice)
	}
}

// coversAny: the connection is active, verified and covers one of the projects.
func coversAny(connection store.Connection, projectIDs []int) bool {
	if !connection.Active || !connection.Verified {
		return false
	}

	return connection.AllProjects || slices.ContainsFunc(projectIDs, func(id int) bool { return slices.Contains(connection.ProjectIDs, id) })
}

// DroppedNotice tells that a project's data started being dropped: the
// first drop in a day, its reason and kind; the Projects page has the rest.
func DroppedNotice(project, kind, reason string, count int64, projectsURL string) Notice {
	return Notice{
		Heading: fmt.Sprintf("%s: data dropped", project),
		Lines: []string{
			fmt.Sprintf("%s %s were not stored (%s).", formatCount(count), kind, reason),
			"This is the first drop of the project in 24 hours; the next notice comes after a day without drops. The Projects page counts every drop of the last 24 hours.",
		},
		URL: projectsURL, LinkText: "Open Projects",
	}
}

// CleanupNotice tells that a cleanup run is done: its scope, the data held
// before and after, and what each step deleted per project.
func CleanupNotice(run store.CleanupRun, projectNames map[int]string, scope, historyURL string) Notice {
	lines := []string{fmt.Sprintf("Data held: %s before, %s after, limit %s; took %s.",
		config.FormatSize(run.SizeBefore), config.FormatSize(run.SizeAfter), config.FormatSize(run.Limit), run.Duration.Round(time.Second))}

	for _, step := range run.Steps {
		var parts []string

		for _, projectID := range slices.Sorted(maps.Keys(step.Projects)) {
			if text := countsText(*step.Projects[projectID]); text != "" {
				parts = append(parts, cmp.Or(projectNames[projectID], "no project")+" "+text)
			}
		}

		if len(parts) > 0 {
			lines = append(lines, fmt.Sprintf("%d. %s: %s", step.Number, step.Name, strings.Join(parts, ", ")))
		}
	}

	return Notice{Heading: "Cleanup done: " + scope, Lines: lines, URL: historyURL, LinkText: "Open Cleanup history"}
}

// RunProjects are the projects a run deleted data of.
func RunProjects(run store.CleanupRun) []int {
	var projectIDs []int

	for _, step := range run.Steps {
		for projectID := range step.Projects {
			if projectID != 0 && !slices.Contains(projectIDs, projectID) {
				projectIDs = append(projectIDs, projectID)
			}
		}
	}

	slices.Sort(projectIDs)

	return projectIDs
}

func countsText(counts store.CleanupCounts) string {
	var parts []string

	for _, part := range []struct {
		count int
		name  string
	}{{counts.Logs, "logs"}, {counts.Events, "events"}, {counts.Thinned, "events thinned"}, {counts.Issues, "issues"}, {counts.StatusChanges, "status changes"}} {
		if part.count > 0 {
			parts = append(parts, formatCount(int64(part.count))+" "+part.name)
		}
	}

	return strings.Join(parts, ", ")
}

// formatCount writes a count with thousands separators (25,458).
func formatCount(count int64) string {
	digits := fmt.Sprint(count)

	for index := len(digits) - 3; index > 0; index -= 3 {
		digits = digits[:index] + "," + digits[index:]
	}

	return digits
}
