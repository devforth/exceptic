package notify

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"maps"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

//go:embed templates/email.html
var emailTemplateFiles embed.FS

var emailTemplates = template.Must(template.New("email.html").Funcs(template.FuncMap{
	"projectBadge": newProjectBadge,
	"levelStyle":   levelStyle,
	"shownLevel":   config.ShownLevel,
	"excerpt":      func(text string) string { return textutil.Shorten(text, exceptionLength) },
	"link":         func(url, text string) emailLink { return emailLink{URL: url, Text: text} },
}).ParseFS(emailTemplateFiles, "templates/email.html"))

// levelStyles color level badges in an email, by config.ShownLevel.
var levelStyles = map[string]template.CSS{
	"error":   "color:#c62828;background:#fdecea",
	"warning": "color:#9a6700;background:#fff4d6",
	"info":    "color:#1f5fbf;background:#e5efff",
	"debug":   "color:#555560;background:#ececf0",
}

func levelStyle(level string) template.CSS {
	return levelStyles[config.ShownLevel(level)]
}

// projectColor is the color of a project badge in an email; anything other
// than "#rrggbb" (a test message has none) falls back to grey.
func projectColor(color string) string {
	if config.ValidColor(color) {
		return color
	}

	return "#555560"
}

// projectBadge is a project as the web app shows it: its name with a capital
// first letter, outlined in its color, lightened under a dark theme.
type projectBadge struct{ Name, Color string }

func newProjectBadge(name, color string) projectBadge {
	return projectBadge{Name: textutil.UpperFirst(name), Color: projectColor(color)}
}

// Class names the badge's color for the dark theme's rule (ProjectStyles).
func (badge projectBadge) Class() string { return "x-p-" + strings.TrimPrefix(badge.Color, "#") }

// darkProjectColor is a project color as the web app shows it on dark: 60% of
// it and 40% white (layout.css, --project-text-share).
func darkProjectColor(color string) string {
	var red, green, blue int

	if _, err := fmt.Sscanf(color, "#%02x%02x%02x", &red, &green, &blue); err != nil {
		return color
	}

	mix := func(part int) int { return (part*60 + 255*40) / 100 }

	return fmt.Sprintf("#%02x%02x%02x", mix(red), mix(green), mix(blue))
}

// ProjectStyles are the dark theme's colors of the project badges in the
// email, one rule per color (inline styles cannot follow the theme).
func (view emailView) ProjectStyles() template.CSS {
	colors := []string{view.Notification.ProjectColor}

	for _, item := range view.Summary.Items {
		colors = append(colors, item.ProjectColor)
	}

	if view.Notice.Table != nil {
		for _, row := range view.Notice.Table.Rows {
			for _, cell := range row {
				if cell.Color != "" {
					colors = append(colors, cell.Color)
				}
			}
		}
	}

	var rules strings.Builder

	seen := map[string]bool{}

	for _, color := range colors {
		badge := newProjectBadge("", color)

		if seen[badge.Color] {
			continue
		}

		seen[badge.Color] = true
		dark := darkProjectColor(badge.Color)
		fmt.Fprintf(&rules, "  .%s { color: %s !important; border-color: %s !important; }\n", badge.Class(), dark, dark)
	}

	// Every color is "#rrggbb" (projectColor), so the rules hold nothing else.
	return template.CSS(rules.String()) //nolint:gosec // only validated hex colors
}

// Email sends the long format, with the stack trace and context, through any
// SMTP server. Port 465 uses implicit TLS; other ports upgrade with STARTTLS
// when the server offers it.
type Email struct {
	SMTP    config.SMTP
	To      string // the address of the connection
	AppName string // APP_NAME: subjects and the footer say where mail comes from
	// Unsubscribe is the link that stops the emails to To (Unsubscriber.URL);
	// empty for a confirmation code and without PUBLIC_URL.
	Unsubscribe string
}

func (email Email) Name() string { return config.ChannelEmail }

// Limits that keep a message readable and within what mail servers accept:
// the throwing part of the stack and the steps right before the error matter
// most, and anyone with the DSN writes every field.
const (
	maxEmailFrames      = 15
	maxEmailBreadcrumbs = 15
	maxEmailExceptions  = 10
	maxEmailContext     = 5 // source lines before and after the throwing one
	maxEmailTags        = 30
)

