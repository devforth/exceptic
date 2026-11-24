package database

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

// slowClock makes every reading of the store's clock a second later, so a
// scan's time runs out after about config.LogSearchTime/second records.
func slowClock(database *Store) {
	start := database.now()

	var readings atomic.Int64

	database.SetClock(func() time.Time { return start.Add(time.Duration(readings.Add(1)) * time.Second) })
}

// A search that runs out of time says how far it got; continuing from there
// finds every other match once.
func TestLogSearchStopsInTime(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)

	writeLogs(t, database, "error", 40, "needle")
	writeLogs(t, database, "info", 40, "hay")
	writeLogs(t, database, "info", 20, "needle")

	all, err := database.ListLogs(ctx, store.LogFilter{Query: "needle"})

	if err != nil || len(all.Logs) != 60 || all.Stopped {
		t.Fatalf("unhurried search: %d logs, %+v, %v", len(all.Logs), all.LogScan, err)
	}

	// A whole count bounds its list at the oldest match.
	if full, err := database.CountLogs(ctx, store.LogFilter{Query: "needle"}); err != nil || full.Count != 60 || full.Stopped || full.FromID != all.Logs[59].ID {
		t.Fatalf("unhurried count: %+v, %v", full, err)
	}

	slowClock(database)

	count, err := database.CountLogs(ctx, store.LogFilter{Query: "needle"})

	if err != nil || !count.Stopped || count.More || count.Count == 0 || count.Count >= 60 || count.SearchedTo.IsZero() {
		t.Fatalf("count: %+v, %v", count, err)
	}

	// The list bounded by the count finds what the count found.
	if count.FromID != count.Rest {
		t.Fatalf("a stopped count bounds its list where it stopped: %+v", count)
	}

	bounded, err := database.ListLogs(ctx, store.LogFilter{Query: "needle", FromID: count.Rest})

	if err != nil || len(bounded.Logs) != count.Count || slices.ContainsFunc(bounded.Logs, func(record store.LogRecord) bool { return record.ID < count.Rest }) {
		t.Fatalf("bounded list: %d logs, want %d (%v)", len(bounded.Logs), count.Count, err)
	}

	var found []int64

	filter := store.LogFilter{Query: "needle"}

	for range 100 {
		page, err := database.ListLogs(ctx, filter)

		if err != nil {
			t.Fatal(err)
		}

		for _, record := range page.Logs {
			found = append(found, record.ID)
		}

		if !page.Stopped {
			break
		}

		if page.Rest <= 0 || page.SearchedTo.IsZero() {
			t.Fatalf("stopped page: %+v", page.LogScan)
		}

		filter.BeforeID = page.Rest
	}

	want := make([]int64, 0, len(all.Logs))

	for _, record := range all.Logs {
		want = append(want, record.ID)
	}

	if !slices.Equal(found, want) {
		t.Fatalf("continued search found %v, want %v", found, want)
	}
}

// Counts that keys answer exactly take no time limit.
func TestLogCountByKeysTakesNoTimeLimit(t *testing.T) {
	database := openTest(t)

	writeLogs(t, database, "error", 3*int(config.LogSearchTime/time.Second), "line")
	slowClock(database)

	count, err := database.CountLogs(context.Background(), store.LogFilter{Levels: []string{"error"}})

	if err != nil || count.Stopped || count.Count != 3*int(config.LogSearchTime/time.Second) {
		t.Fatalf("count: %+v, %v", count, err)
	}
}

// A search whose time runs out before any walk reads a key searched nothing:
// it says so (searched to now, not to the oldest log) and goes on from the top.
func TestLogSearchStoppedBeforeTheFirstKey(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)

	writeLogs(t, database, "error", 5, "needle")

	written := database.now()
	start := written

	var readings atomic.Int64

	// The deadline is one reading away: the first check of a walk is past it.
	database.SetClock(func() time.Time { return start.Add(time.Duration(readings.Add(1)) * config.LogSearchTime) })

	count, err := database.CountLogs(ctx, store.LogFilter{Query: "needle"})

	if err != nil || !count.Stopped || count.Count != 0 || count.SearchedTo.Before(written) {
		t.Fatalf("count that read nothing: %+v, %v", count, err)
	}

	page, err := database.ListLogs(ctx, store.LogFilter{Query: "needle"})

	if err != nil || !page.Stopped || len(page.Logs) != 0 || page.Rest <= 0 || page.SearchedTo.Before(written) {
		t.Fatalf("list that read nothing: %+v, %v", page.LogScan, err)
	}

	database.SetClock(func() time.Time { return start })

	rest, err := database.ListLogs(ctx, store.LogFilter{Query: "needle", BeforeID: page.Rest})

	if err != nil || len(rest.Logs) != 5 || rest.Stopped {
		t.Fatalf("going on found %d logs, %+v, %v", len(rest.Logs), rest.LogScan, err)
	}
}
