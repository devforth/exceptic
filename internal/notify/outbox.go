package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/devforth/exceptic/internal/platform/logging"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// wakeWorker starts the worker of an outbox channel if needed and tells it
// there is work. No worker starts once the dispatcher is done (Close): the
// dispatcher closes dispatched under workersMutex, so while it runs the
// WaitGroup is never at zero here.
func (notifier *Notifier) wakeWorker(channel string) {
	notifier.workersMutex.Lock()
	defer notifier.workersMutex.Unlock()

	notifier.wakeLocked(channel, true)
}

// wakeLocked wakes the worker of channel, starting one when start is set;
// it returns the worker, nil when there is none.
func (notifier *Notifier) wakeLocked(channel string, start bool) *worker {
	current := notifier.workers[channel]

	if current == nil {
		select {
		case <-notifier.dispatched:
			return nil
		default:
		}

		if !start {
			return nil
		}

		current = &worker{wake: make(chan struct{}, 1), unmarked: map[int64]time.Time{}}
		notifier.workers[channel] = current

		notifier.running.Go(func() { notifier.runWorker(channel, current) })
	}

	select {
	case current.wake <- struct{}{}:
	default: // already woken
	}

	return current
}

// RetryNow ends the rate-limit pause of a connection and sends its due
// messages: called after its waiting messages were made due (a save or
// Retry now), since that is usually the fix.
func (notifier *Notifier) RetryNow(channel string) {
	notifier.workersMutex.Lock()
	defer notifier.workersMutex.Unlock()

	if current := notifier.wakeLocked(channel, true); current != nil {
		current.retryNow.Store(true)
	}
}

// ConnectionDeleted wakes the worker of a deleted connection, whose messages
// went with it, so it stops now instead of at its next poll.
func (notifier *Notifier) ConnectionDeleted(channel string) {
	notifier.workersMutex.Lock()
	defer notifier.workersMutex.Unlock()

	notifier.wakeLocked(channel, false)
}

// runWorker sends the due messages of one connection: woken by new messages,
// and every PollInterval for retries that became due. It stops when the
// connection has nothing left to send, and at Close.
func (notifier *Notifier) runWorker(channel string, current *worker) {
	ticker := time.NewTicker(notifier.options.PollInterval)

	defer ticker.Stop()

	for {
		select {
		case <-current.wake:
		case <-ticker.C:
		case <-notifier.dispatched:
			// Everything is in the outbox: one last pass, then stop.
			notifier.sendDue(channel, current)

			return
		case <-notifier.ctx.Done():
			return
		}

		notifier.sendDue(channel, current)

		if notifier.stopIdle(channel, current) {
			return
		}
	}
}

// stopIdle removes the worker when its connection has no message left to
// send, due or waiting for a retry (a deleted connection's went with it).
// A message queued after the check wakes the worker under the same lock
// before it is removed, so it goes on, or starts a new worker after.
func (notifier *Notifier) stopIdle(channel string, current *worker) bool {
	if len(current.unmarked) > 0 || notifier.ctx.Err() != nil {
		return false
	}

	pending, err := notifier.options.Store.HasPendingOutbox(notifier.ctx, channel)

	if err != nil {
		notifier.options.Logger.Error("read notification outbox", "connection", channel, "err", err)

		return false
	}

	if pending {
		return false
	}

	notifier.workersMutex.Lock()
	defer notifier.workersMutex.Unlock()

	if len(current.wake) > 0 {
		return false
	}

	delete(notifier.workers, channel)

	return true
}

// sendDue sends due messages oldest first. A failed send ends the pass and
// waits for its retry while later messages go on; a rate limit pauses the
// worker for the delay the channel asked for.
func (notifier *Notifier) sendDue(channel string, current *worker) {
	notifier.markSent(current)

	if current.retryNow.Swap(false) {
		current.pausedUntil = time.Time{}
	}

	for notifier.ctx.Err() == nil && !notifier.options.Now().Before(current.pausedUntil) {
		messages, err := notifier.options.Store.DueOutbox(notifier.ctx, channel, notifier.options.Now(), dueBatch)

		if err != nil {
			notifier.options.Logger.Error("read notification outbox", "connection", channel, "err", err)

			return
		}

		if len(messages) == 0 {
			return
		}

		// One channel per batch: the connection is read and its secret decrypted once.
		built, err := notifier.channel(channel)

		for _, message := range messages {
			if _, sent := current.unmarked[message.ID]; sent || !notifier.attempt(current, message, built, err) {
				return
			}
		}

		if len(messages) < dueBatch {
			return
		}
	}
}

// markSent records again the messages sent while the store refused writes,
// so they are not sent twice.
func (notifier *Notifier) markSent(current *worker) {
	for id, sentAt := range current.unmarked {
		if err := notifier.options.Store.MarkOutboxSent(context.Background(), id, sentAt); err != nil {
			return
		}

		delete(current.unmarked, id)
	}
}

// errGone are failures that retrying cannot fix.
var errGone = errors.New("gone")

