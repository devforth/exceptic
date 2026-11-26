package config

import (
	"cmp"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/platform/textutil"
)

// DefaultAppName names the server when APP_NAME is not set.
const DefaultAppName = "Exceptic"

// MaxAppName is the longest APP_NAME, in characters.
const MaxAppName = 60

// parseAppName reads APP_NAME: the name shown in the sidebar, page titles,
// emails and to coding agents.
func parseAppName(value string) (string, error) {
	name := strings.Join(strings.Fields(value), " ")

	if name == "" {
		return DefaultAppName, nil
	}

	if utf8.RuneCountInString(name) > MaxAppName {
		return "", fmt.Errorf("APP_NAME: at most %d characters", MaxAppName)
	}

	return name, nil
}

// AppSlug is the name agents register the MCP server under: "Acme Errors" →
// "acme-errors", "exceptic" when no Latin letter or digit is left.
func AppSlug(appName string) string {
	return cmp.Or(textutil.Slug(appName), textutil.Slug(DefaultAppName))
}
