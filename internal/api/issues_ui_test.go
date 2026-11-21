package api_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/agentprompt"
	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// act runs apply_issue_action and returns the status code.
func (user uiUser) act(action string, ids ...string) int {
	user.server.t.Helper()

	body, err := json.Marshal(map[string]any{"action": action, "issue_ids": ids})

	if err != nil {
		user.server.t.Fatal(err)
	}

	return user.call(http.MethodPost, "apply_issue_action", string(body)).Code
}

// sendEnvelope sends one envelope as an SDK does and waits until it is stored.
func (server *restServer) sendEnvelope(body string) {
	server.t.Helper()

	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, fixtures.BackendEnvelopeURL, bytes.NewReader([]byte(body))))

	if response.Code != http.StatusOK {
		server.t.Fatalf("ingest: %d %s", response.Code, response.Body)
	}

	waitStored(server.t, server.app)
}

type uiIssue struct {
	ID           string     `json:"id"`
	Project      string     `json:"project"`
	Title        string     `json:"title"`
	Level        string     `json:"level"`
	Status       string     `json:"status"`
	ResolvedAt   *time.Time `json:"resolved_at"`
	MCPReference string     `json:"mcp_reference"`
}

type uiIssues struct {
	Items      []uiIssue `json:"items"`
	NextCursor string    `json:"next_cursor"`
}

func (user uiUser) issueIDs(query string) []string {
	user.server.t.Helper()

	var issues uiIssues

	user.read("list_issues?limit=100&"+query, &issues)

	ids := make([]string, 0, len(issues.Items))

	for _, issue := range issues.Items {
		ids = append(ids, issue.ID)
	}

	return ids
}

func (user uiUser) count(query string) int {
	user.server.t.Helper()

	var count struct {
		Count int `json:"count"`
	}

	user.read("count_issues?"+query, &count)

	return count.Count
}

// The issue list: several projects and levels at once, the count of the same
// filters, the levels to choose from and each row's request for an agent.
func TestUIIssueList(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	user := server.webUser(store.RoleUser, false)

	var all uiIssues

	user.read("list_issues?limit=100&status=all", &all)

	if len(all.Items) != 9 || user.count("status=all") != 9 {
		t.Fatalf("every issue: %d listed, %d counted, want 9", len(all.Items), user.count("status=all"))
	}

	both := user.issueIDs("status=all&project=backend&project=frontend")
	frontend := user.issueIDs("status=all&project=frontend")

	if len(both) != 9 || len(frontend) == 0 || user.count("status=all&project=frontend") != len(frontend) {
		t.Errorf("both projects %d, frontend %d (counted %d)", len(both), len(frontend), user.count("status=all&project=frontend"))
	}

	errorsAndWarnings := user.issueIDs("status=all&level=error&level=warning")

	if len(errorsAndWarnings) == 0 || len(errorsAndWarnings) >= 9 || user.count("status=all&level=error,warning") != len(errorsAndWarnings) {
		t.Errorf("errors and warnings: %d listed, %d counted", len(errorsAndWarnings), user.count("status=all&level=error,warning"))
	}

	var levels struct {
		Levels []string `json:"levels"`
	}

	user.read("list_issue_levels", &levels)

	if want := config.InLevelOrder([]string{"error", "warning", "info"}, config.Levels); !slices.Equal(levels.Levels, want) {
		t.Errorf("levels %v, want %v highest first", levels.Levels, want)
	}

	for _, issue := range all.Items {
		if issue.MCPReference != agentprompt.MCPReference(config.DefaultAppName, issue.ID) {
			t.Errorf("issue %s: mcp_reference %q", issue.ID, issue.MCPReference)
		}
	}

	// Numbered pages: the cursor is an offset.
	var second uiIssues

	user.read("list_issues?status=all&limit=2&cursor=2", &second)

	if len(second.Items) != 2 || second.Items[0].ID != all.Items[2].ID || second.NextCursor != "4" {
		t.Errorf("offset 2: %+v", second)
	}

	if response := user.call(http.MethodGet, "count_issues?project=ghost", ""); response.Code != http.StatusBadRequest {
		t.Errorf("unknown project: %d", response.Code)
	}

	// Agents and scripts do not get the request for an agent.
	var rest page[struct {
		MCPReference string `json:"mcp_reference"`
	}]

	server.get("/api/v1/list_issues", &rest)

	if len(rest.Items) == 0 || rest.Items[0].MCPReference != "" {
		t.Errorf("REST list_issues: %+v", rest.Items)
	}
}

