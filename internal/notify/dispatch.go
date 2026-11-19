package notify

import (
	"encoding/json"
	"time"
)

// dispatch routes notifications to connections, applies the flood limit per
// connection and writes messages to the outbox.
func (notifier *Notifier) dispatch() {
	windowEnds := notifier.options.windowEnds

	if windowEnds == nil {
		ticker := time.NewTicker(notifier.options.Window)

		defer ticker.Stop()

		windowEnds = ticker.C
	}

	windows := map[string]*window{}

	flush := func() {
		for connectionID, current := range windows {
			if current.held > 0 {
				summary := Summary{Count: current.held, Window: notifier.options.Window, Items: current.examples}
				notifier.enqueue([]string{connectionID}, kindSummary, summary)
			}
		}

		clear(windows)
	}

	for {
		select {
		case notification, ok := <-notifier.queue:
			if !ok {
				flush()

				return
			}

			notifier.route(notification, windows)
		case <-windowEnds:
			flush()
		}
	}
}

func (notifier *Notifier) route(notification Notification, windows map[string]*window) {
	if !notifier.inBurst {
		connections, err := notifier.options.Store.ListConnections(notifier.ctx)

		if err != nil {
			notifier.options.Logger.Error("read notification connections", "err", err)

			return
		}

		notifier.connections = connections
	}

	notifier.inBurst = len(notifier.queue) > 0

	var send []string

	for _, connection := range notifier.connections {
		if !matches(connection, notification) {
			continue
		}

		current := windows[connection.ID]

		if current == nil {
			current = &window{}
			windows[connection.ID] = current
		}

		if current.sent < notifier.options.MaxPerWindow {
			send = append(send, connection.ID)
			current.sent++
		} else {
			current.held++

			if len(current.examples) < maxSummaryItems {
				current.examples = append(current.examples, notification)
			}
		}
	}

	if len(send) > 0 {
		notifier.enqueue(send, kindNotification, notification)
	}
}

// enqueue writes a message for connections in one batch and wakes their workers.
func (notifier *Notifier) enqueue(connectionIDs []string, kind string, message any) {
	payload, err := json.Marshal(message)

	if err != nil {
		notifier.options.Logger.Error("encode notification", "err", err)

		return
	}

	// The outbox channel of a connection is its id.
	if err := notifier.options.Store.EnqueueOutbox(notifier.ctx, connectionIDs, kind, payload, notifier.options.Now()); err != nil {
		notifier.options.Logger.Error("write notification to outbox", "connection_ids", connectionIDs, "err", err)

		return
	}

	for _, connectionID := range connectionIDs {
		notifier.wakeWorker(connectionID)
	}
}
