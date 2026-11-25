package notify

import (
	"cmp"
	"fmt"
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
	// Label and Tone (success, warning) mark the email: its tag and the stripe of its card.
	Label   string
	Tone    string
	Heading string
	// Lines are the text of chats; the email shows them under its heading, or its Stats instead.
	Lines []string
	Stats []NoticeStat `json:",omitempty"`
	// Table follows: an HTML table in email, aligned columns in a code block in chats.
	Table *NoticeTable `json:",omitempty"`
	// URL is the page of the web app that shows more; empty without PUBLIC_URL.
	URL      string
	LinkText string
}

// NoticeStat is a figure an email shows as a tile.
type NoticeStat struct{ Name, Value string }

// NoticeTable is a table of a notice: its column names, rows and the columns
// of numbers, which the email aligns right.
type NoticeTable struct {
	Columns []string
	Rows    [][]NoticeCell
	Numeric []int `json:",omitempty"`
}

// NoticeCell is a cell: its text, a detail the email shows after it (a
// step's name), and a project's color, which makes it a badge in the email.
type NoticeCell struct {
	Text   string
	Detail string `json:",omitempty"`
	Color  string `json:",omitempty"`
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
	all := [][]string{table.Columns}

	for _, row := range table.Rows {
		texts := make([]string, len(row))

		for index, cell := range row {
			texts[index] = cell.Text
		}

		all = append(all, texts)
	}

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

// fitText is the table as text in at most room characters: the rows that
// fit, then how many more were left out.
func (table NoticeTable) fitText(width, room int) string {
	for shown := len(table.Rows); ; shown-- {
		cut := table
		cut.Rows = table.Rows[:shown]
		text := cut.text(width)

		if more := len(table.Rows) - shown; more > 0 {
			text += fmt.Sprintf("\n…and %d more rows", more)
		}

		if shown == 0 || utf8.RuneCountInString(text) <= room {
			return text
		}
	}
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

// noticeFor is a notice and the projects it is about, on its way to the
// dispatcher; build, when set, writes it for each connection (NoticeWithin).
type noticeFor struct {
	notice     Notice
	projectIDs []int
	build      func(store.Access) (Notice, bool)
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

	notifier.queueNotice(noticeFor{notice: notice, projectIDs: slices.Clone(projectIDs)})
}

// NoticeWithin queues a notice about projects that build writes for each
// connection covering one of them, from the projects it covers as an access:
// a connection gets nothing of the others (false: nothing at all).
func (notifier *Notifier) NoticeWithin(projectIDs []int, build func(store.Access) (Notice, bool)) {
	if notifier.options.Store == nil {
		return
	}

	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()

	if notifier.closed {
		return
	}

	notifier.queueNotice(noticeFor{projectIDs: slices.Clone(projectIDs), build: build})
}

// queueNotice: under the mutex, with the notifier open.
func (notifier *Notifier) queueNotice(notice noticeFor) {
	select {
	case notifier.queue <- queued{notice: &notice}:
	default:
		notifier.options.Logger.Warn("notification queue full, dropping a notice", "heading", notice.notice.Heading)
	}
}

// routeNotice writes a notice for every connection that covers one of its
// projects; notices are few (a day's first drop, a cleanup run), so no flood limit.
// A notice built per access is built once for the connections that see the
// same of its projects, and written for all of them in one batch.
func (notifier *Notifier) routeNotice(notice noticeFor) {
	connections, err := notifier.options.Store.ListConnections(notifier.ctx)

	if err != nil {
		notifier.options.Logger.Error("read notification connections", "err", err)

		return
	}

	var send []string

	groups := map[string]*noticeGroup{}

	for _, connection := range connections {
		if !coversAny(connection, notice.projectIDs) {
			continue
		}

		if notice.build == nil {
			send = append(send, connection.ID)

			continue
		}

		access := connectionAccess(connection)
		covered := slices.DeleteFunc(slices.Clone(notice.projectIDs), func(id int) bool { return !access.Sees(id) })
		// What a connection is shown depends only on this: all projects, or which of the notice's.
		key := "all"

		if !access.All() {
			key = fmt.Sprint(covered)
		}

		group := groups[key]

		if group == nil {
			group = &noticeGroup{access: access, covered: covered}
			groups[key] = group
		}

		group.connectionIDs = append(group.connectionIDs, connection.ID)
	}

	for _, group := range groups {
		if shown, ok := notice.build(group.access); ok {
			notifier.enqueue(group.connectionIDs, group.covered, kindNotice, shown)
		}
	}

	if len(send) > 0 {
		notifier.enqueue(send, notice.projectIDs, kindNotice, notice.notice)
	}
}

// noticeGroup is the connections that see the same of a notice's projects.
type noticeGroup struct {
	access        store.Access
	covered       []int
	connectionIDs []string
}

// connectionAccess is the projects a connection covers, as a viewer's access.
func connectionAccess(connection store.Connection) store.Access {
	if connection.AllProjects {
		return store.Access{}
	}

	return store.OnlyProjects(connection.ProjectIDs...)
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
func DroppedNotice(project config.Project, kind, reason string, count int64, projectsURL string) Notice {
	return Notice{
		Label: "Data dropped", Tone: "warning",
		Heading: fmt.Sprintf("%s: data dropped", project.Name),
		Lines:   []string{"First drop in 24 hours; the Projects page counts the rest."},
		Table: &NoticeTable{
			Columns: []string{"What", "Why", "Count"},
			Rows:    [][]NoticeCell{{{Text: kind}, {Text: reason}, {Text: formatCount(count)}}},
			Numeric: []int{2},
		},
		URL: projectsURL, LinkText: "Open Projects",
	}
}

// CleanupNotice tells that a cleanup run is done: its scope, the data held
// before and after, and what each step deleted per project.
// Projects are the projects by id (their shown color), stepNames each step's
// short name by number: chats show the number only, the email both.
func CleanupNotice(run store.CleanupRun, projects map[int]config.Project, stepNames map[int]string, scope, historyURL string) Notice {
	table := &NoticeTable{Columns: []string{"Step", "Project", "Deleted"}, Numeric: []int{2}}

	for _, step := range run.Steps {
		for _, projectID := range store.StepProjects(step.Projects) {
			if text := countsText(*step.Projects[projectID]); text != "" {
				project := projects[projectID]
				name := cmp.Or(project.Name, "no project")

				if projectID == store.OtherProjects {
					name = "Other"
				}

				table.Rows = append(table.Rows, []NoticeCell{
					{Text: strconv.Itoa(step.Number), Detail: stepNames[step.Number]}, {Text: name, Color: project.Color}, {Text: text},
				})
			}
		}
	}

	before, after, limit, took := config.FormatSize(run.SizeBefore), config.FormatSize(run.SizeAfter), config.FormatSize(run.Limit), run.Duration.Round(time.Second).String()

	return Notice{
		Label: "Cleanup done", Tone: "success",
		Heading: "Cleanup done: " + scope,
		Lines:   []string{fmt.Sprintf("%s → %s of %s, %s", before, after, limit, took)},
		Stats:   []NoticeStat{{"Before", before}, {"After", after}, {"Limit", limit}, {"Took", took}},
		Table:   table, URL: historyURL, LinkText: "Open Cleanup history",
	}
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
