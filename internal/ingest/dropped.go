package ingest

import (
	"cmp"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/sentry"
	"github.com/devforth/exceptic/internal/storage/store"
)

// dropKind is what a drop counts: items of a rate limit category, or
// envelopes refused before they were read.
type dropKind int

const (
	dropEvents dropKind = iota
	dropLogs
	dropEnvelopes
	dropKindCount
)

var dropKindNames = [dropKindCount]string{"events", "logs", "envelopes"}

// dropReasons are store.DropReasons by index.
var dropReasons = []string{store.DropRateLimit, store.DropBusy, store.DropQueueShare, store.DropTooLarge, store.DropInvalid, store.DropIncomplete, store.DropSDKRateLimit, store.DropSDKOverflow}

type dropReason int

const (
	dropRateLimit dropReason = iota
	dropBusy
	dropQueueShare
	dropTooLarge
	dropInvalid
	dropIncomplete
	dropSDKRateLimit
	dropSDKOverflow
	dropReasonCount
)

// dropHours is the window of DropCounter, one bucket an hour.
const dropHours = 24

// DropCounter counts per project what ingest refused or the project's SDK
// reported not sending, in hourly buckets of the last day, in memory only;
// a project takes a fixed few KB. The first drop of a project in an hour is
// logged as a warning, the rest only counted.
type DropCounter struct {
	logger   *slog.Logger
	mutex    sync.Mutex
	projects map[int]*projectDrops

	// noticed is when each project's drops were last notified, notice tells
	// the first drop of a project in dropHours (OnFirstDrop).
	noticed map[int]time.Time
	notice  func(project config.Project, kind, reason string, count int64, at time.Time)
}

type projectDrops struct {
	hours  [dropHours]dropHour
	warned time.Time // the hour of the last warning
}

type dropHour struct {
	start  time.Time
	counts [dropReasonCount][dropKindCount]int64
}

func newDropCounter(logger *slog.Logger) *DropCounter {
	return &DropCounter{logger: logger, projects: map[int]*projectDrops{}, noticed: map[int]time.Time{}}
}

// OnFirstDrop calls notice, in its own goroutine, at the first drop of a
// project in dropHours since its last notice; noticed are the last notices
// stored, so a restart does not repeat one. Set before ingest starts.
func (counter *DropCounter) OnFirstDrop(noticed map[int]time.Time, notice func(project config.Project, kind, reason string, count int64, at time.Time)) {
	counter.mutex.Lock()
	defer counter.mutex.Unlock()

	maps.Copy(counter.noticed, noticed)
	counter.notice = notice
}

// add counts count of kind dropped from project for reason at now.
func (counter *DropCounter) add(project config.Project, reason dropReason, kind dropKind, count int64, now time.Time) {
	if count <= 0 {
		return
	}

	hour := now.Truncate(time.Hour)

	counter.mutex.Lock()

	drops := counter.projects[project.ID]

	if drops == nil {
		counter.forgetQuiet(now) // deleted projects go once their day is over

		drops = &projectDrops{}
		counter.projects[project.ID] = drops
	}

	bucket := &drops.hours[hour.Unix()/3600%dropHours]

	if !bucket.start.Equal(hour) {
		*bucket = dropHour{start: hour}
	}

	total := &bucket.counts[reason][kind]
	*total = min(*total, math.MaxInt64-count) + count
	warn := !drops.warned.Equal(hour)
	drops.warned = hour
	first := counter.notice != nil && !now.Before(counter.noticed[project.ID].Add(dropHours*time.Hour))

	if first {
		counter.noticed[project.ID] = now
	}

	notice := counter.notice

	counter.mutex.Unlock()

	if first {
		go notice(project, dropKindNames[kind], store.DropReasons[dropReasons[reason]], count, now)
	}

	if warn {
		counter.logger.Warn("data dropped; counted on the Projects page, the next warning of this project in an hour at the earliest",
			"project", project.Name, "reason", store.DropReasons[dropReasons[reason]], "kind", dropKindNames[kind], "count", count)
	}
}

// forgetQuiet removes the projects with nothing in the window.
func (counter *DropCounter) forgetQuiet(now time.Time) {
	for projectID, drops := range counter.projects {
		if !slices.ContainsFunc(drops.hours[:], func(hour dropHour) bool { return inWindow(hour.start, now) }) {
			delete(counter.projects, projectID)
		}
	}
}

func inWindow(hour, now time.Time) bool {
	return !hour.IsZero() && hour.After(now.Add(-dropHours*time.Hour))
}

// Dropped is what was dropped of the project in the last 24 hours (by the
// hour), largest first.
func (counter *DropCounter) Dropped(projectID int) []store.Dropped {
	return counter.counts(projectID, time.Now())
}

func (counter *DropCounter) counts(projectID int, now time.Time) []store.Dropped {
	var totals [dropReasonCount][dropKindCount]int64

	counter.mutex.Lock()

	if drops := counter.projects[projectID]; drops != nil {
		for _, hour := range drops.hours {
			if !inWindow(hour.start, now) {
				continue
			}

			for reason := range totals {
				for kind := range totals[reason] {
					totals[reason][kind] = min(totals[reason][kind], math.MaxInt64-hour.counts[reason][kind]) + hour.counts[reason][kind]
				}
			}
		}
	}

	counter.mutex.Unlock()

	var dropped []store.Dropped

	for reason := range totals {
		for kind, count := range totals[reason] {
			if count > 0 {
				dropped = append(dropped, store.Dropped{Reason: dropReasons[reason], Kind: dropKindNames[kind], Count: count})
			}
		}
	}

	slices.SortStableFunc(dropped, func(first, second store.Dropped) int { return cmp.Compare(second.Count, first.Count) })

	return dropped
}

// addItems counts the events and logs of an envelope dropped whole.
func (counter *DropCounter) addItems(project config.Project, reason dropReason, counts itemCounts, now time.Time) {
	counter.add(project, reason, dropEvents, int64(counts[categoryEvents]), now)
	counter.add(project, reason, dropLogs, int64(counts[categoryLogs]), now)
}

// addReports counts what the SDK's client reports say it dropped of what
// is stored (events and logs) for a reason the server cannot see: not the
// application's own choice (sampling, before_send, …), and not a failed send
// (network_error, send_error), which SDKs also report for the refusals
// counted here (sentry-python for any answer but 2xx and 429).
func (counter *DropCounter) addReports(project config.Project, envelope *sentry.Envelope, now time.Time) {
	for _, item := range envelope.Items {
		if item.Header.Type != sentry.ItemClientReport {
			continue
		}

		for _, discarded := range sentry.ParseClientReport(item.Payload) {
			reason, kind, ok := reportedDrop(discarded)

			if ok {
				counter.add(project, reason, kind, discarded.Quantity, now)
			}
		}
	}
}

// reportedDrop maps an SDK's discard to what is counted.
func reportedDrop(discarded sentry.Discarded) (dropReason, dropKind, bool) {
	var kind dropKind

	switch discarded.Category {
	case "error", "default", "security":
		kind = dropEvents
	case "log_item":
		kind = dropLogs
	default:
		return 0, 0, false
	}

	switch discarded.Reason {
	case "ratelimit_backoff":
		return dropSDKRateLimit, kind, true
	case "queue_overflow", "buffer_overflow", "cache_overflow", "backpressure":
		return dropSDKOverflow, kind, true
	}

	return 0, 0, false
}
