package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/devforth/exceptic/internal/storage/storetest"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/secret"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/projects"
	"github.com/devforth/exceptic/internal/storage/store"
)

func openStore(t *testing.T) *database.Store {
	t.Helper()

	database := storetest.Open(t)

	if err := database.SeedProject(context.Background(), store.Project{ID: 1, Name: "backend", PublicKey: "0123456789abcdef0123456789abcdef"}, time.Now()); err != nil {
		t.Fatal(err)
	}

	return database
}

func testFactory(t *testing.T) notify.Factory {
	t.Helper()

	box, err := secret.New("test-secret-key-test-secret-key-0123")

	if err != nil {
		t.Fatal(err)
	}

	return notify.Factory{Box: box}
}

func TestConnectionsCreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	factory := testFactory(t)
	service := newConnections(slog.New(slog.NewTextHandler(io.Discard, nil)), database, factory, config.Projects{}, &config.Config{})
	firstSecret, _ := factory.EncryptSecret(notify.SlackSecret{WebhookURL: "https://hooks.slack.com/services/first"})

	connection := store.Connection{Name: "alerts", Type: config.ChannelSlack, AllProjects: true, Levels: []string{"error"}, Active: true}

	if _, err := service.create(ctx, connection, firstSecret); err != nil {
		t.Fatal(err)
	}

	connections, err := service.List(ctx, store.Access{})

	if err != nil || len(connections) != 1 {
		t.Fatalf("connections = %v, %v; want the new one", connections, err)
	}

	created := connections[0]

	// A chat needs no confirmation: its destination is created confirmed, holding the secret.
	if created.DestinationID == "" || !created.Verified || created.Secret != firstSecret {
		t.Errorf("created = %+v, want a confirmed destination with the secret", created)
	}

	secondSecret, _ := factory.EncryptSecret(notify.SlackSecret{WebhookURL: "https://hooks.slack.com/services/second"})
	created.Name = "renamed"

	if err := service.update(ctx, created, secondSecret); err != nil {
		t.Fatal(err)
	}

	updated, err := service.Get(ctx, store.Access{}, created.ID)

	if err != nil || updated.Name != "renamed" || updated.Secret != secondSecret || updated.DestinationID != created.DestinationID {
		t.Errorf("updated = %+v, %v; want the new name and secret on the same destination", updated, err)
	}

	if err := service.SetActive(ctx, updated, false); err != nil {
		t.Fatal(err)
	}

	if inactive, _ := service.Get(ctx, store.Access{}, created.ID); inactive.Active {
		t.Errorf("connection still active after SetActive(false)")
	}

	if err := service.Delete(ctx, store.Access{}, created.ID); err != nil {
		t.Fatal(err)
	}

	// Deleted from another tab: not an error.
	if err := service.Delete(ctx, store.Access{}, created.ID); err != nil {
		t.Errorf("second delete = %v, want nil", err)
	}

	if _, err := service.Get(ctx, store.Access{}, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
}

