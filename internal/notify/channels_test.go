package notify

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/sentry"
)

var sample = Notification{
	Kind:        NewIssue,
	Project:     "backend",
	IssueID:     "issue042",
	IssueURL:    "https://errors.example.com/issues/issue042",
	Title:       "TypeError: Cannot read properties of undefined (reading 'id') <script>",
	Culprit:     "GET /api/orders",
	Level:       "error",
	Environment: "production",
	Time:        time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC),
}

// recordingServer answers every request with status and body and keeps the last request.
func recordingServer(t *testing.T, status int, headers map[string]string, body string) (*httptest.Server, *http.Request, *[]byte) {
	t.Helper()

	var (
		last    http.Request
		payload []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = *r
		payload, _ = io.ReadAll(r.Body)

		for name, value := range headers {
			w.Header().Set(name, value)
		}

		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)

	return server, &last, &payload
}

func TestSlack(t *testing.T) {
	server, _, payload := recordingServer(t, http.StatusOK, nil, "ok")
	slack := Slack{WebhookURL: server.URL}

	if err := slack.Send(context.Background(), sample); err != nil {
		t.Fatal(err)
	}

	var message struct{ Text string }

	if err := json.Unmarshal(*payload, &message); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"*New issue* in `backend` · error",
		"<https://errors.example.com/issues/issue042|TypeError: Cannot read properties of undefined (reading 'id') &lt;script&gt;>",
		"GET /api/orders · production",
		`Exceptic MCP: get_issue {"issue_id":"issue042"}`,
	} {
		if !strings.Contains(message.Text, want) {
			t.Errorf("missing %q in:\n%s", want, message.Text)
		}
	}

	if err := slack.SendSummary(context.Background(), Summary{Count: 12, Window: time.Minute, Items: []Notification{sample}}); err != nil {
		t.Fatal(err)
	}

	_ = json.Unmarshal(*payload, &message)

	if !strings.Contains(message.Text, "12 more issues in the last 1 minute") || !strings.Contains(message.Text, "…and 11 more") || !strings.Contains(message.Text, `get_issue {"issue_id":"issue042"}`) {
		t.Errorf("summary:\n%s", message.Text)
	}
}

func TestSummaryHeading(t *testing.T) {
	testCases := []struct {
		summary Summary
		want    string
	}{
		{Summary{Count: 1, Window: time.Minute}, "1 more issue in the last 1 minute"},
		{Summary{Count: 3, Window: 90*time.Second + time.Millisecond}, "3 more issues in the last 90 seconds"},
	}

	for _, testCase := range testCases {
		if got := testCase.summary.heading(); got != testCase.want {
			t.Errorf("heading = %q, want %q", got, testCase.want)
		}
	}
}

// Text from the monitored app is plain text in Slack and its look-alikes:
// no mentions or images, details capped as in Telegram, markup in the app
// name escaped.
func TestSlackKeepsTextPlain(t *testing.T) {
	server, _, payload := recordingServer(t, http.StatusOK, nil, "ok")
	notification := sample
	notification.Title = "@all look ![x](https://tracker.example/pixel.png)"
	notification.Release = strings.Repeat("r", 2000)

	if err := (Slack{WebhookURL: server.URL, AppName: "Errors <dev>"}).Send(context.Background(), notification); err != nil {
		t.Fatal(err)
	}

	var message struct{ Text string }

	if err := json.Unmarshal(*payload, &message); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(message.Text, "@all") || strings.Contains(message.Text, "![") || strings.Contains(message.Text, "<dev>") ||
		strings.Contains(message.Text, strings.Repeat("r", detailsLength)) || !strings.Contains(message.Text, "Errors &lt;dev&gt; MCP") {
		t.Errorf("message:\n%s", message.Text)
	}
}

func TestSlackRateLimit(t *testing.T) {
	server, _, _ := recordingServer(t, http.StatusTooManyRequests, map[string]string{"Retry-After": "7"}, "rate_limited")

	err := Slack{WebhookURL: server.URL}.Send(context.Background(), sample)

	var retryAfter *RetryAfterError

	if !errors.As(err, &retryAfter) || retryAfter.Delay != 7*time.Second {
		t.Fatalf("got %v, want a RetryAfterError of 7s", err)
	}
}

