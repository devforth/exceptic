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
	// EventLevels and LogLevels: the levels recorded; nil uses the defaults.
	EventLevels *[]string
	LogLevels   *[]string
	Color       string // "#rrggbb", or "" for the palette color
	CreatedAt   time.Time
}

// ErrProjectConflict: the name or the key is already used by another project.
var ErrProjectConflict = errors.New("project name or key already used")

// ProjectUsage is what each project holds, by project ID.
type ProjectUsage struct {
	// Bytes of its issues, events and logs, before compression.
	Bytes map[int]int64
	// Counted is what Bytes are part of: the bytes of every issue, event and
	// log, of deleted projects too, and of the ingest queue, counted the same way.
	Counted int64
	// LastReceived is when its newest issue or log came; missing when it has none.
	LastReceived map[int]time.Time
	// Counting: data stored before the per-project counters is still being
	// counted (once, in the background), so Bytes fall short until it is done.
	Counting bool
}