type fact struct {
	Name, Value string
	Show        string // "level" or "project": the value shown as its badge
}

// emailLink is a button of an email.
type emailLink struct{ URL, Text string }

// emailFrame is what marks an email around its content: the tag over the
// card, the color of the tag and of the card's top, the card's width.
type emailFrame struct {
	Label             string
	Tone              string // the class of its dark colors: x-tone-<tone>
	Color, Background string
	Width             int
}

// emailTones are the colors of a frame by tone, the design system's tokens
// (error, warning, success; info is the accent on --selected).
var emailTones = map[string][2]string{
	"error":   {"#c62828", "#fdecea"},
	"warning": {"#9a6700", "#fff4d6"},
	"success": {"#2e7d32", "#e8f5e9"},
	"info":    {"#4f46e5", "#eef0ff"},
}

// Widths of the card: an issue shows code, the others a short text.
const (
	issueEmailWidth = 720
	shortEmailWidth = 600
)

func frame(label, tone string, width int) emailFrame {
	colors, ok := emailTones[tone]

	if !ok {
		tone, colors = "info", emailTones["info"]
	}

	return emailFrame{Label: label, Tone: tone, Color: colors[0], Background: colors[1], Width: width}
}

// IsNumeric says whether a column holds numbers, aligned right.
func (table NoticeTable) IsNumeric(column int) bool { return slices.Contains(table.Numeric, column) }

type breadcrumbLine struct{ Time, Category, Message string }

// emailView is what templates/email.html receives.
type emailView struct {
	Kind         string
	Notification Notification
	Event        *sentry.Event
	Facts        []fact
	Breadcrumbs  []breadcrumbLine
	Frame        emailFrame
	To           string // set by send
	Unsubscribe  string // set by send
	Heading      string
	Summary      Summary
	Notice       Notice
	Note         string
	// PromptFile is the name of the attached agent prompt, empty without one.
	PromptFile string
	AppName    string // set by send

	// Verification email.
	Code     string
	ValidFor string
}

// Reference is the MCP request of an issue in the message (empty for a test).
func (view emailView) Reference(notification Notification) string {
	return mcpReference(view.AppName, notification)
}

// Exceptions returns the chain to show, outermost first, its text cut.
func (emailView) Exceptions(list sentry.ExceptionList) []sentry.Exception {
	exceptions := list.OutermostFirst()
	exceptions = exceptions[:min(len(exceptions), maxEmailExceptions)]

	for index := range exceptions {
		exceptions[index].Type = textutil.Shorten(exceptions[index].Type, fieldLength)
		exceptions[index].Value = textutil.Shorten(exceptions[index].Value, exceptionLength)
	}

	return exceptions
}

// InAppFrames returns the frames to show for an exception, their text cut:
// the newest in-app ones, or the newest frames at all when none is in-app.
func (emailView) InAppFrames(stacktrace *sentry.Stacktrace) []sentry.Frame {
	frames := stacktrace.NewestFirst()

	var inApp []sentry.Frame

	for _, frame := range frames {
		if frame.IsInApp() {
			inApp = append(inApp, frame)
		}
	}

	if len(inApp) > 0 {
		frames = inApp
	}

	frames = frames[:min(len(frames), maxEmailFrames)]

	for index, frame := range frames {
		frames[index] = sentry.Frame{
			Function: textutil.Shorten(frame.Function, fieldLength), Filename: textutil.Shorten(frame.Filename, fieldLength),
			Module: textutil.Shorten(frame.Module, fieldLength), Lineno: frame.Lineno, ContextLine: textutil.Shorten(frame.ContextLine, fieldLength),
			PreContext: shortLines(frame.PreContext[max(0, len(frame.PreContext)-maxEmailContext):]), PostContext: shortLines(frame.PostContext[:min(len(frame.PostContext), maxEmailContext)]),
		}
	}

	return frames
}

func shortLines(lines []string) []string {
	short := make([]string, len(lines))

	for index, line := range lines {
		short[index] = textutil.Shorten(line, fieldLength)
	}

	return short
}

