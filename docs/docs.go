// Package docs reads the user documentation the web app quotes as field hints
// (its generated help), so a field is explained in one place: the tables of
// configuration.md (environment variables) and ui.md (fields of the settings pages).
package docs

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

var (
	//go:embed configuration.md
	configuration string
	//go:embed ui.md
	ui string
)

var (
	// tableRow is a table row: | key | … | description |, the key in
	// backticks for a variable, plain for a field.
	tableRow     = regexp.MustCompile("^\\| `?([^|`]+?)`? \\|(?:[^|]*\\|)*?([^|]+)\\|\\s*$")
	heading      = regexp.MustCompile(`^## (.+)$`)
	markdownLink = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)

	settings = parseTables(configuration)
	fields   = parseTables(ui)
)

// Fields are what ui.md says of every field, by section and label, for the
// web app's generated help.
func Fields() map[string]map[string]string {
	result := map[string]map[string]string{}

	for name, text := range fields {
		section, label, _ := strings.Cut(name, "/")

		if result[section] == nil {
			result[section] = map[string]string{}
		}

		result[section][label] = text
	}

	return result
}

// Settings are what configuration.md says of every environment variable, by
// name, for the web app's generated help; an error when a name is described twice.
func Settings() (map[string]string, error) {
	result := map[string]string{}

	for name, text := range settings {
		_, variable, _ := strings.Cut(name, "/")

		if _, ok := result[variable]; ok {
			return nil, fmt.Errorf("docs/configuration.md describes %s twice", variable)
		}

		result[variable] = text
	}

	return result, nil
}

// parseTables reads the description of every table row, keyed by
// "section/key"; the header and separator rows are skipped.
func parseTables(markdown string) map[string]string {
	result := map[string]string{}
	section := ""

	for _, line := range strings.Split(markdown, "\n") {
		if match := heading.FindStringSubmatch(line); match != nil {
			section = strings.TrimSpace(match[1])

			continue
		}

		match := tableRow.FindStringSubmatch(line)

		if match == nil || strings.Trim(match[2], " -") == "" || match[1] == "Field" || match[1] == "Variable" {
			continue
		}

		text := markdownLink.ReplaceAllString(strings.TrimSpace(match[2]), "$1")
		result[section+"/"+strings.TrimSpace(match[1])] = strings.ReplaceAll(text, "`", "")
	}

	return result
}
