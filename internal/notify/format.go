package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/platform/config"
)

// Wording shared by every channel; email's long format is in email.go.

func (kind Kind) label() string {
	switch kind {
	case Regression:
		return "Regression"
	case Test:
		return "Test message"
	default:
		return "New issue"
	}
}

// details is the second line of a short message: where and in which environment.
func (notification Notification) details() string {
	var parts []string

	for _, part := range []string{notification.Culprit, notification.Environment, notification.Release} {
		if part != "" {
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, " · ")
}

// mcpReference is a short, copyable request an agent can use without the
// attached prompt (see agentprompt.MCPReference); none for a test message.
// It works even when the panel's links are hidden.
func mcpReference(appName string, notification Notification) string {
	if notification.IssueID == "" || notification.Kind == Test {
		return ""
	}

	return agentprompt.MCPReference(appNameOr(appName), notification.IssueID)
}

// appNameOr is APP_NAME as channels got it, or the default for channels
// built without it.
func appNameOr(appName string) string {
	if appName == "" {
		return config.DefaultAppName
	}

	return appName
}

func (summary Summary) heading() string {
	issues := "issues"

	if summary.Count == 1 {
		issues = "issue"
	}

	heading := fmt.Sprintf("%d more %s in the last %s", summary.Count, issues, config.HumanDuration(summary.Window.Round(time.Second)))

	if summary.Project != "" {
		heading = summary.Project + ": " + heading
	}

	return heading
}

// note explains why the issues were not sent one by one.
func (summary Summary) note() string {
	more := summary.Count - len(summary.Items)
	note := "Sent together to avoid flooding the chat."

	if more > 0 {
		note = fmt.Sprintf("…and %d more. %s", more, note)
	}

	return note
}

// How much of free text a message keeps.
const (
	titleLength      = 300 // the title of the issue a message is about
	shortTitleLength = 150 // a title in a summary list or an email subject
	detailsLength    = 500
	fieldLength      = 200  // environment, release and the short fields of an email
	exceptionLength  = 2000 // an exception value or a message in an email
	breadcrumbLength = 300
)

// Description is what the delivery log shows of an outbox message.
type Description struct {
	Label   string // "New issue", "Regression", "Test message", "Summary" or a notice's label
	Project string
	Title   string // the issue; for a summary, how many issues it covers
	IssueID string
	Items   []Description // a summary's first issues
	More    int           // issues of a summary not in Items
}

// Describe reads an outbox message (kind "notification", "summary" or
// "notice") for display. A payload that does not parse gives what could be read.
func Describe(kind string, payload []byte) Description {
	if kind == kindNotice {
		var notice Notice

		_ = json.Unmarshal(payload, &notice)

		return Description{Label: notice.Label, Title: notice.Heading}
	}

	if kind == kindSummary {
		var summary Summary

		_ = json.Unmarshal(payload, &summary)

		description := Description{Label: "Summary", Title: summary.heading(), More: max(summary.Count-len(summary.Items), 0)}

		for _, item := range summary.Items {
			description.Items = append(description.Items, describeNotification(item))
		}

		return description
	}

	var notification Notification

	_ = json.Unmarshal(payload, &notification)

	return describeNotification(notification)
}

func describeNotification(notification Notification) Description {
	return Description{
		Label: notification.Kind.label(), Project: notification.Project,
		Title: notification.Title, IssueID: notification.IssueID,
	}
}
