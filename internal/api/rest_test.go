// Tests run the whole server (ingest + API) against a real database filled
// by replaying the recorded SDK requests.
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/app"

	"github.com/devforth/exceptic/internal/api"

	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/storetest"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/platform/config"
)

const (
	token = "0123456789abcdef0123456789abcdef"
)

type restServer struct {
	t        *testing.T
	app      *app.App
	handler  http.Handler
	database *database.Store
}

func newRESTServer(t *testing.T, allowedIPs ...string) *restServer {
	t.Helper()

	return newRESTServerWith(t, func(settings *config.Config) {
		for _, value := range allowedIPs {
			settings.AllowedIPs = append(settings.AllowedIPs, netip.MustParsePrefix(value))
		}
	})
}

// newRESTServerWith starts the server with the test settings changed by configure.
func newRESTServerWith(t *testing.T, configure func(settings *config.Config)) *restServer {
	t.Helper()
	database := storetest.Open(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	settings := &config.Config{
		AppName:        config.DefaultAppName,
		Projects:       fixtures.Projects(),
		AllowedOrigins: []string{"*"},
		APIToken:       token,
	}

	configure(settings)

	built, err := app.New(context.Background(), app.Options{Settings: settings, Logger: logger, Store: database, Version: "test", Web: fixtures.WebApp(t)})

	if err != nil {
		t.Fatal(err)
	}

	// Its pipeline stores what the server accepts; it stops before the store closes (cleanups run last added first).
	t.Cleanup(built.Start())

	return &restServer{t: t, app: built, handler: built.Handler, database: database}
}

// ingestFixtures sends every recorded SDK request to the server.
func (server *restServer) ingestFixtures() {
	server.t.Helper()

	for _, fixture := range fixtures.All(server.t) {
		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, fixture.HTTP())

		if response.Code != http.StatusOK {
			server.t.Fatalf("%s: ingest got %d %s", fixture.Name, response.Code, response.Body)
		}
	}

	waitStored(server.t, server.app)
}

// waitStored waits until the pipeline has stored everything the server accepted.
func waitStored(t *testing.T, built *app.App) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	defer cancel()

	if err := built.WaitStored(ctx); err != nil {
		t.Fatalf("envelopes not stored: %v", err)
	}
}

func (server *restServer) do(method, target, body string) *httptest.ResponseRecorder {
	server.t.Helper()

	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)

	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)

	return response
}

// get requests target and decodes the JSON response into result, failing on a non-200 status.
func (server *restServer) get(target string, result any) {
	server.t.Helper()
	server.decode(server.do(http.MethodGet, target, ""), "GET "+target, result)
}

// post sends body to an endpoint that changes data and decodes the answer.
func (server *restServer) post(target, body string, result any) {
	server.t.Helper()
	server.decode(server.do(http.MethodPost, target, body), "POST "+target, result)
}

func (server *restServer) decode(response *httptest.ResponseRecorder, request string, result any) {
	server.t.Helper()

	if response.Code != http.StatusOK {
		server.t.Fatalf("%s: %d %s", request, response.Code, response.Body)
	}

	if err := json.Unmarshal(response.Body.Bytes(), result); err != nil {
		server.t.Fatalf("%s: %v", request, err)
	}
}

type restIssue struct {
	ID         string `json:"id"`
	Project    string `json:"project"`
	Title      string `json:"title"`
	Location   string `json:"location"`
	Status     string `json:"status"`
	EventCount int    `json:"event_count"`
}

