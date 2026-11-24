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

	"github.com/devforth/exceptic/docs"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/server"
	"github.com/devforth/exceptic/internal/services"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
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

// outputSchemas: the times as RFC 3339 strings, raw JSON as any value, the
// states of a value as their union.
var outputSchemas = &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[time.Time]():           {Type: "string", Format: "date-time"},
	reflect.TypeFor[json.RawMessage]():     {},
	reflect.TypeFor[services.EventState](): {Type: "string", Enum: enum(services.EventStates)},
}}

func enum[Value ~string](values []Value) []any {
	result := make([]any, len(values))

	for index, value := range values {
		result[index] = string(value)
	}

	return result
}

func typescriptCatalog(endpoints []Endpoint) (string, error) {
	endpoints = slices.DeleteFunc(slices.Clone(endpoints), func(endpoint Endpoint) bool { return !endpoint.Serves(TransportUI) })
	slices.SortFunc(endpoints, func(a, b Endpoint) int { return strings.Compare(a.Name, b.Name) })

	var file strings.Builder

	file.WriteString("// Generated from the catalog of internal/api by TestTypeScriptCatalog; do not edit: make api-types.\n\n")
	file.WriteString("/** An RFC 3339 time in UTC. */\nexport type IsoTime = string;\n\n")
	fmt.Fprintf(&file, "/** Where the web app's requests go. */\nexport const apiPrefix = %s;\n\n", quote(server.UIAPIPrefix))
	fmt.Fprintf(&file, "/** The largest page of a list. */\nexport const maxLimit = %d;\n\n", MaxLimit)

	for _, write := range []func(*strings.Builder, []Endpoint) error{writeEndpoints, writePlainRequests, writeErrorBody} {
		if err := write(&file, endpoints); err != nil {
			return "", err
		}
	}

	writeChoices(&file, endpoints)
	writeLevels(&file)

	if err := writeRules(&file); err != nil {
		return "", err
	}

	return file.String(), nil
}

func writeEndpoints(file *strings.Builder, endpoints []Endpoint) error {
	file.WriteString("/** Inputs and outputs of every request the web app may make. */\nexport interface Endpoints {\n")

	for _, endpoint := range endpoints {
		// The web app reads JSON only; a text answer (the agent prompt) is for agents.
		if endpoint.output == reflect.TypeFor[Text]() {
			return fmt.Errorf("%s answers text: the web app reads JSON only", endpoint.Name)
		}

		input, err := typescriptType(endpoint.InputSchema, "\t\t")

		if err != nil {
			return fmt.Errorf("%s input: %w", endpoint.Name, err)
		}

		output, err := typescriptOutput(endpoint.output, "\t\t")

		if err != nil {
			return fmt.Errorf("%s output: %w", endpoint.Name, err)
		}

		fmt.Fprintf(file, "\t%s: {\n\t\tin: %s;\n\t\tout: %s;\n\t};\n", endpoint.Name, input, output)
	}

	file.WriteString("}\n\n/** How each request is sent, and what the app must know before offering it. */\nexport const endpoints = {\n")

	for _, endpoint := range endpoints {
		method := "POST"

		if endpoint.ReadOnly {
			method = "GET"
		}

		fmt.Fprintf(file, "\t%s: { method: '%s', admin: %t, destructive: %t, confirm: %t },\n",
			endpoint.Name, method, endpoint.Admin, endpoint.Destructive, endpoint.Confirm)
	}

	file.WriteString("} as const;\n\n")

	return nil
}

// writePlainRequests: the requests that are no endpoint (PlainRequests).
func writePlainRequests(file *strings.Builder, _ []Endpoint) error {
	file.WriteString("/** Inputs and outputs of the plain requests: logins and credentials, which no endpoint handles. */\nexport interface PlainRequests {\n")

	for _, request := range PlainRequests {
		input, err := typescriptOutput(request.input, "\t\t")

		if err != nil {
			return fmt.Errorf("%s input: %w", request.Name, err)
		}

		output, err := typescriptOutput(request.output, "\t\t")

		if err != nil {
			return fmt.Errorf("%s output: %w", request.Name, err)
		}

		fmt.Fprintf(file, "\t%s: {\n\t\tin: %s;\n\t\tout: %s;\n\t};\n", request.Name, input, output)
	}

	file.WriteString("}\n\n/** How each plain request is sent. */\nexport const plainRequests = {\n")

	for _, request := range PlainRequests {
		fmt.Fprintf(file, "\t%s: { method: '%s' },\n", request.Name, request.Method)
	}

	file.WriteString("} as const;\n\n")

	return nil
}