// Tags returns the tags to show, sorted by name, their text cut.
func (emailView) Tags(tags sentry.Tags) []fact {
	names := slices.Sorted(maps.Keys(tags))
	shown := make([]fact, 0, min(len(names), maxEmailTags))

	for _, name := range names[:min(len(names), maxEmailTags)] {
		shown = append(shown, fact{Name: textutil.Shorten(name, fieldLength), Value: textutil.Shorten(tags[name], fieldLength)})
	}

	return shown
}

func (email Email) Send(ctx context.Context, notification Notification) error {
	tone := map[Kind]string{NewIssue: "error", Regression: "warning"}[notification.Kind]
	view := emailView{Kind: notification.Kind.label(), Notification: notification, Event: notification.Event, Frame: frame(notification.Kind.label(), tone, issueEmailWidth)}

	for _, item := range []fact{
		{Name: "Project", Value: notification.Project, Show: "project"},
		{Name: "Level", Value: notification.Level, Show: "level"},
		{Name: "Environment", Value: notification.Environment},
		{Name: "Release", Value: notification.Release},
		{Name: "Time", Value: notification.Time.UTC().Format("2006-01-02 15:04:05 UTC")},
	} {
		if item.Value != "" {
			view.Facts = append(view.Facts, item)
		}
	}

	if event := notification.Event; event != nil {
		if event.User != nil && (event.User.ID != "" || event.User.Email != "") {
			view.Facts = append(view.Facts, fact{Name: "User", Value: textutil.Shorten(strings.TrimSpace(string(event.User.ID)+" "+event.User.Email), fieldLength)})
		}

		breadcrumbs := event.Breadcrumbs[max(0, len(event.Breadcrumbs)-maxEmailBreadcrumbs):]

		for _, breadcrumb := range breadcrumbs {
			line := breadcrumbLine{Category: textutil.Shorten(breadcrumb.Category, fieldLength), Message: textutil.Shorten(textutil.StripTerminalCodes(breadcrumb.Message), breadcrumbLength)}

			if !breadcrumb.Timestamp.IsZero() {
				line.Time = breadcrumb.Timestamp.UTC().Format("15:04:05")
			}

			view.Breadcrumbs = append(view.Breadcrumbs, line)
		}
	}

	subject := fmt.Sprintf("[%s] %s: %s", notification.Project, notification.Kind.label(), textutil.Shorten(notification.Title, shortTitleLength))
	text := email.plainText(notification)

	if notification.Kind == Test {
		return email.send(ctx, subject, text, "notification", view)
	}

	// The prompt for a coding agent goes as a file: mail clients run no
	// script, so it cannot be copied with a button as on the issue page, and
	// in the body it would bury the error.
	view.PromptFile = promptFileName(notification)
	text += "\nA prompt for a coding agent that reproduces and explains this error is attached (" + view.PromptFile + ").\n"
	prompt := attachment{Name: view.PromptFile, ContentType: "text/markdown; charset=utf-8", Data: []byte(agentPrompt(notification))}

	return email.send(ctx, subject, text, "notification", view, prompt)
}

func (email Email) SendSummary(ctx context.Context, summary Summary) error {
	view := emailView{Heading: summary.heading(), Summary: summary, Note: summary.note(), Frame: frame("Summary", "info", shortEmailWidth)}

	var text strings.Builder

	text.WriteString(summary.heading() + "\n\n")

	for _, item := range summary.Items {
		fmt.Fprintf(&text, "- [%s] %s %s\n", item.Project, item.Title, item.IssueURL)

		if reference := mcpReference(email.AppName, item); reference != "" {
			fmt.Fprintf(&text, "  %s\n", reference)
		}
	}

	text.WriteString("\n" + summary.note() + "\n")

	// Like a notification's subject: the project in brackets.
	subject := "[" + appNameOr(email.AppName) + "] " + summary.heading()

	if summary.Project != "" {
		subject = "[" + summary.Project + "] " + strings.TrimPrefix(summary.heading(), summary.Project+": ")
	}

	return email.send(ctx, subject, text.String(), "summary", view)
}

func (email Email) SendNotice(ctx context.Context, notice Notice) error {
	text := notice.Heading + "\n\n" + strings.Join(notice.Lines, "\n") + "\n"

	if notice.Table != nil {
		text += "\n" + notice.Table.text(emailTableWidth) + "\n"
	}

	if notice.URL != "" {
		text += "\n" + notice.URL + "\n"
	}

	return email.send(ctx, "["+appNameOr(email.AppName)+"] "+notice.Heading, text, "notice", emailView{Heading: notice.Heading, Notice: notice, Frame: frame(cmp.Or(notice.Label, "Notice"), notice.Tone, shortEmailWidth)})
}

