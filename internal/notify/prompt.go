package notify

import (
	"cmp"
	"fmt"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/platform/textutil"
)

// agentPrompt is the prompt for a coding agent that Telegram and email attach
// to an error; the Slack bot builds its own from the same input (slackPrompt).
func agentPrompt(notification Notification) string {
	return agentprompt.BuildWithin(promptInput(notification), promptLength, func(text string) string { return text })
}

func promptInput(notification Notification) agentprompt.Input {
	return agentprompt.Input{
		Project: notification.Project, Title: notification.Title, Level: notification.Level, Culprit: notification.Culprit,
		Kind: notification.Kind.label(), Event: notification.Event,
	}
}

// promptCaption introduces the prompt in its Slack thread.
const promptCaption = "Prompt for a coding agent: it reproduces and explains the error without changing code."

// promptFileName names the prompt file after the issue, such as
// "backend-error-k3x9q2mf.md", so prompts saved from several messages do not
// overwrite each other. Project names may hold anything, hence the slugs.
func promptFileName(notification Notification) string {
	project := cmp.Or(textutil.Slug(notification.Project), "project")
	level := cmp.Or(textutil.Slug(notification.Level), "error")

	return fmt.Sprintf("%s-%s-%s.md", project, level, notification.IssueID)
}
