package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/storetest"

	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

// fakeChannel records what it was asked to send and can fail first.
type fakeChannel struct {
	mutex     sync.Mutex
	sent      []Notification
	summaries []Summary
	notices   []Notice
	attempts  int
	failFirst int   // fail this many attempts before succeeding
	failWith  error // the error of a failing attempt
}

func (channel *fakeChannel) Name() string { return "fake" }

func (channel *fakeChannel) attempt() error {
	channel.attempts++

	if channel.attempts <= channel.failFirst {
		if channel.failWith != nil {
			return channel.failWith
		}

		return errors.New("temporary failure")
	}

	return nil
}

func (channel *fakeChannel) Send(_ context.Context, notification Notification) error {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()

	if err := channel.attempt(); err != nil {
		return err
	}

	channel.sent = append(channel.sent, notification)

	return nil
}

func (channel *fakeChannel) SendSummary(_ context.Context, summary Summary) error {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()

	if err := channel.attempt(); err != nil {
		return err
	}

	channel.summaries = append(channel.summaries, summary)

	return nil
}

func (channel *fakeChannel) SendNotice(_ context.Context, notice Notice) error {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()

	if err := channel.attempt(); err != nil {
		return err
	}

	channel.notices = append(channel.notices, notice)

	return nil
}

func (channel *fakeChannel) noticeCount() int {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()

	return len(channel.notices)
}

func (channel *fakeChannel) sentCount() int {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()

	return len(channel.sent)
}

func (channel *fakeChannel) attemptCount() int {
	channel.mutex.Lock()
	defer channel.mutex.Unlock()

	return channel.attempts
}

// fakeFactory hands out the channel registered for each connection id.
type fakeFactory struct {
	mutex    sync.Mutex
	channels map[string]Channel
}

func (factory *fakeFactory) Channel(connection store.Connection) (Channel, error) {
	factory.mutex.Lock()
	defer factory.mutex.Unlock()

	if channel, ok := factory.channels[connection.ID]; ok {
		return channel, nil
	}

	return nil, fmt.Errorf("no channel for connection %s", connection.ID)
}

// testSetup is a database with connections and a fake channel for each.
type testSetup struct {
	t        *testing.T
	database *database.Store
	factory  *fakeFactory
}

func newSetup(t *testing.T) *testSetup {
	t.Helper()

	database := storetest.Open(t)

	return &testSetup{t: t, database: database, factory: &fakeFactory{channels: map[string]Channel{}}}
}

// connect creates an active connection receiving the given projects (nil:
// all projects) and levels, delivering to channel.
func (setup *testSetup) connect(channel Channel, projects []int, levels ...string) string {
	setup.t.Helper()

	if len(levels) == 0 {
		levels = []string{"error", "fatal"}
	}

	destination, err := setup.database.CreateDestination(context.Background(), store.Destination{Type: "slack", Verified: true}, time.Now())

	if err != nil {
		setup.t.Fatal(err)
	}

	id, err := setup.database.CreateConnection(context.Background(), store.Connection{
		Name: "test", DestinationID: destination, AllProjects: projects == nil, ProjectIDs: projects,
		Levels: levels, Active: true,
	}, time.Now())

	if err != nil {
		setup.t.Fatal(err)
	}

	setup.factory.mutex.Lock()
	setup.factory.channels[id] = channel
	setup.factory.mutex.Unlock()

	return id
}

// start runs a notifier with fast test timings; options override them.
func (setup *testSetup) start(options Options) *Notifier {
	if options.Store == nil {
		options.Store = setup.database
	}

	options.Channels = setup.factory

	if options.PollInterval == 0 {
		options.PollInterval = 5 * time.Millisecond
	}

	if options.Schedule == nil {
		options.Schedule = []time.Duration{time.Millisecond}
	}

	if options.Window == 0 {
		options.Window = time.Hour
	}

	return New(options)
}

func (setup *testSetup) outboxCounts() map[string]int {
	setup.t.Helper()

	messages, err := setup.database.ListOutbox(context.Background(), store.OutboxFilter{})

	if err != nil {
		setup.t.Fatal(err)
	}

	counts := map[string]int{}

	for _, message := range messages {
		counts[message.Status]++
	}

	return counts
}

func closeNotifier(t *testing.T, notifier *Notifier) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	defer cancel()

	if err := notifier.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

