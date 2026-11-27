package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// The logs list deletes chosen lines, or every log like one (its project's
// copies of the same message), for admins only.
func TestUIDeleteLogs(t *testing.T) {
	server := newRESTServer(t)
	admin := server.webUser(store.RoleAdmin, false)
	user := server.webUser(store.RoleUser, false)
	at := time.Now()

	logs := []store.NewLog{
		{ProjectID: 1, Timestamp: at, Level: "info", Body: "kept"},
		{ProjectID: 1, Timestamp: at, Level: "error", Body: "chosen"},
		{ProjectID: 2, Timestamp: at, Level: "warn", Body: "retry 1"},
	}

	for index := range 5 {
		logs = append(logs, store.NewLog{ProjectID: 1, Timestamp: at, Level: []string{"info", "warn"}[index%2], Body: fmt.Sprintf("retry %d", index)})
	}

	// As ingest stores them: the copies of a message share its fingerprint.
	for index := range logs {
		logs[index].Fingerprint = logs[index].Body[:min(5, len(logs[index].Body))]
	}

	if err := server.database.SaveLogs(context.Background(), logs, 0); err != nil {
		t.Fatal(err)
	}

	type row struct {
		ID      int64  `json:"id"`
		Project string `json:"project"`
		Body    string `json:"body"`
	}

	list := func() map[string]int64 {
		t.Helper()

		var page struct {
			Items []row `json:"items"`
		}

		admin.read("list_logs?limit=100", &page)

		ids := map[string]int64{}

		for _, item := range page.Items {
			ids[item.Project+" "+item.Body] = item.ID
		}

		return ids
	}

	remove := func(caller uiUser, input map[string]any) (int, int) {
		t.Helper()

		body, err := json.Marshal(input)

		if err != nil {
			t.Fatal(err)
		}

		response := caller.call(http.MethodPost, "delete_logs", string(body))

		var output struct {
			Deleted int `json:"deleted"`
		}

		_ = json.Unmarshal(response.Body.Bytes(), &output)

		return response.Code, output.Deleted
	}

	ids := list()

	if code, _ := remove(user, map[string]any{"log_ids": []int64{ids["backend chosen"]}}); code != http.StatusForbidden {
		t.Errorf("a user deletes logs: %d", code)
	}

	// The web app's only: the REST token has no user who may administer.
	if response := server.do(http.MethodPost, "/api/v1/delete_logs", fmt.Sprintf(`{"log_ids":[%d]}`, ids["backend retry 3"])); response.Code != http.StatusNotFound {
		t.Errorf("REST delete_logs: %d", response.Code)
	}

	for _, input := range []map[string]any{
		{},
		{"log_ids": make([]int64, 1001)},
	} {
		if code, _ := remove(admin, input); code != http.StatusBadRequest {
			t.Errorf("%v: %d", input, code)
		}
	}

	if code, deleted := remove(admin, map[string]any{"log_ids": []int64{ids["backend chosen"], ids["backend chosen"], 999_999}}); code != http.StatusOK || deleted != 1 {
		t.Errorf("chosen: %d, deleted %d", code, deleted)
	}

	var retries []int64

	for name, id := range ids {
		if strings.HasPrefix(name, "backend retry") {
			retries = append(retries, id)
		}
	}

	if code, deleted := remove(admin, map[string]any{"log_ids": retries}); code != http.StatusOK || deleted != len(retries) {
		t.Errorf("many: %d, deleted %d of %d", code, deleted, len(retries))
	}

	left := list()

	if keys := slices.Sorted(maps.Keys(left)); !slices.Equal(keys, []string{"backend kept", "frontend retry 1"}) {
		t.Errorf("left %v", keys)
	}

	var count struct {
		Count int `json:"count"`
	}

	admin.read("count_logs", &count)

	if count.Count != 2 {
		t.Errorf("count %d", count.Count)
	}

	if problems, err := server.database.Verify(); err != nil || len(problems) > 0 {
		t.Errorf("verify: %v %v", problems, err)
	}
}

