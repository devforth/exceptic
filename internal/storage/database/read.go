package database

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// newestFirst merges queries newest first by the sequence number ending their keys.
func newestFirst[T any](reader kv.Reader, queries []kv.Query[T], visit func(entry kv.Entry[T]) bool) error {
	_, err := merged(reader, queries, true, visit)

	return err
}

// oldestFirst merges queries oldest first by the sequence number ending their keys.
func oldestFirst[T any](reader kv.Reader, queries []kv.Query[T], visit func(entry kv.Entry[T]) bool) error {
	_, err := merged(reader, queries, false, visit)

	return err
}

// merged walks the queries in sequence order, key by key: the keys a walk
// skips (Raw) take their turn too, so the walks advance together. When a
// context stops them, rest is where the merge can go on newest first: every
// record visited has a sequence at or above it, every one not visited yet is
// below it (math.MaxUint64 when a walk stopped before its first key).
func merged[T any](reader kv.Reader, queries []kv.Query[T], newest bool, visit func(entry kv.Entry[T]) bool) (rest uint64, err error) {
	type head struct {
		cursor   *kv.Cursor[T]
		entry    kv.Entry[T]
		sequence uint64 // of the last key read
		read     bool
		kept     bool
		valid    bool
	}

	step := func(current *head) bool {
		current.entry, current.kept, current.valid = current.cursor.Step()

		if current.valid {
			current.sequence, current.read = sequenceOf(current.entry.Key), true
		}

		return !current.cursor.Stopped()
	}

	heads := make([]*head, 0, len(queries))
	going := true

	for _, query := range queries {
		if newest {
			query = query.Newest()
		}

		current := &head{cursor: query.Open(reader)}
		heads = append(heads, current)
		going = step(current) && going
	}

	for going {
		var next *head

		for _, current := range heads {
			if current.valid && (next == nil || (current.sequence > next.sequence) == newest) {
				next = current
			}
		}

		if next == nil || next.kept && !visit(next.entry) {
			break
		}

		going = step(next)
	}

	for _, current := range heads {
		if !going {
			rest = max(rest, restOf(current.cursor.Stopped(), current.read, current.valid, current.kept, current.sequence))
		}

		err = errors.Join(err, current.cursor.Close())
	}

	return rest, err
}

// restOf is one above the newest sequence a stopped newest-first walk may
// still give: its kept head is not visited yet, a skipped head or the last
// key a stopped walk read is done with.
func restOf(stopped, read, valid, kept bool, sequence uint64) uint64 {
	switch {
	case valid && kept:
		return sequence + 1
	case valid || stopped && read:
		return sequence
	case stopped:
		return math.MaxUint64
	default:
		return 0
	}
}

func before[T any](query kv.Query[T], sequence uint64) kv.Query[T] {
	if sequence == 0 {
		return query
	}

	return query.Before(kv.U64(sequence))
}

func eventSummary(meta eventMeta) store.EventSummary {
	return store.EventSummary{
		ID: int64(meta.Sequence), ProjectID: meta.ProjectID, IssueID: meta.IssueID, EventID: meta.EventID, Timestamp: meta.Timestamp,
		ReceivedAt: meta.ReceivedAt, Level: meta.Level, Environment: meta.Environment, Release: meta.Release,
		TraceID: meta.TraceID, SpanID: meta.SpanID, ParentSpanID: meta.ParentSpanID, Thinned: meta.Thinned,
	}
}

func fullEvent(record eventRecord) store.Event {
	return store.Event{EventSummary: eventSummary(record.eventMeta), Payload: json.RawMessage(record.Payload)}
}

