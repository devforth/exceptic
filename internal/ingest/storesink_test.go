package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/fixtures"
	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Replays every recorded SDK request into a real database, twice.
func TestFixturesEndToEnd(t *testing.T) {
	server := newTestServer(t, testSettings())
	database := server.db

	recorded := fixtures.All(t)
	eventFixtures := 0

	for _, fixture := range recorded {
		if strings.Contains(fixture.Name, "-event-") {
			eventFixtures++
		}
	}

	// The second pass sends the same event_ids again, like an SDK retry.
	for pass := 1; pass <= 2; pass++ {
		for _, fixture := range recorded {
			if response := server.send(fixture.HTTP()); response.Code != http.StatusOK {
				t.Fatalf("pass %d, %s: got %d %s", pass, fixture.Name, response.Code, response.Body)
			}
		}

		server.waitStored()
	}

	if events := countEvents(t, database, nil); events != eventFixtures {
		t.Fatalf("events = %d, want %d (duplicates must be ignored)", events, eventFixtures)
	}

	if issues := countIssues(t, database, store.IssueFilter{}, nil); issues != eventFixtures {
		// Every event fixture is a different error, so each is its own issue.
		t.Fatalf("issues = %d, want %d", issues, eventFixtures)
	}

	if frontend := countIssues(t, database, store.IssueFilter{ProjectIDs: []int{2}}, nil); frontend != 4 {
		t.Fatalf("frontend issues = %d, want 4", frontend)
	}

	mismatched := countIssues(t, database, store.IssueFilter{}, func(issue store.Issue) bool {
		return issue.EventCount != countEvents(t, database, func(event store.EventSummary) bool { return event.IssueID == issue.ID })
	})

	if mismatched != 0 {
		t.Fatalf("%d issues have event_count out of sync", mismatched)
	}
	// Logs have no ID, so a retried batch is stored again: both passes count.
	if logs := countLogs(t, database, store.LogFilter{}); logs == 0 || logs%2 != 0 {
		t.Fatalf("logs = %d", logs)
	}

	if linked := countEvents(t, database, func(event store.EventSummary) bool { return event.TraceID != "" }); linked != eventFixtures {
		t.Fatalf("events with trace_id = %d, want all %d", linked, eventFixtures)
	}

	if culprits := countIssues(t, database, store.IssueFilter{}, func(issue store.Issue) bool { return issue.Culprit == "" }); culprits != 0 {
		t.Fatalf("%d issues without a culprit", culprits)
	}
}

type recordingNotifier struct {
	notifications []notify.Notification
}

func (notifier *recordingNotifier) Notify(notification notify.Notification) {
	notifier.notifications = append(notifier.notifications, notification)
}

// sendEvent sends an envelope with one event to project 1 or 2 and waits until it is stored.
func (server *testServer) sendEvent(projectID int, eventID, payload string) {
	server.t.Helper()

	key := map[int]string{1: fixtures.BackendKey, 2: fixtures.BrowserKey}[projectID]
	body := `{"event_id":"` + eventID + `"}` + "\n" + `{"type":"event"}` + "\n" + payload + "\n"
	server.sendStored(projectID, key, body)
}

// sendLogs sends an envelope with logs (JSON objects) to project 1 or 2 and waits until they are stored.
func (server *testServer) sendLogs(projectID int, logs []string) {
	server.t.Helper()

	key := map[int]string{1: fixtures.BackendKey, 2: fixtures.BrowserKey}[projectID]
	body := "{}\n" + `{"type":"log"}` + "\n" + `{"items":[` + strings.Join(logs, ",") + "]}\n"
	server.sendStored(projectID, key, body)
}

func (server *testServer) sendStored(projectID int, key, body string) {
	server.t.Helper()

	if response := server.send(newRequest(http.MethodPost, envelopeURL(strconv.Itoa(projectID), key), []byte(body))); response.Code != http.StatusOK {
		server.t.Fatalf("got %d %s", response.Code, response.Body)
	}

	server.waitStored()
}