// waitFor polls condition until it holds or two seconds pass.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)

	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func issue(id int, level string) Notification {
	return Notification{Kind: NewIssue, ProjectID: 1, Project: "backend", IssueID: fmt.Sprintf("issue%03d", id), Title: fmt.Sprintf("Error %d", id), Level: level}
}

func titles(notifications []Notification) string {
	var result []string

	for _, notification := range notifications {
		result = append(result, notification.Title)
	}

	return strings.Join(result, ", ")
}

// Each connection gets only its projects and levels.
func TestRouting(t *testing.T) {
	setup := newSetup(t)

	backendErrors := &fakeChannel{}
	everythingWarnings := &fakeChannel{}
	disabled := &fakeChannel{}

	setup.connect(backendErrors, []int{1}, "error")
	setup.connect(everythingWarnings, nil, "warning", "error")

	disabledID := setup.connect(disabled, nil, "error")
	connection, _ := setup.database.GetConnection(context.Background(), disabledID)
	connection.Active = false

	if err := setup.database.UpdateConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}

	notifier := setup.start(Options{})

	for _, notification := range []Notification{
		{ProjectID: 1, Title: "backend error", Level: "error"},
		{ProjectID: 2, Title: "frontend warning", Level: "warning"},
		{ProjectID: 1, Title: "backend info", Level: "info"},
		// Unknown levels count as errors: better one message too many.
		{ProjectID: 1, Title: "backend panic", Level: "panic"},
	} {
		notifier.Notify(notification)
	}

	closeNotifier(t, notifier)

	if got := titles(backendErrors.sent); got != "backend error, backend panic" {
		t.Errorf("project 1, errors: %s", got)
	}

	if got := titles(everythingWarnings.sent); got != "backend error, frontend warning, backend panic" {
		t.Errorf("all projects, warnings and errors: %s", got)
	}

	if len(disabled.sent) != 0 {
		t.Errorf("a disabled connection got %s", titles(disabled.sent))
	}
}

// A notification keeps the title and culprit an issue keeps: an error
// message of megabytes must not reach memory, the outbox and every channel.
func TestNotificationTextIsCapped(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}

	setup.connect(channel, nil)

	notifier := setup.start(Options{})
	huge := strings.Repeat("x", 2<<20)

	notifier.Notify(Notification{ProjectID: 1, Level: "error", Title: huge, Culprit: huge, Environment: huge, Release: huge})
	closeNotifier(t, notifier)

	if len(channel.sent) != 1 {
		t.Fatalf("%d notifications sent", len(channel.sent))
	}

	sent := channel.sent[0]

	for name, length := range map[string]int{
		"title": len(sent.Title), "culprit": len(sent.Culprit), "environment": len(sent.Environment), "release": len(sent.Release),
	} {
		if length > store.MaxTitleLength*3 {
			t.Errorf("%s has %d bytes", name, length)
		}
	}
}

func TestFloodLimitPerConnection(t *testing.T) {
	setup := newSetup(t)
	busy := &fakeChannel{}
	quiet := &fakeChannel{}

	setup.connect(busy, nil, "error")
	setup.connect(quiet, nil, "fatal")

	notifier := setup.start(Options{MaxPerWindow: 3})

	for id := range 40 {
		notifier.Notify(issue(id, "error"))
	}

	notifier.Notify(issue(100, "fatal"))

	// Close ends the window: the held notifications go out as a summary.
	closeNotifier(t, notifier)

	if len(busy.sent) != 3 || len(busy.summaries) != 1 || busy.summaries[0].Count != 37 || len(busy.summaries[0].Items) != maxSummaryItems {
		t.Fatalf("busy: sent %d, summaries %+v", len(busy.sent), busy.summaries)
	}

	if busy.summaries[0].Items[0].Title != "Error 3" {
		t.Fatalf("summary starts with %q, want the first held issue", busy.summaries[0].Items[0].Title)
	}

	// The limit of one connection does not hold back another.
	if len(quiet.sent) != 1 || len(quiet.summaries) != 0 {
		t.Fatalf("quiet: sent %d, summaries %d", len(quiet.sent), len(quiet.summaries))
	}
}

