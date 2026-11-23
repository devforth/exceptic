package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

func openTest(t *testing.T) *Store {
	t.Helper()

	database, err := OpenMemory()

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if !database.closed.Load() {
			checkCollections(t, database)
			_ = database.Close()
		}
	})

	return database
}

// checkCollections verifies the stored data (Store.Verify).
func checkCollections(t *testing.T, database *Store) {
	t.Helper()

	if problems, err := database.Verify(); err != nil || len(problems) > 0 {
		t.Errorf("verify: %v %v", problems, err)
	}
}

func saveEvent(t *testing.T, database *Store, fingerprint string, at time.Time) store.SaveResult {
	t.Helper()

	result, err := database.SaveEvent(context.Background(), store.NewEvent{
		ProjectID: 1, EventID: fmt.Sprintf("%032x", at.UnixNano()), Fingerprint: fingerprint, Title: "Error: " + fingerprint,
		Level: "error", Timestamp: at, ReceivedAt: at, Payload: []byte(`{}`), TraceID: "trace-" + fingerprint,
	})

	if err != nil {
		t.Fatal(err)
	}

	return result
}

func storedIssue(database *Store, fingerprint string) *issueRecord {
	database.issuesMutex.RLock()

	defer database.issuesMutex.RUnlock()

	return database.table.byFingerprint[fingerprintKey(1, fingerprint)]
}

// An issue counts its stored events per day, not per event: with no limit
// on events, a busy issue stays small in memory and on disk.
func TestIssueStaysSmallWithManyEvents(t *testing.T) {
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

	for index := range 300 {
		saveEvent(t, database, "busy", start.Add(time.Duration(index)*time.Second))
	}

	issue := storedIssue(database, "busy")

	if issue.Stored != 300 || issue.EventCount != 300 || len(issue.EventDays) != 1 || issue.EventDays[0].Count != 300 {
		t.Fatalf("stored %d, count %d, days %v", issue.Stored, issue.EventCount, issue.EventDays)
	}
}

// "Any event in the range" for a range that covers a day in part reads the
// events; whole days need only the counts.
func TestIssueDateRange(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	morning := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	saveEvent(t, database, "morning", morning)
	saveEvent(t, database, "evening", morning.Add(12*time.Hour))

	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

	for name, test := range map[string]struct {
		since, until time.Time
		want         int
	}{
		"the whole day":       {day, day.Add(24 * time.Hour), 2},
		"the morning":         {day, day.Add(12 * time.Hour), 1},
		"between the two":     {morning.Add(time.Hour), morning.Add(11 * time.Hour), 0},
		"the next day":        {day.Add(24 * time.Hour), day.Add(48 * time.Hour), 0},
		"from the evening on": {morning.Add(12 * time.Hour), time.Time{}, 1},
	} {
		count, err := database.CountIssues(ctx, store.IssueFilter{SeenSince: test.since, SeenUntil: test.until})

		if err != nil || count != test.want {
			t.Errorf("%s: %d issues (%v), want %d", name, count, err, test.want)
		}
	}
}

// An issue whose events the cleanup deleted stays listed for a range that
// holds its first or last seen time.
func TestIssueDateRangeWithoutEvents(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	first := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	saveEvent(t, database, "emptied", first)
	saveEvent(t, database, "emptied", first.Add(24*time.Hour))

	err := database.update(func(writer *batchWriter) error {
		_, _, err := writer.deleteIssueEvents(storedIssue(database, "emptied").ID, 0, func() bool { return true })

		return err
	})

	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

	for name, test := range map[string]struct {
		since, until time.Time
		want         int
	}{
		"the first day":    {day, day.Add(24 * time.Hour), 1},
		"the last morning": {first.Add(23 * time.Hour), first.Add(25 * time.Hour), 1},
		"the day after":    {day.Add(48 * time.Hour), day.Add(72 * time.Hour), 0},
	} {
		count, err := database.CountIssues(ctx, store.IssueFilter{SeenSince: test.since, SeenUntil: test.until})

		if err != nil || count != test.want {
			t.Errorf("%s: %d issues (%v), want %d", name, count, err, test.want)
		}
	}
}

