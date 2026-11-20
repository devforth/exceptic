package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
)

// typescriptFile is the web app's view of the catalog, generated from it.
const typescriptFile = "../../web/src/lib/api/catalog.gen.ts"

// TestTypeScriptCatalog keeps the web app's types of the catalog in step
// with it: the endpoints the web app may call, their inputs and outputs from
// the Go types, and the values the app shows (choices, levels). With
// UPDATE_API_TYPES=1 (make api-types) it writes the file.
func TestTypeScriptCatalog(t *testing.T) {
	generated, err := typescriptCatalog(Endpoints(nil, nil))

	if err != nil {
		t.Fatal(err)
	}

	if os.Getenv("UPDATE_API_TYPES") != "" {
		if err := os.WriteFile(typescriptFile, []byte(generated), 0o644); err != nil {
			t.Fatal(err)
		}

		return
	}

	committed, err := os.ReadFile(typescriptFile)

	if err != nil {
		t.Fatal(err)
	}

	if string(committed) != generated {
		t.Fatalf("%s is stale: run make api-types", typescriptFile)
	}
}

// outputSchemas: the times as RFC 3339 strings, raw JSON as any value.
var outputSchemas = &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[time.Time]():       {Type: "string", Format: "date-time"},
	reflect.TypeFor[json.RawMessage](): {},
}}

func typescriptCatalog(endpoints []Endpoint) (string, error) {
	endpoints = slices.DeleteFunc(slices.Clone(endpoints), func(endpoint Endpoint) bool { return !endpoint.Serves(TransportUI) })
	slices.SortFunc(endpoints, func(a, b Endpoint) int { return strings.Compare(a.Name, b.Name) })

	var file strings.Builder

	file.WriteString("// Generated from the catalog of internal/api by TestTypeScriptCatalog; do not edit: make api-types.\n\n")
	file.WriteString("/** An RFC 3339 time in UTC. */\nexport type IsoTime = string;\n\n")
	fmt.Fprintf(&file, "/** Where the web app's requests go. */\nexport const apiPrefix = %s;\n\n", quote(server.UIAPIPrefix))
	fmt.Fprintf(&file, "/** The largest page of a list. */\nexport const maxLimit = %d;\n\n", MaxLimit)
	file.WriteString("/** Inputs and outputs of every request the web app may make. */\nexport interface Endpoints {\n")

	for _, endpoint := range endpoints {
		input, err := typescriptType(endpoint.InputSchema, "\t\t")

		if err != nil {
			return "", fmt.Errorf("%s input: %w", endpoint.Name, err)
		}

		output := "string"

		if endpoint.output != reflect.TypeFor[Text]() {
			schema, err := jsonschema.ForType(endpoint.output, outputSchemas)

			if err != nil {
				return "", fmt.Errorf("%s output: %w", endpoint.Name, err)
			}

			if output, err = typescriptType(schema, "\t\t"); err != nil {
				return "", fmt.Errorf("%s output: %w", endpoint.Name, err)
			}
		}

		fmt.Fprintf(&file, "\t%s: {\n\t\tin: %s;\n\t\tout: %s;\n\t};\n", endpoint.Name, input, output)
	}

	file.WriteString("}\n\n/** How each request is sent, and what the app must know before offering it. */\nexport const endpoints = {\n")

	for _, endpoint := range endpoints {
		method := "POST"

		if endpoint.ReadOnly {
			method = "GET"
		}

		fmt.Fprintf(&file, "\t%s: { method: '%s', text: %t, admin: %t, destructive: %t, confirm: %t },\n",
			endpoint.Name, method, endpoint.output == reflect.TypeFor[Text](), endpoint.Admin, endpoint.Destructive, endpoint.Confirm)
	}

	file.WriteString("} as const;\n\n/** The values input fields take. */\nexport const choices = {\n")

	for _, endpoint := range endpoints {
		if len(endpoint.Choices) == 0 {
			continue
		}

		fmt.Fprintf(&file, "\t%s: {\n", endpoint.Name)

		for _, field := range slices.Sorted(maps.Keys(endpoint.Choices)) {
			fmt.Fprintf(&file, "\t\t%s: %s,\n", field, quoteList(endpoint.Choices[field]))
		}

		file.WriteString("\t},\n")
	}

	file.WriteString("} as const;\n\n")
	fmt.Fprintf(&file, "/** Event and log levels, highest first, as the app lists them. */\nexport const levels = {\n\tevent: %s,\n\tlog: %s\n} as const;\n\n",
		quoteList(config.HighestFirst(config.Levels)), quoteList(config.HighestFirst(config.LogLevels)))
	file.WriteString("/** The color group a level is shown in. */\nexport const shownLevel = {\n")

	var known []string

	for _, level := range append(slices.Clone(config.Levels), config.LogLevels...) {
		if !slices.Contains(known, level) {
			known = append(known, level)
			fmt.Fprintf(&file, "\t%s: %s,\n", level, quote(config.ShownLevel(level)))
		}
	}

	file.WriteString("} as const;\n")

	return file.String(), nil
}

