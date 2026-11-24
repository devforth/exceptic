package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Retention: how much data is kept. The database stays under
// MaxDatabaseBytes: the cleanup (package database) deletes the least useful data
// first when it nears it.
type Retention struct {
	MaxDatabaseBytes int64 // 0: no limit
}

const (
	// LogsPerMessage caps a message logged over and over (the same text
	// with other numbers or IDs) as it is written: its newest copies stay.
	LogsPerMessage = 10
	// EventsPerIssuePerMinute caps the events an issue stores a minute (by
	// receive time); the others count without being stored.
	EventsPerIssuePerMinute = 10

	// The cleanup (database/cleanup.go; order in docs/configuration.md):
	// it starts when the data held reaches CleanupStartPercent of the limit
	// and deletes down to CleanupStopPercent.
	CleanupStartPercent = 99
	CleanupStopPercent  = 80
	// CleanupFilesPercent: when the files still take more than this part of
	// the limit after compacting, a cleanup runs whatever the data held.
	CleanupFilesPercent = 125
	CleanupAge          = 30 * 24 * time.Hour
	CleanupRecentAge    = 7 * 24 * time.Hour
	OldEventsPerIssue   = 20
	// StalePercent: once deleted and replaced records may take more than
	// this part of the bytes counted, the size check compacts them away.
	StalePercent = 10
)

// DefaultMaxDatabaseBytes is the default of MAX_DATABASE_SIZE.
const DefaultMaxDatabaseBytes = 2 << 30 // 2 GB

// MinDatabaseBytes is the smallest MAX_DATABASE_SIZE: below it the ingest
// queue's share holds only a few of the largest envelopes and the cleanup
// runs nearly all the time.
const MinDatabaseBytes = 500 << 20 // 500 MB

// InboxPercent of MAX_DATABASE_SIZE (or of its default when it is off) may
// wait in the ingest queue; beyond it new envelopes get 429.
const InboxPercent = 20

// MaxInboxBytes is the ingest queue's share of maxDatabaseBytes, or of the
// default when the limit is off (0).
func MaxInboxBytes(maxDatabaseBytes int64) int64 {
	if maxDatabaseBytes <= 0 {
		maxDatabaseBytes = DefaultMaxDatabaseBytes
	}

	return maxDatabaseBytes * InboxPercent / 100
}

func parseRetention(getenv func(string) string) (Retention, error) {
	retention := Retention{MaxDatabaseBytes: DefaultMaxDatabaseBytes}

	if value := strings.TrimSpace(getenv("MAX_DATABASE_SIZE")); value != "" {
		size, err := parseSize(value)

		if err != nil {
			return retention, fmt.Errorf("MAX_DATABASE_SIZE: %w", err)
		}

		if size != 0 && size < MinDatabaseBytes {
			return retention, fmt.Errorf("MAX_DATABASE_SIZE: %s is below the minimum %s (or 0 for no limit)", FormatSize(size), FormatSize(MinDatabaseBytes))
		}

		retention.MaxDatabaseBytes = size
	}

	return retention, nil
}

var sizeUnits = []struct {
	suffix string
	bytes  int64
}{
	{"TB", 1 << 40},
	{"GB", 1 << 30},
	{"MB", 1 << 20},
	{"KB", 1 << 10},
	{"B", 1},
}

// parseSize reads "500MB", "2GB" or "1.5GB" (binary units: 1 GB = 1024 MB);
// "0" means no limit.
func parseSize(value string) (int64, error) {
	if value == "0" {
		return 0, nil
	}

	upper := strings.ToUpper(strings.ReplaceAll(value, " ", ""))

	for _, unit := range sizeUnits {
		if number, ok := strings.CutSuffix(upper, unit.suffix); ok {
			amount, err := strconv.ParseFloat(number, 64)
			size := amount * float64(unit.bytes)

			// The cap refuses Inf and keeps MaxInboxBytes from overflowing.
			if err != nil || math.IsNaN(size) || size < 1 || size > math.MaxInt64/100 {
				break
			}

			return int64(size), nil
		}
	}

	return 0, fmt.Errorf("invalid size %q (e.g. 500MB, 2GB, or 0 for no limit)", value)
}

// FormatSize shows a byte count the way MAX_DATABASE_SIZE is written.
func FormatSize(bytes int64) string {
	for _, unit := range sizeUnits {
		if bytes >= unit.bytes && unit.bytes > 1 {
			return strconv.FormatFloat(float64(bytes)/float64(unit.bytes), 'f', 1, 64) + " " + unit.suffix
		}
	}

	return strconv.FormatInt(bytes, 10) + " B"
}
