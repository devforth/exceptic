package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/platform/logging"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/database"
	"github.com/devforth/exceptic/internal/storage/store"
)

// Pipeline stores envelopes through the inbox: its handler accepts them,
// processors parse them, one writer stores them.
type Pipeline struct {
	handler *Handler
	store   *database.Store
	sink    StoreSink // maps parsed events and logs to records and reports them
	logger  *slog.Logger

	// MaxInboxBytes: beyond it new envelopes get 429.
	MaxInboxBytes int64
	// BatchSize, BatchBytes and BatchWait bound one write.
	BatchSize  int
	BatchBytes int
	BatchWait  time.Duration
	// ParseBytes bounds the inbox bytes of the envelopes being parsed or waiting for the writer.
	ParseBytes int64
	// LogsPerMessage caps the copies of a repeated log message (0: none).
	LogsPerMessage int
	// EventsPerIssue caps the events an issue stores a minute (0: none).
	EventsPerIssue int
	// Rescan is how often the inbox is read again for entries missed.
	Rescan time.Duration

	// fresh takes entries (without bodies) straight from requests; when full, missed asks for a rescan.
	fresh  chan database.InboxEntry
	missed atomic.Bool
	wake   chan struct{}

	stored atomic.Int64 // envelopes stored since start
	// inFlight holds entries handed out and not stored yet.
	inFlight sync.Map

	// progress is closed and replaced when the writer ends a batch; writing is set during one.
	progressMutex sync.Mutex
	progress      chan struct{}
	writing       bool
}

// processed is a parsed entry; events match Envelope.Events in order.
type processed struct {
	database.Envelope
	project config.Project
	events  []*sentry.Event
}

// NewPipeline builds the ingest handler, which writes to the inbox of db, and
// the pipeline that stores what it accepts while Run runs; sink maps and reports.
func NewPipeline(logger *slog.Logger, settings *config.Config, projects config.ProjectList, db *database.Store, sink StoreSink) *Pipeline {
	handler := newHandler(logger, settings, projects)
	pipeline := &Pipeline{
		handler: handler, store: db, sink: sink, logger: logger,
		MaxInboxBytes: config.MaxInboxBytes(config.DefaultMaxDatabaseBytes), LogsPerMessage: config.LogsPerMessage, EventsPerIssue: config.EventsPerIssuePerMinute, BatchSize: 512, BatchBytes: 4 << 20, BatchWait: 5 * time.Millisecond, ParseBytes: 32 << 20, Rescan: 10 * time.Second,
		fresh: make(chan database.InboxEntry, 4*runtime.GOMAXPROCS(0)), wake: make(chan struct{}, 1), progress: make(chan struct{}),
	}
	handler.pipeline = pipeline

	return pipeline
}

// Handler serves POST /api/{projectID}/envelope/.
func (pipeline *Pipeline) Handler() *Handler { return pipeline.handler }

// Dropped counts what ingest refused per project.
func (pipeline *Pipeline) Dropped() *DropCounter { return pipeline.handler.dropped }