// A search that runs out of time says so: the count says how far back it
// searched and bounds the list to that, and a list goes on by its cursor.
func TestLogSearchStopsInTime(t *testing.T) {
	server := newRESTServer(t)
	viewer := server.webUser(store.RoleUser, false)
	at := time.Now()

	var logs []store.NewLog

	for index := range 60 {
		body := "hay"

		if index < 5 || index >= 55 {
			body = "needle"
		}

		logs = append(logs, store.NewLog{ProjectID: 1, Timestamp: at, Level: "info", Body: fmt.Sprintf("%s %d", body, index)})
	}

	if err := server.database.SaveLogs(context.Background(), logs, 0); err != nil {
		t.Fatal(err)
	}

	// Every reading of the clock a second later: a search stops after a few logs.
	var readings atomic.Int64

	server.database.SetClock(func() time.Time { return at.Add(time.Duration(readings.Add(1)) * time.Second) })

	var count struct {
		Count      int        `json:"count"`
		Stopped    bool       `json:"stopped"`
		SearchedTo *time.Time `json:"searched_to"`
		FromID     int64      `json:"from_id"`
	}

	viewer.read("count_logs?query=needle", &count)

	if !count.Stopped || count.Count == 0 || count.Count >= 10 || count.SearchedTo == nil || count.FromID == 0 {
		t.Fatalf("count: %+v", count)
	}

	type page struct {
		Items []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
		Stopped    bool   `json:"stopped"`
	}

	var bounded page

	viewer.read(fmt.Sprintf("list_logs?query=needle&from_id=%d", count.FromID), &bounded)

	if len(bounded.Items) != count.Count || bounded.Stopped || bounded.NextCursor != "" {
		t.Fatalf("list within the count: %+v", bounded)
	}

	found := 0
	cursor := ""

	for range 100 {
		var next page

		server.get("/api/v1/list_logs?query=needle&cursor="+cursor, &next)

		found += len(next.Items)

		if next.NextCursor == "" {
			break
		}

		if !next.Stopped {
			t.Fatalf("a full page in a sparse search: %+v", next)
		}

		cursor = next.NextCursor
	}

	if found != 10 {
		t.Fatalf("the search found %d needles, want 10", found)
	}

	// Stopped while still skipping an offset: no cursor, which would go on
	// from the rows the offset was to skip.
	var skipping page

	server.get("/api/v1/list_logs?query=needle&offset=8", &skipping)

	if !skipping.Stopped || len(skipping.Items) != 0 || skipping.NextCursor != "" {
		t.Fatalf("an offset page that stopped: %+v", skipping)
	}

	// A trace's logs say when their search stopped too.
	traceID := strings.Repeat("ab", 16)
	traced := make([]store.NewLog, 0, 30)

	for index := range 30 {
		traced = append(traced, store.NewLog{ProjectID: 1, Timestamp: at, Level: "info", Body: fmt.Sprintf("traced %d", index), TraceID: traceID, Attributes: []byte(`{}`)})
	}

	if err := server.database.SaveLogs(context.Background(), traced, 0); err != nil {
		t.Fatal(err)
	}

	var trace struct {
		Logs    []json.RawMessage `json:"logs"`
		Stopped bool              `json:"stopped"`
	}

	server.get("/api/v1/list_trace?trace_id="+traceID, &trace)

	if !trace.Stopped || len(trace.Logs) >= 30 {
		t.Fatalf("a trace whose search stopped: %d logs, stopped %v", len(trace.Logs), trace.Stopped)
	}
}

// The project filter of the logs list offers only the projects some log is
// in (one key looked up per project, no log read), and only those the
// viewer sees.
func TestUILogProjects(t *testing.T) {
	server := newRESTServer(t)
	logs := []store.NewLog{{ProjectID: 2, Timestamp: time.Now(), Level: "info", Body: "started", Fingerprint: "started"}}

	if err := server.database.SaveLogs(context.Background(), logs, 0); err != nil {
		t.Fatal(err)
	}

	var projects struct {
		Projects []string `json:"projects"`
	}

	for _, check := range []struct {
		viewer uiUser
		want   []string
	}{
		{server.webUser(store.RoleAdmin, false), []string{"frontend"}},
		{server.webUserWith(store.RoleUser, store.OnlyProjects(1)), []string{}},
		{server.webUserWith(store.RoleUser, store.OnlyProjects(1, 2)), []string{"frontend"}},
	} {
		projects.Projects = nil
		check.viewer.read("list_log_projects", &projects)

		if !slices.Equal(projects.Projects, check.want) {
			t.Errorf("projects with logs %v, want %v", projects.Projects, check.want)
		}
	}
}