// Lists read newest first and continue before an id, as the API's cursors
// do; logs of every level are merged in order.
func TestListCursorsAndPages(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)

	for index := range 5 {
		saveEvent(t, database, "one", start.Add(time.Duration(index)*time.Minute))
	}

	issue := storedIssue(database, "one")
	first, _ := database.ListEvents(ctx, store.EventFilter{IssueID: issue.ID, Limit: 2})
	next, _ := database.ListEvents(ctx, store.EventFilter{IssueID: issue.ID, Limit: 10, BeforeID: first[1].ID})

	if len(first) != 2 || len(next) != 3 || !first[0].ReceivedAt.After(first[1].ReceivedAt) || next[0].ID >= first[1].ID {
		t.Fatalf("pages %v / %v", first, next)
	}

	if byTrace, _ := database.ListEvents(ctx, store.EventFilter{TraceID: "trace-one", Limit: 10}); len(byTrace) != 5 {
		t.Errorf("by trace: %d events", len(byTrace))
	}

	var logs []store.NewLog

	for index := range 7 {
		level := []string{"info", "error"}[index%2]
		logs = append(logs, store.NewLog{ProjectID: 1 + index%2, Timestamp: start, Level: level, Body: fmt.Sprintf("line %d", index)})
	}

	if err := database.SaveLogs(ctx, logs, 0); err != nil {
		t.Fatal(err)
	}

	page, _ := database.ListLogs(ctx, store.LogFilter{Limit: 3, Offset: 2})
	rest, _ := database.ListLogs(ctx, store.LogFilter{Limit: 10, BeforeID: page[2].ID})

	if len(page) != 3 || page[0].Body != "line 4" || len(rest) != 2 || rest[0].Body != "line 1" {
		t.Fatalf("logs %v / %v", page, rest)
	}

	if count, more, _ := database.CountLogs(ctx, store.LogFilter{ProjectIDs: []int{2}, Levels: []string{"error"}}); count != 3 || more {
		t.Errorf("project 2 errors: %d", count)
	}
}

// Deleting an issue removes its events, their index keys and its history;
// only the store's own bookkeeping is left.
func TestDeleteIssue(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	result := saveEvent(t, database, "gone", time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC))

	if size, err := database.DatabaseSize(ctx); err != nil || size <= 0 {
		t.Errorf("size %d %v", size, err)
	}

	if _, err := database.SetIssuesStatus(ctx, []string{result.IssueID}, store.StatusResolved, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	if history, _ := database.IssueStatusHistory(ctx, result.IssueID); len(history) != 1 {
		t.Fatalf("history %v", history)
	}

	if deleted, err := database.DeleteIssues(ctx, []string{result.IssueID, result.IssueID, "missing0"}); err != nil || deleted != 1 {
		t.Fatalf("deleted %d, %v", deleted, err)
	}

	for _, check := range []func() (int, error){
		func() (int, error) {
			count, _, err := database.events.Primary().Count(database.db, 0)
			return count, err
		},
		func() (int, error) {
			count, _, err := database.status.Primary().Count(database.db, 0)
			return count, err
		},
		func() (int, error) {
			count, _, err := database.issues.Primary().Count(database.db, 0)
			return count, err
		},
	} {
		if count, err := check(); err != nil || count != 0 {
			t.Errorf("left after the delete: %d (%v)", count, err)
		}
	}

	if counters := database.db.Counters(); len(counters) != 0 {
		t.Errorf("counters left: %v", counters)
	}

	if _, err := database.GetIssue(ctx, result.IssueID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("issue: %v", err)
	}

	checkCollections(t, database)

	if err := database.Close(); err != nil || database.Close() != nil {
		t.Error("a second close fails")
	}
}

