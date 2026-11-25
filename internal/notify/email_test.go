package notify

import (
	"bufio"
	"context"
	"fmt"
	"html"
	"io"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

func fixtureEvent(t *testing.T, name string) *sentry.Event {
	t.Helper()

	envelope, err := sentry.ParseEnvelope(fixtures.Envelope(t, name))

	if err != nil {
		t.Fatal(err)
	}

	event, err := sentry.ParseEvent(envelope.Items[0].Payload, envelope.Header.EventID, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	return event
}

func TestEmail(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)
	event := fixtureEvent(t, "node-event-cause-chain")

	notification := Notification{
		Kind:         NewIssue,
		Project:      "backend",
		ProjectColor: "#db2777",
		IssueID:      "issue005",
		IssueURL:     "https://errors.example.com/issues/issue005",
		Title:        event.Title(),
		Culprit:      "GET /api/debug-sentry/cause",
		Level:        "error",
		Environment:  event.Environment,
		Time:         time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
		Event:        event,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if err := (Email{SMTP: settings, To: "ops@example.com"}).Send(ctx, notification); err != nil {
		t.Fatal(err)
	}

	session := <-sessions
	commands := strings.Join(session.Commands, "\n")

	for _, want := range []string{"AUTH PLAIN", "MAIL FROM:<errors@example.com>", "RCPT TO:<ops@example.com>"} {
		if !strings.Contains(commands, want) {
			t.Errorf("SMTP session is missing %q:\n%s", want, commands)
		}
	}

	subject, text, htmlPart := fixtures.ParseEmail(t, session.Data)

	for _, body := range []string{text, html.UnescapeString(htmlPart)} {
		if !strings.Contains(body, `Exceptic MCP: get_issue {"issue_id":"issue005"}`) {
			t.Error("missing copyable MCP reference in email body")
		}
	}

	if subject != "[backend] New issue: Error: Failed to load orders" {
		t.Errorf("subject = %q", subject)
	}

	// The agent prompt is attached, not in the body.
	prompt := fixtures.ReadEmail(t, session.Data).Attachments["backend-error-issue005.md"]

	for _, want := range []string{"## Exception", "Kind: New issue", "Do not change any code"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("attached prompt is missing %q", want)
		}
	}

	if strings.Contains(htmlPart, "## Exception") || strings.Contains(text, "## Exception") || !strings.Contains(htmlPart, "backend-error-issue005.md") || !strings.Contains(text, "backend-error-issue005.md") {
		t.Error("the body must mention the attached prompt, not contain it")
	}

	// The project is marked with its color.
	if !strings.Contains(htmlPart, "border:1px solid #db2777;") {
		t.Error("the project badge is missing")
	}

	// The long format: cause chain, source line of the throw, link, environment.
	for _, want := range []string{"Caused by", "connect ECONNREFUSED", `throw new Error(&#34;Failed to load orders&#34;`, `href="https://errors.example.com/issues/issue005"`, "development"} {
		if !strings.Contains(htmlPart, want) {
			t.Errorf("HTML part is missing %q", want)
		}
	}

	if !strings.Contains(text, "Caused by Error: connect ECONNREFUSED") || !strings.Contains(text, "https://errors.example.com/issues/issue005") {
		t.Errorf("text part:\n%s", text)
	}
}

// Every free text of an event is cut in the body and the prompt, and a long
// chain shows its outermost exceptions, so mail servers accept the message.
func TestEmailCapsEventText(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)
	huge := strings.Repeat("x", 100<<10)
	inApp := true
	frame := sentry.Frame{
		Function: huge, Filename: huge, Module: huge, InApp: &inApp, ContextLine: huge,
		PreContext: slices.Repeat([]string{huge}, 20), PostContext: slices.Repeat([]string{huge}, 20),
	}
	chain := &sentry.Event{
		Tags: sentry.Tags{}, User: &sentry.User{ID: sentry.FlexString(huge), Email: huge},
		Breadcrumbs: slices.Repeat(sentry.BreadcrumbList{{Category: huge, Message: huge}}, maxEmailBreadcrumbs),
	}

	for range maxEmailExceptions + 5 {
		chain.Exception = append(chain.Exception, sentry.Exception{Type: huge, Value: huge, Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{frame}}})
	}

	for index := range 100 {
		chain.Tags[fmt.Sprintf("tag%d", index)] = huge
	}

	for _, event := range []*sentry.Event{chain, {Message: sentry.Message(strings.Repeat(huge, 10))}} {
		notification := Notification{Kind: NewIssue, Project: "backend", IssueID: "issue005", Title: "Error", Level: "error", Event: event}

		if err := (Email{SMTP: settings, To: "ops@example.com"}).Send(context.Background(), notification); err != nil {
			t.Fatal(err)
		}

		if size := len((<-sessions).Data); size > 400<<10 {
			t.Errorf("email of %d bytes", size)
		}
	}
}