// Each project has its own limit and its own summary, so a flood of one
// project neither holds back nor joins another's.
func TestFloodLimitPerProject(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}
	setup.connect(channel, nil)

	notifier := setup.start(Options{MaxPerWindow: 3})

	for id := range 10 {
		notifier.Notify(issue(id, "error"))
	}

	frontend := issue(100, "error")
	frontend.ProjectID, frontend.Project = 2, "frontend"
	notifier.Notify(frontend)

	closeNotifier(t, notifier)

	if len(channel.sent) != 4 || channel.sent[3].Project != "frontend" {
		t.Fatalf("sent %s, want 3 of backend and frontend's", titles(channel.sent))
	}

	if len(channel.summaries) != 1 || channel.summaries[0].Project != "backend" || channel.summaries[0].Count != 7 {
		t.Fatalf("summaries %+v, want one of backend's 7", channel.summaries)
	}

	if heading := channel.summaries[0].heading(); !strings.HasPrefix(heading, "backend: 7 more issues in the last ") {
		t.Fatalf("heading %q", heading)
	}
}

func TestFloodLimitResetsEveryWindow(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}
	setup.connect(channel, nil)

	windowEnds := make(chan time.Time)
	notifier := setup.start(Options{MaxPerWindow: 1, windowEnds: windowEnds})

	notifier.Notify(issue(1, "error"))
	notifier.Notify(issue(2, "error")) // held: over the limit of this window

	// Once the queue is empty, the dispatcher takes the window's end only
	// after it has routed both.
	waitFor(t, "both routed", func() bool { return len(notifier.queue) == 0 })

	windowEnds <- time.Now()

	notifier.Notify(issue(3, "error")) // a new window: sent individually
	closeNotifier(t, notifier)

	if len(channel.sent) != 2 || channel.sent[1].IssueID != "issue003" || len(channel.summaries) != 1 {
		t.Fatalf("sent %d, summaries %d", len(channel.sent), len(channel.summaries))
	}
}

func TestRetriesUntilSent(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 2}
	setup.connect(channel, nil)

	notifier := setup.start(Options{})

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the third attempt to succeed", func() bool { return channel.sentCount() == 1 })
	closeNotifier(t, notifier)

	if channel.attempts != 3 || setup.outboxCounts()["sent"] != 1 {
		t.Fatalf("attempts %d, outbox %v", channel.attempts, setup.outboxCounts())
	}
}

func TestRetryAfterDelayIsRespected(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 1, failWith: &RetryAfterError{Delay: 80 * time.Millisecond, Err: errors.New("429")}}
	setup.connect(channel, nil)

	notifier := setup.start(Options{})
	start := time.Now()

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the retry", func() bool { return channel.sentCount() == 1 })

	if elapsed := time.Since(start); elapsed < 80*time.Millisecond {
		t.Fatalf("retried after %s, the channel asked for 80ms", elapsed)
	}

	closeNotifier(t, notifier)
}

func TestGivesUpAfterRetryFor(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 1_000_000}
	setup.connect(channel, nil)

	notifier := setup.start(Options{Schedule: []time.Duration{10 * time.Millisecond}, RetryFor: 100 * time.Millisecond})

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the message to be marked failed", func() bool { return setup.outboxCounts()["failed"] == 1 })
	closeNotifier(t, notifier)

	// The exact count depends on machine speed; what matters is that it
	// retried before giving up.
	if channel.attempts < 2 {
		t.Fatalf("gave up after %d attempt, want retries within RetryFor", channel.attempts)
	}
}

// A message that could not be sent before shutdown is sent after the next start.
func TestPendingMessagesSurviveRestart(t *testing.T) {
	setup := newSetup(t)
	down := &fakeChannel{failFirst: 1}
	setup.connect(down, nil)

	first := setup.start(Options{Schedule: []time.Duration{time.Hour}})

	first.Notify(issue(1, "error"))
	waitFor(t, "the first failed attempt", func() bool { return down.attemptCount() == 1 })
	closeNotifier(t, first)

	// The retry is due in an hour; after a restart the message is still pending.
	if counts := setup.outboxCounts(); counts["pending"] != 1 {
		t.Fatalf("after shutdown: %v", counts)
	}

	second := setup.start(Options{Now: func() time.Time { return time.Now().Add(2 * time.Hour) }})

	waitFor(t, "the pending message after restart", func() bool { return down.sentCount() == 1 })
	closeNotifier(t, second)

	if down.sent[0].Title != "Error 1" {
		t.Fatalf("sent %+v", down.sent[0])
	}
}

// unmarkingStore fails to record sent messages while failMarks is set, as a
// full disk does.
type unmarkingStore struct {
	store.Store
	failMarks atomic.Bool
	marks     atomic.Int32
}