func (database *Store) ListEvents(_ context.Context, filter store.EventFilter) ([]store.EventSummary, error) {
	beforeSequence := uint64(max(filter.BeforeID, 0))

	var queries []kv.Query[eventRecord]

	switch {
	case filter.TraceID != "":
		queries = append(queries, database.events.By("trace", kv.Text(filter.TraceID)))
	case filter.IssueID != "":
		queries = append(queries, database.events.Primary(kv.Text(filter.IssueID)))
	default:
		for _, rank := range allRanks {
			queries = append(queries, database.events.By("age", kv.U8(rank)))
		}
	}

	for index := range queries {
		queries[index] = before(queries[index], beforeSequence).Head()
	}

	events := []store.EventSummary{}

	err := newestFirst(database.db, queries, func(entry kv.Entry[eventRecord]) bool {
		if filter.IssueID == "" || entry.Record.IssueID == filter.IssueID {
			events = append(events, eventSummary(entry.Record.eventMeta))
		}

		return filter.Limit <= 0 || len(events) < filter.Limit
	})

	return events, err
}

func (database *Store) GetEvent(_ context.Context, projectID int, eventID string) (store.Event, error) {
	record, err := database.events.GetBy(database.db, "id", eventIDKey(projectID, eventID))

	if err != nil {
		return store.Event{}, notFound(err)
	}

	return fullEvent(record), nil
}

func (database *Store) LatestEvent(_ context.Context, issueID string) (store.Event, error) {
	entry, err := database.events.Primary(kv.Text(issueID)).Newest().First(database.db)

	if err != nil {
		return store.Event{}, notFound(err)
	}

	return fullEvent(entry.Record), nil
}

// logQueries picks the walks a log filter reads; they stop when ctx ends.
// logsBefore is the sequence a filter's walks start below, or 0 for none.
func (database *Store) logsBefore(filter store.LogFilter) uint64 {
	beforeSequence := uint64(max(filter.BeforeID, 0))

	if !filter.Until.IsZero() {
		if end := database.sequenceAfter(filter.Until.Add(markSlack)); end > 0 && (beforeSequence == 0 || end < beforeSequence) {
			beforeSequence = end
		}
	}

	return beforeSequence
}

func (database *Store) logQueries(ctx context.Context, filter store.LogFilter) []kv.Query[logRecord] {
	beforeSequence := database.logsBefore(filter)

	var queries []kv.Query[logRecord]

	switch {
	case filter.TraceID != "":
		queries = append(queries, database.logs.By("trace", kv.Text(filter.TraceID)))
	case len(filter.SpanIDs) > 0:
		spans := slices.Clone(filter.SpanIDs)
		slices.Sort(spans)

		for _, span := range slices.Compact(spans) {
			queries = append(queries, database.logs.By("span", kv.Text(span)))
		}
	case len(filter.ProjectIDs) > 0:
		for _, project := range filter.ProjectIDs {
			for _, rank := range ranksOf(filter.Levels) {
				queries = append(queries, database.logs.By("project", kv.U32(uint32(project)), kv.U8(rank)))
			}
		}
	default:
		for _, rank := range ranksOf(filter.Levels) {
			queries = append(queries, database.logs.Primary(kv.U8(rank)))
		}
	}

	for index := range queries {
		queries[index] = before(queries[index], beforeSequence).WithContext(ctx)
	}

	return queries
}

func logMatches(record logRecord, filter store.LogFilter) bool {
	switch {
	case len(filter.ProjectIDs) > 0 && !slices.Contains(filter.ProjectIDs, record.ProjectID),
		len(filter.Levels) > 0 && !slices.Contains(filter.Levels, record.Level),
		filter.TraceID != "" && record.TraceID != filter.TraceID,
		len(filter.SpanIDs) > 0 && !slices.Contains(filter.SpanIDs, record.SpanID),
		!filter.Since.IsZero() && record.Timestamp.Before(filter.Since),
		!filter.Until.IsZero() && !record.Timestamp.Before(filter.Until),
		filter.Query != "" && !textutil.ContainsFold(record.Body, filter.Query):
		return false
	}

	return true
}

// errSearchTime ends a log scan at config.LogSearchTime.
var errSearchTime = errors.New("log search time is up")

// searchBudget is a scan's context that also ends at deadline by the store's
// clock; only Err tells it, which a kv walk checks at every key.
type searchBudget struct {
	context.Context
	now      func() time.Time
	deadline time.Time
}

