// Package notify sends alerts about new and regressed issues to the
// connections set up in the UI, delivered through the outbox.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Kind is why a notification is sent.
type Kind string

const (
	// NewIssue: an error seen for the first time.
	NewIssue Kind = "new_issue"
	// Regression: a resolved issue happened again.
	Regression Kind = "regression"
	// Test: the "Send test" button of a connection.
	Test Kind = "test"
)

// Notification is one alert about an issue.
type Notification struct {
	Kind      Kind
	ProjectID int
	Project   string
	// ProjectColor marks the project in emails ("#rrggbb"; may be empty).
	ProjectColor string `json:",omitempty"`
	IssueID      string
	IssueURL     string // link to the issue in the UI; empty without PUBLIC_URL
	Title        string
	Culprit      string
	Level        string
	Environment  string
	Release      string
	Time         time.Time
	// EventID is the stored event, read again when a message is sent.
	EventID string `json:",omitempty"`
	// Event is that event for formats with room for details (email); may be nil.
	Event *sentry.Event `json:"-"`
}

// capped keeps of the free text what an issue keeps: titles come from error
// messages, and a notification is held in memory, the outbox and summaries.
func (notification Notification) capped() Notification {
	notification.Title = textutil.Shorten(notification.Title, store.MaxTitleLength)
	notification.Culprit = textutil.Shorten(notification.Culprit, store.MaxCulpritLength)
	notification.Environment = textutil.Shorten(notification.Environment, fieldLength)
	notification.Release = textutil.Shorten(notification.Release, fieldLength)

	return notification
}

// Summary stands for notifications held back by the flood limit.
type Summary struct {
	Count  int
	Window time.Duration
	// Items are the first few held notifications, to show what they were about.
	Items []Notification
}

// maxSummaryItems is how many held notifications a summary lists.
const maxSummaryItems = 10

// Channel delivers notifications to one destination.
type Channel interface {
	Name() string
	Send(ctx context.Context, notification Notification) error
	SendSummary(ctx context.Context, summary Summary) error
}

// RetryAfterError asks the notifier to retry after Delay (a rate limit response).
type RetryAfterError struct {
	Delay time.Duration
	Err   error
}

func (err *RetryAfterError) Error() string {
	return fmt.Sprintf("%v (retry after %s)", err.Err, err.Delay)
}

func (err *RetryAfterError) Unwrap() error { return err.Err }

// DeliveryError is a test message the destination refused or did not get,
// unlike a channel the server could not build.
type DeliveryError struct{ Err error }

func (err *DeliveryError) Error() string { return err.Err.Error() }

func (err *DeliveryError) Unwrap() error { return err.Err }

// Store is what the notifier needs from the database (implemented by store.Store).
type Store interface {
	ListConnections(ctx context.Context) ([]store.Connection, error)
	GetConnection(ctx context.Context, id string) (store.Connection, error)
	GetEvent(ctx context.Context, projectID int, eventID string) (store.Event, error)

	EnqueueOutbox(ctx context.Context, channels []string, kind string, payload []byte, now time.Time) error
	DueOutbox(ctx context.Context, channel string, now time.Time, limit int) ([]store.OutboxMessage, error)
	PendingChannels(ctx context.Context) ([]string, error)
	MarkOutboxSent(ctx context.Context, id int64, now time.Time) error
	MarkOutboxRetry(ctx context.Context, id int64, lastError string, next, now time.Time) error
	MarkOutboxFailed(ctx context.Context, id int64, lastError string, now time.Time) error
	CleanOutbox(ctx context.Context, sentBefore, failedBefore time.Time) (int64, error)
}

// ChannelFactory builds the channel of a connection (implemented by Factory).
type ChannelFactory interface {
	Channel(connection store.Connection) (Channel, error)
}

// Options configures a Notifier. Zero values get the defaults below and the
// config.DefaultNotify* settings.
type Options struct {
	Store    Store
	Channels ChannelFactory
	Logger   *slog.Logger

	MaxPerWindow int // per connection
	Window       time.Duration
	QueueSize    int // in-memory notifications not yet in the outbox

	// Schedule is the wait before each retry; the last one repeats.
	Schedule   []time.Duration
	RetryFor   time.Duration
	KeepSent   time.Duration
	KeepFailed time.Duration

	PollInterval time.Duration // how often workers look for due retries
	Now          func() time.Time

	windowEnds <-chan time.Time // tests: ends the windows instead of a Window ticker
}

const (
	defaultMaxPerWindow = 5
	defaultWindow       = time.Minute
	defaultQueueSize    = 1000
	defaultPollInterval = time.Second
)

var defaultSchedule = []time.Duration{
	2 * time.Second, 10 * time.Second, 30 * time.Second, time.Minute,
	5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour,
}

