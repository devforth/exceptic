package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/api"
	"github.com/devforth/exceptic/internal/app"
	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/store"
	"github.com/devforth/exceptic/internal/storage/storetest"
)

const notificationsKey = "0123456789abcdef0123456789abcdef"

// notificationsServer has every connection type: SMTP (a fake server whose
// mails arrive on mails), a key for chat tokens and a fake Telegram API.
type notificationsServer struct {
	*restServer
	mails <-chan fixtures.SMTPSession
	admin uiUser
}

func newNotificationsServer(t *testing.T, configure ...func(*config.Config)) *notificationsServer {
	t.Helper()

	smtp, mails := fixtures.FakeSMTP(t)
	settings := &config.Config{
		AppName: config.DefaultAppName, Projects: fixtures.Projects(), AllowedOrigins: []string{"*"},
		Notifications: config.Notifications{SecretKey: notificationsKey, SMTP: smtp},
	}

	for _, change := range configure {
		change(settings)
	}

	box, err := secret.New(notificationsKey)

	if err != nil {
		t.Fatal(err)
	}

	telegram := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getChat") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":-100,"type":"group","title":"Ops chat"}}`))

			return
		}

		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(telegram.Close)

	database := storetest.Open(t)
	built, err := app.New(context.Background(), app.Options{
		Settings: settings, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Store: database, Version: "test",
		Factory: notify.Factory{
			SMTP: settings.Notifications.SMTP, Box: box, TelegramURL: telegram.URL,
			PublicURL: settings.Notifications.PublicURL, Unsubscribe: notify.NewUnsubscriber(notificationsKey),
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(built.Start())

	server := &notificationsServer{restServer: &restServer{t: t, app: built, handler: built.Handler, database: database}, mails: mails}
	server.admin = server.webUser(store.RoleAdmin, false)

	return server
}

// send calls name as the admin with input as JSON (GET: as the query) and
// decodes the answer into output; it returns the status.
func (server *notificationsServer) send(name string, input map[string]any, output any) int {
	server.t.Helper()

	endpoint := endpointNamed(server.t, name)
	method, _, _ := strings.Cut(api.Route(api.TransportUI, endpoint), " ")

	var response *httptest.ResponseRecorder

	if method == http.MethodGet {
		query := make([]string, 0, len(input))

		for key, value := range input {
			query = append(query, key+"="+value.(string))
		}

		response = server.admin.call(method, name+"?"+strings.Join(query, "&"), "")
	} else {
		body, err := json.Marshal(input)

		if err != nil {
			server.t.Fatal(err)
		}

		response = server.admin.call(method, name, string(body))
	}

	if output != nil {
		if err := json.Unmarshal(response.Body.Bytes(), output); err != nil {
			server.t.Fatalf("%s: %d %s", name, response.Code, response.Body)
		}
	}

	return response.Code
}

// must sends a request that has to succeed.
func (server *notificationsServer) must(name string, input map[string]any, output any) {
	server.t.Helper()

	var raw json.RawMessage

	if code := server.send(name, input, &raw); code != http.StatusOK {
		server.t.Fatalf("%s %v: %d %s", name, input, code, raw)
	}

	if output != nil {
		if err := json.Unmarshal(raw, output); err != nil {
			server.t.Fatal(err)
		}
	}
}

// refused sends a request that has to be refused as a form: 400 with the body.
func (server *notificationsServer) refused(name string, input map[string]any) api.ErrorBody {
	server.t.Helper()

	return server.refusedWith(http.StatusBadRequest, name, input)
}

// refusedWith is refused with another status: 409 for a conflict.
func (server *notificationsServer) refusedWith(status int, name string, input map[string]any) api.ErrorBody {
	server.t.Helper()

	var body api.ErrorBody

	if code := server.send(name, input, &body); code != status {
		server.t.Fatalf("%s %v: %d %+v, want %d", name, input, code, body, status)
	}

	return body
}

func endpointNamed(t *testing.T, name string) api.Endpoint {
	t.Helper()

	for _, endpoint := range api.Endpoints(nil, nil) {
		if endpoint.Name == name {
			return endpoint
		}
	}

	t.Fatalf("no endpoint %s", name)

	return api.Endpoint{}
}

// webhook is a fake Slack incoming webhook that keeps the posted bodies.
type webhook struct {
	mutex  sync.Mutex
	bodies []string
	url    string
	// rateLimited answers that many requests with 429 and a Retry-After of an hour.
	rateLimited int
}

func newWebhook(t *testing.T) *webhook {
	t.Helper()

	hook := &webhook{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		hook.mutex.Lock()
		defer hook.mutex.Unlock()

		hook.bodies = append(hook.bodies, string(body))

		if hook.rateLimited > 0 {
			hook.rateLimited--

			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}))
	t.Cleanup(server.Close)
	hook.url = server.URL + "/services/T0/B0/webhooksecretsegment"

	return hook
}

func (hook *webhook) count() int {
	hook.mutex.Lock()
	defer hook.mutex.Unlock()

	return len(hook.bodies)
}

type connectionRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	Active      bool   `json:"active"`
	Delivery    *struct {
		Retrying  int    `json:"retrying"`
		LastError string `json:"last_error"`
		Failing   bool   `json:"failing"`
	} `json:"delivery"`
}

type connectionDetails struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	VerifiedAt         string   `json:"verified_at"`
	AllProjects        bool     `json:"all_projects"`
	ProjectIDs         []int    `json:"project_ids"`
	Levels             []string `json:"levels"`
	IncludeIssueLink   bool     `json:"include_issue_link"`
	ChatID             string   `json:"chat_id"`
	SlackChannel       string   `json:"slack_channel"`
	SavedTelegramToken string   `json:"saved_telegram_token"`
	SavedWebhookURL    string   `json:"saved_webhook_url"`
	SavedSlackBotToken string   `json:"saved_slack_bot_token"`
	SecretReadable     bool     `json:"secret_readable"`
	Retrying           int      `json:"retrying"`
}

func (server *notificationsServer) connections() map[string]connectionRow {
	server.t.Helper()

	var page struct {
		Items []connectionRow `json:"items"`
	}

	server.must("list_connections", nil, &page)

	rows := map[string]connectionRow{}

	for _, row := range page.Items {
		rows[row.Name] = row
	}

	return rows
}

func (server *notificationsServer) create(input map[string]any) string {
	server.t.Helper()

	var created connectionRow

	server.must("create_connection", input, &created)

	return created.ID
}

func (server *notificationsServer) details(id string) (connectionDetails, string) {
	server.t.Helper()

	var raw json.RawMessage

	server.must("get_connection", map[string]any{"connection_id": id}, &raw)

	var details connectionDetails

	if err := json.Unmarshal(raw, &details); err != nil {
		server.t.Fatal(err)
	}

	return details, string(raw)
}

const (
	telegramToken = "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	slackBotToken = "xoxb-1234567890-abcdefghij"
)

// The list says where each connection sends; the form reads one with its
// project ids and its saved tokens masked, never whole.
func TestUIConnections(t *testing.T) {
	server := newNotificationsServer(t)
	hook := newWebhook(t)

	webhookID := server.create(map[string]any{"type": "slack", "name": "Team chat", "webhook_url": hook.url, "projects": []string{"1"}, "levels": []string{"error"}})
	telegramID := server.create(map[string]any{"type": "telegram", "name": "Ops", "bot_token": telegramToken, "chat_id": "-100"})
	botID := server.create(map[string]any{"type": "slack", "name": "Bot", "slack_bot_token": slackBotToken, "slack_channel": "C0123456789"})

	rows := server.connections()

	for name, want := range map[string]string{"Team chat": "incoming webhook", "Ops": "chat -100", "Bot": "bot in C0123456789, prompt in thread"} {
		if rows[name].Destination != want {
			t.Errorf("%s sends to %q, want %q", name, rows[name].Destination, want)
		}
	}

	webhook, raw := server.details(webhookID)

	if webhook.AllProjects || !slices.Equal(webhook.ProjectIDs, []int{1}) || !slices.Equal(webhook.Levels, []string{"error"}) || !strings.HasSuffix(webhook.SavedWebhookURL, "/…") || strings.Contains(raw, hook.url) || strings.Contains(raw, path.Base(hook.url)) {
		t.Errorf("webhook connection: %s", raw)
	}

	telegram, raw := server.details(telegramID)

	if telegram.ChatID != "-100" || telegram.SavedTelegramToken == "" || strings.Contains(raw, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") || !telegram.SecretReadable || telegram.IncludeIssueLink {
		t.Errorf("telegram connection: %s", raw)
	}

	bot, raw := server.details(botID)

	if bot.SlackChannel != "C0123456789" || bot.SavedSlackBotToken != "xoxb-1234567890-…" || bot.SavedWebhookURL != "" || strings.Contains(raw, "abcdefghij") {
		t.Errorf("bot connection: %s", raw)
	}

	// Editing without a new URL keeps the stored one.
	server.must("update_connection", map[string]any{"connection_id": webhookID, "name": "Backend chat"}, nil)
	server.must("send_test_message", map[string]any{"connection_id": webhookID}, nil)

	if hook.count() != 1 {
		t.Fatalf("the kept webhook got %d messages", hook.count())
	}

	if code := server.send("get_connection", map[string]any{"connection_id": "zzzzzzzz"}, nil); code != http.StatusNotFound {
		t.Errorf("a missing connection: %d", code)
	}

	// A user's role may not read connections.
	user := server.webUser(store.RoleUser, false)

	if response := user.call(http.MethodGet, "get_connection?connection_id="+webhookID, ""); response.Code != http.StatusForbidden {
		t.Errorf("a user read a connection: %d", response.Code)
	}
}

// The form of a new connection: each type and what it lacks, the first
// available one chosen, the recorded levels, and issue links only with
// PUBLIC_URL.
func TestUIConnectionDefaults(t *testing.T) {
	type defaults struct {
		Types []struct {
			Type             string `json:"type"`
			Available        bool   `json:"available"`
			Reason           string `json:"reason"`
			IncludeIssueLink bool   `json:"include_issue_link"`
			NamePlaceholder  string `json:"name_placeholder"`
		} `json:"types"`
		DefaultType         string   `json:"default_type"`
		Levels              []string `json:"levels"`
		IssueLinksAvailable bool     `json:"issue_links_available"`
	}

	server := newNotificationsServer(t, func(settings *config.Config) {
		settings.Notifications.SMTP = config.SMTP{}
	})

	var got defaults

	server.must("get_connection_defaults", nil, &got)

	if len(got.Types) != 3 || got.Types[0].Type != config.ChannelEmail || got.Types[0].Available || got.Types[0].Reason != "set SMTP_HOST and EMAIL_FROM" {
		t.Errorf("types: %+v", got.Types)
	}

	if got.DefaultType != config.ChannelTelegram || !slices.Equal(got.Levels, []string{"fatal", "error"}) || got.IssueLinksAvailable {
		t.Errorf("defaults: %+v", got)
	}

	if !got.Types[0].IncludeIssueLink || got.Types[2].IncludeIssueLink || got.Types[2].NamePlaceholder != services.DefaultSlackName || got.Types[0].NamePlaceholder != "team" {
		t.Errorf("per type: %+v", got.Types)
	}

	linked := newNotificationsServer(t, func(settings *config.Config) { settings.Notifications.PublicURL = "https://errors.example.com" })

	linked.must("get_connection_defaults", nil, &got)

	if !got.IssueLinksAvailable || got.DefaultType != config.ChannelEmail || !slices.Equal(got.Levels, []string{"fatal", "error"}) {
		t.Errorf("with PUBLIC_URL and SMTP: %+v", got)
	}
}

// Issue links are on by default for email and off for chats; a connection
// created without the field gets its type's default.
func TestIssueLinkDefaults(t *testing.T) {
	server := newNotificationsServer(t)
	quiet, linked := newWebhook(t), newWebhook(t)

	server.create(map[string]any{"type": "slack", "name": "Public chat", "webhook_url": quiet.url})
	server.create(map[string]any{"type": "slack", "name": "Team chat", "webhook_url": linked.url, "include_issue_link": true})

	for name, want := range map[string]bool{"Public chat": false, "Team chat": true} {
		details, _ := server.details(server.connections()[name].ID)

		if details.IncludeIssueLink != want {
			t.Errorf("%s includes the link: %v", name, details.IncludeIssueLink)
		}
	}
}

// A chat is tested before it is saved; a saved one fills empty fields with
// its stored tokens. Not an agent's tool.
func TestUITestChat(t *testing.T) {
	server := newNotificationsServer(t)
	hook := newWebhook(t)

	type result struct {
		Sent    bool   `json:"sent"`
		Message string `json:"message"`
	}

	var got result

	server.must("test_chat", map[string]any{"type": "slack", "webhook_url": hook.url, "name": "Ops"}, &got)

	if !got.Sent || hook.count() != 1 || got.Message != "Test message sent: check the chat." {
		t.Fatalf("before saving: %+v, %d messages", got, hook.count())
	}

	if len(server.connections()) != 0 {
		t.Fatal("a test created a connection")
	}

	server.must("test_chat", map[string]any{"type": "telegram", "bot_token": "nope", "chat_id": "-100"}, &got)

	if got.Sent || !strings.Contains(got.Message, "bot token") {
		t.Errorf("invalid token: %+v", got)
	}

	server.must("test_chat", map[string]any{"type": "slack", "webhook_url": "http://127.0.0.1:1/services/x"}, &got)

	if got.Sent || !strings.HasPrefix(got.Message, "Not delivered: ") {
		t.Errorf("unreachable: %+v", got)
	}

	id := server.create(map[string]any{"type": "slack", "name": "Ops", "webhook_url": hook.url})

	server.must("test_chat", map[string]any{"connection_id": id}, &got)

	if !got.Sent || hook.count() != 2 {
		t.Errorf("saved connection: %+v, %d messages", got, hook.count())
	}

	if endpoint := endpointNamed(t, "test_chat"); endpoint.Serves(api.TransportMCP) || endpoint.Serves(api.TransportREST) || !endpoint.Admin {
		t.Error("test_chat is the web app's, for admins")
	}
}

var confirmationCode = regexp.MustCompile(`\b(\d{6})\b`)

// receiveCode waits for the next mail and returns its code and recipient commands.
func (server *notificationsServer) receiveCode() (code string, commands string) {
	server.t.Helper()

	select {
	case mail := <-server.mails:
		_, text, _ := fixtures.ParseEmail(server.t, mail.Data)

		return confirmationCode.FindString(text), strings.Join(mail.Commands, "\n")
	case <-time.After(5 * time.Second):
		server.t.Fatal("no confirmation email")

		return "", ""
	}
}

func (server *notificationsServer) expectNoMail(why string) {
	server.t.Helper()

	select {
	case <-server.mails:
		server.t.Fatal(why)
	case <-time.After(100 * time.Millisecond):
	}
}

type emailStep struct {
	Email      string `json:"email"`
	State      string `json:"state"`
	VerifiedAt string `json:"verified_at"`
	Notice     string `json:"notice"`
}

// The email step: a code, a wrong one, the right one; one connection per
// address, which stays confirmed after the connection is deleted.
func TestUIEmailSteps(t *testing.T) {
	server := newNotificationsServer(t)

	if body := server.refused("start_email_verification", map[string]any{"email": "not an address"}); body.Fields["email"] != "Enter a valid email address." {
		t.Errorf("invalid address: %+v", body)
	}

	var step emailStep

	server.must("start_email_verification", map[string]any{"email": "Ops@Example.COM"}, &step)

	if step.State != services.EmailCode || step.Email != "ops@example.com" || !strings.HasPrefix(step.Notice, "We sent a 6-digit code to ops@example.com") {
		t.Fatalf("after Verify: %+v", step)
	}

	code, commands := server.receiveCode()

	if !strings.Contains(commands, "RCPT TO:<ops@example.com>") {
		t.Fatalf("code sent to %s", commands)
	}

	// Saving before the code is confirmed is refused at the address.
	if body := server.refusedWith(http.StatusConflict, "create_connection", map[string]any{"type": "email", "email": "ops@example.com"}); body.Fields["email"] == "" {
		t.Errorf("save unconfirmed: %+v", body)
	}

	wrongCode := "000000"

	if code == wrongCode {
		wrongCode = "111111"
	}

	if body := server.refused("confirm_email", map[string]any{"email": "ops@example.com", "code": wrongCode}); body.Fields["code"] != "Wrong code." {
		t.Errorf("wrong code: %+v", body)
	}

	server.must("confirm_email", map[string]any{"email": "ops@example.com", "code": code}, &step)

	if step.State != services.EmailVerified || step.VerifiedAt == "" || step.Notice != "Address confirmed." {
		t.Fatalf("right code: %+v", step)
	}

	id := server.create(map[string]any{"type": "email", "email": "ops@example.com", "name": "Ops"})

	if details, _ := server.details(id); details.VerifiedAt == "" {
		t.Error("the form must say when the address was confirmed")
	}

	// One connection per address: no code, the existing one named.
	if body := server.refusedWith(http.StatusConflict, "start_email_verification", map[string]any{"email": "OPS@example.com"}); body.Duplicate == nil || body.Duplicate.ID != id || body.Duplicate.Name != "Ops" {
		t.Errorf("an address already added: %+v", body)
	}

	server.expectNoMail("an address already added must not get a code")

	if body := server.refusedWith(http.StatusConflict, "create_connection", map[string]any{"type": "email", "email": "ops@example.com"}); body.Duplicate == nil || body.Duplicate.ID != id {
		t.Errorf("a second connection to the address: %+v", body)
	}

	// After deleting the connection the address stays confirmed: no new code.
	server.must("delete_connection", map[string]any{"connection_id": id}, nil)
	server.must("start_email_verification", map[string]any{"email": "ops@example.com"}, &step)

	if step.State != services.EmailVerified || step.Notice != "Confirmed earlier: no code needed." {
		t.Errorf("a confirmed address again: %+v", step)
	}

	server.expectNoMail("a confirmed address must not get another code")
}

// "Send the code again" within the cooldown mails nothing; after it a new
// code replaces the old one.
func TestResendCooldown(t *testing.T) {
	server := newNotificationsServer(t)

	var step emailStep

	server.must("start_email_verification", map[string]any{"email": "dev@example.com"}, &step)
	oldCode, _ := server.receiveCode()
	server.must("start_email_verification", map[string]any{"email": "dev@example.com"}, &step)

	if step.State != services.EmailCode || !strings.Contains(step.Notice, "send again in") {
		t.Errorf("immediate resend: %+v", step)
	}

	server.expectNoMail("a resend within the cooldown must not send mail")

	previous := services.ResendCooldown
	services.ResendCooldown = 0

	t.Cleanup(func() { services.ResendCooldown = previous })

	server.must("start_email_verification", map[string]any{"email": "dev@example.com"}, nil)
	newCode, _ := server.receiveCode()

	// Only the newest code works.
	if oldCode != newCode {
		server.refused("confirm_email", map[string]any{"email": "dev@example.com", "code": oldCode})
	}

	server.must("confirm_email", map[string]any{"email": "dev@example.com", "code": newCode}, &step)

	if step.State != services.EmailVerified {
		t.Errorf("new code: %+v", step)
	}
}

// A refused form names the problem of each field, and the connection that
// already sends there.
func TestUIConnectionFieldErrors(t *testing.T) {
	server := newNotificationsServer(t)
	hook := newWebhook(t)

	for field, input := range map[string]map[string]any{
		"name":            {"type": "slack", "webhook_url": hook.url, "name": strings.Repeat("x", 101)},
		"projects":        {"type": "slack", "webhook_url": hook.url, "all_projects": false, "projects": []string{}},
		"levels":          {"type": "slack", "webhook_url": hook.url, "levels": []string{}},
		"bot_token":       {"type": "telegram", "bot_token": "nope", "chat_id": "-100"},
		"chat_id":         {"type": "telegram", "bot_token": telegramToken, "chat_id": "nope nope"},
		"webhook_url":     {"type": "slack", "webhook_url": "hooks.slack.com"},
		"slack_channel":   {"type": "slack", "slack_bot_token": slackBotToken},
		"slack_bot_token": {"type": "slack", "slack_bot_token": "nope", "slack_channel": "C0123456789"},
		"type":            {"type": "discord"},
	} {
		if body := server.refused("create_connection", input); body.Fields[field] == "" {
			t.Errorf("%s: %+v", field, body)
		}
	}

	if len(server.connections()) != 0 {
		t.Fatal("a refused form created a connection")
	}

	// Projects by id or name; all_projects false with some is only those.
	id := server.create(map[string]any{"type": "slack", "name": "Chat", "webhook_url": hook.url, "all_projects": false, "projects": []string{"1", "frontend"}})

	if details, _ := server.details(id); details.AllProjects || len(details.ProjectIDs) != 2 {
		t.Errorf("projects: %+v", details)
	}

	if body := server.refusedWith(http.StatusConflict, "create_connection", map[string]any{"type": "slack", "webhook_url": hook.url}); body.Duplicate == nil || body.Duplicate.ID != id || body.Duplicate.Name != "Chat" {
		t.Errorf("the same webhook again: %+v", body)
	}

	// Saving a connection with its own webhook is fine; moving another one onto it is not.
	server.must("update_connection", map[string]any{"connection_id": id, "webhook_url": hook.url}, nil)

	otherID := server.create(map[string]any{"type": "slack", "name": "Other", "webhook_url": newWebhook(t).url})

	if body := server.refusedWith(http.StatusConflict, "update_connection", map[string]any{"connection_id": otherID, "webhook_url": hook.url}); body.Duplicate == nil || body.Duplicate.ID != id {
		t.Errorf("moving onto a used webhook: %+v", body)
	}
}

// A connection saved without a name gets one: the part of the address before
// "@", the name of the Telegram chat, or "Slack".
func TestConnectionDefaultNames(t *testing.T) {
	server := newNotificationsServer(t)
	ctx := context.Background()
	now := time.Now()

	if _, err := server.database.CreateDestination(ctx, store.Destination{Type: config.ChannelEmail, Address: "ops.team@example.com", Verified: true, VerifiedAt: now}, now); err != nil {
		t.Fatal(err)
	}

	for _, input := range []map[string]any{
		{"type": "email", "email": "ops.team@example.com"},
		{"type": "telegram", "bot_token": telegramToken, "chat_id": "-100"},
		{"type": "slack", "webhook_url": newWebhook(t).url},
	} {
		input["name"] = ""
		server.create(input)
	}

	rows := server.connections()

	for _, name := range []string{"ops.team", "Ops chat", "Slack"} {
		if _, ok := rows[name]; !ok {
			t.Errorf("no connection named %q: %v", name, rows)
		}
	}
}

// A Slack connection can use a bot instead of a webhook; a new webhook alone
// replaces a stored bot and its channel, a new bot token alone the webhook.
func TestSlackBotConnection(t *testing.T) {
	server := newNotificationsServer(t)
	id := server.create(map[string]any{"type": "slack", "name": "Bot chat", "slack_bot_token": slackBotToken, "slack_channel": "C0123456789"})

	if body := server.refusedWith(http.StatusConflict, "create_connection", map[string]any{"type": "slack", "slack_bot_token": slackBotToken, "slack_channel": "C0123456789"}); body.Duplicate == nil {
		t.Errorf("the same bot channel twice: %+v", body)
	}

	hook := newWebhook(t)
	server.must("update_connection", map[string]any{"connection_id": id, "webhook_url": hook.url, "slack_channel": "C0123456789"}, nil)

	if details, _ := server.details(id); details.SavedSlackBotToken != "" || details.SlackChannel != "" || details.SavedWebhookURL == "" {
		t.Errorf("the bot is still stored: %+v", details)
	}

	server.must("send_test_message", map[string]any{"connection_id": id}, nil)

	if hook.count() != 1 {
		t.Error("the test message must go through the webhook")
	}

	server.must("update_connection", map[string]any{"connection_id": id, "slack_bot_token": slackBotToken, "slack_channel": "C0123456789"}, nil)

	if details, _ := server.details(id); details.SavedWebhookURL != "" || details.SavedSlackBotToken != "xoxb-1234567890-…" {
		t.Errorf("the webhook is still stored: %+v", details)
	}
}

// The list's switch changes only active: it turns off a connection whose
// tokens cannot be read, which a save of the form refuses, and never turns
// on an unconfirmed address.
func TestSetConnectionActive(t *testing.T) {
	server := newNotificationsServer(t)
	ctx := context.Background()
	now := time.Now()
	destinationID, err := server.database.CreateDestination(ctx, store.Destination{Type: config.ChannelTelegram, Secret: "not encrypted", Verified: true, VerifiedAt: now}, now)

	if err != nil {
		t.Fatal(err)
	}

	broken := store.NewConnection(config.ChannelTelegram)
	broken.DestinationID, broken.Name = destinationID, "Broken"
	brokenID, err := server.database.CreateConnection(ctx, broken, now)

	if err != nil {
		t.Fatal(err)
	}

	if rows := server.connections(); !strings.Contains(rows["Broken"].Destination, "cannot be decrypted") {
		t.Errorf("destination: %+v", rows["Broken"])
	}

	if details, _ := server.details(brokenID); details.SecretReadable || details.SavedTelegramToken != "" {
		t.Errorf("unreadable secret: %+v", details)
	}

	server.refused("update_connection", map[string]any{"connection_id": brokenID, "active": false})
	server.must("set_connection_active", map[string]any{"connection_id": brokenID, "active": false}, nil)

	if server.connections()["Broken"].Active {
		t.Error("still active")
	}

	server.must("set_connection_active", map[string]any{"connection_id": brokenID, "active": true}, nil)

	if !server.connections()["Broken"].Active {
		t.Error("not active again")
	}
}

// The delivery log: counts that match the list under the filters, summaries
// with their issues, retrying messages, and the health of each connection.
func TestUIDeliveries(t *testing.T) {
	server := newNotificationsServer(t)
	brokenID := server.create(map[string]any{"type": "slack", "name": "Broken chat", "webhook_url": newWebhook(t).url})
	workingID := server.create(map[string]any{"type": "slack", "name": "Working chat", "webhook_url": newWebhook(t).url})

	ctx := context.Background()
	now := time.Now()
	notification := `{"Kind":"new_issue","Project":"backend","Title":"Error: queue is full","IssueID":"issue007"}`
	messages := map[string]int64{}

	for _, channel := range []string{brokenID, workingID} {
		if err := server.database.EnqueueOutbox(ctx, []string{channel}, []int{1}, "notification", []byte(notification), now); err != nil {
			t.Fatal(err)
		}

		messages[channel] = server.newest(channel)
	}

	summary := `{"Count":12,"Window":60000000000,"Items":[{"Kind":"new_issue","Project":"backend","Title":"Error: disk is full","IssueID":"issue008"}]}`

	if err := server.database.EnqueueOutbox(ctx, []string{workingID}, []int{1}, "summary", []byte(summary), now); err != nil {
		t.Fatal(err)
	}

	if err := server.database.MarkOutboxRetry(ctx, messages[brokenID], "slack: 500 internal_error", now.Add(time.Hour), now); err != nil {
		t.Fatal(err)
	}

	if err := server.database.MarkOutboxSent(ctx, messages[workingID], now); err != nil {
		t.Fatal(err)
	}

	rows := server.connections()

	if broken := rows["Broken chat"]; broken.Delivery == nil || !broken.Delivery.Failing || broken.Delivery.Retrying != 1 || broken.Delivery.LastError != "slack: 500 internal_error" {
		t.Errorf("broken: %+v", broken.Delivery)
	}

	if working := rows["Working chat"]; working.Delivery == nil || working.Delivery.Failing {
		t.Errorf("working: %+v", working.Delivery)
	}

	if details, _ := server.details(brokenID); details.Retrying != 1 {
		t.Errorf("retrying: %d", details.Retrying)
	}

	type delivery struct {
		ConnectionID string `json:"connection_id"`
		Message      string `json:"message"`
		Title        string `json:"title"`
		IssueID      string `json:"issue_id"`
		Status       string `json:"status"`
		Retrying     bool   `json:"retrying"`
		UpdatedAt    string `json:"updated_at"`
		Items        []struct {
			Title   string `json:"title"`
			IssueID string `json:"issue_id"`
		} `json:"items"`
		More int `json:"more"`
	}

	var page struct {
		Items []delivery `json:"items"`
	}

	var count struct {
		Count int `json:"count"`
	}

	for _, filter := range []map[string]any{{}, {"status": "sent"}, {"connection_id": workingID}, {"connection_id": brokenID, "status": "pending"}} {
		server.must("list_deliveries", filter, &page)
		server.must("count_deliveries", filter, &count)

		if count.Count != len(page.Items) || count.Count == 0 {
			t.Errorf("%v: count %d, listed %d", filter, count.Count, len(page.Items))
		}
	}

	server.must("list_deliveries", map[string]any{}, &page)

	byID := map[string][]delivery{}

	for _, row := range page.Items {
		byID[row.ConnectionID] = append(byID[row.ConnectionID], row)
	}

	if broken := byID[brokenID]; len(broken) != 1 || !broken[0].Retrying || broken[0].IssueID != "issue007" {
		t.Errorf("broken: %+v", broken)
	}

	// Newest first: the summary, then the sent message.
	if working := byID[workingID]; len(working) != 2 || working[0].Message != "Summary" || len(working[0].Items) != 1 || working[0].Items[0].IssueID != "issue008" ||
		working[0].More != 11 || working[1].Status != store.OutboxSent || working[1].Retrying || working[1].UpdatedAt == "" {
		t.Errorf("working: %+v", working)
	}

	server.must("list_deliveries", map[string]any{"cursor": "100"}, &page)

	if len(page.Items) != 0 {
		t.Errorf("past the end: %+v", page.Items)
	}

	server.must("retry_connection", map[string]any{"connection_id": brokenID}, nil)

	if due, _ := server.database.DueOutbox(ctx, brokenID, time.Now().Add(time.Second), 10); len(due) != 1 {
		t.Error("the waiting message must be due now")
	}
}

// newest is the id of the newest outbox message of a connection.
func (server *notificationsServer) newest(connectionID string) int64 {
	server.t.Helper()

	messages, err := server.database.ListOutbox(context.Background(), store.OutboxFilter{Channel: connectionID, Limit: 1})

	if err != nil || len(messages) == 0 {
		server.t.Fatalf("no queued message: %v", err)
	}

	return messages[0].ID
}

// A rate limit pauses a connection only until it is fixed: a save or Retry
// now sends its waiting messages at once, not after the Retry-After.
func TestFixEndsRateLimitPause(t *testing.T) {
	for _, fix := range []string{"retry_connection", "update_connection"} {
		t.Run(fix, func(t *testing.T) {
			server := newNotificationsServer(t)
			hook := newWebhook(t)
			hook.mutex.Lock()
			hook.rateLimited = 1
			hook.mutex.Unlock()

			id := server.create(map[string]any{"type": "slack", "name": "Chat", "webhook_url": hook.url})

			server.ingestFixtures()
			waitUntil(t, "the rate-limited attempt", func() bool { return hook.count() == 1 })

			server.must(fix, map[string]any{"connection_id": id}, nil)
			waitUntil(t, "the waiting messages", func() bool { return hook.count() > 1 })
		})
	}
}

// waitUntil fails the test when condition does not hold within 5 seconds.
func waitUntil(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// The link in an email stops the emails to its address: GET asks first (mail
// scanners open every link), POST, the button or a provider's one-click
// request, pauses every connection sending there. It is open even behind
// ALLOWED_IPS, since providers send it from their servers; a wrong token
// changes nothing.
func TestUnsubscribeLink(t *testing.T) {
	server := newNotificationsServer(t, func(settings *config.Config) {
		settings.Notifications.PublicURL = "https://errors.example.com"
		settings.AllowedIPs = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")} // httptest's peer: the admin
	})

	var step emailStep

	server.must("start_email_verification", map[string]any{"email": "ops@example.com"}, &step)

	code, _ := server.receiveCode()

	server.must("confirm_email", map[string]any{"email": "ops@example.com", "code": code}, &step)

	first := server.create(map[string]any{"type": "email", "email": "ops@example.com", "name": "Ops"})
	connection, err := server.database.GetConnection(context.Background(), first)

	if err != nil {
		t.Fatal(err)
	}

	link := notify.NewUnsubscriber(notificationsKey).URL("https://errors.example.com", connection.DestinationID)
	path := strings.TrimPrefix(link, "https://errors.example.com")
	open := func(method, path string) (int, string) {
		request := httptest.NewRequest(method, path, nil)
		request.RemoteAddr = "203.0.113.5:40000" // a mail provider, outside ALLOWED_IPS
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)

		return response.Code, response.Body.String()
	}

	if status, body := open(http.MethodGet, path); status != http.StatusOK || !strings.Contains(body, "<button type=\"submit\">Stop these emails</button>") {
		t.Fatalf("GET: %d %s", status, body)
	}

	if connection, _ := server.database.GetConnection(context.Background(), first); !connection.Active {
		t.Fatal("opening the link paused the connection")
	}

	if status, _ := open(http.MethodPost, strings.Replace(path, "token=", "token=0", 1)); status != http.StatusNotFound {
		t.Errorf("a wrong token: %d", status)
	}

	if status, body := open(http.MethodPost, path); status != http.StatusOK || !strings.Contains(body, "Notifications to ops@example.com are paused") {
		t.Fatalf("POST: %d %s", status, body)
	}

	if connection, _ := server.database.GetConnection(context.Background(), first); connection.Active {
		t.Error("the connection is still active")
	}
}
