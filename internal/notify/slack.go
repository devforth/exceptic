package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/server"
)

// Slack posts to an incoming webhook
// (https://api.slack.com/messaging/webhooks; Mattermost and Rocket.Chat
// accept the same {"text": ...} payload with the same link syntax), or, with
// a bot token, through chat.postMessage, which also lets the agent prompt go
// in a thread under the message: the message itself stays short.
type Slack struct {
	WebhookURL string
	BotToken   string
	Channel    string
	AppName    string // APP_NAME, in the MCP request of an issue
	// apiURL is replaced in tests with a fake server.
	apiURL string
	// privateWebhook lets the webhook be a private address: tests' fake servers only.
	privateWebhook bool
}

// webhookTimeout bounds a post to a webhook.
const webhookTimeout = 30 * time.Second

// webhookClient posts to the webhook URL a user typed: only to public
// addresses, without redirects (server.PublicClient), so a connection
// cannot make the server reach its own network.
func (slack Slack) webhookClient() *http.Client {
	if slack.privateWebhook {
		return &http.Client{Timeout: webhookTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}

	return server.PublicClient(webhookTimeout)
}

// maxSlackText keeps a message under Slack's 40,000-character limit.
const maxSlackText = 39000

func (slack Slack) Name() string { return config.ChannelSlack }

func (slack Slack) Send(ctx context.Context, notification Notification) error {
	icon := ":rotating_light:"

	if notification.Kind == Regression {
		icon = ":repeat:"
	}

	lines := []string{
		fmt.Sprintf("%s *%s* in `%s` · %s", icon, notification.Kind.label(), slackEscape(notification.Project), slackEscape(notification.Level)),
		"*" + slackLink(notification.IssueURL, textutil.Shorten(notification.Title, titleLength)) + "*",
	}

	if details := notification.details(); details != "" {
		lines = append(lines, slackText(textutil.Shorten(details, detailsLength)))
	}

	if reference := mcpReference(slack.AppName, notification); reference != "" {
		lines = append(lines, "`"+slackEscape(reference)+"`")
	}

	if slack.BotToken == "" {
		return slack.post(ctx, strings.Join(lines, "\n"))
	}

	timestamp, err := slack.postMessage(ctx, strings.Join(lines, "\n"), "")

	if err != nil || notification.Event == nil {
		return err
	}

	// The message is delivered; a failed prompt must not make the outbox
	// send it again (a duplicate in the channel), so its error is dropped.
	_, _ = slack.postMessage(ctx, slackPrompt(notification), timestamp)

	return nil
}

// slackPrompt is the agent prompt as a code block for the thread.
func slackPrompt(notification Notification) string {
	header := promptCaption + "\n"
	prompt := agentprompt.BuildWithin(promptInput(notification), maxSlackText-utf8.RuneCountInString(header)-6, func(text string) string {
		// A code block ends at the next three backticks: keep them out of it.
		return slackEscape(strings.ReplaceAll(text, "```", "ʼʼʼ"))
	})

	return header + "```" + prompt + "```"
}

func (slack Slack) SendSummary(ctx context.Context, summary Summary) error {
	lines := []string{":warning: *" + summary.heading() + "*"}

	// Each issue a numbered block of its own, its MCP request under it, an empty line between.
	for index, item := range summary.Items {
		lines = append(lines, "", fmt.Sprintf("*%d.* `%s` %s", index+1, slackEscape(item.Project), slackLink(item.IssueURL, textutil.Shorten(item.Title, shortTitleLength))))

		if reference := mcpReference(slack.AppName, item); reference != "" {
			lines = append(lines, "`"+slackEscape(reference)+"`")
		}
	}

	lines = append(lines, "", "_"+summary.note()+"_")

	if slack.BotToken != "" {
		_, err := slack.postMessage(ctx, strings.Join(lines, "\n"), "")

		return err
	}

	return slack.post(ctx, strings.Join(lines, "\n"))
}

func (slack Slack) SendNotice(ctx context.Context, notice Notice) error {
	lines := []string{":information_source: *" + slackEscape(notice.Heading) + "*"}

	for _, line := range notice.Lines {
		lines = append(lines, slackEscape(line))
	}

	if notice.Table != nil {
		lines = append(lines, "```"+slackEscape(strings.ReplaceAll(notice.Table.text(chatTableWidth), "```", "ʼʼʼ"))+"```")
	}

	if notice.URL != "" {
		lines = append(lines, slackLink(notice.URL, notice.LinkText))
	}

	if slack.BotToken != "" {
		_, err := slack.postMessage(ctx, strings.Join(lines, "\n"), "")

		return err
	}

	return slack.post(ctx, strings.Join(lines, "\n"))
}

// postMessage posts as the bot (https://api.slack.com/methods/chat.postMessage),
// in the thread of threadTimestamp when given, and returns the timestamp of
// the new message. Slack answers 200 with {"ok": false, "error": ...} on
// failures such as not_in_channel; the token travels in a header, so errors
// never contain it.
func (slack Slack) postMessage(ctx context.Context, text, threadTimestamp string) (string, error) {
	payload := map[string]any{"channel": slack.Channel, "text": text, "unfurl_links": false, "unfurl_media": false}

	if threadTimestamp != "" {
		payload["thread_ts"] = threadTimestamp
	}

	body, err := json.Marshal(payload)

	if err != nil {
		return "", err
	}

	apiURL := slack.apiURL

	if apiURL == "" {
		apiURL = "https://slack.com/api"
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL+"/chat.postMessage", bytes.NewReader(body))

	if err != nil {
		return "", err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Authorization", "Bearer "+slack.BotToken)

	response, err := http.DefaultClient.Do(request)

	if err != nil {
		return "", fmt.Errorf("slack: %w", stripURL(err))
	}

	defer func() { _ = response.Body.Close() }()

	var answer struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		Needed    string `json:"needed"` // the missing scope of missing_scope
		Timestamp string `json:"ts"`
	}

	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	_ = json.Unmarshal(data, &answer)

	if response.StatusCode == http.StatusTooManyRequests {
		return "", retryAfter(response, fmt.Errorf("slack: rate limited"))
	}

	if response.StatusCode != http.StatusOK || !answer.OK {
		return "", fmt.Errorf("slack: %d %s%s", response.StatusCode, answer.Error, slackErrorHint(answer.Error, answer.Needed))
	}

	return answer.Timestamp, nil
}

func (slack Slack) post(ctx context.Context, text string) error {
	body, err := json.Marshal(map[string]string{"text": text})

	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, slack.WebhookURL, bytes.NewReader(body))

	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	return doRequest(slack.webhookClient(), request)
}

