// Package helpers holds the pure formatting functions of the UI.
package helpers

import (
	"fmt"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
)

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