// A resolved issue reopens only for an event received after the resolve: one
// that waited in the inbox meanwhile only counts.
func TestEventBeforeResolveDoesNotReopen(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	resolvedAt := time.Date(2026, 11, 10, 8, 10, 0, 0, time.UTC)
	result := saveEvent(t, database, "fixed", resolvedAt.Add(-10*time.Minute))
	database.SetClock(func() time.Time { return resolvedAt })

	if _, err := database.SetIssuesStatus(ctx, []string{result.IssueID}, store.StatusResolved, store.Actor{Source: store.SourceAPI}); err != nil {
		t.Fatal(err)
	}

	if saveEvent(t, database, "fixed", resolvedAt.Add(-5*time.Minute)).Regression || storedIssue(database, "fixed").Status != store.StatusResolved {
		t.Fatal("an event received before the resolve reopened the issue")
	}

	if !saveEvent(t, database, "fixed", resolvedAt.Add(time.Minute)).Regression || storedIssue(database, "fixed").Status != store.StatusUnresolved {
		t.Fatal("an event received after the resolve did not reopen the issue")
	}
}

// Verify finds broken index keys in any collection, and events of no issue.
func TestVerifyFindsWhatIsLeft(t *testing.T) {
	ctx := context.Background()
	database, err := OpenMemory()

	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = database.Close() }()

	if err := database.EnqueueOutbox(ctx, []string{"conn0001"}, []int{1}, "notification", []byte(`{}`), time.Now()); err != nil {
		t.Fatal(err)
	}

	result := saveEvent(t, database, "orphan", time.Now())

	if _, err := database.SetIssuesStatus(ctx, []string{result.IssueID}, store.StatusResolved, store.Actor{}); err != nil {
		t.Fatal(err)
	}

	if problems, err := database.Verify(); err != nil || len(problems) > 0 {
		t.Fatalf("before: %v %v", problems, err)
	}

	// The message's record goes, its index keys stay; the issue leaves the table, its events and status changes stay.
	messages, _ := database.ListOutbox(ctx, store.OutboxFilter{})

	if err := database.db.Update(true, func(batch *kv.Batch) error {
		batch.Delete(append([]byte("outbox/"), sequenceKey(uint64(messages[0].ID))...))

		return nil
	}); err != nil {
		t.Fatal(err)
	}

	database.issuesMutex.Lock()
	database.table.remove(result.IssueID)
	database.issuesMutex.Unlock()

	problems, err := database.Verify()
	found := strings.Join(problems, "\n")

	if err != nil || !strings.Contains(found, "outbox.pending key") || !strings.Contains(found, "1 events stored, 0 of them belong to an issue") ||
		!strings.Contains(found, "of missing issue "+result.IssueID) {
		t.Fatalf("problems: %v %v", problems, err)
	}
}

// cancelAfterFirst lets one batch run, then reports the context cancelled.
type cancelAfterFirst struct {
	context.Context
	checks int
}

func (ctx *cancelAfterFirst) Err() error {
	ctx.checks++

	if ctx.checks > 1 {
		return context.Canceled
	}

	return nil
}

// An issue with more events than one batch holds is deleted over several
// batches, its record in the last one.
func TestDeleteLargeIssueInBatches(t *testing.T) {
	database := openTest(t)
	clockAt(database, time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC))
	writeEvents(t, database, "large", "error", cleanupChunk+10)
	issueID := storedIssue(database, "large").ID

	deleted, err := database.DeleteIssues(&cancelAfterFirst{Context: context.Background()}, []string{issueID})

	if !errors.Is(err, context.Canceled) || deleted != 0 {
		t.Fatalf("after one batch: deleted %d, %v", deleted, err)
	}

	if left := storedEvents(database, "large"); left != 10 {
		t.Errorf("%d events left after one batch, want 10", left)
	}

	if deleted, err := database.DeleteIssues(context.Background(), []string{issueID}); err != nil || deleted != 1 {
		t.Fatalf("deleted %d, %v", deleted, err)
	}

	if storedIssue(database, "large") != nil {
		t.Error("issue left")
	}
}