func writeErrorBody(file *strings.Builder, _ []Endpoint) error {
	body, err := typescriptOutput(reflect.TypeFor[ErrorBody](), "")

	if err != nil {
		return fmt.Errorf("error body: %w", err)
	}

	fmt.Fprintf(file, "/** The answer to a refused request: fields by input name, the record that already exists. */\nexport type ErrorBody = %s;\n\n", body)

	return nil
}

// typescriptOutput is the type of a Go value as JSON: NoContent is nothing.
func typescriptOutput(goType reflect.Type, indent string) (string, error) {
	switch goType {
	case reflect.TypeFor[NoContent]():
		return "void", nil
	}

	schema, err := jsonschema.ForType(goType, outputSchemas)

	if err != nil {
		return "", err
	}

	return typescriptType(schema, indent)
}

func writeChoices(file *strings.Builder, endpoints []Endpoint) {
	file.WriteString("/** The values input fields take. */\nexport const choices = {\n")

	for _, endpoint := range endpoints {
		if len(endpoint.Choices) == 0 {
			continue
		}

		fmt.Fprintf(file, "\t%s: {\n", endpoint.Name)

		for _, field := range slices.Sorted(maps.Keys(endpoint.Choices)) {
			fmt.Fprintf(file, "\t\t%s: %s,\n", field, quoteList(endpoint.Choices[field]))
		}

		file.WriteString("\t},\n")
	}

	file.WriteString("} as const;\n\n")
}

func writeLevels(file *strings.Builder) {
	fmt.Fprintf(file, "/** Event and log levels, highest first, as the app lists them. */\nexport const levels = {\n\tevent: %s,\n\tlog: %s\n} as const;\n\n",
		quoteList(config.HighestFirst(config.Levels)), quoteList(config.HighestFirst(config.LogLevels)))
	file.WriteString("/** The color group a level is shown in, the other names SDKs use included. */\nexport const shownLevel = {\n")

	var known []string

	for _, level := range slices.Concat(config.Levels, config.LogLevels, slices.Sorted(maps.Keys(config.EventLevelAliases))) {
		if !slices.Contains(known, level) {
			known = append(known, level)
			fmt.Fprintf(file, "\t%s: %s,\n", level, quote(config.ShownLevel(level)))
		}
	}

	file.WriteString("} as const;\n\n")
}

// writeRules: the values the server's rules use, so the app never repeats them.
// shownSettings are the environment variables whose description the web app
// shows as a hint; a name the app uses but this lacks fails its type check.
var shownSettings = []string{"MAX_DATABASE_SIZE"}