// New issues and regressions are notified; repeats of a known issue are not.
func TestStoreSinkNotifies(t *testing.T) {
	notifier := &recordingNotifier{}
	server := startTestServer(t, testSettings(), StoreSink{Logger: slog.New(slog.DiscardHandler), Notifier: notifier, PublicURL: "https://errors.example.com"})
	boom := `{"exception":{"values":[{"type":"Error","value":"boom"}]}}`

	for _, id := range []string{"00000000000000000000000000000001", "00000000000000000000000000000002"} {
		server.sendEvent(1, id, boom)
	}

	if len(notifier.notifications) != 1 || notifier.notifications[0].Kind != notify.NewIssue {
		t.Fatalf("after a new issue and a repeat: %+v", notifier.notifications)
	}

	first := notifier.notifications[0]

	if first.IssueURL != "https://errors.example.com/issues/"+first.IssueID || first.Project != "backend" || first.Title != "Error: boom" || first.EventID != "00000000000000000000000000000001" {
		t.Fatalf("notification: %+v", first)
	}

	if _, err := server.db.SetIssuesStatus(context.Background(), []string{first.IssueID}, store.StatusResolved, store.Actor{}); err != nil {
		t.Fatal(err)
	}

	server.sendEvent(1, "00000000000000000000000000000003", boom)

	if len(notifier.notifications) != 2 || notifier.notifications[1].Kind != notify.Regression {
		t.Fatalf("after the resolved issue happened again: %+v", notifier.notifications)
	}
}

// A message logged in a loop keeps its newest config.LogsPerMessage copies.
func TestStoreSinkLimitsRepeatedLogs(t *testing.T) {
	server := newTestServer(t, testSettings())

	for _, projectID := range []int{1, 2} {
		for batch := range 4 {
			var logs []string

			for index := range 25 {
				logs = append(logs, fmt.Sprintf(`{"level":"warn","body":"retry %d of order %d"}`, index, batch))
			}

			server.sendLogs(projectID, logs)
		}
	}

	for _, projectID := range []int{1, 2} {
		if count := countLogs(t, server.db, store.LogFilter{ProjectIDs: []int{projectID}}); count != config.LogsPerMessage {
			t.Errorf("project %d: %d logs, want %d", projectID, count, config.LogsPerMessage)
		}
	}
}

// Events of every level are stored and reported to the notifier (whose
// connections filter by level); logs of levels a project does not record
// are dropped.
func TestStoreSinkRecordsChosenLevels(t *testing.T) {
	notifier := &recordingNotifier{}
	settings := testSettings()
	warnOnly := []string{"warn"}
	settings.Projects[1].LogLevels = &warnOnly
	server := startTestServer(t, settings, StoreSink{Logger: slog.New(slog.DiscardHandler), Notifier: notifier})

	for _, projectID := range []int{1, 2} {
		for index, level := range []string{"error", "warning", "info", "fatal", ""} {
			server.sendEvent(projectID, fmt.Sprintf("%031d%d", projectID, index), fmt.Sprintf(`{"level":%q,"message":"message %s"}`, level, level))
		}

		server.sendLogs(projectID, []string{`{"level":"info","body":"a"}`, `{"level":"warn","body":"b"}`, `{"level":"error","body":"c"}`})
	}

	database := server.db

	// Every level, and an event without a level (an error), of each project.
	if events := countEvents(t, database, nil); events != 10 {
		t.Errorf("%d events stored, want 10", events)
	}

	if notified := len(notifier.notifications); notified != 10 {
		t.Errorf("%d notifications, want 10", notified)
	}

	if logs := countLogs(t, database, store.LogFilter{ProjectIDs: []int{1}}); logs != 3 {
		t.Errorf("backend: %d logs, want every level", logs)
	}

	if logs := countLogs(t, database, store.LogFilter{ProjectIDs: []int{2}, Levels: []string{"warn"}}); logs != 1 || countLogs(t, database, store.LogFilter{ProjectIDs: []int{2}}) != 1 {
		t.Error("frontend records only warn logs")
	}
}

// Counts through the store, so they hold for every implementation.

func countEvents(t *testing.T, database store.Store, keep func(store.EventSummary) bool) int {
	t.Helper()

	events, err := database.ListEvents(context.Background(), store.EventFilter{Limit: 1 << 20})

	if err != nil {
		t.Fatal(err)
	}

	count := 0

	for _, event := range events {
		if keep == nil || keep(event) {
			count++
		}
	}

	return count
}

func countIssues(t *testing.T, database store.Store, filter store.IssueFilter, keep func(store.Issue) bool) int {
	t.Helper()

	filter.Limit = 1 << 20
	issues, err := database.ListIssues(context.Background(), filter)

	if err != nil {
		t.Fatal(err)
	}

	count := 0

	for _, issue := range issues {
		if keep == nil || keep(issue) {
			count++
		}
	}

	return count
}

func countLogs(t *testing.T, database store.Store, filter store.LogFilter) int {
	t.Helper()

	count, _, err := database.CountLogs(context.Background(), filter)

	if err != nil {
		t.Fatal(err)
	}

	return count
}
