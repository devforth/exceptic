package store

import (
	"time"
)

// Outbox statuses of a notification message.
const (
	OutboxPending = "pending"
	OutboxSent    = "sent"
	OutboxFailed  = "failed"
)

// OutboxStatuses are the statuses a message takes.
var OutboxStatuses = []string{OutboxPending, OutboxSent, OutboxFailed}

// OutboxMessage is one message waiting to be sent to one channel. The store
// keeps the payload opaque; package notify encodes and decodes it.
type OutboxMessage struct {
	ID        int64
	Channel   string // the connection id
	Kind      string // "notification" or "summary"
	Payload   []byte
	Attempts  int
	CreatedAt time.Time
}

// OutboxRecord is an outbox message with what happened to it, as stored and
// as the delivery log shows it.
type OutboxRecord struct {
	OutboxMessage
	Status        string    `json:"status"`
	LastError     string    `json:"last_error,omitempty"`
	NextAttemptAt time.Time `json:"next_attempt_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Retrying: waiting for another attempt after a failed one.
func (record OutboxRecord) Retrying() bool {
	return record.Status == OutboxPending && record.Attempts > 0
}

// OutboxFilter selects outbox messages, newest first. Zero values mean "any".
type OutboxFilter struct {
	Channel string
	Status  string
	Limit   int
	Offset  int
}

// DeliveryHealth sums up the delivery of one connection: whether its last
// attempts fail, for the notifications list.
type DeliveryHealth struct {
	Pending     int
	Retrying    int // pending after a failed attempt
	Failed      int // given up
	LastSentAt  time.Time
	LastError   string
	LastErrorAt time.Time
}

// Failing: messages are waiting for a retry, or the latest outcome was a failure.
func (health DeliveryHealth) Failing() bool {
	return health.Retrying > 0 || (!health.LastErrorAt.IsZero() && health.LastErrorAt.After(health.LastSentAt))
}