type page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func TestAccessControl(t *testing.T) {
	server := newRESTServer(t, "10.0.0.0/8")

	request := func(remote, path string, withToken bool) int {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.RemoteAddr = remote

		if withToken {
			request.Header.Set("Authorization", "Bearer "+token)
		}

		response := httptest.NewRecorder()
		server.handler.ServeHTTP(response, request)

		return response.Code
	}

	if code := request("10.1.1.1:1", "/api/v1/list_issues", true); code != http.StatusOK {
		t.Fatalf("allowed ip with token: %d", code)
	}

	if code := request("10.1.1.1:1", "/api/v1/list_issues", false); code != http.StatusUnauthorized {
		t.Fatalf("allowed ip without token: %d", code)
	}

	if code := request("203.0.113.7:1", "/api/v1/list_issues", true); code != http.StatusForbidden {
		t.Fatalf("other ip with token: %d", code)
	}
	// Health and ingest are not behind the allowlist.
	if code := request("203.0.113.7:1", "/health", false); code != http.StatusOK {
		t.Fatalf("health from other ip: %d", code)
	}

	if code := request("203.0.113.7:1", "/api/1/envelope/", false); code == http.StatusForbidden {
		t.Fatal("ingest must not be behind the IP allowlist")
	}

	// An unknown API address: a JSON 404, behind the same protection.
	if code := request("10.1.1.1:1", "/api/v1/no-such-route", true); code != http.StatusNotFound {
		t.Fatalf("unknown route: %d", code)
	}

	if code := request("10.1.1.1:1", "/api/v1/no-such-route", false); code != http.StatusUnauthorized {
		t.Fatalf("unknown route without token: %d", code)
	}

	// The web app's API: behind the allowlist too, then its login, never the token.
	if code := request("203.0.113.7:1", "/api/ui/no-such-route", false); code != http.StatusForbidden {
		t.Fatalf("web app API from other ip: %d", code)
	}

	if code := request("10.1.1.1:1", "/api/ui/no-such-route", true); code != http.StatusUnauthorized {
		t.Fatalf("web app API without a session: %d", code)
	}
}

// Every REST endpoint of the catalog is served, reads as GET and changes as
// POST; the other method, and an endpoint of other transports, get the JSON
// 404 of unknown API addresses.
func TestRoutesFollowTheCatalog(t *testing.T) {
	server := newRESTServer(t)

	for _, endpoint := range api.Endpoints(nil, nil) {
		if !endpoint.Serves(api.TransportREST) {
			_, path, _ := strings.Cut(api.Route(api.TransportREST, endpoint), " ")

			for _, method := range []string{http.MethodGet, http.MethodPost} {
				if response := server.do(method, path, ""); !strings.Contains(response.Body.String(), "unknown API endpoint") {
					t.Errorf("%s of %s is a REST route: %d", endpoint.Name, endpoint.Transports, response.Code)
				}
			}

			continue
		}

		route := api.Route(api.TransportREST, endpoint)
		method, path, _ := strings.Cut(route, " ")

		if want := map[bool]string{true: http.MethodGet, false: http.MethodPost}[endpoint.ReadOnly]; method != want {
			t.Errorf("%s: %s", endpoint.Name, route)
		}

		other := map[string]string{http.MethodGet: http.MethodPost, http.MethodPost: http.MethodGet}[method]

		if response := server.do(other, path, ""); !strings.Contains(response.Body.String(), "unknown API endpoint") {
			t.Errorf("%s %s: %d %s, want the unknown endpoint answer", other, path, response.Code, response.Body)
		}

		if response := server.do(method, path, ""); strings.Contains(response.Body.String(), "unknown API endpoint") {
			t.Errorf("%s is not served", route)
		}
	}
}

func TestProjects(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	var result page[struct {
		ID               int    `json:"id"`
		Name             string `json:"name"`
		UnresolvedIssues int    `json:"unresolved_issues"`
		DSN              string `json:"dsn"`
	}]
	server.get("/api/v1/list_projects", &result)

	if len(result.Items) != 2 || result.Items[0].Name != "backend" || result.Items[0].UnresolvedIssues != 5 || result.Items[1].UnresolvedIssues != 4 {
		t.Fatalf("projects = %+v", result.Items)
	}

	// httptest requests go to example.com.
	if want := "http://" + fixtures.BackendKey + "@example.com/1"; result.Items[0].DSN != want {
		t.Fatalf("DSN = %q, want %q", result.Items[0].DSN, want)
	}
}