func (database *unmarkingStore) MarkOutboxSent(ctx context.Context, id int64, now time.Time) error {
	database.marks.Add(1)

	if database.failMarks.Load() {
		return errors.New("no space left on device")
	}

	return database.Store.MarkOutboxSent(ctx, id, now)
}

// A message the store could not record as sent is recorded later, not sent again.
func TestSentMessageIsNotSentAgainWhenNotRecorded(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}
	setup.connect(channel, nil)

	database := &unmarkingStore{Store: setup.database}
	database.failMarks.Store(true)

	notifier := setup.start(Options{Store: database})

	notifier.Notify(issue(1, "error"))
	waitFor(t, "several polls after the send", func() bool { return database.marks.Load() >= 5 })
	database.failMarks.Store(false)
	waitFor(t, "the message recorded as sent", func() bool { return setup.outboxCounts()["sent"] == 1 })
	closeNotifier(t, notifier)

	if sent := channel.sentCount(); sent != 1 {
		t.Fatalf("sent %d times", sent)
	}
}

// A rate limit stops the pass: the other due messages wait instead of
// spending their attempts on the same limit.
func TestRateLimitStopsTheBatch(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 1_000_000, failWith: &RetryAfterError{Delay: time.Hour, Err: errors.New("429")}}
	id := setup.connect(channel, nil)

	for index := range 3 {
		payload, _ := json.Marshal(issue(index, "error"))

		if err := setup.database.EnqueueOutbox(context.Background(), []string{id}, []int{1}, kindNotification, payload, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	notifier := setup.start(Options{})

	waitFor(t, "the first attempt", func() bool { return channel.attemptCount() > 0 })
	closeNotifier(t, notifier)

	if attempts := channel.attemptCount(); attempts != 1 {
		t.Fatalf("%d attempts after a rate limit", attempts)
	}
}

// A Retry-After past the retry window fails the message it came with and
// does not pause the connection: the next message is sent at once.
func TestRetryAfterBeyondRetryForDoesNotPause(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 1, failWith: &RetryAfterError{Delay: 48 * time.Hour, Err: errors.New("429")}}
	setup.connect(channel, nil)

	notifier := setup.start(Options{RetryFor: 24 * time.Hour})

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the first message to fail", func() bool { return setup.outboxCounts()["failed"] == 1 })

	notifier.Notify(issue(2, "error"))
	waitFor(t, "the second message", func() bool { return channel.sentCount() == 1 })
	closeNotifier(t, notifier)
}

// Disabling a connection fails its queued messages instead of sending them later.
func TestDisabledConnectionFailsQueuedMessages(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 1}
	id := setup.connect(channel, nil)

	notifier := setup.start(Options{Schedule: []time.Duration{50 * time.Millisecond}})

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the first failed attempt", func() bool { return channel.attemptCount() == 1 })

	connection, _ := setup.database.GetConnection(context.Background(), id)
	connection.Active = false

	if err := setup.database.UpdateConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}

	waitFor(t, "the message to fail", func() bool { return setup.outboxCounts()["failed"] == 1 })
	closeNotifier(t, notifier)

	if channel.sentCount() != 0 {
		t.Fatal("a disabled connection must not receive the retry")
	}
}

// The outbox keeps no copy of the event, so a large one sent to connections
// that keep failing costs the database nothing; every attempt reads it from
// the store, so the email channel still gets the full event.
func TestOutboxReadsTheStoredEvent(t *testing.T) {
	setup := newSetup(t)
	channels := []*fakeChannel{{failFirst: 2}, {failFirst: 2}, {failFirst: 2}}

	for _, channel := range channels {
		setup.connect(channel, nil)
	}

	event := fixtureEvent(t, "node-event-cause-chain")
	payload := strings.Replace(string(event.Raw), "{", `{"extra":{"large":"`+strings.Repeat("x", 1<<20)+`"},`, 1)
	result, err := setup.database.SaveEvent(t.Context(), store.NewEvent{
		ProjectID: 1, EventID: event.EventID, Fingerprint: "chain", Title: "Error", Level: "error", ReceivedAt: time.Now(), Payload: []byte(payload),
	})

	if err != nil {
		t.Fatal(err)
	}

	notifier := setup.start(Options{})
	notification := issue(1, "error")
	notification.IssueID, notification.EventID = result.IssueID, event.EventID

	notifier.Notify(notification)
	waitFor(t, "every channel to retry", func() bool { return setup.outboxCounts()["sent"] == len(channels) })
	closeNotifier(t, notifier)

	messages, err := setup.database.ListOutbox(t.Context(), store.OutboxFilter{})

	if err != nil || len(messages) != len(channels) {
		t.Fatalf("outbox: %d messages, %v", len(messages), err)
	}

	for _, message := range messages {
		if len(message.Payload) > 1000 {
			t.Errorf("an outbox message of %d bytes", len(message.Payload))
		}
	}

	for _, channel := range channels {
		if sent := channel.sent[0].Event; sent == nil || len(sent.Exception) < 2 {
			t.Fatalf("the email channel needs the full event, got %+v", sent)
		}
	}
}

