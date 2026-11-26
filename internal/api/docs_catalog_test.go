package api

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// The table of the REST endpoints and the MCP tools (docs/api.md) and the
// list of the agents' skills (docs/mcp.md) are generated from the catalog and the
// skills' front matter, the one source of what agents and readers are told:
// a stale document fails, UPDATE_API_TYPES=1 (make api-types) rewrites it.
func TestDocsCatalog(t *testing.T) {
	endpoints := append(Endpoints(nil, nil), fetchSkill)
	blocks := []struct{ file, name, content string }{
		{"../../docs/mcp.md", "skills", skillList()},
		{"../../docs/api.md", "endpoints", endpointTable(endpoints)},
	}

	for _, block := range blocks {
		text, err := os.ReadFile(block.file)

		if err != nil {
			t.Fatal(err)
		}

		updated, err := replaceBlock(string(text), block.name, block.content)

		if err != nil {
			t.Fatalf("%s: %v", block.file, err)
		}

		if updated == string(text) {
			continue
		}

		if os.Getenv("UPDATE_API_TYPES") == "" {
			t.Errorf("%s: the %s block is stale; run make api-types", block.file, block.name)

			continue
		}

		if err := os.WriteFile(block.file, []byte(updated), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// replaceBlock puts content between <!-- name --> and <!-- /name -->.
func replaceBlock(text, name, content string) (string, error) {
	begin, end := "<!-- "+name+": generated from internal/api by make api-types -->\n", "<!-- /"+name+" -->"
	start, finish := strings.Index(text, begin), strings.Index(text, end)

	if start < 0 || finish < start {
		return "", fmt.Errorf("no %q … %q block", strings.TrimSpace(begin), end)
	}

	return text[:start+len(begin)] + content + text[finish:], nil
}

// endpointTable is one table of the endpoints of the REST API and the MCP
// tools, in the catalog's order: who may call them as a user (an agent; the
// REST token runs every REST endpoint) and through which transport.
func endpointTable(endpoints []Endpoint) string {
	var table strings.Builder

	table.WriteString("| Endpoint | Who | REST | MCP | Input (required first) | Output | |\n|---|---|---|---|---|---|---|\n")

	mark := func(served bool) string {
		if served {
			return "✓"
		}

		return ""
	}

	for _, endpoint := range endpoints {
		rest, mcp := endpoint.Serves(TransportREST), endpoint.Serves(TransportMCP)

		if !rest && !mcp {
			continue
		}

		name := "`" + endpoint.Name + "`"

		if rest {
			method := "POST"

			if endpoint.ReadOnly {
				method = "GET"
			}

			name = "`" + method + " " + endpoint.Name + "`"
		}

		who := "all"

		if endpoint.Admin {
			who = "admin"
		}

		fmt.Fprintf(&table, "| %s | %s | %s | %s | %s | %s | %s |\n", name, who, mark(rest), mark(mcp),
			inputNames(endpoint), outputFields(endpoint.output, 0), strings.ReplaceAll(endpoint.Description, "|", "\\|"))
	}

	return table.String()
}

// inputNames are an endpoint's inputs in backticks, the required ones first.
func inputNames(endpoint Endpoint) string {
	if endpoint.InputSchema == nil || len(endpoint.InputSchema.Properties) == 0 {
		return "—"
	}

	var required, optional []string

	for name := range endpoint.InputSchema.Properties {
		if slices.Contains(endpoint.InputSchema.Required, name) {
			required = append(required, "`"+name+"`")
		} else {
			optional = append(optional, "`"+name+"`")
		}
	}

	slices.Sort(required)
	slices.Sort(optional)

	return strings.Join(append(required, optional...), ", ")
}

// outputFields names the JSON fields of an output type, a list's or an
// object's own fields in brackets after its name, two levels deep.
func outputFields(output reflect.Type, depth int) string {
	switch {
	case output == nil:
		return "—"
	case output == reflect.TypeFor[Text]():
		return "text"
	case output == reflect.TypeFor[NoContent]():
		return "—"
	}

	fields := []string{}

	for _, field := range jsonFields(output) {
		element := field.Type

		for element.Kind() == reflect.Pointer || element.Kind() == reflect.Slice {
			element = element.Elem()
		}

		name := "`" + field.Name + "`"

		if depth == 0 && isRecord(element) {
			name += " [" + outputFields(element, depth+1) + "]"
		}

		fields = append(fields, name)
	}

	return strings.Join(fields, ", ")
}

// jsonFields are the fields a struct encodes, by their JSON names, embedded ones flattened.
func jsonFields(output reflect.Type) []reflect.StructField {
	for output.Kind() == reflect.Pointer {
		output = output.Elem()
	}

	if output.Kind() != reflect.Struct {
		return nil
	}

	var fields []reflect.StructField

	for field := range output.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")

		switch {
		case name == "-" || !field.IsExported() && !field.Anonymous:
			continue
		case field.Anonymous && name == "":
			fields = append(fields, jsonFields(field.Type)...)

			continue
		case name == "":
			name = field.Name
		}

		field.Name = name
		fields = append(fields, field)
	}

	return fields
}

// isRecord: a type encoded as an object of its own fields.
func isRecord(output reflect.Type) bool {
	return output.Kind() == reflect.Struct && output != reflect.TypeFor[time.Time]() && output != reflect.TypeFor[json.RawMessage]()
}

// skillList lists the skills fetch_skill loads, as their front matter names and describes them.
func skillList() string {
	var list strings.Builder

	for _, skill := range skills {
		fmt.Fprintf(&list, "- `%s`: %s\n", skill.name, skill.description)
	}

	return list.String()
}