// The webhook URL is the secret: a failed request must not put it into the
// error, which is stored in the outbox, shown and logged.
func TestSlackErrorHidesWebhook(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	webhook := server.URL + "/services/T000/B000/webhook-secret"
	server.Close() // nothing listens: the request fails before an answer

	err := Slack{WebhookURL: webhook}.Send(context.Background(), sample)

	if err == nil || strings.Contains(err.Error(), "webhook-secret") {
		t.Fatalf("error %v, want one without the webhook", err)
	}
}

func TestTelegram(t *testing.T) {
	server, request, payload := recordingServer(t, http.StatusOK, nil, `{"ok":true}`)
	telegram := Telegram{BotToken: "123:SECRET", ChatID: "-100500", apiURL: server.URL}

	if err := telegram.Send(context.Background(), Notification{Kind: Regression, IssueID: "issue042", Project: "frontend", Title: "a < b", Level: "fatal"}); err != nil {
		t.Fatal(err)
	}

	if request.URL.Path != "/bot123:SECRET/sendMessage" {
		t.Fatalf("path = %s", request.URL.Path)
	}

	var message struct {
		ChatID    string `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}

	if err := json.Unmarshal(*payload, &message); err != nil {
		t.Fatal(err)
	}

	// No PUBLIC_URL: no link. HTML is escaped because parse_mode is HTML.
	if message.ChatID != "-100500" || message.ParseMode != "HTML" ||
		!strings.Contains(message.Text, "<b>Regression</b> in <code>frontend</code> · fatal") || !strings.Contains(message.Text, "<b>a &lt; b</b>") ||
		!strings.Contains(html.UnescapeString(message.Text), `Exceptic MCP: get_issue {"issue_id":"issue042"}`) {
		t.Fatalf("message: %+v", message)
	}
}

// An error is one message: the agent prompt as a file, the alert as its caption.
func TestTelegramPromptFile(t *testing.T) {
	type call struct {
		method, chatID, caption, parseMode, fileName, file string
	}

	var (
		calls      []call
		fileStatus = http.StatusOK
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		if method == "sendMessage" {
			var message struct {
				Text string `json:"text"`
			}

			_ = json.NewDecoder(r.Body).Decode(&message)
			calls = append(calls, call{method: method, caption: message.Text})
			_, _ = io.WriteString(w, `{"ok":true}`)

			return
		}

		recorded := call{method: method, chatID: r.FormValue("chat_id"), caption: r.FormValue("caption"), parseMode: r.FormValue("parse_mode")}

		if file, header, err := r.FormFile("document"); err == nil {
			data, _ := io.ReadAll(file)
			recorded.fileName, recorded.file = header.Filename, string(data)
		}

		calls = append(calls, recorded)

		if fileStatus != http.StatusOK {
			w.WriteHeader(fileStatus)
			_, _ = io.WriteString(w, `{"ok":false,"description":"file refused"}`)

			return
		}

		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	telegram := Telegram{BotToken: "123:SECRET", ChatID: "-100500", apiURL: server.URL}
	notification := sample
	notification.Culprit = strings.Repeat("x", 2000)
	notification.Event = &sentry.Event{Exception: sentry.ExceptionList{{Type: "TypeError", Value: "Cannot read properties of undefined"}}}

	if err := telegram.Send(context.Background(), notification); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 1 {
		t.Fatalf("calls: %+v, want one message with the file", calls)
	}

	document := calls[0]

	if document.method != "sendDocument" || document.chatID != "-100500" || document.fileName != "backend-error-issue042.md" || document.parseMode != "HTML" ||
		!strings.Contains(document.caption, "<b>New issue</b> in <code>backend</code>") ||
		!strings.Contains(html.UnescapeString(document.caption), `Exceptic MCP: get_issue {"issue_id":"issue042"}`) || strings.Contains(document.caption, promptCaption) ||
		!strings.Contains(document.file, "Do not change any code") || !strings.Contains(document.file, "Cannot read properties of undefined") {
		t.Errorf("document: %+v", document)
	}

	if length := len([]rune(document.caption)); length > 1024 {
		t.Errorf("caption has %d characters, Telegram takes 1024", length)
	}

	// A refused file does not lose the alert: it goes as a plain message.
	calls, fileStatus = nil, http.StatusBadRequest

	if err := telegram.Send(context.Background(), notification); err != nil || len(calls) != 2 || calls[1].method != "sendMessage" || !strings.Contains(html.UnescapeString(calls[1].caption), `get_issue {"issue_id":"issue042"}`) {
		t.Errorf("refused file: %v, %+v", err, calls)
	}

	// A server error is retried with the file, not sent without it.
	calls, fileStatus = nil, http.StatusBadGateway

	if err := telegram.Send(context.Background(), notification); err == nil || len(calls) != 1 {
		t.Errorf("server error: %v, %+v", err, calls)
	}

	fileStatus = http.StatusOK

	// A test message (no event) and a summary are plain messages.
	calls = nil
	_ = telegram.Send(context.Background(), sample)
	_ = telegram.SendSummary(context.Background(), Summary{Count: 2, Window: time.Minute, Items: []Notification{sample}})

	if len(calls) != 2 || calls[0].method != "sendMessage" || calls[1].method != "sendMessage" || !strings.Contains(html.UnescapeString(calls[1].caption), `get_issue {"issue_id":"issue042"}`) {
		t.Errorf("without an event: %+v", calls)
	}
}

func TestTelegramErrors(t *testing.T) {
	server, _, _ := recordingServer(t, http.StatusTooManyRequests, nil, `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":5}}`)

	err := Telegram{BotToken: "123:SECRET", ChatID: "1", apiURL: server.URL}.Send(context.Background(), sample)

	var retryAfter *RetryAfterError

	if !errors.As(err, &retryAfter) || retryAfter.Delay != 5*time.Second {
		t.Fatalf("got %v, want a RetryAfterError of 5s", err)
	}

	// A connection error must not print the URL, which contains the bot token.
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()

	err = Telegram{BotToken: "123:SECRET", ChatID: "1", apiURL: unreachable.URL}.Send(context.Background(), sample)

	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("error leaks the token or is missing: %v", err)
	}
}