func TestOldMessagesAreCleaned(t *testing.T) {
	setup := newSetup(t)
	ctx := context.Background()
	now := time.Now()

	// Two sent and two failed messages, one of each old enough to be deleted.
	for _, row := range []struct {
		status string
		age    time.Duration
	}{{"sent", 2 * time.Hour}, {"sent", time.Minute}, {"failed", 48 * time.Hour}, {"failed", time.Hour}} {
		if err := setup.database.EnqueueOutbox(ctx, []string{"old"}, []int{1}, kindNotification, []byte(`{}`), now.Add(-row.age)); err != nil {
			t.Fatal(err)
		}

		messages, err := setup.database.DueOutbox(ctx, "old", now, 1)

		if err != nil || len(messages) != 1 {
			t.Fatalf("due: %v %v", messages, err)
		}

		if row.status == "sent" {
			err = setup.database.MarkOutboxSent(ctx, messages[0].ID, now.Add(-row.age))
		} else {
			err = setup.database.MarkOutboxFailed(ctx, messages[0].ID, "down", now.Add(-row.age))
		}

		if err != nil {
			t.Fatal(err)
		}
	}

	notifier := setup.start(Options{KeepSent: time.Hour, KeepFailed: 24 * time.Hour})

	if err := notifier.CleanOutbox(ctx); err != nil {
		t.Fatal(err)
	}

	if counts := setup.outboxCounts(); counts["sent"] != 1 || counts["failed"] != 1 {
		t.Errorf("after the cleanup: %v", counts)
	}

	closeNotifier(t, notifier)
}

// blockingChannel keeps its worker busy until released.
type blockingChannel struct {
	fakeChannel
	release chan struct{}
}

func (channel *blockingChannel) Send(ctx context.Context, notification Notification) error {
	<-channel.release

	return channel.fakeChannel.Send(ctx, notification)
}

// A connection stuck in retries (an email provider rate-limiting) must not
// delay the other connections.
func TestSlowConnectionDoesNotDelayOthers(t *testing.T) {
	setup := newSetup(t)
	stuck := &blockingChannel{release: make(chan struct{})}
	fast := &fakeChannel{}

	setup.connect(stuck, nil)
	setup.connect(fast, nil)

	notifier := setup.start(Options{})

	notifier.Notify(issue(1, "error"))
	notifier.Notify(issue(2, "error"))

	waitFor(t, "the fast connection while the other is stuck", func() bool { return fast.sentCount() == 2 })

	close(stuck.release)
	waitFor(t, "the stuck connection after it recovered", func() bool { return stuck.sentCount() == 2 })
	closeNotifier(t, notifier)
}

// Failed attempts are logged while retrying, so a provider that rejects mail
// is visible before the message is given up.
func TestFailedAttemptsAreLogged(t *testing.T) {
	setup := newSetup(t)

	var logs lockedWriter

	channel := &fakeChannel{failFirst: 1, failWith: errors.New("550 too many emails per second")}
	setup.connect(channel, nil)

	notifier := setup.start(Options{Logger: slog.New(slog.NewTextHandler(&logs, nil))})

	notifier.Notify(issue(7, "error"))
	waitFor(t, "the retry", func() bool { return channel.sentCount() == 1 })
	closeNotifier(t, notifier)

	if output := logs.String(); !strings.Contains(output, "notification attempt failed, retrying") || !strings.Contains(output, "550 too many emails") {
		t.Fatalf("log:\n%s", output)
	}
}

type lockedWriter struct {
	mutex   sync.Mutex
	builder strings.Builder
}