// Ping reads the database, so /health fails once it cannot.
func TestPing(t *testing.T) {
	database := openTest(t)

	if err := database.Ping(context.Background()); err != nil {
		t.Fatalf("open store: %v", err)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if err := database.Ping(context.Background()); err == nil {
		t.Error("a closed store answers the ping")
	}
}

// Logs of a message logged over and over keep their newest copies, also
// when one batch adds more than that.
func TestRepeatedMessages(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	at := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	repeat := func(from, to int) []store.NewLog {
		var logs []store.NewLog

		for index := from; index < to; index++ {
			logs = append(logs, store.NewLog{ProjectID: 1, Timestamp: at, Level: "warn", Body: fmt.Sprintf("retry %d", index), Fingerprint: "retry"})
		}

		return logs
	}

	for _, batch := range [][]store.NewLog{repeat(0, 3), repeat(3, 12)} {
		if err := database.SaveLogs(ctx, batch, 4); err != nil {
			t.Fatal(err)
		}
	}

	logs, err := database.ListLogs(ctx, store.LogFilter{})

	if err != nil || len(logs) != 4 || logs[0].Body != "retry 11" || logs[3].Body != "retry 8" {
		t.Fatalf("kept %v (%v)", logs, err)
	}
}

// A dated log list reads only the records received around the range: the
// time marks bound the scan, and the result is the same.
func TestLogDatesUseTimeMarks(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

	// Three batches received hours apart, each marked by the writer.
	for index, at := range []time.Time{start, start.Add(3 * time.Hour), start.Add(6 * time.Hour)} {
		database.SetClock(func() time.Time { return at })

		if err := database.SaveLogs(ctx, []store.NewLog{{ProjectID: 1, Timestamp: at, Level: "info", Body: fmt.Sprintf("batch %d", index)}}, 0); err != nil {
			t.Fatal(err)
		}
	}

	bodies := func(filter store.LogFilter) []string {
		t.Helper()

		logs, err := database.ListLogs(ctx, filter)

		if err != nil {
			t.Fatal(err)
		}

		var result []string

		for _, record := range logs {
			result = append(result, record.Body)
		}

		return result
	}

	if got := bodies(store.LogFilter{Since: start.Add(2 * time.Hour), Until: start.Add(4 * time.Hour)}); len(got) != 1 || got[0] != "batch 1" {
		t.Errorf("middle: %v", got)
	}

	if got := bodies(store.LogFilter{Until: start.Add(time.Hour)}); len(got) != 1 || got[0] != "batch 0" {
		t.Errorf("until: %v", got)
	}

	if got := bodies(store.LogFilter{Since: start.Add(5 * time.Hour)}); len(got) != 1 || got[0] != "batch 2" {
		t.Errorf("since: %v", got)
	}

	if got := bodies(store.LogFilter{Since: time.Unix(0, 0).UTC()}); len(got) != 3 {
		t.Errorf("since 1970: %v", got)
	}

	// The bounds: since the middle, the scan stops before batch 0; until
	// the first hour, it starts after batch 0.
	stop := database.sequenceAt(start.Add(2*time.Hour - markSlack))
	end := database.sequenceAfter(start.Add(time.Hour + markSlack))

	if stop == 0 || end == 0 || stop >= end {
		t.Errorf("marks: stop %d, end %d", stop, end)
	}
}

// The inbox keeps an envelope until the batch that stores its data deletes
// it, and its size follows.
func TestInbox(t *testing.T) {
	database := openTest(t)
	received := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	first, err := database.Append(1, received, []byte("one"))

	if err != nil {
		t.Fatal(err)
	}

	second, _ := database.Append(2, received, []byte("two"))

	if database.InboxBytes() != int64(first.Size+second.Size) {
		t.Errorf("inbox bytes %d", database.InboxBytes())
	}

	pending, err := database.Pending(0, 10, func(sequence uint64) bool { return sequence == first.Sequence })

	if err != nil || len(pending) != 1 || pending[0].Sequence != second.Sequence || pending[0].ProjectID != 2 || pending[0].Size != second.Size {
		t.Fatalf("pending %+v %v", pending, err)
	}

	if body, found, err := database.InboxBody(second.Sequence); err != nil || !found || string(body) != "two" {
		t.Fatalf("body %q %v %v", body, found, err)
	}

	if _, err := database.Write([]Envelope{{Sequence: first.Sequence, InboxSize: first.Size}}); err != nil {
		t.Fatal(err)
	}

	if _, waiting, _ := database.InboxBody(first.Sequence); waiting || database.InboxBytes() != int64(second.Size) {
		t.Errorf("stored entry still waiting: %v, %d bytes", waiting, database.InboxBytes())
	}
}

// Reading a body allocates it once (the engine's copy), not again for the decode.
func TestInboxBodyCopiedOnce(t *testing.T) {
	database := openTest(t)
	body := bytes.Repeat([]byte("x"), 4<<20)
	entry, err := database.Append(1, time.Now(), body)

	if err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)

	read, found, err := database.InboxBody(entry.Sequence)

	runtime.ReadMemStats(&after)

	if err != nil || !found || !bytes.Equal(read, body) {
		t.Fatalf("body of %d bytes, found %v, %v", len(read), found, err)
	}

	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > uint64(len(body))*3/2 {
		t.Errorf("allocated %d bytes for a body of %d", allocated, len(body))
	}
}