func TestVerificationDestinations(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	verification := newVerification(slog.New(slog.NewTextHandler(io.Discard, nil)), database, notify.Factory{}, nil)

	first, err := verification.DestinationFor(ctx, "dev@example.com")

	if err != nil || first.ID == "" || first.Verified || first.Type != config.ChannelEmail {
		t.Fatalf("DestinationFor = %+v, %v; want a new unconfirmed email destination", first, err)
	}

	again, err := verification.DestinationFor(ctx, "dev@example.com")

	if err != nil || again.ID != first.ID {
		t.Errorf("second DestinationFor = %s, %v; want the same destination %s", again.ID, err, first.ID)
	}

	if found, err := verification.EmailDestination(ctx, first.ID); err != nil || found.Address != "dev@example.com" {
		t.Errorf("EmailDestination = %+v, %v", found, err)
	}

	chatID, err := database.CreateDestination(ctx, store.Destination{Type: config.ChannelSlack, Secret: "x", Verified: true}, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	if _, err := verification.EmailDestination(ctx, chatID); !errors.Is(err, ErrNotEmail) {
		t.Errorf("EmailDestination of a chat = %v, want ErrNotEmail", err)
	}
}

func TestSelectedEvent(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	issues := newIssues(slog.New(slog.DiscardHandler), database)
	now := time.Now().UTC()

	save := func(eventID, fingerprint string, received time.Time) string {
		result, err := database.SaveEvent(ctx, store.NewEvent{
			ProjectID: 1, EventID: eventID, Fingerprint: fingerprint, Title: fingerprint, Level: "error",
			Timestamp: received, ReceivedAt: received, Payload: []byte(`{}`),
		})

		if err != nil {
			t.Fatal(err)
		}

		return result.IssueID
	}

	issueID := save("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "first", now.Add(-time.Minute))
	save("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "first", now)
	save("cccccccccccccccccccccccccccccccc", "other", now)

	issue, err := issues.Get(ctx, store.Access{}, issueID)

	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name, eventID, want string
		err                 error
	}{
		{"latest by default", "", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", nil},
		{"UUID form", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil},
		{"event of another issue", "cccccccccccccccccccccccccccccccc", "", store.ErrNotFound},
		{"unknown event", "dddddddddddddddddddddddddddddddd", "", store.ErrNotFound},
	} {
		event, err := issues.SelectedEvent(ctx, issue, test.eventID)

		if !errors.Is(err, test.err) || event.EventID != test.want {
			t.Errorf("%s: event %q, err %v; want %q, %v", test.name, event.EventID, err, test.want, test.err)
		}
	}
}

// Delete says when there was nothing to delete: the tools answer not found.
func TestDeleteIssue(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	issues := newIssues(slog.New(slog.DiscardHandler), database)
	now := time.Now().UTC()

	result, err := database.SaveEvent(ctx, store.NewEvent{
		ProjectID: 1, EventID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Fingerprint: "first", Title: "first", Level: "error",
		Timestamp: now, ReceivedAt: now, Payload: []byte(`{}`),
	})

	if err != nil {
		t.Fatal(err)
	}

	if err := issues.Delete(ctx, store.Access{}, result.IssueID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if err := issues.Delete(ctx, store.Access{}, result.IssueID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting again: %v, want store.ErrNotFound", err)
	}
}

// Saving an active connection retries its waiting messages now: a save is
// usually the fix for why they failed.
func TestSaveRetriesWaitingMessages(t *testing.T) {
	database := storetest.Open(t)

	ctx := context.Background()
	destination, _ := database.CreateDestination(ctx, store.Destination{Type: "slack", Secret: "x", Verified: true}, time.Now())
	id, _ := database.CreateConnection(ctx, store.Connection{Name: "Chat", DestinationID: destination, AllProjects: true, Levels: []string{"error"}, Active: true}, time.Now())
	channel := id

	if err := database.EnqueueOutbox(ctx, []string{channel}, []int{1}, "notification", []byte(`{}`), time.Now()); err != nil {
		t.Fatal(err)
	}

	messages, _ := database.DueOutbox(ctx, channel, time.Now(), 10)
	_ = database.MarkOutboxRetry(ctx, messages[0].ID, "slack: missing_scope", time.Now().Add(5*time.Minute), time.Now())

	if due, _ := database.DueOutbox(ctx, channel, time.Now(), 10); len(due) != 0 {
		t.Fatal("the message must wait for its retry")
	}

	connection, _ := database.GetConnection(ctx, id)
	connections := newConnections(slog.New(slog.DiscardHandler), database, notify.Factory{}, config.Projects{}, &config.Config{})

	if err := connections.update(ctx, connection, ""); err != nil {
		t.Fatal(err)
	}

	if due, _ := database.DueOutbox(ctx, channel, time.Now().Add(time.Second), 10); len(due) != 1 {
		t.Fatal("saving the connection must make its waiting message due now")
	}
}

// The device stamp changes with whatever should end the trust in a browser:
// a password reset, a role change, the user deleted and created again.
func TestDeviceStamp(t *testing.T) {
	iterations := PasswordIterations
	PasswordIterations = 1000

	t.Cleanup(func() { PasswordIterations = iterations })

	ctx := context.Background()
	database := openStore(t)
	users := NewUsers(slog.New(slog.DiscardHandler), database)
	root := store.User{ID: "root", Email: "root", Role: store.RoleAdmin}

	create := func() string {
		id, _, problems, err := users.Create(ctx, root, UserInput{Email: "Dev@example.com", Role: store.RoleUser})

		if err != nil || len(problems) > 0 {
			t.Fatal(err, problems)
		}

		return id
	}
	stamp := func() string {
		value, err := users.DeviceStamp(ctx, " dev@EXAMPLE.com ")

		if err != nil {
			t.Fatal(err)
		}

		return value
	}

	id := create()
	seen := map[string]bool{"": true}
	changed := func(step string) {
		value := stamp()

		if seen[value] {
			t.Fatalf("%s: the stamp did not change", step)
		}

		seen[value] = true
	}

	changed("created")

	before := stamp()

	if _, err := users.UpdateName(ctx, store.User{ID: id, Email: "dev@example.com"}, "Developer"); err != nil || stamp() != before {
		t.Fatal("a new name changed the stamp", err)
	}

	if _, err := users.ResetPassword(ctx, root, id); err != nil {
		t.Fatal(err)
	}

	changed("password reset")

	if _, err := users.Update(ctx, root, id, UserInput{Email: "dev@example.com", Name: "Dev", Role: store.RoleAdmin}); err != nil {
		t.Fatal(err)
	}

	changed("role change")

	// Another admin, so this one is not the last.
	if _, err := database.CreateUser(ctx, store.User{Email: "other@example.com", Role: store.RoleAdmin}, "x"); err != nil {
		t.Fatal(err)
	}

	if err := users.Delete(ctx, root, id); err != nil {
		t.Fatal(err)
	}

	if stamp() != "" {
		t.Fatal("a deleted user has a stamp")
	}

	create()
	changed("created again")
}

// A repeated confirm of an address confirmed meanwhile stays confirmed.
func TestConfirmCodeOfAConfirmedAddress(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	verification := newVerification(slog.New(slog.DiscardHandler), database, notify.Factory{}, nil)
	now := time.Now()
	id, err := database.CreateDestination(ctx, store.Destination{Type: config.ChannelEmail, Address: "dev@example.com", Verified: true, VerifiedAt: now}, now)

	if err != nil {
		t.Fatal(err)
	}

	step, err := verification.ConfirmCode(ctx, id, "123456")

	if err != nil || !step.Verified() || step.Error != "" || step.Address != "dev@example.com" {
		t.Fatalf("step = %+v, %v; want the address confirmed", step, err)
	}
}

// A new project gets the color its form showed, whatever random id it gets.
func TestNewProjectKeepsTheShownColor(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	registry, err := projects.Open(ctx, slog.New(slog.DiscardHandler), database, nil)

	if err != nil {
		t.Fatal(err)
	}

	service := newProjects(database, registry, &config.Config{}, nil)
	shown := service.Color(store.Project{})
	saved, problems, err := service.Save(ctx, store.User{}, url.Values{"name": {"worker"}, "color": {shown}}, store.Project{})

	if err != nil || len(problems) > 0 {
		t.Fatal(err, problems)
	}

	stored, err := service.Get(ctx, store.Access{}, saved.ID)

	// Stored, not left to the palette of the random id, which may match by chance.
	if err != nil || stored.Color != shown {
		t.Fatalf("color = %q, %v; want %q as shown", stored.Color, err, shown)
	}
}

func TestNewPasswordLength(t *testing.T) {
	long := strings.Repeat("п", config.MaxPasswordLength) // 2 bytes each

	problems := Problems{}
	checkNewPassword(problems, "current", long, long)

	if len(problems) > 0 {
		t.Errorf("%d characters refused: %v", config.MaxPasswordLength, problems)
	}

	checkNewPassword(problems, "current", long+"п", long+"п")

	if problems["password"] == "" {
		t.Error("a password over the maximum accepted")
	}
}

// The name of a Telegram connection whose title is unknown fits the limit.
func TestTelegramFallbackName(t *testing.T) {
	factory := testFactory(t)
	factory.TelegramURL = "http://127.0.0.1:1" // nothing answers: the title is unknown
	chatID := "@" + strings.Repeat("c", config.MaxConnectionName)
	secret, err := factory.EncryptSecret(notify.TelegramSecret{BotToken: "123456:ABC", ChatID: chatID})

	if err != nil {
		t.Fatal(err)
	}

	connections := newConnections(slog.New(slog.DiscardHandler), openStore(t), factory, config.Projects{}, &config.Config{})
	name := connections.defaultName(context.Background(), store.Connection{Type: config.ChannelTelegram}, secret)

	if utf8.RuneCountInString(name) > config.MaxConnectionName || !strings.HasPrefix(name, "Telegram @c") {
		t.Fatalf("name = %q (%d characters)", name, utf8.RuneCountInString(name))
	}
}

// Slack bots post to a channel of their workspace: the same channel name in
// another workspace is another place, "#alerts" and "alerts" the same one.
func TestSlackDuplicates(t *testing.T) {
	ctx := context.Background()
	factory := testFactory(t)
	connections := newConnections(slog.New(slog.DiscardHandler), openStore(t), factory, config.Projects{}, &config.Config{})
	secret, err := factory.EncryptSecret(notify.SlackSecret{BotToken: "xoxb-first", Channel: "#alerts"})

	if err != nil {
		t.Fatal(err)
	}

	connection := store.Connection{Name: "alerts", Type: config.ChannelSlack, AllProjects: true, Levels: []string{"error"}, Active: true}

	if _, err := connections.create(ctx, connection, secret); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		secret    notify.SlackSecret
		duplicate bool
	}{
		{notify.SlackSecret{BotToken: "xoxb-first", Channel: "alerts"}, true},
		{notify.SlackSecret{BotToken: "xoxb-second", Channel: "#alerts"}, false},
	} {
		duplicate, err := connections.findDuplicate(ctx, store.Access{}, config.ChannelSlack, test.secret.Key(), "")

		if err != nil || (duplicate != nil) != test.duplicate {
			t.Errorf("%+v: duplicate %v, %v; want %v", test.secret, duplicate, err, test.duplicate)
		}
	}
}

// The storage of the Projects page: the data the database holds as the
// cleanup measures it, the files apart, the percent of the limit capped at
// 100; without a limit none.
func TestStorage(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	registry, err := projects.Open(ctx, slog.New(slog.DiscardHandler), database, nil)

	if err != nil {
		t.Fatal(err)
	}

	full, err := newProjects(database, registry, &config.Config{Retention: config.Retention{MaxDatabaseBytes: 1}}, nil).Storage(ctx, store.Access{})

	if err != nil || full.Percent != 100 || full.Limit == "" || full.UsedBytes == 0 || full.CleanupStopPercent != config.CleanupStopPercent ||
		full.CleanupStartPercent != config.CleanupStartPercent {
		t.Errorf("storage = %+v, %v; want the percent capped at 100", full, err)
	}

	usage, err := database.ProjectUsage(ctx)

	if err != nil || full.UsedBytes != usage.Live || full.FilesBytes != usage.Files || full.Files == "" || full.OtherBytes != usage.Live {
		t.Errorf("storage = %+v, usage %+v, %v; want the data held and the files", full, usage, err)
	}

	unlimited, err := newProjects(database, registry, &config.Config{}, nil).Storage(ctx, store.Access{})

	if err != nil || unlimited.Limit != "" || unlimited.Percent != 0 || unlimited.Used == "" {
		t.Errorf("no size limit: %+v, %v", unlimited, err)
	}
}

// A run over the database leaves out, for a limited viewer, the steps that
// deleted only in hidden projects, and the hidden counts of the others.
func TestVisibleRunShowsHiddenProjectsAsOther(t *testing.T) {
	run := store.CleanupRun{Compacted: 1000, Ratio: 1, Steps: []store.CleanupStep{
		{Number: 1, Freed: 100, Projects: map[int]*store.CleanupCounts{2: {Logs: 5}}},
		{Number: 2, Freed: 200, Projects: map[int]*store.CleanupCounts{1: {Logs: 1}, 2: {Logs: 3}, 3: {Logs: 4}}},
		{Number: 3, Freed: 300},
	}}

	visible, ok := VisibleRun(store.OnlyProjects(1), run)
	shown := visible.Run

	if !ok || len(shown.Steps) != 3 {
		t.Fatalf("steps shown: %+v", shown.Steps)
	}

	// Projects outside access only as one Other row, by no name.
	if other := shown.Steps[1].Projects[store.OtherProjects]; other == nil || other.Logs != 7 || shown.Steps[1].Projects[1].Logs != 1 || len(shown.Steps[1].Projects) != 2 {
		t.Errorf("step 2 shown %v", shown.Steps[1].Projects)
	}

	if other := shown.Steps[0].Projects[store.OtherProjects]; other == nil || other.Logs != 5 || len(shown.Steps[0].Projects) != 1 {
		t.Errorf("step 1 shown %v", shown.Steps[0].Projects)
	}

	if len(run.Steps[1].Projects) != 3 || run.Steps[1].Projects[2].Logs != 3 {
		t.Errorf("the stored run changed: %v", run.Steps[1].Projects)
	}

	if order := store.StepProjects(shown.Steps[1].Projects); order[len(order)-1] != store.OtherProjects {
		t.Errorf("order %v: Other goes last", order)
	}

	// The sizes stay the database's.
	if visible.Left != 400 || !visible.Known {
		t.Errorf("left = %d, want 400 as for every project", visible.Left)
	}

	if all, _ := VisibleRun(store.Access{}, run); all.Run.Steps[1].Projects[store.OtherProjects] != nil {
		t.Errorf("every project: %+v", all.Run.Steps)
	}

	if _, ok := VisibleRun(store.OnlyProjects(1), store.CleanupRun{Project: 2}); ok {
		t.Error("a run of a hidden project is shown")
	}
}

// stoppedLogs answers every log search as one that ran out of time.
type stoppedLogs struct {
	store.Store
	searches int
}

func (logs *stoppedLogs) ListLogs(context.Context, store.LogFilter) (store.LogPage, error) {
	logs.searches++

	return store.LogPage{Logs: []store.LogRecord{}, LogScan: store.LogScan{Stopped: true, Rest: 10}}, nil
}

// A related-logs search that ran out of time is not searched again by the
// trace's window and says it stopped, and its trace may still have logs: the link stays.
func TestRelatedLogsStoppedSearch(t *testing.T) {
	ctx := context.Background()
	logs := &stoppedLogs{Store: openStore(t)}
	issues := newIssues(slog.New(slog.DiscardHandler), logs)
	event := store.Event{EventSummary: store.EventSummary{ProjectID: 1, TraceID: "trace", SpanID: "span", Timestamp: time.Now()}}

	related, err := issues.RelatedLogs(ctx, store.Access{}, event, nil)

	if err != nil || logs.searches != 1 || !related.Stopped {
		t.Errorf("searches %d, stopped %v, %v: a stopped span search must not fall back and says it stopped", logs.searches, related.Stopped, err)
	}

	if found, err := issues.TraceLogs(ctx, store.Access{}, event); err != nil || !found {
		t.Errorf("trace has logs %v, %v: unknown after a stopped search", found, err)
	}
}

// Without logs of its spans, an event's related logs are those of its trace
// within a minute of it by the server's clock and by the device's, not the
// hours between them; a time ingest replaced by the receive time is no drift.
func TestRelatedLogsAroundBothClocks(t *testing.T) {
	ctx := context.Background()
	database := openStore(t)
	issues := newIssues(slog.New(slog.DiscardHandler), database)
	// The server's time; the device's clock is an hour behind it.
	at := time.Now().Truncate(time.Second)
	device := at.Add(-time.Hour)
	logAt := func(body string, timestamp time.Time) store.NewLog {
		return store.NewLog{ProjectID: 1, Timestamp: timestamp, Level: "error", Body: body, TraceID: "trace"}
	}

	if err := database.SaveLogs(ctx, []store.NewLog{
		logAt("by the server's clock", at.Add(-20*time.Second)),
		logAt("by the device's clock", device.Add(10*time.Second)),
		logAt("between the two", at.Add(-30*time.Minute)),
	}, 0); err != nil {
		t.Fatal(err)
	}

	bodies := func(event store.Event, sent time.Time) []string {
		t.Helper()

		related, err := issues.RelatedLogs(ctx, store.Access{}, event, &sentry.Event{Timestamp: sentry.Timestamp{Time: sent}})

		if err != nil {
			t.Fatal(err)
		}

		var found []string

		for _, record := range related.Logs {
			found = append(found, record.Body)
		}

		slices.Sort(found)

		return found
	}

	drifted := store.Event{EventSummary: store.EventSummary{ProjectID: 1, TraceID: "trace", Timestamp: at, ReceivedAt: at.Add(time.Second)}}

	if found := bodies(drifted, device); !slices.Equal(found, []string{"by the device's clock", "by the server's clock"}) {
		t.Errorf("a device an hour behind: %v", found)
	}

	// A time ingest replaced (stored as the receive time) has no drift: only the server's window.
	replaced := store.Event{EventSummary: store.EventSummary{ProjectID: 1, TraceID: "trace", Timestamp: at, ReceivedAt: at}}

	if found := bodies(replaced, device); !slices.Equal(found, []string{"by the server's clock"}) {
		t.Errorf("a replaced time: %v", found)
	}

	if drift := ClockDrift(replaced, &sentry.Event{Timestamp: sentry.Timestamp{Time: device}}); drift != 0 {
		t.Errorf("drift of a replaced time: %s", drift)
	}
}
