package config

import "time"

// The periodic jobs (internal/app/jobs.go, the database lane in
// database.Store.Lane) start apart, so two
// never begin together: the size checks at start and every CleanupInterval
// (and after CleanupWritten bytes), the inbox compactions halfway between
// them, the outbox cleanups a quarter of an interval in and every
// OutboxCleanupInterval, a multiple of CleanupInterval, the purges of deleted
// projects at start, three quarters in and every ProjectPurgeInterval, the
// counting of data stored before the project sizes at start, an eighth in
// and every CleanupInterval until it is done, the compactions after cleanup
// runs at start, when a run ends and three eighths in.
const (
	CleanupInterval       = 10 * time.Minute
	CleanupWritten        = 50 << 20
	InboxCompactionAt     = CleanupInterval / 2
	OutboxCleanupDelay    = CleanupInterval / 4
	OutboxCleanupInterval = 6 * CleanupInterval
	ProjectPurgeAt        = CleanupInterval * 3 / 4
	ProjectPurgeInterval  = 6 * CleanupInterval
	BackfillAt            = CleanupInterval / 8
	CleanupCompactionAt   = CleanupInterval * 3 / 8

	// A pass longer than its limit stops (the size check goes on at once,
	// from where it stopped) or, if it cannot stop, is left running.
	InboxCompactionTimeout = time.Minute
	SizeCheckTimeout       = 5 * time.Minute
	OutboxCleanupTimeout   = time.Minute
	ProjectPurgeTimeout    = 5 * time.Minute
	BackfillTimeout        = time.Minute
	// A compaction cannot stop: one past its limit is left running.
	CleanupCompactionTimeout = 5 * time.Minute
)

// LogSearchTime bounds one walk over the logs (a text search, a date range),
// so each request stays under the HTTP write timeout (30 s).
const LogSearchTime = 20 * time.Second