// A project's "no level" stays apart from "the defaults" (nil) once stored.
func TestProjectKeepsNoLevels(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)

	var none []string

	id, err := database.CreateProject(ctx, store.Project{Name: "api", PublicKey: "0123456789abcdef", LogLevels: &none}, time.Now(), "")

	if err != nil {
		t.Fatal(err)
	}

	project, err := database.GetProject(ctx, id)

	if err != nil || project.LogLevels == nil || len(*project.LogLevels) != 0 {
		t.Fatalf("stored levels: %v (%v)", project.LogLevels, err)
	}

	if id < firstProjectID || id >= firstProjectID+projectIDRange {
		t.Fatalf("id %d is not 8 digits", id)
	}
}

// A database of another format, or one from before the format was recorded, is refused at start.
func TestFormatIsChecked(t *testing.T) {
	dir := t.TempDir()

	for _, test := range []struct {
		change  func(batch *kv.Batch, key kv.Key)
		refusal string
	}{
		{func(*kv.Batch, kv.Key) {}, ""},
		{func(batch *kv.Batch, key kv.Key) { batch.Put(key, []byte("0")) }, "another format: 0"},
		{func(batch *kv.Batch, key kv.Key) { batch.Delete(key) }, "development version"},
	} {
		database, err := Open(dir)

		if err != nil {
			t.Fatal(err)
		}

		writeLogs(t, database, "info", 1, "line")

		if err := database.db.Update(true, func(batch *kv.Batch) error { test.change(batch, kv.Key("format/")); return nil }); err != nil {
			t.Fatal(err)
		}

		_ = database.Close()
		reopened, err := Open(dir)

		if test.refusal == "" && err != nil || test.refusal != "" && (err == nil || !strings.Contains(err.Error(), test.refusal)) {
			t.Fatalf("reopen: %v, want %q", err, test.refusal)
		}

		if err == nil {
			_ = reopened.Close()
		}

		if err := database.db.Update(true, func(batch *kv.Batch) error { batch.Put(kv.Key("format/"), []byte(storeFormat)); return nil }); err == nil {
			t.Fatal("a closed database accepted a write")
		}

		dir = t.TempDir()
	}
}