func TestEmailSummary(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)

	summary := Summary{Count: 3, Window: time.Minute, Items: []Notification{{Project: "backend", IssueID: "issue042", Title: "Error: <b>boom</b>"}}}

	if err := (Email{SMTP: settings, To: "ops@example.com"}).SendSummary(context.Background(), summary); err != nil {
		t.Fatal(err)
	}

	subject, text, htmlPart := fixtures.ParseEmail(t, (<-sessions).Data)

	for _, body := range []string{text, html.UnescapeString(htmlPart)} {
		if !strings.Contains(body, `Exceptic MCP: get_issue {"issue_id":"issue042"}`) {
			t.Error("missing MCP reference in summary")
		}
	}

	if subject != "[Exceptic] 3 more issues in the last 1 minute" || strings.Contains(htmlPart, "<b>boom</b>") {
		t.Fatalf("subject %q; HTML must escape titles:\n%s", subject, htmlPart)
	}
}

func TestEmailConnectionError(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	settings := config.SMTP{Host: "127.0.0.1", Port: port, From: "errors@example.com"}

	err := Email{SMTP: settings, To: "ops@example.com"}.Send(context.Background(), sample)

	if err == nil || !strings.Contains(err.Error(), "smtp connect 127.0.0.1:"+strconv.Itoa(port)) {
		t.Fatalf("got %v", err)
	}
}

func TestVerificationCodeEmail(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)

	if err := (Email{SMTP: settings, To: "new@example.com"}).SendVerificationCode(context.Background(), "482913", 15*time.Minute); err != nil {
		t.Fatal(err)
	}

	session := <-sessions
	subject, text, htmlPart := fixtures.ParseEmail(t, session.Data)

	if !strings.Contains(strings.Join(session.Commands, "\n"), "RCPT TO:<new@example.com>") || !strings.Contains(subject, "482913") ||
		!strings.Contains(text, "482913") || !strings.Contains(htmlPart, "482913") || !strings.Contains(htmlPart, "notifications to this address") || !strings.Contains(text, "15 minutes") {
		t.Fatalf("subject %q\ntext:\n%s\nhtml:\n%s", subject, text, htmlPart)
	}
}

// A test message is about no error, so it carries no agent prompt.
func TestTestEmailHasNoPrompt(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)
	notification := Notification{Kind: Test, Project: "backend", Title: "Test message", Level: "info"}

	if err := (Email{SMTP: settings, To: "ops@example.com"}).Send(context.Background(), notification); err != nil {
		t.Fatal(err)
	}

	email := fixtures.ReadEmail(t, (<-sessions).Data)

	if len(email.Attachments) != 0 || strings.Contains(email.Text, "prompt") || strings.Contains(email.HTML, "prompt") {
		t.Fatalf("attachments %v\ntext:\n%s", slices.Collect(maps.Keys(email.Attachments)), email.Text)
	}
}

// A long non-ASCII subject is folded: a header line holds at most 998 characters.
func TestLongSubjectIsFolded(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)
	title := strings.Repeat("Помилка ", 40)
	notification := Notification{Kind: NewIssue, Project: "backend", IssueID: "issue005", Title: title, Level: "error"}

	if err := (Email{SMTP: settings, To: "ops@example.com"}).Send(context.Background(), notification); err != nil {
		t.Fatal(err)
	}

	data := (<-sessions).Data
	header, _, _ := strings.Cut(data, "\r\n\r\n")

	for line := range strings.SplitSeq(header, "\r\n") {
		if len(line) > 998 {
			t.Fatalf("header line of %d characters", len(line))
		}
	}

	if subject, _, _ := fixtures.ParseEmail(t, data); subject != "[backend] New issue: "+textutil.Shorten(title, shortTitleLength) {
		t.Fatalf("subject = %q", subject)
	}
}

// smtpServer accepts one session and answers every command; quit handles QUIT.
func smtpServer(t *testing.T, quit func(conn net.Conn)) config.SMTP {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()

		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		reader := bufio.NewReader(conn)
		reply := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }

		reply("220 localhost ESMTP")

		for {
			line, err := reader.ReadString('\n')

			if err != nil {
				return
			}

			switch command := strings.ToUpper(strings.TrimSpace(line)); command {
			case "DATA":
				reply("354 go ahead")

				for line != ".\r\n" && err == nil {
					line, err = reader.ReadString('\n')
				}

				reply("250 queued")
			case "QUIT":
				quit(conn)

				return
			default:
				reply("250 ok")
			}
		}
	}()

	return config.SMTP{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, From: "errors@example.com"}
}

// Once the server accepted the message, a dropped QUIT is no failure: a
// retry would deliver the email twice.
func TestEmailAcceptedBeforeFailedQuit(t *testing.T) {
	settings := smtpServer(t, func(conn net.Conn) { _ = conn.Close() })

	if err := (Email{SMTP: settings, To: "ops@example.com"}).Send(context.Background(), sample); err != nil {
		t.Fatal(err)
	}
}

