package config

import "time"

// The periodic jobs (internal/app/jobs.go, the database lane in
// database.Store.Lane) start apart, so two
// never begin together: the size checks at start and every CleanupInterval
// (and after CleanupWritten bytes), the inbox compactions halfway between
// them, the outbox cleanups a quarter of an interval in and every
// OutboxCleanupInterval, a multiple of CleanupInterval, the purges of deleted
// projects at start, three quarters in and every ProjectPurgeInterval.
const (
	CleanupInterval       = 10 * time.Minute
	CleanupWritten        = 50 << 20
	InboxCompactionAt     = CleanupInterval / 2
	OutboxCleanupDelay    = CleanupInterval / 4
	OutboxCleanupInterval = 6 * CleanupInterval
	ProjectPurgeAt        = CleanupInterval * 3 / 4
	ProjectPurgeInterval  = 6 * CleanupInterval

	// A pass longer than its limit stops (the size check goes on at once,
	// from where it stopped) or, if it cannot stop, is left running.
	InboxCompactionTimeout = time.Minute
	SizeCheckTimeout       = 5 * time.Minute
	OutboxCleanupTimeout   = time.Minute
	ProjectPurgeTimeout    = 5 * time.Minute
)