// WaitStored waits until the inbox is empty and no batch is being written:
// every envelope accepted before the call is stored or dropped and reported,
// its notifications queued (sending them is asynchronous).
func (pipeline *Pipeline) WaitStored(ctx context.Context) error {
	for {
		pipeline.progressMutex.Lock()
		progress, settled := pipeline.progress, !pipeline.writing && pipeline.store.InboxBytes() == 0
		pipeline.progressMutex.Unlock()

		if settled {
			return nil
		}

		select {
		case <-progress:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// setWriting marks a batch being written; its end wakes WaitStored.
func (pipeline *Pipeline) setWriting(writing bool) {
	pipeline.progressMutex.Lock()
	defer pipeline.progressMutex.Unlock()

	pipeline.writing = writing

	if !writing {
		close(pipeline.progress)
		pipeline.progress = make(chan struct{})
	}
}

func (pipeline *Pipeline) accept(project config.Project, envelope *sentry.Envelope, body []byte) (map[string]string, error) {
	if pipeline.store.InboxBytes() > pipeline.MaxInboxBytes {
		return nil, &requestError{status: http.StatusTooManyRequests, message: "server busy", retryAfter: "10"}
	}

	entry, err := pipeline.store.Append(project.ID, pipeline.handler.now(), body)

	if errors.Is(err, database.ErrCompacting) {
		return nil, errCompacting
	}

	if err != nil {
		return nil, err
	}

	select {
	case pipeline.fresh <- entry:
	default:
		pipeline.missed.Store(true)

		select {
		case pipeline.wake <- struct{}{}:
		default:
		}
	}

	response := map[string]string{}

	if envelope.Header.EventID != "" {
		response["id"] = envelope.Header.EventID
	}

	return response, nil
}

// markContact records the project's first request with its key, once; a
// failure only logs, since the envelope can still be stored.
func (pipeline *Pipeline) markContact(projectID int, at time.Time) {
	if _, err := pipeline.store.MarkContact(projectID, at); err != nil {
		pipeline.logger.Error("mark project connected", "project", projectID, "err", err)
	}
}

func (pipeline *Pipeline) Run(ctx context.Context) {
	workers := runtime.GOMAXPROCS(0)

	// Small queues: envelopes wait in the inbox on disk, not in memory.
	entries := make(chan database.InboxEntry, workers*2)
	results := make(chan processed, workers*4)

	budget := semaphore.NewWeighted(pipeline.ParseBytes)

	var processors sync.WaitGroup

	for range workers {
		processors.Add(1)

		go func() {
			defer processors.Done()

			pipeline.processEntries(entries, results, budget)
		}()
	}

	writerDone := make(chan struct{})

	go func() {
		defer close(writerDone)

		pipeline.write(ctx, results, budget)
	}()

	pipeline.dispatch(ctx, entries)
	close(entries)
	processors.Wait()
	close(results)
	<-writerDone
}

// dispatch hands fresh entries, and the inbox's at start, after a full queue and every Rescan.
func (pipeline *Pipeline) dispatch(ctx context.Context, entries chan<- database.InboxEntry) {
	rescan := time.NewTicker(pipeline.Rescan)

	defer rescan.Stop()

	scan := true

	for ctx.Err() == nil {
		if scan || pipeline.missed.Swap(false) {
			scan = false

			if !pipeline.handInbox(ctx, entries) {
				return
			}
		}

		select {
		case <-ctx.Done():
			return
		case entry := <-pipeline.fresh:
			if !pipeline.hand(ctx, entries, entry) {
				return
			}
		case <-pipeline.wake:
		case <-rescan.C:
			scan = true
		}
	}
}

func (pipeline *Pipeline) hand(ctx context.Context, entries chan<- database.InboxEntry, entry database.InboxEntry) bool {
	if _, busy := pipeline.inFlight.LoadOrStore(entry.Sequence, true); busy {
		return true
	}

	select {
	case entries <- entry:
		return true
	case <-ctx.Done():
		return false
	}
}

func (pipeline *Pipeline) handInbox(ctx context.Context, entries chan<- database.InboxEntry) bool {
	inFlight := func(sequence uint64) bool {
		_, busy := pipeline.inFlight.Load(sequence)

		return busy
	}

	for after := uint64(0); ctx.Err() == nil; {
		pending, err := pipeline.store.Pending(after, pipeline.BatchSize, inFlight)

		if err != nil {
			pipeline.logger.Error("read inbox", "err", err)

			return true
		}

		for _, entry := range pending {
			after = entry.Sequence

			if !pipeline.hand(ctx, entries, entry) {
				return false
			}
		}

		if len(pending) < pipeline.BatchSize {
			return true
		}
	}

	return false
}

// processEntries reads and parses entries while their bytes fit the budget; the writer frees them.
func (pipeline *Pipeline) processEntries(entries <-chan database.InboxEntry, results chan<- processed, budget *semaphore.Weighted) {
	for entry := range entries {
		weight := pipeline.weight(entry.Size)
		_ = budget.Acquire(context.Background(), weight) // never fails: weight <= ParseBytes, no deadline

		if result, ok := pipeline.processOrDrop(entry); ok {
			results <- result
		} else {
			budget.Release(weight)
			pipeline.inFlight.Delete(entry.Sequence)
		}
	}
}

// weight is an envelope's share of ParseBytes; a larger one takes all of it.
func (pipeline *Pipeline) weight(size int) int64 {
	return min(int64(size), pipeline.ParseBytes)
}

// processOrDrop processes an entry; one that panics would panic again at
// every scan and start, so it is logged and only leaves the inbox.
func (pipeline *Pipeline) processOrDrop(entry database.InboxEntry) (result processed, ok bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			pipeline.logger.Error("envelope dropped: processing it panicked", "project_id", entry.ProjectID, "err", logging.PanicError(recovered))

			result, ok = processed{Envelope: database.Envelope{Sequence: entry.Sequence, InboxSize: entry.Size}}, true
		}
	}()

	return pipeline.process(entry)
}

// process reads an entry's body and parses it; a malformed one only leaves the inbox.
// An entry already stored (handed again from fresh after a scan) gives nothing.
func (pipeline *Pipeline) process(entry database.InboxEntry) (processed, bool) {
	body, found, err := pipeline.store.InboxBody(entry.Sequence)

	if err != nil {
		pipeline.logger.Error("read inbox", "err", err)
	}

	if !found {
		return processed{}, false
	}

	result := processed{Envelope: database.Envelope{Sequence: entry.Sequence, InboxSize: entry.Size}}
	project, ok := pipeline.handler.projects.List().ByID(entry.ProjectID)

	if !ok {
		pipeline.logger.Debug("envelope of a deleted project dropped", "project_id", entry.ProjectID)

		return result, true
	}

	result.project = project
	envelope, err := sentry.ParseEnvelope(body)

	if err != nil {
		pipeline.logger.Debug("malformed envelope dropped", "project", project.Name, "err", err)

		return result, true
	}

	events, logs, err := parseItems(pipeline.logger, project, envelope, entry.Received)

	if err != nil {
		pipeline.logger.Debug("malformed envelope dropped", "project", project.Name, "err", err)

		return result, true
	}

	for _, event := range events {
		result.Events = append(result.Events, pipeline.sink.eventRecord(project, event, entry.Received))
		result.events = append(result.events, event)
	}

	if result.Logs, err = pipeline.sink.logRecords(project, logs); err != nil {
		pipeline.logger.Debug("logs dropped", "project", project.Name, "err", err)
	}

	result.LogsPerMessage = pipeline.LogsPerMessage
	result.EventsPerIssue = pipeline.EventsPerIssue

	return result, true
}

// write commits results in batches and reports the stored events.
func (pipeline *Pipeline) write(ctx context.Context, results <-chan processed, budget *semaphore.Weighted) {
	for first := range results {
		batch := []processed{first}
		size := first.InboxSize
		timer := time.NewTimer(pipeline.BatchWait)

	collect:
		for len(batch) < pipeline.BatchSize && size < pipeline.BatchBytes {
			select {
			case next, ok := <-results:
				if !ok {
					break collect
				}

				batch = append(batch, next)
				size += next.InboxSize
			case <-timer.C:
				break collect
			}
		}

		timer.Stop()
		pipeline.setWriting(true)
		pipeline.commit(ctx, batch, budget)
		pipeline.setWriting(false)
	}
}

// commit stores a batch, frees what its envelopes held and reports the stored events.
func (pipeline *Pipeline) commit(ctx context.Context, batch []processed, budget *semaphore.Weighted) {
	saved, stored := pipeline.storeBatch(ctx, batch)

	for _, envelope := range batch {
		pipeline.inFlight.Delete(envelope.Sequence)
		budget.Release(pipeline.weight(envelope.InboxSize))
	}

	if !stored {
		return
	}

	pipeline.stored.Add(int64(len(batch)))

	for index, envelope := range batch {
		for position, result := range saved[index] {
			pipeline.report(envelope, position, result)
		}
	}
}

// report reports one stored event; a panic loses only that report, not the writer.
func (pipeline *Pipeline) report(envelope processed, position int, result store.SaveResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			pipeline.logger.Error("reporting a stored event panicked", "project", envelope.project.Name, "issue_id", result.IssueID, "err", logging.PanicError(recovered))
		}
	}()

	pipeline.sink.report(envelope.project, envelope.events[position], result, envelope.Events[position].ReceivedAt)
}

