package database

import (
	"cmp"
	"slices"
	"time"

	"github.com/devforth/exceptic/internal/platform/textutil"
	"github.com/devforth/exceptic/internal/storage/kv"
	"github.com/devforth/exceptic/internal/storage/store"
)

// issueRecord is a stored issue, read through its indexes (issue_reads.go).
type issueRecord struct {
	store.Issue
	Fingerprint string `json:"fingerprint"`
	// EventDays counts the stored events per day received, oldest first.
	EventDays []dayCount `json:"event_days"`
	// IgnoredAt is when an ignore until a count of events was set: events
	// received before it (an inbox backlog) do not count toward it.
	IgnoredAt time.Time `json:"ignored_at,omitzero"`

	// storedSize is the size of the stored value (kv.Collection.Write).
	storedSize int
}

type dayCount struct {
	Day   int32 `json:"d"` // days since 1970-01-01, UTC
	Count int   `json:"n"`
}

// minuteCounts counts the events each issue stored in the current Unix
// minute, for the cap of events an issue stores a minute: only the issues
// written to this minute, all forgotten when it changes (a restart too).
// An event of an earlier minute (an inbox backlog) counts in the current one.
type minuteCounts struct {
	minute int64
	stored map[string]int
}

// at moves the counts to minute and returns them.
func (counts *minuteCounts) at(minute int64) map[string]int {
	if minute > counts.minute || counts.stored == nil {
		counts.minute, counts.stored = max(minute, counts.minute), map[string]int{}
	}

	return counts.stored
}

func (issue *issueRecord) addEvent(at time.Time) {
	issue.Stored++
	number := dayOf(at)
	index, found := slices.BinarySearchFunc(issue.EventDays, number, func(entry dayCount, target int32) int { return cmp.Compare(entry.Day, target) })

	if found {
		issue.EventDays[index].Count++

		return
	}

	issue.EventDays = slices.Insert(issue.EventDays, index, dayCount{Day: number, Count: 1})
}

func (issue *issueRecord) removeEvent(at time.Time) {
	issue.Stored = max(issue.Stored-1, 0)
	number := dayOf(at)
	index, found := slices.BinarySearchFunc(issue.EventDays, number, func(entry dayCount, target int32) int { return cmp.Compare(entry.Day, target) })

	if !found {
		return
	}

	if issue.EventDays[index].Count--; issue.EventDays[index].Count <= 0 {
		issue.EventDays = slices.Delete(issue.EventDays, index, index+1)
	}
}

// ignoreUntil sets the bound of an ignored issue's ignore, counted from now
// and its events so far; any other status has none.
func (issue *issueRecord) ignoreUntil(bound store.IgnoreBound, now time.Time) {
	issue.IgnoredUntil, issue.IgnoredUntilCount, issue.IgnoredAt = nil, 0, time.Time{}

	if issue.Status != store.StatusIgnored {
		return
	}

	if bound.For > 0 {
		until := now.Add(bound.For).UTC()
		issue.IgnoredUntil = &until
	}

	if bound.Events > 0 {
		issue.IgnoredUntilCount = issue.EventCount + bound.Events
		issue.IgnoredAt = now.UTC()
	}
}

func (issue *issueRecord) copy() *issueRecord {
	duplicate := *issue
	duplicate.EventDays = slices.Clone(issue.EventDays)

	if issue.ResolvedAt != nil {
		resolvedAt := *issue.ResolvedAt
		duplicate.ResolvedAt = &resolvedAt
	}

	if issue.IgnoredUntil != nil {
		until := *issue.IgnoredUntil
		duplicate.IgnoredUntil = &until
	}

	return &duplicate
}

// fingerprintKey names a fingerprint of a project, for the groups of one batch.
func fingerprintKey(projectID int, fingerprint string) string {
	return string(kv.K(kv.U32(uint32(projectID)), kv.Text(fingerprint)))
}

type match int

const (
	noMatch match = iota
	isMatch
	// checkEvents: the range covers a day of its events in part, so their times decide.
	checkEvents
)

func inRange(filter store.IssueFilter, at time.Time) bool {
	return (filter.SeenSince.IsZero() || !at.Before(filter.SeenSince)) && (filter.SeenUntil.IsZero() || at.Before(filter.SeenUntil))
}

func matches(issue *issueRecord, filter store.IssueFilter, now time.Time) match {
	if len(filter.ProjectIDs) > 0 && !slices.Contains(filter.ProjectIDs, issue.ProjectID) ||
		filter.Status != "" && issue.At(now).Status != filter.Status ||
		len(filter.Levels) > 0 && !slices.Contains(filter.Levels, issue.Level) ||
		filter.Query != "" && !textutil.ContainsFold(issue.Title, filter.Query) && !textutil.ContainsFold(issue.Culprit, filter.Query) {
		return noMatch
	}

	if filter.SeenSince.IsZero() && filter.SeenUntil.IsZero() {
		return isMatch
	}

	yes := func(ok bool) match {
		if ok {
			return isMatch
		}

		return noMatch
	}

	switch filter.DateField {
	case store.DateFirstSeen:
		return yes(inRange(filter, issue.FirstSeen))
	case store.DateLastSeen:
		return yes(inRange(filter, issue.LastSeen))
	}

	// Both are event times, kept when the cleanup deletes the events.
	if inRange(filter, issue.FirstSeen) || inRange(filter, issue.LastSeen) {
		return isMatch
	}

	result := noMatch

	for _, entry := range issue.EventDays {
		start := dayStart(entry.Day)

		switch whole, touched := inRange(filter, start) && inRange(filter, start.Add(day-time.Nanosecond)),
			(filter.SeenUntil.IsZero() || start.Before(filter.SeenUntil)) && (filter.SeenSince.IsZero() || start.Add(day).After(filter.SeenSince)); {
		case whole:
			return isMatch
		case touched:
			result = checkEvents
		}
	}

	return result
}

// anyEventInRange walks the issue's events newest first and stops once they
// are older than the range (by more than markSlack: arrival order follows the
// sequence only roughly).
func (database *Store) anyEventInRange(issueID string, filter store.IssueFilter) (bool, error) {
	found := false

	err := database.events.Primary(kv.Text(issueID)).Newest().Head().Each(database.db, func(entry kv.Entry[eventRecord]) (bool, error) {
		received := entry.Record.ReceivedAt
		found = inRange(filter, received)

		return !found && (filter.SeenSince.IsZero() || !received.Before(filter.SeenSince.Add(-markSlack))), nil
	})

	return found, err
}