// attempt sends one message once through channel (or records why there is
// none, channelErr) and records the result: false when it waits for a retry.
func (notifier *Notifier) attempt(current *worker, message store.OutboxMessage, channel Channel, channelErr error) bool {
	about := []any{"connection", message.Channel, "outbox_id", message.ID, "attempt", message.Attempts + 1}
	err := channelErr

	if err == nil {
		ctx, cancel := context.WithTimeout(notifier.ctx, sendTimeout)
		err = notifier.sendOrFail(ctx, channel, message)

		cancel()

		about = append(about, "channel", channel.Name())
	}

	now := notifier.options.Now()

	// The result is recorded even when a shutdown cancelled the send.
	record := context.Background()

	if err == nil {
		if err := notifier.options.Store.MarkOutboxSent(record, message.ID, now); err != nil {
			notifier.options.Logger.Error("mark notification sent", append(about, "err", err)...)

			current.unmarked[message.ID] = now
		}

		return true
	}

	schedule := notifier.options.Schedule
	delay := schedule[min(message.Attempts, len(schedule)-1)]

	var retryAfter *RetryAfterError

	rateLimited := errors.As(err, &retryAfter) && retryAfter.Delay > 0

	if rateLimited {
		delay = retryAfter.Delay
	}

	next := now.Add(delay)

	if errors.Is(err, errGone) || next.Sub(message.CreatedAt) > notifier.options.RetryFor {
		notifier.options.Logger.Error("notification failed, giving up", append(about, "err", err)...)

		if err := notifier.options.Store.MarkOutboxFailed(record, message.ID, err.Error(), now); err != nil {
			notifier.options.Logger.Error("mark notification failed", append(about, "err", err)...)
		}

		return true
	}

	// Within RetryFor only, so a pause never outlasts the retry window.
	if rateLimited {
		current.pausedUntil = next
	}

	notifier.options.Logger.Warn("notification attempt failed, retrying", append(about, "retry_in", delay, "err", err)...)

	if err := notifier.options.Store.MarkOutboxRetry(record, message.ID, err.Error(), next, now); err != nil {
		notifier.options.Logger.Error("schedule notification retry", append(about, "err", err)...)
	}

	return false
}

// channel builds the channel of an outbox channel id from the stored
// connection, so edits made in the UI apply to queued messages too.
func (notifier *Notifier) channel(outboxChannel string) (Channel, error) {
	connection, err := notifier.options.Store.GetConnection(notifier.ctx, outboxChannel)

	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, fmt.Errorf("%w: the connection was deleted", errGone)
	case err != nil:
		return nil, err
	case !connection.Active:
		return nil, fmt.Errorf("%w: the connection is disabled", errGone)
	}

	return notifier.options.Channels.Channel(connection)
}

// sendOrFail sends a message; one that panics would panic again at every
// retry and start, so it fails at once, its stack in the log, not the outbox.
func (notifier *Notifier) sendOrFail(ctx context.Context, channel Channel, message store.OutboxMessage) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			notifier.options.Logger.Error("sending a notification panicked", "connection", message.Channel, "outbox_id", message.ID, "err", logging.PanicError(recovered))

			err = fmt.Errorf("%w: sending it panicked: %v", errGone, recovered)
		}
	}()

	return notifier.send(ctx, channel, message)
}

func (notifier *Notifier) send(ctx context.Context, channel Channel, message store.OutboxMessage) error {
	switch message.Kind {
	case kindSummary:
		var summary Summary

		if err := json.Unmarshal(message.Payload, &summary); err != nil {
			return fmt.Errorf("%w: unreadable message: %w", errGone, err)
		}

		return channel.SendSummary(ctx, summary)
	case kindNotification:
		var notification Notification

		if err := json.Unmarshal(message.Payload, &notification); err != nil {
			return fmt.Errorf("%w: unreadable message: %w", errGone, err)
		}

		notification.Event = notifier.storedEvent(ctx, notification)

		return channel.Send(ctx, notification)
	default:
		return fmt.Errorf("%w: unknown message kind %q", errGone, message.Kind)
	}
}

// storedEvent reads the notification's event from the store for the formats
// with details; without it (deleted meanwhile) the notification goes out
// without them.
func (notifier *Notifier) storedEvent(ctx context.Context, notification Notification) *sentry.Event {
	if notification.EventID == "" {
		return nil
	}

	stored, err := notifier.options.Store.GetEvent(ctx, notification.ProjectID, notification.EventID)

	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			notifier.options.Logger.Warn("read the event of a notification", "issue_id", notification.IssueID, "event_id", notification.EventID, "err", err)
		}

		return nil
	}

	event, _ := sentry.ParseEvent(stored.Payload, "", notification.Time)

	return event
}

// CleanOutbox deletes sent messages older than KeepSent and failed ones older than KeepFailed.
func (notifier *Notifier) CleanOutbox(ctx context.Context) error {
	now := notifier.options.Now()
	deleted, err := notifier.options.Store.CleanOutbox(ctx, now.Add(-notifier.options.KeepSent), now.Add(-notifier.options.KeepFailed))

	if err == nil && deleted > 0 {
		notifier.options.Logger.Debug("cleaned notification outbox", "deleted", deleted)
	}

	return err
}