// Cancelling the context (shutdown) ends a conversation with a stalled server.
func TestEmailStopsWhenCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	// The server accepts the connection and never greets.
	go func() {
		conn, err := listener.Accept()

		if err == nil {
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	settings := config.SMTP{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, From: "errors@example.com"}

	go func() { result <- (Email{SMTP: settings, To: "ops@example.com"}).Send(ctx, sample) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("a cancelled send succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the send went on after cancel")
	}
}

func TestEmailIssueLinkSetting(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)
	factory := Factory{SMTP: settings}
	notification := Notification{Kind: NewIssue, Project: "backend", Title: "Example issue", Level: "warning", IssueURL: "https://errors.example.com/issues/issue005"}

	for _, hidden := range []bool{false, true} {
		channel, err := factory.Channel(store.Connection{Type: config.ChannelEmail, Address: "ops@example.com", HideIssueLink: hidden})

		if err != nil {
			t.Fatal(err)
		}

		if err := channel.Send(context.Background(), notification); err != nil {
			t.Fatal(err)
		}

		_, text, htmlPart := fixtures.ParseEmail(t, (<-sessions).Data)

		if strings.Contains(text, notification.IssueURL) == hidden || strings.Contains(htmlPart, notification.IssueURL) == hidden || strings.Contains(htmlPart, "Open issue") == hidden {
			t.Fatalf("hide link %v: unexpected link visibility", hidden)
		}

		if !strings.Contains(text, notification.Title) || !strings.Contains(htmlPart, notification.Title) {
			t.Fatal("message content must remain in both parts")
		}

		if err := channel.SendSummary(context.Background(), Summary{Count: 1, Window: time.Minute, Items: []Notification{notification}}); err != nil {
			t.Fatal(err)
		}

		_, text, htmlPart = fixtures.ParseEmail(t, (<-sessions).Data)

		if strings.Contains(text, notification.IssueURL) == hidden || strings.Contains(htmlPart, notification.IssueURL) == hidden {
			t.Fatalf("hide link %v: unexpected summary link visibility", hidden)
		}
	}
}

func TestPromptFileName(t *testing.T) {
	testCases := []struct {
		notification Notification
		want         string
	}{
		{Notification{Project: "backend", Level: "error", IssueID: "issue006"}, "backend-error-issue006.md"},
		{Notification{Project: "Web App / SSR", Level: "fatal", IssueID: "issue012"}, "web-app-ssr-fatal-issue012.md"},
		{Notification{Project: "../../etc", IssueID: "issue003"}, "etc-error-issue003.md"},
	}

	for _, testCase := range testCases {
		if got := promptFileName(testCase.notification); got != testCase.want {
			t.Errorf("promptFileName(%+v) = %q, want %q", testCase.notification, got, testCase.want)
		}
	}
}

func TestEveryLevelHasStyle(t *testing.T) {
	for _, level := range slices.Concat(config.Levels, config.LogLevels) {
		if levelStyle(level) == "" {
			t.Errorf("level %q has no email style", level)
		}
	}
}

// A cleanup notice mails a table: steps with their short names, projects as
// badges in their colors, counts right-aligned; under a card tagged with its
// label and the address it went to.
func TestNoticeEmail(t *testing.T) {
	settings, sessions := fixtures.FakeSMTP(t)
	run := store.CleanupRun{SizeBefore: 2 << 30, SizeAfter: 3 << 29, Limit: 2 << 30, Duration: 29 * time.Second,
		Steps: []store.CleanupStep{{Number: 5, Projects: map[int]*store.CleanupCounts{2: {Logs: 25458}}}}}
	notice := CleanupNotice(run, map[int]config.Project{2: {ID: 2, Name: "frontend", Color: "#db2777"}}, map[int]string{5: "Logs below error"},
		"all projects", "https://errors.example.com/projects/cleanups")

	if err := (Email{SMTP: settings, To: "ops@example.com", AppName: "Exceptic"}).SendNotice(context.Background(), notice); err != nil {
		t.Fatal(err)
	}

	subject, text, htmlPart := fixtures.ParseEmail(t, (<-sessions).Data)

	if subject != "[Exceptic] Cleanup done: all projects" {
		t.Errorf("subject %q", subject)
	}

	for _, want := range []string{">Cleanup done</span>", "Logs below error", "border:1px solid #db2777", ">frontend</span>", `align="right"`, ">25,458 logs</td>",
		">2.0 GB</div>", "https://errors.example.com/projects/cleanups", "Sent by Exceptic to ops@example.com.",
		// A dark theme for the clients that read <style>; the card keeps its stripe.
		"@media (prefers-color-scheme: dark)", `<td class="x-card" style="background:#ffffff;border:1px solid #e4e4ea;border-top:4px solid #16a34a`} {
		if !strings.Contains(htmlPart, want) {
			t.Errorf("the HTML lacks %q", want)
		}
	}

	if !strings.Contains(text, "5     frontend  25,458 logs") {
		t.Errorf("the text part:\n%s", text)
	}
}