// Counts stop one past the limit, by keys or by records alike.
func TestLogCountStopsAtTheLimit(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	limit := store.LogCountLimit
	store.LogCountLimit = 5

	t.Cleanup(func() { store.LogCountLimit = limit })
	writeLogs(t, database, "error", 4, "error")
	writeLogs(t, database, "info", 3, "info")

	for _, test := range []struct {
		filter store.LogFilter
		count  int
		more   bool
	}{
		{store.LogFilter{}, 5, true},
		{store.LogFilter{Levels: []string{"error"}}, 4, false},
		{store.LogFilter{Query: "INFO"}, 3, false},
		{store.LogFilter{ProjectIDs: []int{1, 2}}, 5, true},
	} {
		if count, more, err := database.CountLogs(ctx, test.filter); err != nil || count != test.count || more != test.more {
			t.Errorf("%+v: %d %v %v, want %d %v", test.filter, count, more, err, test.count, test.more)
		}
	}
}

// The search finds the same lines whether the stored bytes can be searched
// first or, for characters JSON escapes, only the decoded line.
func TestLogSearch(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)

	if err := database.SaveLogs(ctx, []store.NewLog{
		{ProjectID: 1, Timestamp: database.now(), Level: "info", Body: `GET /users <id> & "quoted" C:\temp`},
		{ProjectID: 1, Timestamp: database.now(), Level: "info", Body: "nothing here"},
	}, 0); err != nil {
		t.Fatal(err)
	}

	for query, want := range map[string]int{"get /USERS": 1, `"quoted"`: 1, "<id> &": 1, `C:\temp`: 1, "here": 1, "absent": 0, "level": 0} {
		if logs, err := database.ListLogs(ctx, store.LogFilter{Query: query}); err != nil || len(logs) != want {
			t.Errorf("%q: %d logs, want %d (%v)", query, len(logs), want, err)
		}
	}

	// A search whose request has ended stops.
	ended, cancel := context.WithCancel(ctx)
	cancel()

	if logs, err := database.ListLogs(ended, store.LogFilter{Query: "absent"}); len(logs) != 0 || !errors.Is(err, context.Canceled) {
		t.Errorf("ended search: %d logs, %v", len(logs), err)
	}

	if _, _, err := database.CountLogs(ended, store.LogFilter{Query: "absent"}); !errors.Is(err, context.Canceled) {
		t.Errorf("ended count: %v", err)
	}
}

// An outbox message keeps its payload as it was; an event that an earlier
// version stored after it is ignored.
func TestOutboxPayloads(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Now()

	for _, payload := range []string{`{"Title":"boom"}`, `{"Count":3}`} {
		if err := database.EnqueueOutbox(ctx, []string{"conn0001"}, []int{1}, "notification", []byte(payload), now); err != nil {
			t.Fatal(err)
		}
	}

	due, err := database.DueOutbox(ctx, "conn0001", now, 10)

	if err != nil || len(due) != 2 || string(due[0].Payload) != `{"Title":"boom"}` || string(due[1].Payload) != `{"Count":3}` {
		t.Fatalf("due: %+v, %v", due, err)
	}

	encoded, err := encodeOutbox(store.OutboxRecord{OutboxMessage: store.OutboxMessage{Payload: []byte(`{"Title":"boom"}`)}})

	if err != nil {
		t.Fatal(err)
	}

	if message, err := decodeOutbox(append(encoded, `{"exception":{}}`...)); err != nil || string(message.Payload) != `{"Title":"boom"}` {
		t.Fatalf("earlier record: %q, %v", message.Payload, err)
	}

	if count, err := database.CountOutbox(ctx, store.OutboxFilter{Status: store.OutboxPending}); err != nil || count != 2 {
		t.Fatalf("pending: %d, %v", count, err)
	}
}