// supportedKeywords are what typescriptType turns into types or comments (description, default; minimum and maximum it leaves to the server); any other fails the test.
var supportedKeywords = []string{"type", "enum", "items", "properties", "required", "additionalProperties", "description", "default", "minimum", "maximum", "format"}

func typescriptType(schema *jsonschema.Schema, indent string) (string, error) {
	encoded, err := json.Marshal(schema)

	if err != nil {
		return "", err
	}

	var keywords map[string]json.RawMessage

	if err := json.Unmarshal(encoded, &keywords); err != nil {
		return "", err
	}

	for keyword := range keywords {
		if !slices.Contains(supportedKeywords, keyword) {
			return "", fmt.Errorf("unsupported schema keyword %q", keyword)
		}
	}

	if len(schema.Enum) > 0 {
		var values []string

		for _, value := range schema.Enum {
			text, ok := value.(string)

			if !ok {
				return "", fmt.Errorf("enum value %v is not a string", value)
			}

			values = append(values, quote(text))
		}

		return strings.Join(values, " | "), nil
	}

	types := schema.Types

	if schema.Type != "" {
		types = []string{schema.Type}
	}

	if len(types) == 0 {
		return "unknown", nil
	}

	var union []string

	for _, name := range types {
		typed, err := typescriptOf(name, schema, indent)

		if err != nil {
			return "", err
		}

		union = append(union, typed)
	}

	slices.SortStableFunc(union, func(a, b string) int { return boolCompare(a == "null", b == "null") }) // null last

	return strings.Join(union, " | "), nil
}

func typescriptOf(name string, schema *jsonschema.Schema, indent string) (string, error) {
	switch name {
	case "null":
		return "null", nil
	case "boolean":
		return "boolean", nil
	case "integer", "number":
		return "number", nil
	case "string":
		if schema.Format == "date-time" {
			return "IsoTime", nil
		}

		return "string", nil
	case "array":
		item, err := typescriptType(schema.Items, indent)

		if err != nil {
			return "", err
		}

		if strings.Contains(item, " | ") {
			item = "(" + item + ")"
		}

		return item + "[]", nil
	case "object":
		return typescriptObject(schema, indent)
	default:
		return "", fmt.Errorf("unsupported type %q", name)
	}
}

func typescriptObject(schema *jsonschema.Schema, indent string) (string, error) {
	if len(schema.Properties) == 0 {
		if schema.AdditionalProperties != nil && !isFalse(schema.AdditionalProperties) {
			value, err := typescriptType(schema.AdditionalProperties, indent)

			return "Record<string, " + value + ">", err
		}

		return "Record<string, never>", nil
	}

	order := schema.PropertyOrder

	if len(order) == 0 {
		order = slices.Sorted(maps.Keys(schema.Properties))
	}

	var object strings.Builder

	object.WriteString("{\n")

	for _, name := range order {
		property := schema.Properties[name]
		typed, err := typescriptType(property, indent+"\t")

		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}

		if comment := propertyComment(property); comment != "" {
			fmt.Fprintf(&object, "%s\t/** %s */\n", indent, comment)
		}

		optional := "?"

		if slices.Contains(schema.Required, name) {
			optional = ""
		}

		fmt.Fprintf(&object, "%s\t%s%s: %s;\n", indent, name, optional, typed)
	}

	object.WriteString(indent + "}")

	return object.String(), nil
}

func propertyComment(schema *jsonschema.Schema) string {
	parts := []string{schema.Description}

	if schema.Default != nil {
		parts = append(parts, "@default "+string(schema.Default))
	}

	return strings.ReplaceAll(strings.TrimSpace(strings.Join(parts, " ")), "*/", "* /")
}

// isFalse: the schema false, "no other properties".
func isFalse(schema *jsonschema.Schema) bool {
	encoded, err := json.Marshal(schema)

	return err == nil && string(encoded) == "false"
}

func quote(text string) string {
	quoted := strconv.Quote(text)

	return "'" + strings.ReplaceAll(quoted[1:len(quoted)-1], `'`, `\'`) + "'"
}

func quoteList(values []string) string {
	quoted := make([]string, len(values))

	for index, value := range values {
		quoted[index] = quote(value)
	}

	return "[" + strings.Join(quoted, ", ") + "]"
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}