func TestListIssues(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	var all page[restIssue]
	server.get("/api/v1/list_issues", &all)

	if len(all.Items) != 9 || all.NextCursor != "" {
		t.Fatalf("got %d issues, next %q", len(all.Items), all.NextCursor)
	}

	var frontend page[restIssue]
	server.get("/api/v1/list_issues?project=frontend", &frontend)

	for _, item := range frontend.Items {
		if item.Project != "frontend" {
			t.Fatalf("project filter leaked %+v", item)
		}
	}

	var byID page[restIssue]
	server.get("/api/v1/list_issues?project=2", &byID)

	if len(frontend.Items) != 4 || len(byID.Items) != 4 {
		t.Fatalf("frontend by name=%d by id=%d, want 4", len(frontend.Items), len(byID.Items))
	}

	var search page[restIssue]
	server.get("/api/v1/list_issues?query=referenceerror", &search)

	if len(search.Items) != 1 || !strings.HasPrefix(search.Items[0].Title, "ReferenceError") {
		t.Fatalf("search = %+v", search.Items)
	}
	// A % in the search is plain text.
	var wildcard page[restIssue]
	server.get("/api/v1/list_issues?query=%25", &wildcard)

	if len(wildcard.Items) != 0 {
		t.Fatalf("'%%' must not match everything, got %d", len(wildcard.Items))
	}

	// Walk all pages two at a time: every restIssue exactly once.
	seen := map[string]bool{}
	target := "/api/v1/list_issues?limit=2&sort=first_seen"

	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("pagination does not end")
		}

		var current page[restIssue]
		server.get(target, &current)

		for _, item := range current.Items {
			if seen[item.ID] {
				t.Fatalf("restIssue %s on two pages", item.ID)
			}

			seen[item.ID] = true
		}

		if current.NextCursor == "" {
			break
		}

		target = "/api/v1/list_issues?limit=2&sort=first_seen&cursor=" + current.NextCursor
	}

	if len(seen) != 9 {
		t.Fatalf("pages covered %d issues, want 9", len(seen))
	}

	for _, bad := range []string{"?status=open", "?sort=title", "?limit=-1", "?limit=500", "?limit=ten", "?project=nope",
		"?cursor=garbage", "?date_by=week", "?unknown=1", "?status=all&status=resolved"} {
		if response := server.do(http.MethodGet, "/api/v1/list_issues"+bad, ""); response.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", bad, response.Code)
		}
	}
}

type batch struct {
	Issues          []restIssue `json:"issues"`
	MissingIssueIDs []string    `json:"missing_issue_ids"`
}

func TestIssueLifecycle(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	var list page[restIssue]
	server.get("/api/v1/list_issues?query=Failed+to+load+orders", &list)

	if len(list.Items) != 1 {
		t.Fatalf("got %d issues", len(list.Items))
	}

	id := list.Items[0].ID

	var updated batch
	server.post("/api/v1/update_issue_status", `{"issue_ids":["`+id+`"],"status":"resolved"}`, &updated)

	if len(updated.Issues) != 1 || updated.Issues[0].Status != "resolved" {
		t.Fatalf("resolve: %+v", updated)
	}

	var unresolved page[restIssue]
	server.get("/api/v1/list_issues", &unresolved)

	if len(unresolved.Items) != 8 {
		t.Fatalf("default list must hide resolved issues, got %d", len(unresolved.Items))
	}

	// A list field takes repeated or comma-separated values.
	var summaries batch
	server.get("/api/v1/get_issues?issue_ids="+id+",missing0&issue_ids="+id, &summaries)

	if len(summaries.Issues) != 1 || summaries.Issues[0].Status != "resolved" || !slices.Equal(summaries.MissingIssueIDs, []string{"missing0"}) {
		t.Fatalf("get_issues: %+v", summaries)
	}

	for _, body := range []string{`{"issue_ids":["` + id + `"],"status":"closed"}`, `not json`, `{"issue_ids":["` + id + `"],"status":"resolved","extra":1}`, `{"status":"resolved"}`} {
		if response := server.do(http.MethodPost, "/api/v1/update_issue_status", body); response.Code != http.StatusBadRequest {
			t.Errorf("POST %s: got %d, want 400", body, response.Code)
		}
	}

	// The prompt an agent gets, as text.
	response := server.do(http.MethodGet, "/api/v1/get_issue?issue_id="+id, "")

	if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain") || !strings.Contains(response.Body.String(), "Issue ID: "+id) {
		t.Fatalf("get_issue: %d %s", response.Code, response.Body)
	}

	if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
		t.Errorf("get_issue Cache-Control = %q, want no-store", cache)
	}

	var deleted struct {
		Deleted string `json:"deleted"`
	}
	server.post("/api/v1/delete_issue", `{"issue_id":"`+id+`"}`, &deleted)

	for _, path := range []string{"/api/v1/get_issue?issue_id=" + id, "/api/v1/list_issue_events?issue_id=" + id} {
		if response := server.do(http.MethodGet, path, ""); response.Code != http.StatusNotFound {
			t.Errorf("GET %s after delete: %d, want 404", path, response.Code)
		}
	}

	if response := server.do(http.MethodPost, "/api/v1/delete_issue", `{"issue_id":"`+id+`"}`); response.Code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", response.Code)
	}
}