// plainText is the text/plain part for clients that do not show HTML.
func (email Email) plainText(notification Notification) string {
	var text strings.Builder

	fmt.Fprintf(&text, "%s in %s (%s)\n\n%s\n", notification.Kind.label(), notification.Project, notification.Level, notification.Title)

	if details := notification.details(); details != "" {
		text.WriteString(details + "\n")
	}

	if reference := mcpReference(email.AppName, notification); reference != "" {
		text.WriteString("\n" + reference + "\n")
	}

	if notification.IssueURL != "" {
		text.WriteString("\n" + notification.IssueURL + "\n")
	}

	if event := notification.Event; event != nil {
		for index, exception := range (emailView{}).Exceptions(event.Exception) {
			prefix := ""

			if index > 0 {
				prefix = "Caused by "
			}

			fmt.Fprintf(&text, "\n%s%s: %s\n", prefix, exception.Type, exception.Value)

			for _, frame := range (emailView{}).InAppFrames(exception.Stacktrace) {
				fmt.Fprintf(&text, "    at %s (%s:%d)\n", frame.Function, cmp.Or(frame.Filename, frame.Module), frame.Lineno)
			}
		}
	}

	return text.String()
}

// attachment is a file attached to an email.
type attachment struct {
	Name        string
	ContentType string
	Data        []byte
}

func (email Email) send(ctx context.Context, subject, text, templateName string, view emailView, attachments ...attachment) error {
	var htmlBody bytes.Buffer

	view.AppName, view.To, view.Unsubscribe = appNameOr(email.AppName), email.To, email.Unsubscribe

	if err := emailTemplates.ExecuteTemplate(&htmlBody, templateName, view); err != nil {
		return fmt.Errorf("email template: %w", err)
	}

	var extra [][2]string

	// One-click unsubscribing (RFC 8058): mail providers show their own button for it.
	if email.Unsubscribe != "" {
		extra = [][2]string{{"List-Unsubscribe", "<" + email.Unsubscribe + ">"}, {"List-Unsubscribe-Post", "List-Unsubscribe=One-Click"}}
		text += "\nStop these emails: " + email.Unsubscribe + "\n"
	}

	message, err := buildMessage(email.SMTP.From, []string{email.To}, subject, text, htmlBody.String(), extra, attachments...)

	if err != nil {
		return err
	}

	return deliver(ctx, email.SMTP, []string{email.To}, message)
}

// SendVerificationCode mails the code that confirms a new email address. It
// is sent directly, not through the outbox: the person is waiting for it in
// the UI.
func (email Email) SendVerificationCode(ctx context.Context, code string, validFor time.Duration) error {
	subject := "Confirm notifications to this address: " + code
	text := fmt.Sprintf("Your code to confirm notifications to this address:\n\n    %s\n\n"+
		"Enter it in %s within %s. If you did not add this address, ignore this email.\n",
		code, appNameOr(email.AppName), config.HumanDuration(validFor.Round(time.Minute)))

	view := emailView{Code: code, ValidFor: config.HumanDuration(validFor.Round(time.Minute)), Frame: frame("Confirm address", "info", shortEmailWidth)}

	return email.send(ctx, subject, text, "verification", view)
}

