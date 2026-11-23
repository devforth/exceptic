package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
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

	if code, _ := remove(user, map[string]any{"log_ids": []int64{ids["backend chosen"]}}); code != http.StatusNotFound {
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