type uiDetails struct {
	Issue       uiIssue `json:"issue"`
	AgentPrompt string  `json:"agent_prompt"`
	History     []struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Name    string `json:"name"`
		ByEvent bool   `json:"by_event"`
		ViaMCP  bool   `json:"via_mcp"`
	} `json:"history"`
	Events struct {
		Items []struct {
			EventID    string    `json:"event_id"`
			ReceivedAt time.Time `json:"received_at"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	} `json:"events"`
	EventState string `json:"event_state"`
	Event      *struct {
		EventID      string `json:"event_id"`
		ClockDrift   string `json:"clock_drift"`
		TraceID      string `json:"trace_id"`
		TraceHasLogs bool   `json:"trace_has_logs"`
		Message      string `json:"message"`
		Exceptions   []struct {
			Type    string `json:"type"`
			Value   string `json:"value"`
			Handled *bool  `json:"handled"`
			Frames  []struct {
				Function    string `json:"function"`
				Location    string `json:"location"`
				InApp       bool   `json:"in_app"`
				SourceLines []struct {
					Current bool `json:"current"`
				} `json:"source_lines"`
				Vars string `json:"vars"`
			} `json:"frames"`
		} `json:"exceptions"`
		Breadcrumbs []struct {
			Message string `json:"message"`
		} `json:"breadcrumbs"`
		Request *struct {
			Method  string `json:"method"`
			Headers []struct {
				Name string `json:"name"`
			} `json:"headers"`
		} `json:"request"`
		Tags        []namedValue `json:"tags"`
		Contexts    []namedValue `json:"contexts"`
		RelatedLogs []struct {
			Body string `json:"body"`
		} `json:"related_logs"`
		LogsFromRequest bool   `json:"logs_from_request"`
		Payload         string `json:"payload"`
	} `json:"event"`
}

func (user uiUser) details(issueID, eventID string) uiDetails {
	user.server.t.Helper()

	var details uiDetails

	query := url.Values{"issue_id": {issueID}}

	if eventID != "" {
		query.Set("event_id", eventID)
	}

	user.read("get_issue_details?"+query.Encode(), &details)

	return details
}

// The issue page reads the event as the server does: the cause chain
// outermost first, frames newest first with their source, the logs of the
// request, the stored payload and the prompt for an agent.
func TestUIIssueDetails(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	user := server.webUser(store.RoleUser, false)

	for _, id := range user.issueIDs("status=all") {
		if details := user.details(id, ""); details.EventState != "shown" || details.Event == nil || details.Event.Payload == "" || len(details.Events.Items) == 0 {
			t.Errorf("issue %s: state %s, event %v", id, details.EventState, details.Event != nil)
		}
	}

	ids := user.issueIDs("query=Failed+to+load+orders")

	if len(ids) != 1 {
		t.Fatalf("search found %v", ids)
	}

	details := user.details(ids[0], "")
	event := details.Event

	if details.Issue.MCPReference != agentprompt.MCPReference(config.DefaultAppName, ids[0]) {
		t.Errorf("mcp_reference %q", details.Issue.MCPReference)
	}

	for _, want := range []string{"connect ECONNREFUSED", "## Exception", "## Breadcrumbs", "Logs of the same", "Do not change any code"} {
		if !strings.Contains(details.AgentPrompt, want) {
			t.Errorf("the agent prompt lacks %q", want)
		}
	}

	if len(event.Exceptions) != 2 || event.Exceptions[0].Value != "Failed to load orders" || !strings.Contains(event.Exceptions[1].Value, "connect ECONNREFUSED") {
		t.Fatalf("exceptions, outermost first: %+v", event.Exceptions)
	}

	if handled := event.Exceptions[0].Handled; handled == nil || *handled {
		t.Errorf("the outer exception is unhandled: %v", handled)
	}

	top := event.Exceptions[0].Frames[0]

	if !top.InApp || !slices.ContainsFunc(top.SourceLines, func(line struct {
		Current bool `json:"current"`
	}) bool {
		return line.Current
	}) || top.Vars == "" || !strings.HasPrefix(top.Vars, "{\n  ") {
		t.Errorf("the throwing frame, newest first, with its source and indented variables: %+v", top)
	}

	if len(event.Breadcrumbs) == 0 || !event.LogsFromRequest || len(event.RelatedLogs) == 0 || !event.TraceHasLogs {
		t.Errorf("breadcrumbs %d, logs of the request %v (%d), trace has logs %v", len(event.Breadcrumbs), event.LogsFromRequest, len(event.RelatedLogs), event.TraceHasLogs)
	}

	if event.Request == nil || event.Request.Method != "GET" {
		t.Errorf("request %+v", event.Request)
	}

	if contexts := names(event.Contexts); len(contexts) < 2 || !slices.IsSorted(contexts) {
		t.Errorf("contexts not sorted by name: %v", contexts)
	}

	if tags := user.details(user.issueIDs("query=Expected+property")[0], "").Event.Tags; !slices.Contains(tags, namedValue{Name: "feature", Value: "checkout"}) {
		t.Errorf("tags %v", tags)
	}

	if !strings.HasPrefix(event.Payload, "{\n  ") {
		t.Errorf("payload not indented: %.40q", event.Payload)
	}

	// Another issue's event is not shown here.
	other := user.details(user.issueIDs("query=Payment")[0], "")

	if foreign := user.details(ids[0], other.Event.EventID); foreign.EventState != "missing" || foreign.Event != nil {
		t.Errorf("an event of another issue: %s", foreign.EventState)
	}

	for _, id := range []string{"nope", "zzzzzzzz"} {
		if response := user.call(http.MethodGet, "get_issue_details?issue_id="+id, ""); response.Code != http.StatusNotFound {
			t.Errorf("issue %s: %d", id, response.Code)
		}
	}

	// The web app's only: unredacted event data never reaches the REST API or an agent.
	if response := server.do(http.MethodGet, "/api/v1/get_issue_details?issue_id="+ids[0], ""); response.Code != http.StatusNotFound {
		t.Errorf("REST get_issue_details: %d", response.Code)
	}
}

type namedValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func names(pairs []namedValue) []string {
	result := []string{}

	for _, pair := range pairs {
		result = append(result, pair.Name)
	}

	return result
}

// requestEnvelopes builds an error and a log from one backend request, the
// way the Node SDK sends them: the log carries the request span as
// sentry.trace.parent_span_id, the event has it as parent_span_id.
func requestEnvelopes(traceID, handlerSpan, requestSpan, errorText, logText string) (event, logs string) {
	trace := fmt.Sprintf(`{"trace_id":%q,"span_id":%q,"parent_span_id":%q}`, traceID, handlerSpan, requestSpan)
	event = `{}` + "\n" + `{"type":"event"}` + "\n" +
		`{"exception":{"values":[{"type":"Error","value":` + strconv.Quote(errorText) + `}]},"contexts":{"trace":` + trace + `}}`

	spanAttribute := ""

	if requestSpan != "" {
		spanAttribute = fmt.Sprintf(`,"attributes":{"sentry.trace.parent_span_id":{"type":"string","value":%q}}`, requestSpan)
	}

	logs = `{}` + "\n" + `{"type":"log","item_count":1}` + "\n" +
		fmt.Sprintf(`{"items":[{"level":"error","body":%q,"trace_id":%q%s}]}`, logText, traceID, spanAttribute)

	return event, logs
}

// Logs belong to an event by the span of its request, not by the trace a
// browser tab shares; without spans, the trace within a minute.
func TestUIRelatedLogs(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	const trace = "f52636a524ff4d968e1a56e205a2a1d6"

	for _, request := range [][4]string{
		{"aaaaaaaaaaaaaaa1", "bbbbbbbbbbbbbbb1", "Order 298 has no items", "log of the order request"},
		{"aaaaaaaaaaaaaaa2", "bbbbbbbbbbbbbbb2", "Card declined by issuer", "log of the payment request"},
	} {
		event, logs := requestEnvelopes(trace, request[0], request[1], request[2], request[3])
		server.sendEnvelope(event)
		server.sendEnvelope(logs)
	}

	order := user.details(user.issueIDs("query=Order")[0], "").Event

	if !order.LogsFromRequest || len(order.RelatedLogs) != 1 || order.RelatedLogs[0].Body != "log of the order request" || !order.TraceHasLogs {
		t.Errorf("the logs of the order request: %+v", order.RelatedLogs)
	}

	// An SDK that sends no spans: only the trace links the log to the error.
	event, logs := requestEnvelopes("0123456789abcdef0123456789abcdef", "", "", "Export failed", "export log line")
	server.sendEnvelope(event)
	server.sendEnvelope(logs)

	if export := user.details(user.issueIDs("query=Export")[0], "").Event; export.LogsFromRequest || len(export.RelatedLogs) != 1 {
		t.Errorf("the trace fallback: %+v", export.RelatedLogs)
	}

	// A trace without logs does not link to an empty log list.
	alone, _ := requestEnvelopes("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "3333333333333333", "", "alone failure", "")
	server.sendEnvelope(alone)

	if details := user.details(user.issueIDs("query=alone")[0], ""); details.Event.TraceHasLogs || details.Event.TraceID == "" {
		t.Errorf("trace %q has no logs, trace_has_logs %v", details.Event.TraceID, details.Event.TraceHasLogs)
	}
}

// An event from a device whose clock is 3 hours behind: its time is
// corrected, the page says by how much, and the logs the device sent with
// its own clock are still found.
func TestUIClockDrift(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	const trace = "0123456789abcdef0123456789abcdef"

	behind := time.Now().Add(-3 * time.Hour)
	server.sendEnvelope(fmt.Sprintf(`{"sent_at":%q}`, behind.Format(time.RFC3339Nano)) + "\n" + `{"type":"event"}` + "\n" +
		fmt.Sprintf(`{"message":"device clock","level":"error","timestamp":%d,"contexts":{"trace":{"trace_id":%q}}}`, behind.Unix(), trace))
	server.sendEnvelope(`{}` + "\n" + `{"type":"log","item_count":1}` + "\n" +
		fmt.Sprintf(`{"items":[{"timestamp":%d,"level":"error","body":"log by the device clock","trace_id":%q}]}`, behind.Unix(), trace))

	event := user.details(user.issueIDs("")[0], "").Event

	if event.ClockDrift != "3h0m0s" || event.Message != "device clock" {
		t.Errorf("clock drift %q, message %q", event.ClockDrift, event.Message)
	}

	if len(event.RelatedLogs) != 1 || event.RelatedLogs[0].Body != "log by the device clock" {
		t.Errorf("the log of the device: %+v", event.RelatedLogs)
	}
}

// Actions of the list and the page: several issues at once, recorded as the
// user; deleting needs an admin; issues deleted meanwhile are skipped.
func TestUIApplyIssueAction(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	user := server.webUser(store.RoleUser, false)
	admin := server.webUser(store.RoleAdmin, false)
	frontend := user.issueIDs("project=frontend")

	if len(frontend) != 4 {
		t.Fatalf("frontend issues %v", frontend)
	}

	var session struct {
		Unresolved int `json:"unresolved_issues"`
	}

	user.read("get_session", &session)
	unresolved := session.Unresolved

	if code := user.act("ignore", frontend[0], frontend[1]); code != http.StatusOK {
		t.Fatalf("ignore: %d", code)
	}

	if left, ignored := user.count("project=frontend"), user.count("project=frontend&status=ignored"); left != 2 || ignored != 2 {
		t.Errorf("after ignoring 2 of 4: %d unresolved, %d ignored", left, ignored)
	}

	if user.read("get_session", &session); session.Unresolved != unresolved-2 {
		t.Errorf("the menu counts %d unresolved, want %d", session.Unresolved, unresolved-2)
	}

	ignored := user.details(frontend[0], "")

	if len(ignored.History) != 1 || ignored.History[0].To != store.StatusIgnored || ignored.History[0].Name != "Web user" || ignored.History[0].ViaMCP {
		t.Errorf("history %+v", ignored.History)
	}

	// The same status again is no change and no entry.
	user.act("ignore", frontend[0])

	if history := user.details(frontend[0], "").History; len(history) != 1 {
		t.Errorf("a repeated status was recorded: %+v", history)
	}

	// Resolved, then reopened: the time it was resolved comes and goes.
	user.act("resolve", frontend[2])

	if resolved := user.details(frontend[2], "").Issue; resolved.Status != store.StatusResolved || resolved.ResolvedAt == nil {
		t.Errorf("resolved %+v", resolved)
	}

	user.act("reopen", frontend[2])

	if reopened := user.details(frontend[2], "").Issue; reopened.Status != store.StatusUnresolved || reopened.ResolvedAt != nil {
		t.Errorf("reopened %+v", reopened)
	}

	// A user cannot delete: it looks as if nothing were there.
	if code := user.act("delete", frontend[3]); code != http.StatusNotFound {
		t.Errorf("delete as a user: %d", code)
	}

	if user.details(frontend[3], "").Issue.ID != frontend[3] {
		t.Error("a user deleted an issue")
	}

	// An admin deletes; an issue already gone is skipped.
	if code := admin.act("delete", frontend[3], "missing0"); code != http.StatusOK {
		t.Errorf("delete as an admin: %d", code)
	}

	if response := admin.call(http.MethodGet, "get_issue_details?issue_id="+frontend[3], ""); response.Code != http.StatusNotFound {
		t.Errorf("deleted issue: %d", response.Code)
	}

	many := make([]string, 1001)

	for index := range many {
		many[index] = fmt.Sprintf("issue%03d", index)
	}

	for label, code := range map[string]int{
		"unknown action": user.act("archive", frontend[0]),
		"invalid id":     user.act("resolve", "not-an-id"),
		"no issues":      user.act("resolve"),
		"1001 issues":    user.act("resolve", many...),
	} {
		if code != http.StatusBadRequest {
			t.Errorf("%s: %d", label, code)
		}
	}

	// The web app's only: the REST token has no user to record.
	if response := server.do(http.MethodPost, "/api/v1/apply_issue_action", `{"action":"resolve","issue_ids":["`+frontend[0]+`"]}`); response.Code != http.StatusNotFound {
		t.Errorf("REST apply_issue_action: %d", response.Code)
	}
}

// What the issue page shows when its event is not there: a missing one keeps
// the history and the events; an issue whose events the cleanup deleted
// keeps its history; a stored payload that does not read is said so.
func TestUIIssueEventStates(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)
	ctx := context.Background()

	save := func(eventID, title, payload string, seen time.Time) string {
		t.Helper()

		saved, err := server.database.SaveEvent(ctx, store.NewEvent{
			ProjectID: 1, EventID: eventID, Fingerprint: title, Title: title, Level: "error", Timestamp: seen, ReceivedAt: seen, Payload: []byte(payload),
		})

		if err != nil {
			t.Fatal(err)
		}

		return saved.IssueID
	}

	unreadable := save(strings.Repeat("b", 32), "unreadable", `[1]`, time.Now())

	if details := user.details(unreadable, ""); details.EventState != "unreadable" || details.Event != nil || len(details.Events.Items) != 1 {
		t.Errorf("unreadable payload: %s", details.EventState)
	}

	if details := user.details(unreadable, strings.Repeat("0", 32)); details.EventState != "missing" {
		t.Errorf("an event not stored: %s", details.EventState)
	}

	// A large random payload makes the event most of the database, so the
	// cleanup reaches its goal by deleting it and keeps the issue.
	padding := make([]byte, 32<<10)
	_, _ = rand.Read(padding)
	quiet := save(strings.Repeat("a", 32), "quiet error", `{"message":"quiet error","extra":{"padding":"`+hex.EncodeToString(padding)+`"}}`, time.Now().Add(-8*24*time.Hour))

	user.act("resolve", quiet)

	size, err := server.database.DatabaseSize(ctx)

	if err != nil {
		t.Fatal(err)
	}

	if _, err := server.database.Cleanup(ctx, size/2); err != nil {
		t.Fatal(err)
	}

	if _, err := server.database.LatestEvent(ctx, quiet); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the latest event is still stored: %v", err)
	}

	if details := user.details(quiet, ""); details.EventState != "none" || len(details.History) != 1 || len(details.Events.Items) != 0 {
		t.Errorf("an issue without events: %s, history %d", details.EventState, len(details.History))
	}
}

// The events of an issue come 20 at a time; Load more follows the cursor.
func TestUIIssueEventsPages(t *testing.T) {
	server := newRESTServer(t)
	user := server.webUser(store.RoleUser, false)

	for index := range 23 {
		server.sendEnvelope(fmt.Sprintf(`{"event_id":"%032x"}`, index+1) + "\n" + `{"type":"event"}` + "\n" + `{"message":"again and again","level":"error"}`)
	}

	details := user.details(user.issueIDs("")[0], "")

	if len(details.Events.Items) != 20 || details.Events.NextCursor == "" {
		t.Fatalf("first events: %d, cursor %q", len(details.Events.Items), details.Events.NextCursor)
	}

	var more struct {
		Items []struct {
			EventID    string    `json:"event_id"`
			ReceivedAt time.Time `json:"received_at"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}

	user.read("list_issue_events?limit=20&issue_id="+details.Issue.ID+"&cursor="+details.Events.NextCursor, &more)

	if len(more.Items) != 3 || more.NextCursor != "" || more.Items[0].ReceivedAt.IsZero() || slices.ContainsFunc(details.Events.Items, func(item struct {
		EventID    string    `json:"event_id"`
		ReceivedAt time.Time `json:"received_at"`
	}) bool {
		return item.EventID == more.Items[0].EventID
	}) {
		t.Errorf("the rest: %+v", more)
	}
}