func (writer *lockedWriter) Write(data []byte) (int, error) {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	return writer.builder.Write(data)
}

func (writer *lockedWriter) String() string {
	writer.mutex.Lock()
	defer writer.mutex.Unlock()

	return writer.builder.String()
}

func TestCloseIsSafe(t *testing.T) {
	setup := newSetup(t)
	notifier := setup.start(Options{})
	closeNotifier(t, notifier)

	// After Close, Notify is a no-op instead of a panic on a closed queue.
	notifier.Notify(issue(1, "error"))
	closeNotifier(t, notifier)

	// Without a store nothing runs.
	closeNotifier(t, New(Options{}))
}

// A flood keeps only what its summary shows: the count and the first
// maxSummaryItems notifications, not every notification until the window ends.
func TestFloodKeepsOnlySummaryExamples(t *testing.T) {
	setup := newSetup(t)
	setup.connect(&fakeChannel{}, nil)
	notifier := &Notifier{ctx: context.Background(), options: Options{Store: setup.database, MaxPerWindow: 5}}
	connections, err := setup.database.ListConnections(t.Context())

	if err != nil || len(connections) != 1 {
		t.Fatalf("connections = %v, %v", connections, err)
	}

	key := windowKey{connectionID: connections[0].ID, projectID: issue(0, "error").ProjectID}
	windows := map[windowKey]*window{key: {sent: 5}} // the direct quota is used up

	for index := range 1000 {
		notifier.route(issue(index, "error"), windows)
	}

	current := windows[key]

	if current.held != 1000 || len(current.examples) != maxSummaryItems || current.examples[0].IssueID != issue(0, "error").IssueID {
		t.Fatalf("held %d, kept %d examples", current.held, len(current.examples))
	}
}

// panickingChannel panics on the notification titled panicOn.
type panickingChannel struct {
	fakeChannel
	panicOn string
}

func (channel *panickingChannel) Send(ctx context.Context, notification Notification) error {
	if notification.Title == channel.panicOn {
		panic("template failed")
	}

	return channel.fakeChannel.Send(ctx, notification)
}

// A message whose sending panics would panic at every retry and every start:
// it fails at once, logged with the stack and the connection, and the
// worker goes on with the next message.
func TestPanickingSendFailsTheMessage(t *testing.T) {
	setup := newSetup(t)

	var logs lockedWriter

	channel := &panickingChannel{panicOn: issue(1, "error").Title}
	id := setup.connect(channel, nil)
	notifier := setup.start(Options{Logger: slog.New(slog.NewTextHandler(&logs, nil))})

	notifier.Notify(issue(1, "error"))
	notifier.Notify(issue(2, "error"))
	waitFor(t, "the next message", func() bool { return channel.sentCount() == 1 })
	waitFor(t, "the message that panicked to fail", func() bool { return setup.outboxCounts()["failed"] == 1 })
	closeNotifier(t, notifier)

	if output := logs.String(); !strings.Contains(output, "template failed") || !strings.Contains(output, "connection="+id) || !strings.Contains(output, "notify_test.go") {
		t.Fatalf("the panic was not logged with its stack and connection:\n%s", output)
	}
}

// panickingStore panics once when the connections are read.
type panickingStore struct {
	store.Store
	panicked atomic.Bool
}

func (database *panickingStore) ListConnections(ctx context.Context) ([]store.Connection, error) {
	if database.panicked.CompareAndSwap(false, true) {
		panic("damaged connection")
	}

	return database.Store.ListConnections(ctx)
}

// A panic while routing a notification loses only that notification: the
// dispatcher goes on with the next.
func TestPanickingRouteKeepsTheDispatcher(t *testing.T) {
	setup := newSetup(t)

	var logs lockedWriter

	channel := &fakeChannel{}
	setup.connect(channel, nil)

	notifier := setup.start(Options{Store: &panickingStore{Store: setup.database}, Logger: slog.New(slog.NewTextHandler(&logs, nil))})

	notifier.Notify(issue(1, "error"))
	notifier.Notify(issue(2, "error"))
	waitFor(t, "the next notification", func() bool { return channel.sentCount() == 1 })
	closeNotifier(t, notifier)

	if output := logs.String(); !strings.Contains(output, "damaged connection") || !strings.Contains(output, "notify_test.go") {
		t.Fatalf("the panic was not logged with its stack:\n%s", output)
	}
}