// buildMessage assembles a multipart/alternative message (text and HTML),
// wrapped in multipart/mixed when files are attached.
func buildMessage(from string, to []string, subject, text, htmlBody string, extra [][2]string, attachments ...attachment) ([]byte, error) {
	sender, err := mail.ParseAddress(from)

	if err != nil {
		return nil, fmt.Errorf("sender: %w", err)
	}

	boundary := randomHex(16)
	mixedBoundary := randomHex(16)
	contentType := `multipart/alternative; boundary="` + boundary + `"`

	if len(attachments) > 0 {
		contentType = `multipart/mixed; boundary="` + mixedBoundary + `"`
	}

	var message bytes.Buffer

	headers := [][2]string{
		{"From", sender.String()},
		{"To", strings.Join(to, ", ")},
		// Encoded words folded one per line: a header line holds at most 998 characters.
		{"Subject", strings.ReplaceAll(mime.QEncoding.Encode("utf-8", subject), "?= =?", "?=\r\n =?")},
		{"Date", time.Now().Format(time.RFC1123Z)},
		{"Message-ID", "<" + randomHex(16) + "@" + domainOf(sender.Address) + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", contentType},
	}
	headers = append(headers, extra...)

	for _, header := range headers {
		message.WriteString(header[0] + ": " + header[1] + "\r\n")
	}

	if len(attachments) > 0 {
		message.WriteString("\r\n--" + mixedBoundary + "\r\n")
		message.WriteString(`Content-Type: multipart/alternative; boundary="` + boundary + `"` + "\r\n")
	}

	for _, part := range [][2]string{{"text/plain", text}, {"text/html", htmlBody}} {
		message.WriteString("\r\n--" + boundary + "\r\n")
		message.WriteString("Content-Type: " + part[0] + "; charset=utf-8\r\n")
		message.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")

		writer := quotedprintable.NewWriter(&message)

		if _, err := writer.Write([]byte(part[1])); err != nil {
			return nil, err
		}

		if err := writer.Close(); err != nil {
			return nil, err
		}
	}

	message.WriteString("\r\n--" + boundary + "--\r\n")

	if len(attachments) == 0 {
		return message.Bytes(), nil
	}

	for _, file := range attachments {
		name := mime.QEncoding.Encode("utf-8", file.Name)

		message.WriteString("\r\n--" + mixedBoundary + "\r\n")
		message.WriteString("Content-Type: " + file.ContentType + "; name=\"" + name + "\"\r\n")
		message.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\n")
		message.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")

		// Base64 in lines of 76 characters, as MIME requires.
		encoded := base64.StdEncoding.EncodeToString(file.Data)

		for len(encoded) > 76 {
			message.WriteString(encoded[:76] + "\r\n")
			encoded = encoded[76:]
		}

		message.WriteString(encoded + "\r\n")
	}

	message.WriteString("\r\n--" + mixedBoundary + "--\r\n")

	return message.Bytes(), nil
}

// deliver talks SMTP: connect (TLS on 465), STARTTLS when offered, AUTH when
// a username is set, then send to every recipient.
func deliver(ctx context.Context, settings config.SMTP, recipients []string, message []byte) error {
	address := net.JoinHostPort(settings.Host, strconv.Itoa(settings.Port))
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	tlsConfig := &tls.Config{ServerName: settings.Host, MinVersion: tls.VersionTLS12}

	var (
		conn net.Conn
		err  error
	)

	if settings.Port == config.ImplicitTLSPort {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}

	if err != nil {
		return fmt.Errorf("smtp connect %s: %w", address, err)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	// A cancelled ctx (shutdown) ends the conversation at once.
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })

	defer stop()

	client, err := smtp.NewClient(conn, settings.Host)

	if err != nil {
		_ = conn.Close()

		return fmt.Errorf("smtp greeting: %w", err)
	}

	defer func() { _ = client.Close() }()

	if settings.Port != config.ImplicitTLSPort {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}

	// PlainAuth refuses to send the password over a connection that is not
	// encrypted (except to localhost), so a server without TLS fails here.
	if settings.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", settings.Username, settings.Password, settings.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	sender, _ := mail.ParseAddress(settings.From)

	if err := client.Mail(sender.Address); err != nil {
		return fmt.Errorf("smtp sender: %w", err)
	}

	for _, recipient := range recipients {
		address, err := mail.ParseAddress(recipient)

		if err != nil {
			return fmt.Errorf("smtp recipient %q: %w", recipient, err)
		}

		if err := client.Rcpt(address.Address); err != nil {
			return fmt.Errorf("smtp recipient %s: %w", address.Address, err)
		}
	}

	writer, err := client.Data()

	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}

	if _, err := writer.Write(message); err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}

	// The message is accepted: a failed QUIT must not send it again.
	_ = client.Quit()

	return nil
}

func randomHex(size int) string {
	value := make([]byte, size)
	_, _ = rand.Read(value)

	return hex.EncodeToString(value)
}

func domainOf(address string) string {
	if index := strings.LastIndexByte(address, '@'); index >= 0 {
		return address[index+1:]
	}

	return "localhost"
}
