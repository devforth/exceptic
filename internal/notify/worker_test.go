package notify

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/storage/store"
)

// countingStore counts the outbox reads of each channel and can run a hook
// when a worker finds nothing pending.
type countingStore struct {
	store.Store

	mutex sync.Mutex
	reads map[string]int

	// idle runs once, after the real read, when a channel has nothing pending.
	idle atomic.Pointer[func(channel string)]
}

func (database *countingStore) DueOutbox(ctx context.Context, channel string, now time.Time, limit int) ([]store.OutboxMessage, error) {
	database.mutex.Lock()
	database.reads[channel]++
	database.mutex.Unlock()

	return database.Store.DueOutbox(ctx, channel, now, limit)
}

func (database *countingStore) HasPendingOutbox(ctx context.Context, channel string) (bool, error) {
	pending, err := database.Store.HasPendingOutbox(ctx, channel)

	if hook := database.idle.Swap(nil); hook != nil && err == nil && !pending {
		(*hook)(channel)
	}

	return pending, err
}

func (database *countingStore) readCount(channel string) int {
	database.mutex.Lock()
	defer database.mutex.Unlock()

	return database.reads[channel]
}

func (notifier *Notifier) hasWorker(channel string) bool {
	notifier.workersMutex.Lock()
	defer notifier.workersMutex.Unlock()

	return notifier.workers[channel] != nil
}

// stopsReading checks that the channel is not read during several polls.
func stopsReading(t *testing.T, database *countingStore, channel string) {
	t.Helper()

	before := database.readCount(channel)

	time.Sleep(50 * time.Millisecond) // 10 polls

	if after := database.readCount(channel); after != before {
		t.Fatalf("the outbox of a stopped worker was read %d more times", after-before)
	}
}

// A worker with nothing left to send stops and starts again for a new message.
func TestIdleWorkerStopsAndRestarts(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}
	id := setup.connect(channel, nil)
	database := &countingStore{Store: setup.database, reads: map[string]int{}}

	notifier := setup.start(Options{Store: database})

	defer closeNotifier(t, notifier)

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the message", func() bool { return channel.sentCount() == 1 })
	waitFor(t, "the worker to stop", func() bool { return !notifier.hasWorker(id) })
	stopsReading(t, database, id)

	notifier.Notify(issue(2, "error"))
	waitFor(t, "the second message", func() bool { return channel.sentCount() == 1+1 })
	waitFor(t, "the worker to stop again", func() bool { return !notifier.hasWorker(id) })
}

// A worker waiting for a retry stays; once its connection is deleted (with
// its messages) it stops.
func TestDeletedConnectionStopsItsWorker(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{failFirst: 1_000_000}
	id := setup.connect(channel, nil)
	database := &countingStore{Store: setup.database, reads: map[string]int{}}

	notifier := setup.start(Options{Store: database, Schedule: []time.Duration{time.Hour}})

	defer closeNotifier(t, notifier)

	notifier.Notify(issue(1, "error"))
	waitFor(t, "the failed attempt", func() bool { return channel.attemptCount() == 1 })

	reads := database.readCount(id)

	waitFor(t, "the worker to poll for the retry", func() bool { return database.readCount(id) > reads+2 })

	if err := setup.database.DeleteConnection(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	notifier.ConnectionDeleted(id)
	waitFor(t, "the worker to stop", func() bool { return !notifier.hasWorker(id) })
	stopsReading(t, database, id)
}

// A message queued while the worker decides to stop is sent once: the
// worker sees the wake and stays, or a new worker sends it.
func TestMessageQueuedWhileWorkerStops(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}
	id := setup.connect(channel, nil)
	database := &countingStore{Store: setup.database, reads: map[string]int{}}

	notifier := setup.start(Options{Store: database, MaxPerWindow: 1000})

	defer closeNotifier(t, notifier)

	for round := range 20 {
		// Between the worker's last read and its stop, a message arrives.
		hook := func(string) { notifier.enqueue([]string{id}, []int{1}, kindNotification, issue(1000+round, "error")) }

		database.idle.Store(&hook)
		notifier.Notify(issue(round, "error"))
		waitFor(t, "both messages", func() bool { return channel.sentCount() == 2*(round+1) })
		waitFor(t, "the worker to stop", func() bool { return !notifier.hasWorker(id) })
	}

	time.Sleep(20 * time.Millisecond)

	if sent, counts := channel.sentCount(), setup.outboxCounts(); sent != 40 || counts["sent"] != 40 {
		t.Fatalf("sent %d, outbox %v", sent, counts)
	}
}

// RetryNow starts a stopped worker for the messages it made due.
func TestRetryNowStartsAStoppedWorker(t *testing.T) {
	setup := newSetup(t)
	channel := &fakeChannel{}
	id := setup.connect(channel, nil)

	notifier := setup.start(Options{PollInterval: time.Hour})

	defer closeNotifier(t, notifier)

	payload := []byte(`{"Kind":"new_issue","Title":"Error 1","Level":"error"}`)

	if err := setup.database.EnqueueOutbox(context.Background(), []string{id}, []int{1}, kindNotification, payload, time.Now()); err != nil {
		t.Fatal(err)
	}

	notifier.RetryNow(id)
	waitFor(t, "the message", func() bool { return channel.sentCount() == 1 })
}