// Deleting a project removes its issues, events and logs, in several batches,
// and its place in connections; the other projects keep theirs.
func TestDeleteProject(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)

	for _, project := range []store.Project{{ID: 1, Name: "backend", PublicKey: "0123456789abcdef"}, {ID: 2, Name: "frontend", PublicKey: "abcdef0123456789"}} {
		if err := database.SeedProject(ctx, project, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	writeLogs(t, database, "info", 2*deleteChunk+10, "line") // projects 1 and 2 by turns

	for _, project := range []int{1, 2} {
		for index := range 3 {
			if _, err := database.SaveEvent(ctx, store.NewEvent{ProjectID: project, EventID: fmt.Sprintf("%d-%d", project, index), Fingerprint: fmt.Sprint(index),
				Title: "Error", Level: "error", Timestamp: time.Now(), ReceivedAt: time.Now(), Payload: []byte(`{}`)}); err != nil {
				t.Fatal(err)
			}
		}
	}

	destination, _ := database.CreateDestination(ctx, store.Destination{Type: "slack", Verified: true}, time.Now())
	connection, err := database.CreateConnection(ctx, store.Connection{Name: "Ops", DestinationID: destination, ProjectIDs: []int{1, 2}, Active: true}, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	backendOnly, err := database.CreateConnection(ctx, store.Connection{Name: "Backend", DestinationID: destination, ProjectIDs: []int{1}, Active: true}, time.Now())

	if err != nil {
		t.Fatal(err)
	}

	if err := database.DeleteProject(ctx, 1); err != nil {
		t.Fatal(err)
	}

	// From the delete on, nothing more of it is stored.
	if _, err := database.SaveEvent(ctx, store.NewEvent{ProjectID: 1, EventID: "late", Fingerprint: "late",
		Title: "Error", Level: "error", Timestamp: time.Now(), ReceivedAt: time.Now(), Payload: []byte(`{}`)}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("event of a deleted project: %v", err)
	}

	if err := database.SaveLogs(ctx, []store.NewLog{{ProjectID: 1, Level: "info", Body: "late", Timestamp: time.Now()}}, 0); err != nil {
		t.Fatal(err)
	}

	// A purge stopped early goes on at the next call.
	stopped, cancel := context.WithCancel(ctx)
	cancel()

	if err := database.PurgeProjects(stopped); !errors.Is(err, context.Canceled) {
		t.Fatalf("stopped purge: %v", err)
	}

	if issues, _ := database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{1}}); issues != 3 {
		t.Fatalf("a stopped purge deleted %d of 3 issues", 3-issues)
	}

	if logs := countLogs(t, database, store.LogFilter{ProjectIDs: []int{1}}); logs != deleteChunk+5 {
		t.Fatalf("project 1 has %d logs, want %d: a late one stored or a stopped purge deleted some", logs, deleteChunk+5)
	}

	if err := database.PurgeProjects(ctx); err != nil {
		t.Fatal(err)
	}

	if purges, _ := database.purges.Primary().All(database.db); len(purges) != 0 {
		t.Fatalf("purges left: %v", purges)
	}

	for project, want := range map[int]int{1: 0, 2: deleteChunk + 5} {
		if logs := countLogs(t, database, store.LogFilter{ProjectIDs: []int{project}}); logs != want {
			t.Errorf("project %d keeps %d logs, want %d", project, logs, want)
		}

		if issues, _ := database.CountIssues(ctx, store.IssueFilter{ProjectIDs: []int{project}}); issues != map[int]int{1: 0, 2: 3}[project] {
			t.Errorf("project %d keeps %d issues", project, issues)
		}
	}

	if saved, _ := database.GetConnection(ctx, connection); !slices.Equal(saved.ProjectIDs, []int{2}) || !saved.Active {
		t.Errorf("connection projects: %v, active %v", saved.ProjectIDs, saved.Active)
	}

	// Left without projects it matches nothing, so it is shown off.
	if saved, _ := database.GetConnection(ctx, backendOnly); len(saved.ProjectIDs) != 0 || saved.Active {
		t.Errorf("connection of the deleted project only: %v, active %v", saved.ProjectIDs, saved.Active)
	}
}