// slackErrorHint says what to do about the errors a bot setup runs into,
// so the delivery log tells it without a trip to the Slack docs.
func slackErrorHint(code, needed string) string {
	switch code {
	case "missing_scope":
		return fmt.Sprintf(" (the bot token lacks the %s scope: add it under OAuth & Permissions → Bot Token Scopes, then Reinstall to Workspace)", needed)
	case "not_in_channel":
		return " (invite the bot to the channel: /invite @app)"
	case "channel_not_found":
		return " (check the channel ID; a private channel needs the bot invited)"
	case "invalid_auth", "not_authed", "account_inactive", "token_revoked":
		return " (the bot token is not valid: copy the Bot User OAuth Token again)"
	}

	return ""
}

// slackEscape escapes the three characters Slack treats as markup in text.
func slackEscape(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

// slackText escapes text outside code. Mattermost and Rocket.Chat also take a
// bare "@all" as a mention and render Markdown images: a word joiner after
// "@" and "!" keeps both plain text.
func slackText(text string) string {
	return strings.NewReplacer("@", "@\u2060", "![", "!\u2060[").Replace(slackEscape(text))
}

func slackLink(link, text string) string {
	if link == "" {
		return slackText(text)
	}

	return "<" + link + "|" + slackText(strings.ReplaceAll(text, "|", "¦")) + ">"
}

// doRequest sends an HTTP request and turns a non-2xx answer into an error;
// 429 becomes a RetryAfterError with the delay from Retry-After.
func doRequest(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)

	// The URL of a webhook is its secret: errors are stored, shown and logged.
	if err != nil {
		return stripURL(err)
	}

	defer func() { _ = response.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))

	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}

	err = fmt.Errorf("%s: %d %s", request.URL.Host, response.StatusCode, strings.TrimSpace(string(body)))

	if response.StatusCode == http.StatusTooManyRequests {
		return retryAfter(response, err)
	}

	return err
}

// retryAfter is err of a 429 answer with the delay of its Retry-After.
func retryAfter(response *http.Response, err error) *RetryAfterError {
	seconds, _ := strconv.Atoi(response.Header.Get("Retry-After"))

	return &RetryAfterError{Delay: time.Duration(seconds) * time.Second, Err: err}
}
