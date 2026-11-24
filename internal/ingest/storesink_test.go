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

// An issue ignored until 2 more events is notified as a regression at the 2nd.
func TestStoreSinkNotifiesTheEndOfAnIgnore(t *testing.T) {
	notifier := &recordingNotifier{}
	server := startTestServer(t, testSettings(), StoreSink{Logger: slog.New(slog.DiscardHandler), Notifier: notifier})
	boom := `{"exception":{"values":[{"type":"Error","value":"boom"}]}}`

	server.sendEvent(1, "00000000000000000000000000000001", boom)

	if _, err := server.db.IgnoreIssues(context.Background(), []string{notifier.notifications[0].IssueID}, store.IgnoreBound{Events: 2}, store.Actor{}); err != nil {
		t.Fatal(err)
	}

	server.sendEvent(1, "00000000000000000000000000000002", boom)

	if len(notifier.notifications) != 1 {
		t.Fatalf("notified while ignored: %+v", notifier.notifications)
	}

	server.sendEvent(1, "00000000000000000000000000000003", boom)

	if len(notifier.notifications) != 2 || notifier.notifications[1].Kind != notify.Regression || notifier.notifications[1].EventID != "00000000000000000000000000000003" {
		t.Fatalf("after the events the ignore waited for: %+v", notifier.notifications)
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

// An error in a loop stores config.EventsPerIssuePerMinute events a minute
// and counts every one; a different error in the same minute is stored.
func TestStoreSinkCapsEventsPerIssue(t *testing.T) {
	server := newTestServer(t, testSettings())
	key := fixtures.BackendKey
	loop := `{"exception":{"values":[{"type":"Error","value":"in a loop"}]}}`

	const sent = 10000

	for index := range sent {
		body := fmt.Sprintf(`{"event_id":"%032x"}`+"\n"+`{"type":"event"}`+"\n"+loop+"\n", index+1)

		if response := server.send(newRequest(http.MethodPost, envelopeURL("1", key), []byte(body))); response.Code != http.StatusOK {
			t.Fatalf("got %d %s", response.Code, response.Body)
		}
	}

	server.sendEvent(1, fmt.Sprintf("%032x", sent+1), `{"exception":{"values":[{"type":"TypeError","value":"another"}]}}`)

	issues, err := server.db.ListIssues(context.Background(), store.IssueFilter{})

	if err != nil || len(issues) != 2 {
		t.Fatalf("issues %+v (%v)", issues, err)
	}

	for _, issue := range issues {
		stored := countEvents(t, server.db, func(event store.EventSummary) bool { return event.IssueID == issue.ID })

		switch issue.Title {
		case "TypeError: another":
			if stored != 1 || issue.EventCount != 1 || issue.NotStored != 0 {
				t.Errorf("another error: %d stored, %+v", stored, issue)
			}
		default:
			// The loop may run across a minute's end.
			minutes := int(issue.LastSeen.Unix()/60-issue.FirstSeen.Unix()/60) + 1

			if issue.EventCount != sent || stored+issue.NotStored != sent || stored > minutes*config.EventsPerIssuePerMinute {
				t.Errorf("the loop over %d minutes: %d stored, %+v", minutes, stored, issue)
			}
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

// A log without a known level takes it from its severity number (OpenTelemetry
// ranges, as SDKs send it); a known level wins over the number.
func TestStoreSinkLevelFromSeverityNumber(t *testing.T) {
	server := newTestServer(t, testSettings())

	server.sendLogs(1, []string{
		`{"body":"a","severity_number":17}`, `{"level":"bogus","body":"b","severity_number":13}`,
		`{"level":"info","body":"c","severity_number":21}`, `{"body":"d","severity_number":99}`,
	})

	for level, want := range map[string]int{"error": 1, "warn": 1, "info": 2, "fatal": 0} {
		if count := countLogs(t, server.db, store.LogFilter{ProjectIDs: []int{1}, Levels: []string{level}}); count != want {
			t.Errorf("%s: %d logs, want %d", level, count, want)
		}
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

	count, err := database.CountLogs(context.Background(), filter)

	if err != nil {
		t.Fatal(err)
	}

	return count.Count
}