func (budget searchBudget) Err() error {
	if err := budget.Context.Err(); err != nil {
		return err
	}

	if !budget.now().Before(budget.deadline) {
		return errSearchTime
	}

	return nil
}

// scanLogs visits matching logs newest first until visit returns false, ctx
// ends or config.LogSearchTime has passed, which it reports as stopped.
func (database *Store) scanLogs(ctx context.Context, filter store.LogFilter, visit func(record logRecord) bool) (store.LogScan, error) {
	budget := searchBudget{Context: ctx, now: database.now, deadline: database.now().Add(config.LogSearchTime)}
	queries := database.logQueries(budget, filter)
	// Where the walks start: every log they may give is below it.
	top := database.logsBefore(filter)

	if top == 0 {
		top = database.db.Peek()
	}

	from := uint64(max(filter.FromID, 0))

	if !filter.Since.IsZero() {
		from = max(from, database.sequenceAt(filter.Since.Add(-markSlack)))
	}

	for index := range queries {
		if from > 0 {
			queries[index] = queries[index].From(kv.U64(from))
		}

		if rawSearchable(filter.Query) {
			queries[index] = queries[index].Raw(func(value []byte) bool { return textutil.ContainsFold(value, filter.Query) })
		}
	}

	rest, err := merged(database.db, queries, true, func(entry kv.Entry[logRecord]) bool {
		return !logMatches(entry.Record, filter) || visit(entry.Record)
	})

	if rest == math.MaxUint64 {
		// A walk stopped before its first key: it searched nothing below top.
		rest = top
	}

	switch {
	case !errors.Is(err, errSearchTime):
		return store.LogScan{}, err
	case rest <= from:
		// Stopped at its lower bound: nothing was left.
		return store.LogScan{}, nil
	}

	return store.LogScan{Stopped: true, Rest: int64(rest), SearchedTo: database.receivedFrom(rest)}, nil
}

// rawSearchable: the text appears as is in a stored log, since JSON escapes
// none of its characters, so the stored bytes can be searched before decoding.
func rawSearchable(query string) bool {
	if query == "" {
		return false
	}

	for _, char := range query {
		if char < ' ' || strings.ContainsRune(`"\<>&`, char) || char == '\u2028' || char == '\u2029' {
			return false
		}
	}

	return true
}

// countByKeys: the walks of the filter select exactly the logs it means, so
// counting their keys is enough.
func countByKeys(filter store.LogFilter) bool {
	exact := filter.Query == "" && filter.Since.IsZero() && filter.Until.IsZero() && filter.FromID == 0

	if filter.TraceID != "" {
		return exact && len(filter.ProjectIDs) == 0 && len(filter.Levels) == 0 && len(filter.SpanIDs) == 0
	}

	if len(filter.SpanIDs) > 0 {
		return exact && len(filter.ProjectIDs) == 0 && len(filter.Levels) == 0
	}

	return exact
}

func logView(record logRecord) store.LogRecord {
	return store.LogRecord{
		ID: int64(record.Sequence), ProjectID: record.ProjectID, Timestamp: record.Timestamp, Level: record.Level,
		SeverityNumber: record.SeverityNumber, Body: record.Body, TraceID: record.TraceID, SpanID: record.SpanID,
		Attributes: json.RawMessage(record.Attributes),
	}
}

func (database *Store) ListLogs(ctx context.Context, filter store.LogFilter) (store.LogPage, error) {
	page := store.LogPage{Logs: []store.LogRecord{}}
	skip := max(filter.Offset, 0)

	scan, err := database.scanLogs(ctx, filter, func(record logRecord) bool {
		if skip > 0 {
			skip--

			return true
		}

		page.Logs = append(page.Logs, logView(record))

		return filter.Limit <= 0 || len(page.Logs) < filter.Limit
	})
	page.LogScan = scan

	return page, err
}