// storeBatch retries every second; on shutdown the batch stays in the inbox.
// An envelope with a record that cannot be encoded would fail every retry,
// so the batch is then written one envelope at a time and that one is dropped.
func (pipeline *Pipeline) storeBatch(ctx context.Context, batch []processed) ([][]store.SaveResult, bool) {
	envelopes := make([]database.Envelope, len(batch))

	for index, entry := range batch {
		envelopes[index] = entry.Envelope
	}

	for {
		saved, err := pipeline.store.Write(envelopes)

		if err == nil {
			return saved, true
		}

		if unencodable(err) {
			if len(batch) > 1 {
				return pipeline.storeOneByOne(ctx, batch)
			}

			return pipeline.drop(ctx, batch[0], err)
		}

		pipeline.logger.Error("store batch, will retry", "envelopes", len(batch), "err", err)

		select {
		case <-ctx.Done():
			pipeline.logger.Warn("batch left in the inbox for the next start", "envelopes", len(batch))

			return nil, false
		case <-time.After(time.Second):
		}
	}
}

func (pipeline *Pipeline) storeOneByOne(ctx context.Context, batch []processed) ([][]store.SaveResult, bool) {
	results := make([][]store.SaveResult, len(batch))

	for index := range batch {
		saved, stored := pipeline.storeBatch(ctx, batch[index:index+1])

		if !stored {
			return nil, false
		}

		results[index] = saved[0]
	}

	return results, true
}

// drop deletes the inbox entry of an envelope that cannot be stored.
func (pipeline *Pipeline) drop(ctx context.Context, entry processed, reason error) ([][]store.SaveResult, bool) {
	pipeline.logger.Error("envelope dropped: it cannot be stored", "project", entry.project.Name, "err", reason)

	empty := processed{Envelope: database.Envelope{Sequence: entry.Sequence, InboxSize: entry.InboxSize}, project: entry.project}

	if _, stored := pipeline.storeBatch(ctx, []processed{empty}); !stored {
		return nil, false
	}

	return [][]store.SaveResult{nil}, true
}

// unencodable reports a record that JSON cannot encode, which no retry fixes.
func unencodable(err error) bool {
	var (
		marshaler   *json.MarshalerError
		unsupported *json.UnsupportedValueError
	)

	return errors.As(err, &marshaler) || errors.As(err, &unsupported)
}
