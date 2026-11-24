package store

import (
	"errors"
	"time"
)

// Project is a stored project; its ID is part of the DSN.
type Project struct {
	ID        int
	Name      string
	PublicKey string
	// LogLevels: the log levels recorded; nil uses the default.
	LogLevels *[]string
	Color     string // "#rrggbb", or "" for the palette color
	// DiskShare: the percent of MAX_DATABASE_SIZE the project may hold before
	// a cleanup of its own (Share; 0, as stored before it existed, is 100).
	DiskShare int
	CreatedAt time.Time
}

// A disk share goes from MinDiskShare to MaxDiskShare percent in steps of DiskShareStep.
const (
	MinDiskShare  = 5
	MaxDiskShare  = 100
	DiskShareStep = 5
)

// Share is the project's disk share in percent; 100 sets no limit of its own.
func (project Project) Share() int {
	if project.DiskShare <= 0 || project.DiskShare > MaxDiskShare {
		return MaxDiskShare
	}

	return project.DiskShare
}

// ShareLimit is the size a disk share of maxBytes allows; 0 (no limit of
// its own) at 100% or above, without a share or without maxBytes.
func ShareLimit(maxBytes int64, share int) int64 {
	if share <= 0 || share >= MaxDiskShare || maxBytes <= 0 {
		return 0
	}

	return maxBytes * int64(share) / 100
}

// OldestKept is the time of a project's oldest stored log below error, error
// or fatal log and event (the SDK's time of a log, the arrival of an event); zero when none.
type OldestKept struct {
	LogsBelowError, ErrorLogs, Errors time.Time
}

// ErrProjectConflict: the name or the key is already used by another project.
var ErrProjectConflict = errors.New("project name or key already used")

// ProjectUsage is what each project holds, by project ID.
type ProjectUsage struct {
	// Bytes of its issues, events and logs, before compression.
	Bytes map[int]int64
	// OnDisk is its part of Live, as the cleanup by size measures it: Bytes
	// times the size on disk of a byte counted.
	OnDisk map[int]int64
	// Live is what the database holds, as the cleanup by size measures it:
	// every project's part plus the records of no project (users, sessions,
	// settings, history). The ingest queue and deleted records not yet
	// compacted away are left out.
	Live int64
	// Files is what the database's files take on disk.
	Files int64
	// LastReceived is when its newest issue or log came; missing when it has none.
	LastReceived map[int]time.Time
	// FirstContact is when it first sent a request with its key, of any
	// kind; missing before that.
	FirstContact map[int]time.Time
	// Oldest is what each project still keeps, by the kinds the cleanup deletes.
	Oldest map[int]OldestKept
	// Counting: data stored before the per-project counters is still being
	// counted (once, in the background), so Bytes fall short until it is done.
	Counting bool
}