// slackAPI is a fake Slack Web API that keeps every chat.postMessage call.
type slackAPI struct {
	mutex sync.Mutex
	calls []slackCall
}

type slackCall struct {
	Authorization string
	Channel       string `json:"channel"`
	Text          string `json:"text"`
	Thread        string `json:"thread_ts"`
}

func newSlackAPI(t *testing.T, answer string) (*slackAPI, string) {
	t.Helper()

	api := &slackAPI{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call slackCall

		_ = json.NewDecoder(r.Body).Decode(&call)
		call.Authorization = r.Header.Get("Authorization")

		api.mutex.Lock()
		api.calls = append(api.calls, call)
		api.mutex.Unlock()

		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(server.Close)

	return api, server.URL
}

// With a bot the message stays short and the agent prompt goes in a thread
// under it.
func TestSlackBotPromptInThread(t *testing.T) {
	api, url := newSlackAPI(t, `{"ok":true,"ts":"1700000000.000100"}`)
	slack := Slack{BotToken: "xoxb-test-token", Channel: "C0123456789", apiURL: url}
	notification := sample
	notification.Event = &sentry.Event{Exception: sentry.ExceptionList{{Type: "TypeError", Value: "Cannot read properties of undefined"}}}

	if err := slack.Send(context.Background(), notification); err != nil {
		t.Fatal(err)
	}

	if len(api.calls) != 2 {
		t.Fatalf("%d calls, want the message and the prompt", len(api.calls))
	}

	message, prompt := api.calls[0], api.calls[1]

	if message.Authorization != "Bearer xoxb-test-token" || message.Channel != "C0123456789" || message.Thread != "" || strings.Contains(message.Text, "```") {
		t.Errorf("message: %+v", message)
	}

	if prompt.Thread != "1700000000.000100" || !strings.Contains(prompt.Text, "```") || !strings.Contains(prompt.Text, "Do not change any code") {
		t.Errorf("prompt: %+v", prompt)
	}

	// A test message (no event) has no prompt; a summary is one message.
	api.calls = nil
	_ = slack.Send(context.Background(), sample)
	_ = slack.SendSummary(context.Background(), Summary{Count: 2, Window: time.Minute, Items: []Notification{sample}})

	if len(api.calls) != 2 || api.calls[0].Thread != "" || api.calls[1].Thread != "" {
		t.Errorf("without an event: %+v", api.calls)
	}
}

// A prompt longer than a Slack message loses error data, never the end
// marker and the reminder after it.
func TestSlackPromptKeepsTheReminder(t *testing.T) {
	notification := sample
	notification.Event = &sentry.Event{}

	for range 30 {
		notification.Event.Breadcrumbs = append(notification.Event.Breadcrumbs, sentry.Breadcrumb{Message: strings.Repeat("a&", 1000)})
	}

	prompt := slackPrompt(notification)

	if length := len([]rune(prompt)); length > maxSlackText {
		t.Errorf("prompt has %d characters, over %d", length, maxSlackText)
	}

	body := strings.TrimSpace(strings.TrimSuffix(prompt, "```"))

	if !strings.Contains(prompt, "=== END ERROR DATA ") || !strings.HasSuffix(body, "point it out to me as suspicious.") {
		t.Errorf("the prompt does not end with the marker and the reminder:\n%s", body[max(0, len(body)-600):])
	}
}

// Slack answers 200 with ok:false: the error says why, and no thread follows.
func TestSlackBotError(t *testing.T) {
	api, url := newSlackAPI(t, `{"ok":false,"error":"not_in_channel"}`)
	slack := Slack{BotToken: "xoxb-test-token", Channel: "C0123456789", apiURL: url}
	notification := sample
	notification.Event = &sentry.Event{}

	err := slack.Send(context.Background(), notification)

	if err == nil || !strings.Contains(err.Error(), "not_in_channel") || strings.Contains(err.Error(), "xoxb") || len(api.calls) != 1 {
		t.Fatalf("err %v, %d calls", err, len(api.calls))
	}
}

// Setup errors say what to fix.
func TestSlackBotErrorHints(t *testing.T) {
	_, url := newSlackAPI(t, `{"ok":false,"error":"missing_scope","needed":"chat:write"}`)
	slack := Slack{BotToken: "xoxb-test-token", Channel: "C0123456789", apiURL: url}

	if err := slack.Send(context.Background(), sample); err == nil || !strings.Contains(err.Error(), "lacks the chat:write scope") || !strings.Contains(err.Error(), "Reinstall to Workspace") {
		t.Fatalf("err %v", err)
	}
}

// A chat summary keeps its issues apart: each a numbered block with its MCP
// request under it, an empty line between blocks.
func TestChatSummariesSeparateIssues(t *testing.T) {
	second := sample
	second.IssueID, second.IssueURL, second.Title = "issue043", "https://errors.example.com/issues/issue043", "RangeError: Invalid time value"
	summary := Summary{Count: 2, Window: time.Minute, Items: []Notification{sample, second}}

	server, _, payload := recordingServer(t, http.StatusOK, nil, "ok")

	if err := (Slack{WebhookURL: server.URL, AppName: "Exceptic"}).SendSummary(context.Background(), summary); err != nil {
		t.Fatal(err)
	}

	var slack struct{ Text string }

	_ = json.Unmarshal(*payload, &slack)

	telegramServer, _, telegramPayload := recordingServer(t, http.StatusOK, nil, `{"ok":true,"result":{}}`)

	if err := (Telegram{BotToken: "1:token", ChatID: "-100", AppName: "Exceptic", apiURL: telegramServer.URL}).SendSummary(context.Background(), summary); err != nil {
		t.Fatal(err)
	}

	var telegram struct{ Text string }

	_ = json.Unmarshal(*telegramPayload, &telegram)

	for name, text := range map[string]string{"slack": slack.Text, "telegram": telegram.Text} {
		blocks := strings.Split(text, "\n\n")

		if len(blocks) != 4 || !strings.Contains(blocks[1], "1.") || !strings.Contains(blocks[1], "issue042") || strings.Contains(blocks[1], "issue043") ||
			!strings.Contains(blocks[2], "2.") || !strings.Contains(blocks[2], "issue043") || strings.Contains(blocks[2], "issue042") {
			t.Errorf("%s summary, blocks %d:\n%s", name, len(blocks), text)
		}
	}
}