// LogLevels looks for one key per level: a log's key starts with its
// level's rank, and in the project index with its project and rank.
func (database *Store) LogLevels(_ context.Context, projectIDs []int) ([]string, error) {
	var levels []string

	for _, rank := range allRanks {
		queries := []kv.Query[logRecord]{database.logs.Primary(kv.U8(rank))}

		if len(projectIDs) > 0 {
			queries = queries[:0]

			for _, project := range projectIDs {
				queries = append(queries, database.logs.By("project", kv.U32(uint32(project)), kv.U8(rank)))
			}
		}

		found, err := anyKey(database.db, queries)

		if err != nil {
			return nil, err
		}

		if found {
			levels = append(levels, config.LogLevels[rank])
		}
	}

	return levels, nil
}

// anyKey reports whether one of the queries has a key.
func anyKey(reader kv.Reader, queries []kv.Query[logRecord]) (bool, error) {
	found := false

	for _, query := range queries {
		err := query.Limit(1).Keys(reader, func(kv.Key) (bool, error) {
			found = true

			return false, nil
		})

		if err != nil || found {
			return found, err
		}
	}

	return false, nil
}

func (database *Store) CountLogs(ctx context.Context, filter store.LogFilter) (store.LogCount, error) {
	count := 0

	if countByKeys(filter) {
		// One more than the limit tells there are more.
		for _, query := range database.logQueries(ctx, filter) {
			found, _, err := query.Count(database.db, store.LogCountLimit+1-count)

			if count += found; err != nil {
				return store.LogCount{}, err
			}

			if count > store.LogCountLimit {
				return store.LogCount{Count: store.LogCountLimit, More: true}, nil
			}
		}

		return store.LogCount{Count: count}, nil
	}

	var oldest uint64

	scan, err := database.scanLogs(ctx, filter, func(record logRecord) bool {
		count++
		oldest = record.Sequence

		return count <= store.LogCountLimit
	})

	result := store.LogCount{Count: count, FromID: int64(oldest), LogScan: scan}

	if scan.Stopped {
		result.FromID = scan.Rest
	}

	if count > store.LogCountLimit {
		result.Count, result.More = store.LogCountLimit, true
	}

	return result, err
}

func (database *Store) IssuesForLogs(_ context.Context, logs []store.LogRecord) (map[int64]store.LogIssue, error) {
	result := map[int64]store.LogIssue{}
	bySpan := map[string]store.LogIssue{} // trace + "/" + span
	looked := map[string]bool{}

	for _, record := range logs {
		if record.SpanID == "" || looked[record.SpanID] {
			continue
		}

		looked[record.SpanID] = true

		queries := []kv.Query[eventRecord]{
			database.events.By("span", kv.Text(record.SpanID)).Head(), database.events.By("parent", kv.Text(record.SpanID)).Head(),
		}

		err := newestFirst(database.db, queries, func(entry kv.Entry[eventRecord]) bool {
			meta := entry.Record

			database.issuesMutex.RLock()
			issue, ok := database.table.byID[meta.IssueID]
			database.issuesMutex.RUnlock()

			if ok {
				for _, span := range []string{meta.SpanID, meta.ParentSpanID} {
					if key := meta.TraceID + "/" + span; span != "" && bySpan[key] == (store.LogIssue{}) {
						bySpan[key] = store.LogIssue{IssueID: issue.ID, ProjectID: issue.ProjectID, Title: issue.Title}
					}
				}
			}

			return true
		})

		if err != nil {
			return nil, err
		}
	}

	for _, record := range logs {
		if issue, ok := bySpan[record.TraceID+"/"+record.SpanID]; ok && record.SpanID != "" {
			result[record.ID] = issue
		}
	}

	return result, nil
}

func ranksOf(levels []string) []byte {
	if len(levels) == 0 {
		return allRanks
	}

	var ranks []byte

	for _, level := range levels {
		if rank := levelRank(level); !slices.Contains(ranks, rank) {
			ranks = append(ranks, rank)
		}
	}

	return ranks
}