// Deleting a connection deletes a chat's destination with its secret and
// keeps a confirmed email address.
func TestDeleteConnectionDestination(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	now := time.Now()

	for _, destination := range []store.Destination{
		{Type: config.ChannelTelegram, Secret: "sealed-token", Verified: true},
		{Type: config.ChannelEmail, Address: "ops@example.com", Verified: true},
	} {
		id, err := database.CreateDestination(ctx, destination, now)

		if err != nil {
			t.Fatal(err)
		}

		connection, err := database.CreateConnection(ctx, store.Connection{Name: destination.Type, DestinationID: id, AllProjects: true, Active: true}, now)

		if err != nil {
			t.Fatal(err)
		}

		if err := database.DeleteConnection(ctx, connection); err != nil {
			t.Fatal(err)
		}

		_, err = database.GetDestination(ctx, id)

		if kept := err == nil; kept != (destination.Type == config.ChannelEmail) {
			t.Errorf("%s destination kept: %v (%v)", destination.Type, kept, err)
		}
	}
}

// A project deleted before a restart stays deleted for ingest and its data
// goes at the next purge; ids past 32 bits never reach a stored project.
func TestDeleteProjectAcrossRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	if err := database.SeedProject(ctx, store.Project{ID: 1, Name: "backend", PublicKey: "0123456789abcdef"}, time.Now()); err != nil {
		t.Fatal(err)
	}

	if _, err := database.GetProject(ctx, 1<<32+1); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("project 2^32+1: %v, want not found", err)
	}

	event := store.NewEvent{ProjectID: 1, EventID: "first", Fingerprint: "first", Title: "Error", Level: "error",
		Timestamp: time.Now(), ReceivedAt: time.Now(), Payload: []byte(`{}`)}

	if _, err := database.SaveEvent(ctx, event); err != nil {
		t.Fatal(err)
	}

	if err := database.DeleteProject(ctx, 1); err != nil {
		t.Fatal(err)
	}

	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = reopened.Close() })

	event.EventID, event.Fingerprint = "late", "late"

	if _, err := reopened.SaveEvent(ctx, event); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("event of a deleted project after a restart: %v", err)
	}

	if err := reopened.PurgeProjects(ctx); err != nil {
		t.Fatal(err)
	}

	if issues, _ := reopened.CountIssues(ctx, store.IssueFilter{}); issues != 0 {
		t.Fatalf("%d issues of the deleted project left", issues)
	}
}

// Pages kept as the issues go match pages of a full sort, for every order.
func TestIssuePagesMatchAFullSort(t *testing.T) {
	ctx := context.Background()
	database := openTest(t)
	start := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

	for index := range 60 {
		at := start.Add(time.Duration(index%7) * time.Minute) // ties, broken by id

		if _, err := database.SaveEvent(ctx, store.NewEvent{ProjectID: 1 + index%2, EventID: fmt.Sprint(index), Fingerprint: fmt.Sprint(index % 30),
			Title: "Error", Level: "error", Timestamp: at, ReceivedAt: at, Payload: []byte(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}

	for _, sort := range store.IssueSorts {
		all, err := database.ListIssues(ctx, store.IssueFilter{Sort: sort, Offset: maxKeptIssues})

		if err != nil || len(all) != 0 {
			t.Fatalf("%s: %d issues past the end, %v", sort, len(all), err)
		}

		everything, _ := database.ListIssues(ctx, store.IssueFilter{Sort: sort})

		for offset := 0; offset < 30; offset += 7 {
			page, _ := database.ListIssues(ctx, store.IssueFilter{Sort: sort, Offset: offset, Limit: 7})
			want := everything[offset:min(offset+7, len(everything))]

			if !slices.EqualFunc(page, want, func(a, b store.Issue) bool { return a.ID == b.ID }) {
				t.Errorf("%s from %d: got %v, want %v", sort, offset, page, want)
			}
		}
	}
}
