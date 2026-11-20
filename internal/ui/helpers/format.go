// Package helpers holds the pure formatting functions of the UI.
package helpers

import (
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