func writeRules(file *strings.Builder) error {
	file.WriteString("/** The limits of form fields, as the server checks them; a pattern must match the whole value. */\nexport const limits = {\n")

	limits := reflect.ValueOf(config.Limits)

	for index := range limits.NumField() {
		name, _, _ := strings.Cut(limits.Type().Field(index).Tag.Get("json"), ",")

		switch value := limits.Field(index).Interface().(type) {
		case int:
			fmt.Fprintf(file, "\t%s: %d,\n", name, value)
		case string:
			fmt.Fprintf(file, "\t%s: %s,\n", name, quote(value))
		default:
			return fmt.Errorf("limit %s: unsupported type %T", name, value)
		}
	}

	file.WriteString("} as const;\n\n/** Issue statuses in the order of the status filter, with what each means. */\nexport const issueStatuses = [\n")

	for _, status := range store.IssueStatuses {
		fmt.Fprintf(file, "\t{ name: %s, meaning: %s },\n", quote(status), quote(store.StatusMeanings[status]))
	}

	fmt.Fprintf(file, "] as const;\n\n/** The most issues one action takes. */\nexport const maxActionIssues = %d;\n\n", services.MaxActionIssues)
	fmt.Fprintf(file, "/** The events an issue stores a minute; the others only count (not_stored). */\nexport const eventsPerIssuePerMinute = %d;\n\n", config.EventsPerIssuePerMinute)
	fmt.Fprintf(file, "/** How long one log search runs before it stops (count_logs, list_logs: stopped). */\nexport const logSearchSeconds = %d;\n\n", int(config.LogSearchTime/time.Second))
	fmt.Fprintf(file, "/** The kind of a connected agent that is a personal token (list_agents). */\nexport const personalTokenKind = '%s';\n\n", store.GrantToken)
	file.WriteString("/** The palette of project colors: a project follows it by its ID until it has its own color. */\nexport const projectPalette = [\n")

	for _, color := range config.ProjectPalette {
		fmt.Fprintf(file, "\t{ name: %s, color: %s },\n", quote(color.Name), quote(color.Value))
	}

	file.WriteString("] as const;\n\n/** The steps of the cleanup by size in order; those that always run go in full, the rest only while needed. */\nexport const cleanupSteps = [\n")

	for _, step := range database.CleanupSteps() {
		fmt.Fprintf(file, "\t{ name: %s, always: %t },\n", quote(step.Name), step.Always)
	}

	fmt.Fprintf(file, "] as const;\n\n/** The numbers of the cleanup by size. */\nexport const cleanupRules = {\n"+
		"\tstartPercent: %d,\n\tstopPercent: %d,\n\tageDays: %d,\n\trecentAgeDays: %d,\n\toldEventsPerIssue: %d,\n} as const;\n\n",
		config.CleanupStartPercent, config.CleanupStopPercent, config.CleanupAge/(24*time.Hour), config.CleanupRecentAge/(24*time.Hour), config.OldEventsPerIssue)
	fmt.Fprintf(file, "/** A project's disk share of MAX_DATABASE_SIZE, in percent. */\nexport const diskShare = { min: %d, max: %d, step: %d } as const;\n\n",
		store.MinDiskShare, store.MaxDiskShare, store.DiskShareStep)
	file.WriteString("/** What each role may do. */\nexport const roleSummaries = {\n")

	for _, role := range slices.Sorted(maps.Keys(store.RoleSummaries)) {
		fmt.Fprintf(file, "\t%s: %s,\n", role, quote(store.RoleSummaries[role]))
	}

	settings, err := docs.Settings()

	if err != nil {
		return err
	}

	file.WriteString("} as const;\n\n/** The help of the settings pages: docs/ui.md by section and field label, docs/configuration.md by variable. */\nexport const help = {\n\tfields: {\n")

	fields := docs.Fields()

	for _, section := range slices.Sorted(maps.Keys(fields)) {
		fmt.Fprintf(file, "\t\t%s: {\n", quote(section))

		for _, label := range slices.Sorted(maps.Keys(fields[section])) {
			fmt.Fprintf(file, "\t\t\t%s: %s,\n", quote(label), quote(fields[section][label]))
		}

		file.WriteString("\t\t},\n")
	}

	file.WriteString("\t},\n\tsettings: {\n")

	// Only the variables the app names: the rest would only make the bundle larger.
	for _, name := range shownSettings {
		if settings[name] == "" {
			return fmt.Errorf("docs/configuration.md does not describe %s", name)
		}

		fmt.Fprintf(file, "\t\t%s: %s,\n", name, quote(settings[name]))
	}

	file.WriteString("\t}\n} as const;\n")

	return nil
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

// quote is text as a single-quoted string: Go's escapes, " left as it is
// (strconv.Quote escapes every one, so each \" is one) and ' escaped.
func quote(text string) string {
	quoted := strconv.Quote(text)
	inner := strings.ReplaceAll(quoted[1:len(quoted)-1], `\"`, `"`)

	return "'" + strings.ReplaceAll(inner, `'`, `\'`) + "'"
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