type eventSummary struct {
	EventID string `json:"event_id"`
	IssueID string `json:"issue_id"`
	TraceID string `json:"trace_id"`
}

func TestEvents(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	var list page[restIssue]
	server.get("/api/v1/list_issues?query=ReferenceError", &list)
	issueID := list.Items[0].ID

	var events page[eventSummary]
	server.get("/api/v1/list_issue_events?issue_id="+issueID, &events)

	if len(events.Items) != 1 {
		t.Fatalf("events = %+v", events.Items)
	}

	// Lookup by event_id, also in UUID form with dashes; the payload is REST only.
	id := events.Items[0].EventID
	dashed := id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]

	var event struct {
		eventSummary
		Payload struct {
			Exception struct {
				Values []struct{ Type string } `json:"values"`
			} `json:"exception"`
		} `json:"payload"`
	}
	server.get("/api/v1/get_event_payload?project=frontend&event_id="+dashed, &event)

	if event.EventID != id || event.IssueID != issueID || event.Payload.Exception.Values[0].Type != "ReferenceError" {
		t.Fatalf("event = %+v", event)
	}
	// Event IDs are per project.
	if response := server.do(http.MethodGet, "/api/v1/get_event_payload?project=backend&event_id="+id, ""); response.Code != http.StatusNotFound {
		t.Fatalf("event from another project: %d", response.Code)
	}

	if response := server.do(http.MethodGet, "/api/v1/get_event?project=missing&event_id="+id, ""); response.Code != http.StatusNotFound {
		t.Fatalf("event of an unknown project: %d", response.Code)
	}
}

type logRecord struct {
	Level   string `json:"level"`
	Body    string `json:"body"`
	TraceID string `json:"trace_id"`
}

func TestLogsAndTraces(t *testing.T) {
	server := newRESTServer(t)
	server.ingestFixtures()

	var all page[logRecord]
	server.get("/api/v1/list_logs?limit=100", &all)

	if len(all.Items) == 0 {
		t.Fatal("no logs")
	}

	var warnings page[logRecord]
	server.get("/api/v1/list_logs?level=warn&project=backend", &warnings)

	for _, record := range warnings.Items {
		if record.Level != "warn" {
			t.Fatalf("level filter leaked %+v", record)
		}
	}

	var pino page[logRecord]
	server.get("/api/v1/list_logs?query=pino", &pino)

	if len(pino.Items) == 0 || len(pino.Items) >= len(all.Items) {
		t.Fatalf("query matched %d of %d", len(pino.Items), len(all.Items))
	}

	// Pages two at a time cover all logs.
	total := 0
	target := "/api/v1/list_logs?limit=2"

	for {
		var current page[logRecord]
		server.get(target, &current)
		total += len(current.Items)

		if current.NextCursor == "" {
			break
		}

		target = "/api/v1/list_logs?limit=2&cursor=" + current.NextCursor
	}

	if total != len(all.Items) {
		t.Fatalf("pages covered %d logs, want %d", total, len(all.Items))
	}

	var future page[logRecord]
	server.get("/api/v1/list_logs?since=2100-01-01T00:00:00Z", &future)

	if len(future.Items) != 0 {
		t.Fatalf("since filter: %d", len(future.Items))
	}

	if response := server.do(http.MethodGet, "/api/v1/list_logs?since=yesterday", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("bad since: %d", response.Code)
	}

	// The browser fixtures share a trace between an error and logs.
	var withTrace page[logRecord]
	server.get("/api/v1/list_logs?project=frontend", &withTrace)

	traceID := ""

	for _, record := range withTrace.Items {
		if record.TraceID != "" {
			traceID = record.TraceID
		}
	}

	if traceID == "" {
		t.Skip("no frontend log with a trace id in fixtures")
	}

	var trace struct {
		TraceID string         `json:"trace_id"`
		Events  []eventSummary `json:"events"`
		Logs    []struct {
			TraceID    string          `json:"trace_id"`
			Attributes json.RawMessage `json:"attributes"`
		} `json:"logs"`
	}
	server.get("/api/v1/list_trace?trace_id="+traceID, &trace)

	if len(trace.Logs) == 0 {
		t.Fatalf("trace %s has no logs", traceID)
	}

	for _, record := range trace.Logs {
		if record.TraceID != traceID || !bytes.HasPrefix(record.Attributes, []byte("{")) {
			t.Fatalf("trace log %+v", record)
		}
	}
}
