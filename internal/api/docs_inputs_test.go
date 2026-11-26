package api

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
)

// TestDocsInputs: the table of list_issues' input in docs/api.md is its
// schema (the descriptions agents get too); UPDATE_API_TYPES=1 (make
// api-types) rewrites a stale one.
func TestDocsInputs(t *testing.T) {
	const file = "../../docs/api.md"

	text, err := os.ReadFile(file)

	if err != nil {
		t.Fatal(err)
	}

	index := slices.IndexFunc(Endpoints(nil, nil), func(endpoint Endpoint) bool { return endpoint.Name == "list_issues" })
	updated, err := replaceBlock(string(text), "list-issues-input", inputTable(Endpoints(nil, nil)[index]))

	if err != nil {
		t.Fatal(err)
	}

	if updated != string(text) {
		if os.Getenv("UPDATE_API_TYPES") == "" {
			t.Errorf("%s: the list-issues-input block is stale; run make api-types", file)
		} else if err := os.WriteFile(file, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fixtures.DocValues(t, "docs/api.md",
		fixtures.DocValue{Phrase: "413: a body over {} KiB.", Value: maxBody >> 10},
		fixtures.DocValue{Phrase: "`limit` is 1–{}, default", Value: MaxLimit},
		fixtures.DocValue{Phrase: "`limit` is 1–100, default {}.", Value: DefaultLimit},
	)
	fixtures.DocValues(t, "docs/configuration.md",
		fixtures.DocValue{Phrase: "when they come to {}% of the limit or more", Value: arrivedPercent})
}

// inputTable is a table of an endpoint's inputs in the order of its input
// type: the description, the values it takes and its bounds.
func inputTable(endpoint Endpoint) string {
	var table strings.Builder

	table.WriteString("| Parameter | |\n|---|---|\n")

	for _, name := range endpoint.InputSchema.PropertyOrder {
		property := endpoint.InputSchema.Properties[name]
		text := property.Description
		values, many := property.Enum, false

		if property.Items != nil && len(property.Items.Enum) > 0 {
			values, many = property.Items.Enum, true
		}

		var names []string

		for _, value := range values {
			if value != "" {
				names = append(names, fmt.Sprintf("`%v`", value))
			}
		}

		switch {
		case len(names) > 0 && many:
			text += " Any of " + strings.Join(names, ", ") + "."
		case len(names) > 0:
			text += " One of " + strings.Join(names, ", ") + "."
		}

		if property.Maximum != nil {
			text += fmt.Sprintf(" At most %v", *property.Maximum)

			if property.Default != nil {
				text += fmt.Sprintf(", %s by default", property.Default)
			}

			text += "."
		}

		fmt.Fprintf(&table, "| `%s` | %s |\n", name, strings.ReplaceAll(text, "|", "\\|"))
	}

	return table.String()
}

// TestSkillPlaceholders: every placeholder of the instructions and the skills is filled in.
func TestSkillPlaceholders(t *testing.T) {
	texts := []string{serverInstructions("Exceptic")}

	for _, skill := range skills {
		texts = append(texts, skill.content)
	}

	for _, text := range texts {
		if strings.Contains(text, "{{") {
			t.Errorf("a placeholder is left: %.200q", text[strings.Index(text, "{{"):])
		}
	}
}