// Message kinds in the outbox.
const (
	kindNotification = "notification"
	kindSummary      = "summary"
)

// sendTimeout bounds one attempt to reach a chat or mail server.
const sendTimeout = 30 * time.Second

// dueBatch is how many due messages a worker reads at once.
const dueBatch = 20

type Notifier struct {
	options Options
	queue   chan Notification

	// ctx is cancelled when Close gives up waiting: sends in progress stop.
	ctx    context.Context
	cancel context.CancelFunc

	dispatched chan struct{} // closed when the dispatcher has written everything
	done       chan struct{}
	running    sync.WaitGroup

	workersMutex sync.Mutex
	workers      map[string]*worker // by outbox channel (the connection id)

	mutex  sync.Mutex
	closed bool

	// connections are read again once the queue has emptied: a burst of new
	// issues reads them once, and an edit applies to the next notification.
	connections []store.Connection
	inBurst     bool
}

type worker struct {
	wake chan struct{} // new messages in the outbox
	// unmarked are sent messages the store failed to record as sent, by id.
	unmarked    map[int64]time.Time
	pausedUntil time.Time   // set by a channel's rate limit
	retryNow    atomic.Bool // RetryNow ends the pause
}

// window is the flood limit state of one connection.
type window struct {
	sent int
	// held counts the notifications over the limit; examples keeps the first
	// maxSummaryItems of them for the summary. Only those are kept: a flood of
	// errors with large events must not pile up in memory until the window ends.
	held     int
	examples []Notification
}

// New starts the goroutines. Without a store Notify does nothing.
func New(options Options) *Notifier {
	setDefaults(&options)

	ctx, cancel := context.WithCancel(context.Background())
	notifier := &Notifier{
		options:    options,
		queue:      make(chan Notification, options.QueueSize),
		ctx:        ctx,
		cancel:     cancel,
		dispatched: make(chan struct{}),
		done:       make(chan struct{}),
		workers:    map[string]*worker{},
	}

	if options.Store == nil || options.Channels == nil {
		close(notifier.done)

		return notifier
	}

	// Messages left from before a restart get their workers right away.
	pending, err := options.Store.PendingChannels(ctx)

	if err != nil {
		options.Logger.Error("read pending notifications", "err", err)
	}

	notifier.running.Go(func() {
		for _, channel := range pending {
			notifier.wakeWorker(channel)
		}

		notifier.dispatch()
		close(notifier.dispatched)
	})

	go func() {
		notifier.running.Wait()
		close(notifier.done)
	}()

	return notifier
}

func setDefaults(options *Options) {
	if options.MaxPerWindow <= 0 {
		options.MaxPerWindow = defaultMaxPerWindow
	}

	if options.Window <= 0 {
		options.Window = defaultWindow
	}

	if options.QueueSize <= 0 {
		options.QueueSize = defaultQueueSize
	}

	if len(options.Schedule) == 0 {
		options.Schedule = defaultSchedule
	}

	if options.RetryFor <= 0 {
		options.RetryFor = config.DefaultNotifyRetryFor
	}

	if options.KeepSent <= 0 {
		options.KeepSent = config.DefaultNotifyKeepSent
	}

	if options.KeepFailed <= 0 {
		options.KeepFailed = config.DefaultNotifyKeepFailed
	}

	if options.PollInterval <= 0 {
		options.PollInterval = defaultPollInterval
	}

	if options.Now == nil {
		options.Now = time.Now
	}

	if options.Logger == nil {
		options.Logger = slog.New(slog.DiscardHandler)
	}
}

// Notify queues a notification. It never blocks: when the in-memory queue is
// full (the database is stuck) the notification is dropped and logged.
func (notifier *Notifier) Notify(notification Notification) {
	if notifier.options.Store == nil {
		return
	}

	notification = notification.capped()

	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()

	if notifier.closed {
		return
	}

	select {
	case notifier.queue <- notification:
	default:
		notifier.options.Logger.Warn("notification queue full, dropping", "issue_id", notification.IssueID, "title", notification.Title)
	}
}

// Close stops accepting notifications, writes the queued ones (and the
// pending summaries) to the outbox, sends what is due and returns. Messages
// waiting for a retry stay in the outbox and are sent after the next start.
// If ctx ends first, sends in progress are cancelled.
func (notifier *Notifier) Close(ctx context.Context) error {
	notifier.mutex.Lock()

	if !notifier.closed {
		notifier.closed = true

		if notifier.options.Store != nil {
			close(notifier.queue)
		}
	}

	notifier.mutex.Unlock()

	select {
	case <-notifier.done:
		return nil
	case <-ctx.Done():
		notifier.cancel()

		<-notifier.done

		return ctx.Err()
	}
}
