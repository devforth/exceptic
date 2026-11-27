package database

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// TestGrowDatabase multiplies the logs and events of a copy of a real
// database until its files reach GROW_TO, for measuring the cleanup on a big
// database without filling it through ingest. Runs only on request, on a
// copy (never the live data directory: stop the server, copy it first):
//
//	cp -a data data/grow-src && GROW_DIR=$PWD/data/grow-src GROW_TO=7.5GB \
//	  go test ./internal/storage/database -run TestGrowDatabase -count=1 -timeout 0 -v
//
// Copies keep their times (the cleanup sees the same ages); events join their
// issue with a new event id; nothing is capped (all copies of a message, every
// event of a minute).
func TestGrowDatabase(t *testing.T) {
	dir, target := os.Getenv("GROW_DIR"), os.Getenv("GROW_TO")

	if dir == "" || target == "" {
		t.Skip("GROW_DIR and GROW_TO grow a copy of a database on request")
	}

	// go test runs in the package's directory: a relative path would open a new, empty database there.
	if !filepath.IsAbs(dir) {
		t.Fatalf("GROW_DIR %q: give an absolute path", dir)
	}

	if _, err := os.Stat(filepath.Join(dir, "leveldb")); err != nil {
		t.Fatalf("GROW_DIR %q holds no database: %v", dir, err)
	}

	limit, err := parseGrowSize(target)

	if err != nil {
		t.Fatal(err)
	}

	database, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	}()

	size := func() int64 { return dirBytes(t, filepath.Join(dir, "leveldb")) }

	t.Logf("start: %d MB", size()>>20)

	for round := 1; size() < limit; round++ {
		logs := copyLogs(t, database, size, limit)
		events := copyEvents(t, database, size, limit)

		t.Logf("round %d: %d logs, %d events copied, files %d MB", round, logs, events, size()>>20)

		if logs+events == 0 {
			t.Fatal("nothing to copy")
		}
	}
}

// growBatch is how many copies go into one write.
const growBatch = 2000

func copyLogs(t *testing.T, database *Store, size func() int64, limit int64) int {
	var batch []store.NewLog

	copied := 0
	last := growMark(t, database)
	flush := func() {
		if len(batch) == 0 {
			return
		}

		if _, err := database.Write([]Envelope{{Logs: batch}}); err != nil {
			t.Fatal(err)
		}

		copied += len(batch)
		batch = batch[:0]
	}

	err := database.logs.Primary().Each(database.db, func(entry kv.Entry[logRecord]) (bool, error) {
		record := entry.Record

		// Only what was there when the round began, never its own copies.
		if record.Sequence > last {
			return true, nil
		}

		batch = append(batch, store.NewLog{
			ProjectID: record.ProjectID, Timestamp: record.Timestamp, Level: record.Level, SeverityNumber: record.SeverityNumber,
			Body: record.Body, TraceID: record.TraceID, SpanID: record.SpanID, Attributes: record.Attributes, Fingerprint: record.Fingerprint,
		})

		if len(batch) < growBatch {
			return true, nil
		}

		flush()

		return size() < limit, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if size() < limit {
		flush()
	}

	return copied
}

func copyEvents(t *testing.T, database *Store, size func() int64, limit int64) int {
	var batch []store.NewEvent

	copied := 0
	last := growMark(t, database)
	issues := map[string]issueRecord{}
	flush := func() {
		if len(batch) == 0 {
			return
		}

		if _, err := database.Write([]Envelope{{Events: batch}}); err != nil {
			t.Fatal(err)
		}

		copied += len(batch)
		batch = batch[:0]
	}

	err := database.events.Primary().Each(database.db, func(entry kv.Entry[eventRecord]) (bool, error) {
		record := entry.Record

		if record.Sequence > last {
			return true, nil
		}

		issue, ok := issues[record.IssueID]

		if !ok {
			found, err := database.issues.Get(database.db, kv.K(kv.Text(record.IssueID)))

			if err != nil {
				// An event of an issue deleted meanwhile: the cleanup's to remove.
				return true, nil
			}

			issue = found
			issues[record.IssueID] = issue
		}

		batch = append(batch, store.NewEvent{
			ProjectID: record.ProjectID, EventID: newEventID(t), Fingerprint: issue.Fingerprint, Title: issue.Title, Culprit: issue.Culprit,
			Level: record.Level, Environment: record.Environment, Release: record.Release, TraceID: record.TraceID,
			SpanID: record.SpanID, ParentSpanID: record.ParentSpanID, Timestamp: record.Timestamp, ReceivedAt: record.ReceivedAt,
			Payload: record.Payload,
		})

		if len(batch) < growBatch/4 {
			return true, nil
		}

		flush()

		return size() < limit, nil
	})

	if err != nil {
		t.Fatal(err)
	}

	if size() < limit {
		flush()
	}

	return copied
}

func newEventID(t *testing.T) string {
	id := make([]byte, 16)

	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}

	return hex.EncodeToString(id)
}

// growMark is the last sequence given out: a round copies records up to it.
func growMark(t *testing.T, database *Store) uint64 {
	sequence, err := database.db.Next()

	if err != nil {
		t.Fatal(err)
	}

	return sequence
}

// parseGrowSize reads 7.5GB or 800MB.
func parseGrowSize(text string) (int64, error) {
	units := map[string]float64{"GB": 1 << 30, "MB": 1 << 20}

	for suffix, unit := range units {
		if number, ok := strings.CutSuffix(text, suffix); ok {
			value, err := strconv.ParseFloat(number, 64)

			return int64(value * unit), err
		}
	}

	return 0, fmt.Errorf("size %q: want a number with GB or MB", text)
}

// dirBytes is the bytes of every file under dir.
func dirBytes(t *testing.T, dir string) int64 {
	var total int64

	err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		// The database compacts meanwhile: a file it removed is no error.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		if err != nil || entry.IsDir() {
			return err
		}

		info, err := entry.Info()

		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		if err == nil {
			total += info.Size()
		}

		return err
	})

	if err != nil {
		t.Fatal(err)
	}

	return total
}
