package notify

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	// Table follows the lines: an HTML table in email, aligned columns in a code block in chats.
	Table *NoticeTable `json:",omitempty"`
	// URL is the page of the web app that shows more; empty without PUBLIC_URL.
	URL      string
	LinkText string
}

// NoticeTable is a table of a notice: its column names and rows.
type NoticeTable struct {
	Columns []string
	Rows    [][]string
}

// Widths of a table in plain text: chats are read on phones, the text part of an email on wider screens.
const (
	chatTableWidth  = 40
	emailTableWidth = 72
	minColumnWidth  = 4
	columnGap       = "  "
)

// text is the table as aligned columns of plain text at most width
// characters wide: the widest columns narrow until it fits, and a cell longer
// than its column wraps onto more lines of its row, padded like the others.
func (table NoticeTable) text(width int) string {
	all := append([][]string{table.Columns}, table.Rows...)
	widths := make([]int, len(table.Columns))

	for _, row := range all {
		for index, cell := range row {
			widths[index] = max(widths[index], utf8.RuneCountInString(cell))
		}
	}

	room := width - len(columnGap)*(len(widths)-1)

	for sum(widths) > room {
		widest := slices.Index(widths, slices.Max(widths))

		if widths[widest] <= minColumnWidth {
			break
		}

		widths[widest]--
	}

	var lines []string

	wraps := false

	for _, row := range all {
		for index, cell := range row {
			wraps = wraps || utf8.RuneCountInString(cell) > widths[index]
		}
	}

	for number, row := range all {
		wrapped := make([][]string, len(row))
		height := 0

		for index, cell := range row {
			wrapped[index] = wrap(cell, widths[index])
			height = max(height, len(wrapped[index]))
		}

		// Rows of several lines are told apart by an empty line.
		if wraps && number > 1 {
			lines = append(lines, "")
		}

		for line := range height {
			cells := make([]string, len(row))

			for index := range row {
				text := ""

				if line < len(wrapped[index]) {
					text = wrapped[index][line]
				}

				cells[index] = text + strings.Repeat(" ", widths[index]-utf8.RuneCountInString(text))
			}

			lines = append(lines, strings.TrimRight(strings.Join(cells, columnGap), " "))
		}
	}

	return strings.Join(lines, "\n")
}

func sum(values []int) int {
	total := 0

	for _, value := range values {
		total += value
	}

	return total
}

// wrap breaks text into lines of at most width characters at spaces, and a
// word longer than that into pieces.
func wrap(text string, width int) []string {
	var lines []string

	line := ""

	for _, word := range strings.Fields(text) {
		for utf8.RuneCountInString(word) > width {
			if line != "" {
				lines, line = append(lines, line), ""
			}

			runes := []rune(word)
			lines, word = append(lines, string(runes[:width])), string(runes[width:])
		}

		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= width:
			line += " " + word
		default:
			lines, line = append(lines, line), word
		}
	}

	if line != "" || len(lines) == 0 {
		lines = append(lines, line)
	}

	return lines
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
		Lines:   []string{"First drop in 24 hours; the Projects page counts the rest."},
		Table:   &NoticeTable{Columns: []string{"What", "Why", "Count"}, Rows: [][]string{{kind, reason, formatCount(count)}}},
		URL:     projectsURL, LinkText: "Open Projects",
	}
}

// CleanupNotice tells that a cleanup run is done: its scope, the data held
// before and after, and what each step deleted per project.
func CleanupNotice(run store.CleanupRun, projectNames map[int]string, scope, historyURL string) Notice {
	table := &NoticeTable{Columns: []string{"Step", "Project", "Deleted"}}

	// The step by its number: the Cleanup history names it.
	for _, step := range run.Steps {
		for _, projectID := range slices.Sorted(maps.Keys(step.Projects)) {
			if text := countsText(*step.Projects[projectID]); text != "" {
				table.Rows = append(table.Rows, []string{strconv.Itoa(step.Number), cmp.Or(projectNames[projectID], "no project"), text})
			}
		}
	}

	line := fmt.Sprintf("%s → %s of %s, %s", config.FormatSize(run.SizeBefore), config.FormatSize(run.SizeAfter), config.FormatSize(run.Limit), run.Duration.Round(time.Second))

	return Notice{Heading: "Cleanup done: " + scope, Lines: []string{line}, Table: table, URL: historyURL, LinkText: "Open Cleanup history"}
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
