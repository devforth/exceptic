// Package helpers holds the pure formatting functions of the UI.
package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
)

// Short returns the first n bytes, for IDs shown in tables (free text: textutil.Shorten).
func Short(text string, n int) string {
	if len(text) <= n {
		return text
	}

	return text[:n]
}

// TimeAgo is a short relative time ("5m ago"). The exact time goes in a title attribute.
func TimeAgo(value time.Time) string {
	elapsed := time.Since(value)

	// A time to come (the next retry of a message) reads "in 4m".
	if elapsed < -time.Minute {
		return "in " + strings.TrimSuffix(TimeAgo(time.Now().Add(elapsed)), " ago") // the same distance back
	}

	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
	default:
		return value.UTC().Format("2006-01-02")
	}
}

// LevelClass maps Sentry and log levels to one of the CSS badge colors.
func LevelClass(level string) string {
	return "level-" + config.ShownLevel(level)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// CleanLog removes terminal color codes that loggers like pino add.
func CleanLog(text string) string {
	return ansiEscape.ReplaceAllString(text, "")
}

// PrettyJSON indents JSON for display; anything else is returned as is.
func PrettyJSON(raw json.RawMessage) string {
	var buffer bytes.Buffer

	if err := json.Indent(&buffer, raw, "", "  "); err != nil {
		return string(raw)
	}

	return buffer.String()
}

// HandledBadge says whether the SDK caught an exception, with the badge class.
type HandledBadge struct {
	Label, Class string
}

// Handled is the handled or unhandled badge of an exception; nil when the SDK does not say.
func Handled(mechanism *sentry.Mechanism) *HandledBadge {
	if mechanism == nil || mechanism.Handled == nil {
		return nil
	}

	if *mechanism.Handled {
		return &HandledBadge{Label: "handled", Class: LevelClass("debug")}
	}

	return &HandledBadge{Label: "unhandled", Class: LevelClass("error")}
}

// MaskSecret shows enough of a stored secret to recognize it, not to use
// it: the start up to the last "/" or ":" of its first part (the webhook
// host and path, the bot id), the type and ids of a Slack token, and the
// last 4 characters. Empty stays empty.
func MaskSecret(secret string) string {
	runes := []rune(secret)

	if len(runes) == 0 {
		return ""
	}

	if len(runes) <= 8 {
		return "…"
	}

	prefix := 0
	slackIDs := len(slackTokenIDs.FindString(secret))

	// Up to the last separator in the first half, so the secret part stays hidden;
	// "-" also appears in random text, so it counts only in the ids of a Slack token.
	for index, char := range runes[:len(runes)/2] {
		if char == '/' || char == ':' || (char == '-' && index < slackIDs) {
			prefix = index + 1
		}
	}

	return string(runes[:prefix]) + "…" + string(runes[len(runes)-4:])
}

var slackTokenIDs = regexp.MustCompile(`^xox[a-z]-(?:[0-9]+-)*`)
